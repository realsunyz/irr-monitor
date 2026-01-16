package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/realSunyz/irr-monitor/pkg/nrtm"
)

type Bot struct {
	bot      *bot.Bot
	channels []any
}

func NewBot(token string, channels []any) (*Bot, error) {
	opts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("Failed to create bot: %w", err)
	}

	return &Bot{
		bot:      b,
		channels: channels,
	}, nil
}

func defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	if update.Message.Text == "/status" {
		ripeSerial, ripeLastCheck, ripeLastASN, apnicCount, apnicLastCheck, apnicFile, apnicLastASN := Status.GetStatus()

		var sb strings.Builder
		sb.WriteString("📊 <b>IRR Monitor Status</b>\n\n")

		sb.WriteString("<b>🇪🇺 RIPE (NRTM)</b>\n")
		if ripeSerial > 0 {
			sb.WriteString(fmt.Sprintf("  Serial: %d\n", ripeSerial))
			sb.WriteString(fmt.Sprintf("  Last Check: %s\n", formatTimeAgo(ripeLastCheck)))
			if ripeLastASN != "" {
				sb.WriteString(fmt.Sprintf("  Last ASN: %s\n", ripeLastASN))
			}
		} else {
			sb.WriteString("  Not initialized\n")
		}

		sb.WriteString("\n<b>🌏 APNIC (Delegated)</b>\n")
		if apnicCount > 0 {
			sb.WriteString(fmt.Sprintf("  Total ASNs: %d\n", apnicCount))
			sb.WriteString(fmt.Sprintf("  Last Check: %s\n", formatTimeAgo(apnicLastCheck)))
			if apnicFile != "" {
				sb.WriteString(fmt.Sprintf("  File: %s\n", apnicFile))
			}
			if apnicLastASN != "" {
				sb.WriteString(fmt.Sprintf("  Last ASN: %s\n", apnicLastASN))
			}
		} else {
			sb.WriteString("  Not initialized\n")
		}

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      sb.String(),
			ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			log.Printf("Error sending status message: %v", err)
		}
	}
}

func (b *Bot) Start(ctx context.Context) {
	go b.bot.Start(ctx)
	log.Println("Telegram bot started")
}

func (b *Bot) NotifyNewASN(ctx context.Context, source string, autNum *nrtm.AutNum) error {
	message := formatASNMessage(source, autNum)

	var lastErr error
	for _, channel := range b.channels {
		_, err := b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    channel,
			Text:      message,
			ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			log.Printf("Error sending message to channel %v: %v", channel, err)
			lastErr = err
		}
	}

	return lastErr
}

func formatASNMessage(source string, autNum *nrtm.AutNum) string {
	var sb strings.Builder

	sb.WriteString("<b>New ASN Allocation</b>\n\n")
	sb.WriteString(fmt.Sprintf("<b>ASN:</b> %s\n", autNum.ASN))

	if autNum.AsName != "" {
		sb.WriteString(fmt.Sprintf("<b>Name:</b> %s\n", escapeHTML(autNum.AsName)))
	}

	if autNum.Descr != "" {
		sb.WriteString(fmt.Sprintf("<b>Description:</b> %s\n", escapeHTML(autNum.Descr)))
	}

	if autNum.Country != "" {
		sb.WriteString(fmt.Sprintf("<b>Country:</b> %s\n", autNum.Country))
	}

	sb.WriteString(fmt.Sprintf("<b>Source:</b> %s\n", source))
	sb.WriteString(fmt.Sprintf("\n#%s", source))

	return sb.String()
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(s)
}

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "Just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return t.Format("2006-01-02 15:04")
}
