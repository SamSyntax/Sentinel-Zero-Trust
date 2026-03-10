package sentinelinit

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
	}

	logger.InfoContext(ctx, "iptables rules applied successfully")

}

func setupIPTables(l *slog.Logger) error {
	rules := buildIPTablesRules()

	if *dryRun {
		l.Info("DRY RUN applying rules", "Rules", strings.Join(rules, "\r\n"))
		return nil
	}

	for _, rule := range rules {
		parts := strings.Fields(rule)
		if len(parts) < 2 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Rule failed: %s -> %v\n%s", rule, err, output)
		}
	}
	return nil
}

func buildIPTablesRules() []string {
	var rules []string

	rules = append(rules, "-t nat -N SENTINEL_INBOUND")
	rules = append(rules, "-t nat -N SENTINEL_REDIRECT")
	rules = append(rules, "-t nat -N SENTINEL_OUTPUT")

	for port := range strings.SplitSeq(*excludePorts, ",") {
		if port == "" {
			continue
		}
		rules = append(rules, fmt.Sprintf("-t nat -A SENTINEL_INBOUND -p tcp --dport %s -j RETURN", port))
		rules = append(rules, fmt.Sprintf("-t nat -A SENTINEL_OUTPUT -p tcp --dport %s -j RETURN", port))
	}
	rules = append(rules, "-t nat -A PREROUTING -p tcp -j SENTINEL_INBOUND")
	rules = append(rules, "-t nat -A SENTINEL_INBOUND -p tcp -j SENTINEL_IN_REDIRECT")
	rules = append(rules, fmt.Sprintf("-t nat -A SENTINEL_IN_REDIRECT -p tcp -j REDIRECT --to-ports %d", *inboundPort))
	rules = append(rules, fmt.Sprintf("-t nat -A SENTINEL_OUTPUT -m owner --uid-owner %d -j RETURN", *proxyUID))
	rules = append(rules, "-t nat -A SENTINEL_OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN")
	rules = append(rules, "-t nat -A SENTINEL_OUTPUT -j SENTINEL_REDIRECT")
	rules = append(rules, fmt.Sprintf("-t nat -A SENTINEL_REDIRECT -p tcp -j REDIRECT --to-ports %d", *outboundPort))
	rules = append(rules, "-t nat -N SENTINEL_IN_REDIRECT")

	return rules

}
