package config

import (
	"context"
)

type LoggerConfig struct {
	Level       string
	IsJSON      bool
	AddSource   bool
	AppVersion  string
	ServiceName string
	Env         string
	Context     context.Context
}

