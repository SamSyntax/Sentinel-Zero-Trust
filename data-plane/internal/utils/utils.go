package utils

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
)

type ServiceAccountToken string

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
