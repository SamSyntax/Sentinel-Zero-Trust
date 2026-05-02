package config

import (
	"context"
	"os"
	"strconv"
)

type ProxyConfig struct {
	ProxyPort           int
	InboundPort         int
	OutboundPort        int
	ServiceName         string
	KubernetesNamespace string
	CACertPath          string
	TargetURL           string
	TargetGRPC          string
	TrustedDomain       string
	ControlPlaneURL     string
	PodName             string
	PodUID              string
	ServiceAccount      string
	LoggerConfig        LoggerConfig
}
type LoggerConfig struct {
	Level       string
	IsJSON      bool
	AddSource   bool
	AppVersion  string
	ServiceName string
	Env         string
	Context     context.Context
}

func CreateProxyConfig(port int, name string, loggerCfg LoggerConfig) ProxyConfig {
	return ProxyConfig{
		ProxyPort:    port,
		ServiceName:  name,
		LoggerConfig: loggerCfg,
	}
}

func (pc *ProxyConfig) Load() {
	pc.TargetURL = os.Getenv("TARGET_URL")
	pc.TargetGRPC = os.Getenv("TARGET_GRPC")
	pc.ControlPlaneURL = os.Getenv("CONTROL_PLANE_URL")
	pc.CACertPath = os.Getenv("CA_CERT_PATH")
	pc.TrustedDomain = os.Getenv("SPIFFE_TRUSTED_DOMAIN")
	pc.KubernetesNamespace = os.Getenv("KUBERNETES_NAMESPACE")
	pc.PodName = os.Getenv("POD_NAME")
	pc.PodUID = os.Getenv("POD_UID")
	pc.ServiceAccount = os.Getenv("SERVICE_ACCOUNT")

	if port := os.Getenv("INBOUND_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			pc.InboundPort = p
		}
	}
	if port := os.Getenv("OUTBOUND_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			pc.OutboundPort = p
		}
	}

	if pc.InboundPort == 0 {
		pc.InboundPort = 15006
	}
	if pc.OutboundPort == 0 {
		pc.OutboundPort = 15001
	}
	if pc.TargetURL == "" {
		pc.TargetURL = "http://localhost:3005"
	}
	if pc.ControlPlaneURL == "" {
		pc.ControlPlaneURL = "http://localhost:8081/api/v1/identity/issue"
	}
	if pc.CACertPath == "" {
		pc.CACertPath = "../certs/root_ca.crt"
	}
	if pc.TrustedDomain == "" {
		pc.TrustedDomain = "cluster.local"
	}
	if pc.PodName == "" {
		pc.PodName = "unknown-pod"
	}
	if pc.PodUID == "" {
		pc.PodUID = "unknown-pod-uid"
	}
	if pc.ServiceAccount == "" {
		pc.ServiceAccount = pc.ServiceName
	}
}
