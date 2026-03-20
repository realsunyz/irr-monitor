package telegram

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/realSunyz/irr-monitor/internal/preferences"
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
	bot                  *bot.Bot
	channels             []any
	preferences          preferences.Store
	awaitingSponsorInput map[int64]bool
	mu                   sync.RWMutex
}

func NewBot(token string, channels []any, preferencesFile string) (*Bot, error) {
	store := preferences.NewJSONStore(preferencesFile)
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("failed to load preferences: %w", err)
	}

	wrapper := &Bot{
		channels:             channels,
		preferences:          store,
		awaitingSponsorInput: make(map[int64]bool),
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, sdkBot *bot.Bot, update *models.Update) {
			wrapper.handleUpdate(ctx, sdkBot, update)
		}),
	}

	sdkBot, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	wrapper.bot = sdkBot
	return wrapper, nil
}

func (b *Bot) Start(ctx context.Context) {
	go b.bot.Start(ctx)
	log.Println("Telegram bot started")
}

func (b *Bot) NotifyNewASN(ctx context.Context, source string, autNum *AutNum) error {
	event := buildNotificationEvent(source, autNum)

	lastErr := b.sendToChannels(ctx, event.Message)
	b.sendMatchingPMs(ctx, event)

	return lastErr
}

func (b *Bot) handleUpdate(ctx context.Context, sdkBot *bot.Bot, update *models.Update) {
	if update == nil {
		return
	}

	switch {
	case update.CallbackQuery != nil:
		b.handleCallbackQuery(ctx, sdkBot, update.CallbackQuery)
	case update.Message != nil:
		b.handleMessage(ctx, sdkBot, update.Message)
	}
}

func (b *Bot) handleMessage(ctx context.Context, sdkBot *bot.Bot, message *models.Message) {
	if message == nil || message.Text == "" {
		return
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		return
	}

	if text == "/status" {
		b.sendStatus(ctx, sdkBot, message.Chat.ID)
		return
	}

	if !isPrivateMessage(message) {
		return
	}

	if strings.HasPrefix(text, "/") && text != "/push" {
		return
	}

	switch text {
	case "/push":
		b.clearAwaitingSponsorInput(message.Chat.ID)
		b.sendMenu(ctx, sdkBot, message.Chat.ID, menuMain, "")
		return
	}

	if b.isAwaitingSponsorInput(message.Chat.ID) {
		prefs, err := b.preferences.Update(message.Chat.ID, func(p *preferences.UserPreferences) error {
			p.SponsoringOrgs = mergeSponsorValues(p.SponsoringOrgs, parseSponsorInput(text))
			return nil
		})
		b.clearAwaitingSponsorInput(message.Chat.ID)
		if err != nil {
			log.Printf("Error saving sponsor filters for %d: %v", message.Chat.ID, err)
			b.sendPlainText(ctx, sdkBot, message.Chat.ID, "Failed to save sponsor filters. Please try again.")
			return
		}
		b.sendMenuMessage(ctx, sdkBot, message.Chat.ID, menuSponsor, "Sponsor filters updated.\n\n", prefs)
	}
}

func (b *Bot) handleCallbackQuery(ctx context.Context, sdkBot *bot.Bot, query *models.CallbackQuery) {
	if query == nil {
		return
	}

	var callbackText string
	prefs, exists := b.preferences.Get(query.From.ID)
	if !exists {
		prefs = preferences.UserPreferences{}
	}
	menu := menuForAction(query.Data)
	shouldUpdateMenu := true

	switch query.Data {
	case callbackCustomSponsor:
		b.setAwaitingSponsorInput(query.From.ID, true)
		callbackText = "Send sponsor filters in PM."
		shouldUpdateMenu = false
		b.sendPlainText(ctx, sdkBot, query.From.ID, sponsorInputPrompt())
	default:
		updatedPrefs, err := b.preferences.Update(query.From.ID, func(p *preferences.UserPreferences) error {
			_, err := applyFilterAction(p, query.Data)
			return err
		})
		if err != nil {
			log.Printf("Error handling callback %q for %d: %v", query.Data, query.From.ID, err)
			callbackText = "Failed to update filters."
			b.answerCallbackQuery(ctx, sdkBot, query.ID, callbackText)
			return
		}
		prefs = updatedPrefs

		if query.Data == callbackClearSponsor || query.Data == callbackClearAll {
			b.clearAwaitingSponsorInput(query.From.ID)
		}
	}

	b.answerCallbackQuery(ctx, sdkBot, query.ID, callbackText)
	if shouldUpdateMenu {
		b.updateMenuFromCallback(ctx, sdkBot, query, menu, prefs)
	}
}

func (b *Bot) sendToChannels(ctx context.Context, message string) error {
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

func (b *Bot) sendMatchingPMs(ctx context.Context, event *NotificationEvent) {
	for _, record := range b.preferences.List() {
		if !event.MatchesPreferences(record.Preferences) {
			continue
		}

		_, err := b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    record.UserID,
			Text:      event.Message,
			ParseMode: models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: ptr(true),
			},
		})
		if err != nil {
			log.Printf("Error sending PM notification to %d: %v", record.UserID, err)
		}
	}
}

