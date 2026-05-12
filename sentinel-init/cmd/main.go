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
	mode         = flag.String("mode", "TPROXY", "REDIRECT or TPROXY")
	inboundPort  = flag.Int("inboundPort", 15006, "Port for inbound traffic")
	outboundPort = flag.Int("outboundPort", 15001, "Port for outbound traffic")
	servicePort  = flag.String("servicePort", "3005", "Port that the actual service is listening on")
	proxyUID     = flag.Int("proxyUID", 1337, "UID of proxy user to exclude")
	proxyMark    = flag.Int("proxyMark", 0x111, "Mark for TPROXY mode")
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

	logger.InfoContext(ctx, "running in mode", slog.String("mode", *mode))

	if err := setupIPTables(logger); err != nil {
		logger.WarnContext(ctx, "iptables setup warning - continuing anyway", slog.String("error", err.Error()))
	}

	if *mode == "TPROXY" {
		if err := setupRouting(logger); err != nil {
			logger.WarnContext(ctx, "routing setup warning - TPROXY may not work without routing rules", slog.String("error", err.Error()))
		}
	}

	logger.InfoContext(ctx, "iptables rules applied successfully")
}

func setupIPTables(l *slog.Logger) error {
	var rules []string
	if *mode == "TPROXY" {
		rules = buildTProxyRules()
	} else {
		rules = buildRedirectRules()
	}

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
			l.WarnContext(context.Background(), "iptables command failed (continuing anyway)",
				slog.String("command", strings.Join(parts, " ")),
				slog.String("error", err.Error()),
				slog.String("output", string(output)))
			continue
		}
		l.InfoContext(context.Background(), "iptables rule applied", slog.String("rule", rule))
	}
	return nil
}
func buildTProxyRules() []string {
	var rules []string

	// Create mangle chain FIRST (future me remember about the hours lost because of this)
	rules = append(rules, "iptables -t mangle -N SENTINEL_INBOUND")

	// NAT table for outbound - can flush this
	rules = append(rules, "iptables -t nat -F")
	rules = append(rules, "iptables -t nat -N SENTINEL_OUTPUT")

	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -m owner --uid-owner %d -j RETURN", *proxyUID))

	// Inbound traffic should go through the proxy. The service port is excluded only
	// from OUTPUT to allow local health checks and localhost app traffic to bypass
	// redirection without weakening inbound mTLS enforcement.
	inboundExcludePorts := []string{"443", "9090"}
	for _, port := range inboundExcludePorts {
		rules = append(rules, fmt.Sprintf("iptables -t mangle -A SENTINEL_INBOUND -p tcp --dport %s -j RETURN", port))
	}

	outboundExcludePorts := []string{*servicePort, "443", "9090"}
	for _, port := range outboundExcludePorts {
		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))
	}

	// Custom exclude ports from flag
	for port := range strings.SplitSeq(*excludePorts, ",") {
		if port == "" {
			continue
		}
		rules = append(rules, fmt.Sprintf("iptables -t mangle -A SENTINEL_INBOUND -p tcp --dport %s -j RETURN", port))
		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))
	}

	// Jump into SENTINEL_INBOUND from PREROUTING
	rules = append(rules, "iptables -t mangle -A PREROUTING -p tcp -j SENTINEL_INBOUND")

	// Allow return traffic from established connections to pass through
	rules = append(rules, "iptables -t mangle -A SENTINEL_INBOUND -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN")

	// TPROXY catch-all for new inbound connections
	rules = append(rules, "iptables -t mangle -A SENTINEL_INBOUND -p tcp -j TPROXY --on-port "+fmt.Sprintf("%d", *inboundPort)+" --on-ip 0.0.0.0 --tproxy-mark "+fmt.Sprintf("0x%x/0xfff", *proxyMark))

	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN")
	rules = append(rules, "iptables -t nat -A OUTPUT -p tcp --dport 443 -j RETURN")
	rules = append(rules, "iptables -t nat -A OUTPUT -p tcp -j SENTINEL_OUTPUT")
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp -j REDIRECT --to-ports %d", *outboundPort))

	return rules
}

func buildRedirectRules() []string {
	var rules []string

	rules = append(rules, "iptables -t nat -F")

	rules = append(rules, "iptables -t nat -N SENTINEL_INBOUND")
	rules = append(rules, "iptables -t nat -N SENTINEL_REDIRECT")
	rules = append(rules, "iptables -t nat -N SENTINEL_OUTPUT")
	rules = append(rules, "iptables -t nat -N SENTINEL_IN_REDIRECT")

	defaultExcludePorts := []string{"9090"}
	for _, port := range defaultExcludePorts {
		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))
	}

	for port := range strings.SplitSeq(*excludePorts, ",") {
		if port == "" {
			continue
		}

		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))

	}
	rules = append(rules, "iptables -t nat -A PREROUTING -p tcp -j SENTINEL_INBOUND")
	rules = append(rules, "iptables -t nat -A OUTPUT -p tcp -j SENTINEL_OUTPUT")
	rules = append(rules, "iptables -t nat -A SENTINEL_INBOUND -p tcp -j SENTINEL_IN_REDIRECT")
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_IN_REDIRECT -p tcp -j REDIRECT --to-ports %d", *inboundPort))
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -m owner --uid-owner %d -j RETURN", *proxyUID))
	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN")

	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -j SENTINEL_REDIRECT")
	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_REDIRECT -p tcp -j REDIRECT --to-ports %d", *outboundPort))

	return rules
}

func setupRouting(l *slog.Logger) error {
	rules := []string{
		fmt.Sprintf("ip rule add fwmark 0x%x lookup 100", *proxyMark),
		"ip route add local 0.0.0.0/0 dev lo table 100",
	}

	for _, rule := range rules {
		parts := strings.Fields(rule)
		if len(parts) < 2 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			l.DebugContext(context.Background(), "routing command output", slog.String("command", rule), slog.String("output", string(output)))
		}
	}
	l.InfoContext(context.Background(), "routing rules applied for TPROXY")

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

// func buildIPTablesRules() []string {
// 	var rules []string
//
// 	rules = append(rules, "iptables -t nat -N SENTINEL_INBOUND")
// 	rules = append(rules, "iptables -t nat -N SENTINEL_REDIRECT")
// 	rules = append(rules, "iptables -t nat -N SENTINEL_OUTPUT")
// 	rules = append(rules, "iptables -t nat -N SENTINEL_IN_REDIRECT")
//
// 	for port := range strings.SplitSeq(*excludePorts, ",") {
// 		if port == "" {
// 			continue
// 		}
// 		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_INBOUND -p tcp --dport %s -j RETURN", port))
// 		rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))
// 	}
// 	rules = append(rules, "iptables -t nat -A PREROUTING -p tcp -j SENTINEL_INBOUND")
// 	rules = append(rules, "iptables -t nat -A SENTINEL_INBOUND -p tcp -j SENTINEL_IN_REDIRECT")
// 	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_IN_REDIRECT -p tcp -j REDIRECT --to-ports %d", *inboundPort))
// 	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_OUTPUT -m owner --uid-owner %d -j RETURN", *proxyUID))
// 	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN")
// 	rules = append(rules, "iptables -t nat -A SENTINEL_OUTPUT -j SENTINEL_REDIRECT")
// 	rules = append(rules, fmt.Sprintf("iptables -t nat -A SENTINEL_REDIRECT -p tcp -j REDIRECT --to-ports %d", *outboundPort))
//
// 	return rules
//
// }
