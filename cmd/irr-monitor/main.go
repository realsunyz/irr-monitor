package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/realSunyz/irr-monitor/pkg/apnic"
	"github.com/realSunyz/irr-monitor/pkg/monitor"
	"github.com/realSunyz/irr-monitor/pkg/nrtm"
	"github.com/realSunyz/irr-monitor/pkg/telegram"
)

type Config struct {
	TelegramToken string
	Channels      []any
	RIRs          []string
	PollInterval  time.Duration
	StateFile     string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting IRR Monitor...")

	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	tgBot, err := telegram.NewBot(config.TelegramToken, config.Channels)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}
	tgBot.Start(ctx)
	log.Println("Telegram bot initialized")

	state := monitor.NewState(config.StateFile)
	if err := state.Load(); err != nil {
		log.Printf("Warning: Failed to load state: %v (starting fresh)", err)
	}

	callback := func(source string, autNum *nrtm.AutNum) {
		if err := tgBot.NotifyNewASN(ctx, source, autNum); err != nil {
			log.Printf("Error sending notification: %v", err)
		}
	}

	dataDir := filepath.Dir(config.StateFile)
	apnicMonitor := apnic.NewMonitor(dataDir, callback)
	go apnicMonitor.Start(ctx)

	asnMonitor := monitor.NewASNMonitor(config.RIRs, state, config.PollInterval, callback)
	log.Printf("Starting RIPE ASN monitoring, poll interval: %s", config.PollInterval)
	asnMonitor.Start(ctx)

	log.Println("IRR Monitor stopped")
}

func loadConfig() (*Config, error) {
	config := &Config{
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		StateFile:     getEnvOrDefault("STATE_FILE", "/data/state.json"),
	}

	channelsStr := os.Getenv("TELEGRAM_CHANNELS")
	if channelsStr == "" {
		log.Fatal("TELEGRAM_CHANNELS environment variable is required")
	}
	channelParts := strings.Split(channelsStr, ",")
	for _, ch := range channelParts {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		if strings.HasPrefix(ch, "@") {
			config.Channels = append(config.Channels, ch)
		} else {
			id, err := strconv.ParseInt(ch, 10, 64)
			if err != nil {
				log.Printf("Warning: Invalid channel '%s': %v", ch, err)
				continue
			}
			config.Channels = append(config.Channels, id)
		}
	}

	if len(config.Channels) == 0 {
		log.Fatal("No valid channels provided")
	}

	rirsStr := getEnvOrDefault("MONITOR_RIRS", "APNIC,RIPE")
	rirParts := strings.Split(rirsStr, ",")
	for _, rir := range rirParts {
		rir = strings.TrimSpace(strings.ToUpper(rir))
		if rir != "" {
			config.RIRs = append(config.RIRs, rir)
		}
	}

	pollIntervalStr := getEnvOrDefault("POLL_INTERVAL", "60")
	pollIntervalSec, err := strconv.Atoi(pollIntervalStr)
	if err != nil {
		pollIntervalSec = 60
	}
	config.PollInterval = time.Duration(pollIntervalSec) * time.Second

	if config.TelegramToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	return config, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
