package config

import "context"

type ProxyConfig struct {
	ProxyPort    int
	Name         string
	LoggerConfig LoggerConfig
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

var GlobalConfig ProxyConfig
