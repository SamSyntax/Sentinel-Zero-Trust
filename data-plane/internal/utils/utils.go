package utils

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sys/unix"
)

const (
	SO_ORIGINAL_DST = 80
)

type ServiceAccountToken string

type KubernetesClaims struct {
	jwt.RegisteredClaims
	Kubernetes struct {
		Namespace      string `json:"namespace"`
		ServiceAccount struct {
			Name string `json:"name"`
		} `json:"serviceaccount"`
	} `json:"kubernetes.io"`
}

func ParseJWT(tokenString string) (*KubernetesClaims, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &KubernetesClaims{})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*KubernetesClaims)
	if !ok {
		return nil, fmt.Errorf("failed to parse claims")
	}
	return claims, nil
}

func (k *KubernetesClaims) GetSpiffeId(trustedDomain string) string {
	spiffeId := fmt.Sprintf("spiffe://%s/ns/%s/sa/%s", trustedDomain, k.Kubernetes.Namespace, k.Kubernetes.ServiceAccount.Name)
	return spiffeId
}

func GetHostAddress() string {
	var ip net.IP
	ifaces, _ := net.Interfaces()
	for _, a := range ifaces {
		addrs, _ := a.Addrs()
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				ip = net.IPv4(byte('1'), byte('2'), byte('3'), byte('4'))
			}
		}
	}
	return ip.To4().String()
}

func TestService(l *slog.Logger) {
	mux := http.NewServeMux()
	ip := GetHostAddress()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Target application %s reached successfully.\n", ip)
	})
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		l.ErrorContext(context.Background(), "failed to serve", slog.String("error", err.Error()))
	}
}

func GetServiceAccountToken(l *slog.Logger) (ServiceAccountToken, error) {
	file, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		l.WarnContext(context.Background(), "failed to get pod service account token", slog.String("error", err.Error()))
		return "", err
	}
	l.InfoContext(context.Background(), "service account token loaded successfully")
	return ServiceAccountToken(file), nil
}

func GetOriginalDest(conn *net.TCPConn) (net.Addr, error) {
	file, err := conn.File()
	if err != nil {
		return nil, fmt.Errorf("Failed to get file: %w", err)
	}
	defer file.Close()

	mreq, err := unix.GetsockoptIPv6Mreq(int(file.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return nil, fmt.Errorf("Failed to get original destination: %w", err)
	}

	ip := net.IP(mreq.Multiaddr[4:8])
	port := binary.BigEndian.Uint16(mreq.Multiaddr[2:4])

	addr := &net.TCPAddr{
		IP:   ip,
		Port: int(port),
	}

	return addr, nil
}
