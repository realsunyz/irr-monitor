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

	"github.com/realSunyz/irr-monitor/internal/apnic"
	"github.com/realSunyz/irr-monitor/internal/ripe"
	"github.com/realSunyz/irr-monitor/internal/state"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

type Config struct {
	TelegramToken string
	Channels      []any
	PollInterval  time.Duration
	StateFile     string
}

func main() {
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

	bot, err := telegram.NewBot(config.TelegramToken, config.Channels)
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

	ripeMonitor := ripe.NewMonitor(st, config.PollInterval, callback)
	log.Printf("Starting RIPE ASN monitoring, poll interval: %s", config.PollInterval)
	ripeMonitor.Start(ctx)

	log.Println("IRR Monitor stopped")
}

func loadConfig() *Config {
	config := &Config{
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		StateFile:     getEnvOrDefault("STATE_FILE", "/data/state.json"),
	}

	channelsStr := os.Getenv("TELEGRAM_CHANNELS")
	if channelsStr == "" {
		log.Fatal("TELEGRAM_CHANNELS environment variable is required")
	}

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

	if len(config.Channels) == 0 {
		log.Fatal("No valid channels provided")
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
