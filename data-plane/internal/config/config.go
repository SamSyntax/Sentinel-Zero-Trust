package config

import (
	"context"
	"os"
	"strconv"
)

type ProxyConfig struct {
	ProxyPort       int
	InboundPort     int
	OutboundPort    int
	Name            string
	CACertPath      string
	TargetURL       string
	ControlPlaneURL string
	LoggerConfig    LoggerConfig
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
		Name:         name,
		LoggerConfig: loggerCfg,
	}
}

func (pc *ProxyConfig) Load() {
	pc.TargetURL = os.Getenv("TARGET_URL")
	pc.ControlPlaneURL = os.Getenv("CONTROL_PLANE_URL")
	pc.CACertPath = os.Getenv("CA_CERT_PATH")

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
		pc.TargetURL = "http://localhost:8080"
	}
	if pc.ControlPlaneURL == "" {
		pc.ControlPlaneURL = "http://localhost:8081/api/v1/identity/issue"
	}
	if pc.CACertPath == "" { pc.CACertPath = "../certs/root_ca.crt"
	}
}