func (b *Bot) sendStatus(ctx context.Context, sdkBot *bot.Bot, chatID int64) {
	ripeSerial, ripeLastCheck, _, ripeFile, ripeDiff, arinSerial, arinLastCheck, _, arinFile, arinDiff, apnicCount, apnicLastCheck, apnicFile, _ := Status.GetStatus()

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

	sb.WriteString("\n- ARIN\n")
	if arinSerial > 0 {
		sb.WriteString(fmt.Sprintf("    Serial: %d\n", arinSerial))
		sb.WriteString(fmt.Sprintf("    Last Check: %s\n", formatTimeAgo(arinLastCheck)))
		if arinFile != "" {
			sb.WriteString(fmt.Sprintf("    Delegated File: %s\n", arinFile))
			sb.WriteString(fmt.Sprintf("    Last Delegated Diff: %d\n", arinDiff))
		}
	} else {
		sb.WriteString("    Not Initialized\n")
	}

	sb.WriteString("\n- RIPE\n")
	if ripeSerial > 0 {
		sb.WriteString(fmt.Sprintf("    Serial: %d\n", ripeSerial))
		sb.WriteString(fmt.Sprintf("    Last Check: %s\n", formatTimeAgo(ripeLastCheck)))
		if ripeFile != "" {
			sb.WriteString(fmt.Sprintf("    Delegated File: %s\n", ripeFile))
			sb.WriteString(fmt.Sprintf("    Last Delegated Diff: %d\n", ripeDiff))
		}
	} else {
		sb.WriteString("    Not Initialized\n")
	}

	_, err := sdkBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
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

func (b *Bot) sendMenu(ctx context.Context, sdkBot *bot.Bot, chatID int64, menu, prefix string) {
	prefs, ok := b.preferences.Get(chatID)
	if !ok {
		prefs = preferences.UserPreferences{}
	}
	b.sendMenuMessage(ctx, sdkBot, chatID, menu, prefix, prefs)
}

func (b *Bot) sendMenuMessage(ctx context.Context, sdkBot *bot.Bot, chatID int64, menu, prefix string, prefs preferences.UserPreferences) {
	text, markup := renderMenu(menu, prefs)
	if prefix != "" {
		text = prefix + text
	}

	_, err := sdkBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: markup,
	})
	if err != nil {
		log.Printf("Error sending filter menu to %d: %v", chatID, err)
	}
}

func (b *Bot) updateMenuFromCallback(ctx context.Context, sdkBot *bot.Bot, query *models.CallbackQuery, menu string, prefs preferences.UserPreferences) {
	if query == nil || query.Message.Message == nil {
		return
	}

	text, markup := renderMenu(menu, prefs)
	_, err := sdkBot.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      query.Message.Message.Chat.ID,
		MessageID:   query.Message.Message.ID,
		Text:        text,
		ReplyMarkup: markup,
	})
	if err != nil {
		if isMessageNotModifiedError(err) {
			return
		}
		log.Printf("Error updating filter menu for %d: %v", query.From.ID, err)
	}
}

func (b *Bot) sendPlainText(ctx context.Context, sdkBot *bot.Bot, chatID int64, text string) {
	_, err := sdkBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		log.Printf("Error sending message to %d: %v", chatID, err)
	}
}

func (b *Bot) answerCallbackQuery(ctx context.Context, sdkBot *bot.Bot, queryID, text string) {
	_, err := sdkBot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: queryID,
		Text:            text,
	})
	if err != nil {
		log.Printf("Error answering callback query %s: %v", queryID, err)
	}
}

func isMessageNotModifiedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

func (b *Bot) isAwaitingSponsorInput(userID int64) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.awaitingSponsorInput[userID]
}

func (b *Bot) setAwaitingSponsorInput(userID int64, awaiting bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if awaiting {
		b.awaitingSponsorInput[userID] = true
		return
	}
	delete(b.awaitingSponsorInput, userID)
}

func (b *Bot) clearAwaitingSponsorInput(userID int64) {
	b.setAwaitingSponsorInput(userID, false)
}

func isPrivateMessage(message *models.Message) bool {
	return message != nil && message.Chat.Type == "private"
}

func mergeSponsorValues(existing, additions []string) []string {
	if len(existing) == 0 && len(additions) == 0 {
		return nil
	}

	merged := make([]string, 0, len(existing)+len(additions))
	seen := make(map[string]struct{}, len(existing)+len(additions))

	for _, value := range append(append([]string{}, existing...), additions...) {
		value = normalizeSponsorValue(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}

	if len(merged) == 0 {
		return nil
	}

	sort.Strings(merged)
	return merged
}

func formatASNMessage(source string, autNum *AutNum) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("<b>New %s ASN Assignment</b>\n\n", source))
	sb.WriteString(fmt.Sprintf("<b>aut-num:</b> %s\n", autNum.ASN))
	sb.WriteString(fmt.Sprintf("<b>as-name:</b> %s\n", escapeHTML(autNum.AsName)))

	if shouldIncludeDescr(source, autNum) && autNum.Descr != "" {
		sb.WriteString(fmt.Sprintf("<b>descr:</b> %s\n", escapeHTML(autNum.Descr)))
	}

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
	} else if source == "ARIN" {
		sb.WriteString(fmt.Sprintf("<a href=\"https://search.arin.net/rdap/?query=%s\">ARIN RDAP</a>", autNum.ASN))
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
	"MNT-VN-VNNIC":   "VNNIC",
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

func getNIRName(autNum *AutNum) string {
	if autNum == nil {
		return ""
	}
	return nirMapping[autNum.MntBy]
}

func shouldIncludeDescr(source string, autNum *AutNum) bool {
	if source == "ARIN" {
		return true
	}

	if source != "APNIC" {
		return false
	}

	_, isNIR := nirMapping[autNum.MntBy]
	return isNIR
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
