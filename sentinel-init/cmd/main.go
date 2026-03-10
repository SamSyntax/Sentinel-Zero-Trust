package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sentinel-init/internal/config"
	"sentinel-init/internal/logger"
	"strings"
)

var (
	mode         = flag.String("mode", "REDIRECT", "REDIRECT or TPROXY")
	inboundPort  = flag.Int("inboundPort", 15006, "Port for inbound traffic")
	outboundPort = flag.Int("outboundPort", 15001, "Port for outbound traffic")
	proxyUID     = flag.Int("proxyUID", 1337, "UID of proxy user to exclude")
	excludePorts = flag.String("excludePorts", "", "Comma-separated ports to exclude")
	dryRun       = flag.Bool("dryRun", false, "Print rules without applying")
)

func main() {
	flag.Parse()
	env := os.Getenv("ENV")

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "Debug"
	}
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "sentinel-init-container"
	}
	ctx := context.WithValue(context.Background(), "APP_NAME", serviceName)
	var loggerCfg config.LoggerConfig = config.LoggerConfig{
		Level:       logLevel,
		IsJSON:      true,
		AddSource:   false,
		AppVersion:  "0.0.1",
		ServiceName: serviceName,
		Env:         env,
		Context:     ctx,
	}
	logger, cleanup := logger.InitLogger(loggerCfg, os.Stdout, 5000)
	defer cleanup()
	if err := setupIPTables(logger); err != nil {
		logger.ErrorContext(ctx, "ip tables setup error",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}

	logger.InfoContext(ctx, "iptables rules applied successfully")

}

func setupIPTables(l *slog.Logger) error {
	rules := buildIPTablesRules()

	iptablesPath := findIptables()
	l.InfoContext(context.Background(), "using iptables", slog.String("path", iptablesPath))

	if *dryRun {
		l.Info("DRY RUN applying rules", "Rules", strings.Join(rules, "\n"))
		return nil
	}

	for _, rule := range rules {
		parts := strings.Fields(rule)
		if len(parts) < 2 {
			continue
		}
		parts[0] = iptablesPath
		cmd := exec.Command(parts[0], parts[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			l.ErrorContext(context.Background(), "iptables command failed",
				slog.String("command", strings.Join(parts, " ")),
				slog.String("error", err.Error()),
				slog.String("output", string(output)))
			return fmt.Errorf("Rule failed: %s -> %v\n%s", rule, err, output)
		}
		l.InfoContext(context.Background(), "iptables rule applied", slog.String("rule", rule))
	}
	return nil
}

func findIptables() string {
	paths := []string{
		"/usr/sbin/iptables",
		"/usr/bin/iptables",
		"/sbin/iptables",
		"/bin/iptables",
		"iptables",
	}
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return "iptables" 
}

func buildIPTablesRules() []string {
	var rules []string

	rules = append(rules, "iptables -t nat -N SENTINEL_INBOUND")
	rules = append(rules, "iptables -t nat -N SENTINEL_REDIRECT")
	rules = append(rules, "iptables -t nat -N SENTINEL_OUTPUT")
	rules = append(rules, "iptables -t nat -N SENTINEL_IN_REDIRECT")

	for port := range strings.SplitSeq(*excludePorts, ",") {
		if port == "" {
			continue
		}
		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_INBOUND -p tcp --dport %s -j RETURN", port))
		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))
	}
	rules = append(rules, "iptables -t nat -A PREROUTING -p tcp -j SENTINEL_INBOUND")
	rules = append(rules, "iptables -t nat -A SENTINEL_INBOUND -p tcp -j SENTINEL_IN_REDIRECT")
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_IN_REDIRECT -p tcp -j REDIRECT --to-ports %d", *inboundPort))
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -m owner --uid-owner %d -j RETURN", *proxyUID))
	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN")
	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -j SENTINEL_REDIRECT")
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_REDIRECT -p tcp -j REDIRECT --to-ports %d", *outboundPort))

	return rules

}
