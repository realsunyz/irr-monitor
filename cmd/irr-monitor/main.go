package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/realSunyz/irr-monitor/internal/apnic"
	"github.com/realSunyz/irr-monitor/internal/arin"
	"github.com/realSunyz/irr-monitor/internal/nrtm"
	"github.com/realSunyz/irr-monitor/internal/nrtmtest"
	"github.com/realSunyz/irr-monitor/internal/ripe"
	"github.com/realSunyz/irr-monitor/internal/state"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

type Config struct {
	TelegramToken   string
	Channels        []any
	PollInterval    time.Duration
	StateFile       string
	PreferencesFile string
}

func main() {
	if handled := handleCLI(os.Args[1:], os.Stdout, os.Stderr); handled {
		return
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting IRR Monitor...")

	config := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	bot, err := telegram.NewBot(config.TelegramToken, config.Channels, config.PreferencesFile)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}
	bot.Start(ctx)
	log.Println("Telegram bot initialized")

	st := state.New(config.StateFile)
	if err := st.Load(); err != nil {
		log.Printf("Warning: Failed to load state: %v", err)
	}

	callback := func(source string, autNum *telegram.AutNum) {
		if err := bot.NotifyNewASN(ctx, source, autNum); err != nil {
			log.Printf("Error sending notification: %v", err)
		}
	}

	dataDir := filepath.Dir(config.StateFile)

	apnicMonitor := apnic.NewMonitor(dataDir, callback)
	go apnicMonitor.Start(ctx)

	arinMonitor := arin.NewMonitor(st, config.PollInterval, callback)
	go arinMonitor.Start(ctx)

	ripeMonitor := ripe.NewMonitor(st, config.PollInterval, callback)
	log.Printf("Starting RIPE ASN monitoring, poll interval: %s", config.PollInterval)
	ripeMonitor.Start(ctx)

	log.Println("IRR Monitor stopped")
}

func handleCLI(args []string, stdout, stderr io.Writer) bool {
	if len(args) == 0 {
		return false
	}

	switch args[0] {
	case "test-nrtm":
		os.Exit(runNRTMTest(args[1:], stdout, stderr))
		return true
	default:
		return false
	}
}

func runNRTMTest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test-nrtm", flag.ContinueOnError)
	fs.SetOutput(stderr)

	source := fs.String("source", "all", "registry to test: arin, ripe, all")
	timeout := fs.Duration("timeout", 10*time.Second, "timeout for each request")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if fs.NArg() > 0 {
		*source = fs.Arg(0)
	}

	registries, err := registriesForSource(*source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	fmt.Fprintf(stdout, "Testing NRTM connectivity at %s\n\n", time.Now().Format(time.RFC3339))

	exitCode := 0
	for _, registry := range registries {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout*3)
		result, err := nrtmtest.Probe(ctx, registry, *timeout)
		cancel()

		printProbeResult(stdout, result, err)
		if err != nil {
			exitCode = 1
		}
	}

	return exitCode
}

func registriesForSource(source string) ([]nrtm.Registry, error) {
	all := []nrtm.Registry{arin.Registry, ripe.Registry}

	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "all":
		return all, nil
	case "arin":
		return all[:1], nil
	case "ripe":
		return all[1:], nil
	default:
		return nil, fmt.Errorf("unsupported source %q, expected one of: arin, ripe, all", source)
	}
}

func printProbeResult(w io.Writer, result *nrtmtest.Result, probeErr error) {
	fmt.Fprintf(w, "[%s]\n", result.Registry.Name)
	fmt.Fprintf(w, "  Serial URL: %s\n", result.Registry.SerialURL)

	if result.Serial > 0 {
		fmt.Fprintf(w, "  Current Serial: %d (%s)\n", result.Serial, result.SerialLatency)
	}

	if result.TCPLatency > 0 {
		fmt.Fprintf(w, "  TCP Connect: ok (%s)\n", result.TCPLatency)
	}

	if result.ResponseLine != "" {
		fmt.Fprintf(w, "  NRTM Query: ok (%s)\n", result.QueryLatency)
		fmt.Fprintf(w, "  First Line: %s\n", result.ResponseLine)
	}

	if probeErr != nil {
		fmt.Fprintf(w, "  Result: failed\n")
		fmt.Fprintf(w, "  Error: %v\n", probeErr)
	} else {
		fmt.Fprintf(w, "  Result: success\n")
	}

	fmt.Fprintln(w)
}

func loadConfig() *Config {
	defaultStateFile := filepath.Join("data", "state.json")
	config := &Config{
		TelegramToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		StateFile:       getEnvOrDefault("STATE_FILE", defaultStateFile),
		PreferencesFile: getEnvOrDefault("PREFERENCES_FILE", filepath.Join(filepath.Dir(getEnvOrDefault("STATE_FILE", defaultStateFile)), "preferences.json")),
	}

	channelsStr := os.Getenv("TELEGRAM_CHANNELS")
	if channelsStr != "" {
		for _, ch := range strings.Split(channelsStr, ",") {
			ch = strings.TrimSpace(ch)
			if ch == "" {
				continue
			}
			if strings.HasPrefix(ch, "@") {
				config.Channels = append(config.Channels, ch)
			} else {
				if id, err := strconv.ParseInt(ch, 10, 64); err == nil {
					config.Channels = append(config.Channels, id)
				}
			}
		}
	}

	pollIntervalStr := getEnvOrDefault("POLL_INTERVAL", "60")
	pollIntervalSec, _ := strconv.Atoi(pollIntervalStr)
	if pollIntervalSec <= 0 {
		pollIntervalSec = 60
	}
	config.PollInterval = time.Duration(pollIntervalSec) * time.Second

	if config.TelegramToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	return config
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
