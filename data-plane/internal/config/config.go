package config

import (
	"context"
	"os"
)

type ProxyConfig struct {
	ProxyPort       int
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
	if pc.TargetURL== "" {
		pc.TargetURL= "http://localhost:8080"
	}
	if pc.ControlPlaneURL == "" {
		pc.ControlPlaneURL = "http://localhost:8081/api/v1/identity/issue"
	}
	if pc.CACertPath == "" {
		pc.CACertPath = "../certs/root_ca.crt"
	}
}

