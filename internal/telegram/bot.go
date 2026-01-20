package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type AutNum struct {
	ASN               string
	AsName            string
	Descr             string
	Country           string
	Org               string
	OrgName           string
	OrgType           string
	OrgCountry        string
	SponsoringOrg     string
	SponsoringOrgName string
	MntBy             string
	Source            string
}

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
		return nil, fmt.Errorf("failed to create bot: %w", err)
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
		ripeSerial, ripeLastCheck, _, apnicCount, apnicLastCheck, apnicFile, _ := Status.GetStatus()

		var sb strings.Builder
		sb.WriteString("IRR Monitor Status\n\n")

		sb.WriteString("- APNIC\n")
		if apnicCount > 0 {
			sb.WriteString(fmt.Sprintf("    Total ASNs: %d\n", apnicCount))
			sb.WriteString(fmt.Sprintf("    Last Check: %s\n", formatTimeAgo(apnicLastCheck)))
			if apnicFile != "" {
				sb.WriteString(fmt.Sprintf("    File: %s\n", apnicFile))
			}
		} else {
			sb.WriteString("    Not Initialized\n")
		}

		sb.WriteString("\n- RIPE\n")
		if ripeSerial > 0 {
			sb.WriteString(fmt.Sprintf("    Serial: %d\n", ripeSerial))
			sb.WriteString(fmt.Sprintf("    Last Check: %s\n", formatTimeAgo(ripeLastCheck)))
		} else {
			sb.WriteString("    Not Initialized\n")
		}

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      sb.String(),
			ParseMode: models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: ptr(true),
			},
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

func (b *Bot) NotifyNewASN(ctx context.Context, source string, autNum *AutNum) error {
	message := formatASNMessage(source, autNum)

	var lastErr error
	for _, channel := range b.channels {
		_, err := b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    channel,
			Text:      message,
			ParseMode: models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: ptr(true),
			},
		})
		if err != nil {
			log.Printf("Error sending message to channel %v: %v", channel, err)
			lastErr = err
		}
	}

	return lastErr
}

func formatASNMessage(source string, autNum *AutNum) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("<b>New %s ASN Assignment</b>\n\n", source))
	sb.WriteString(fmt.Sprintf("<b>aut-num:</b> %s\n", autNum.ASN))
	sb.WriteString(fmt.Sprintf("<b>as-name:</b> %s\n", escapeHTML(autNum.AsName)))

	if autNum.Country != "" {
		sb.WriteString(fmt.Sprintf("<b>country:</b> %s\n", autNum.Country))
	}

	if autNum.Org != "" || autNum.OrgName != "" {
		sb.WriteString("\n")
		if autNum.Org != "" {
			sb.WriteString(fmt.Sprintf("<b>organization:</b> %s\n", autNum.Org))
		}
		if autNum.OrgName != "" {
			sb.WriteString(fmt.Sprintf("<b>org-name:</b> %s\n", escapeHTML(autNum.OrgName)))
		}
		if autNum.OrgType != "" {
			sb.WriteString(fmt.Sprintf("<b>org-type:</b> %s\n", autNum.OrgType))
		}
		if autNum.OrgCountry != "" {
			sb.WriteString(fmt.Sprintf("<b>country:</b> %s\n", autNum.OrgCountry))
		}
	}

	if source == "APNIC" || source == "RIPE" {
		sponsoredBy := getSponsoredBy(autNum)
		if sponsoredBy != "" {
			sb.WriteString(fmt.Sprintf("\n%s\n", sponsoredBy))
		}
	}

	asnNum := strings.TrimPrefix(autNum.ASN, "AS")

	sb.WriteString("\n")
	if source == "RIPE" {
		sb.WriteString(fmt.Sprintf("<a href=\"https://apps.db.ripe.net/db-web-ui/lookup?source=ripe&amp;key=%s&amp;type=aut-num\">RIPE DB</a>", autNum.ASN))
	} else {
		sb.WriteString(fmt.Sprintf("<a href=\"https://wq.apnic.net/static/search.html?query=%s\">APNIC DB</a>", autNum.ASN))
	}
	sb.WriteString(fmt.Sprintf(" | <a href=\"https://bgp.tools/as/%s\">BGP.TOOLS</a>", asnNum))

	sb.WriteString(fmt.Sprintf("\n\n#%s", source))

	return sb.String()
}

var nirMapping = map[string]string{
	"MNT-APJII-ID":   "IDNIC",
	"MAINT-CNNIC-AP": "CNNIC",
	"MAINT-JPNIC":    "JPNIC",
	"MNT-KRNIC-AP":   "KRNIC",
	"MAINT-TW-TWNIC": "TWNIC",
	"MAINT-IN-IRINN": "IRINN",
	"MAINT-VN-VNNIC": "VNNIC",
}

func getSponsoredBy(autNum *AutNum) string {
	if autNum.SponsoringOrg != "" {
		if autNum.SponsoringOrgName != "" {
			return fmt.Sprintf("Sponsored by %s (%s).", escapeHTML(autNum.SponsoringOrgName), autNum.SponsoringOrg)
		}
		return fmt.Sprintf("Sponsored by %s.", autNum.SponsoringOrg)
	}

	if autNum.OrgType == "LIR" {
		return ""
	}

	if nirName, ok := nirMapping[autNum.MntBy]; ok {
		return fmt.Sprintf("Sponsored by %s (%s).", nirName, autNum.MntBy)
	}

	return ""
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(s)
}

func ptr[T any](v T) *T {
	return &v
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
