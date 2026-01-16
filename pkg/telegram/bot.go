package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

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

	if update.Message.Text == "/start" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      "🔍 *IRR Monitor Bot*\n\nI will notify you about new ASN allocations from APNIC and RIPE.\n\nThis chat ID: `" + fmt.Sprintf("%d", update.Message.Chat.ID) + "`",
			ParseMode: models.ParseModeMarkdown,
		})
		if err != nil {
			log.Printf("Error sending start message: %v", err)
		}
		return
	}

	if strings.HasPrefix(update.Message.Text, "/chatid") {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      fmt.Sprintf("Your chat ID is: `%d`", update.Message.Chat.ID),
			ParseMode: models.ParseModeMarkdown,
		})
		if err != nil {
			log.Printf("Error sending chat ID: %v", err)
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
