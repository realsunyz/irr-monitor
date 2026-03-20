package telegram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/realSunyz/irr-monitor/internal/preferences"
)

const (
	menuMain    = "main"
	menuASN     = "asn"
	menuRIR     = "rir"
	menuSponsor = "sponsor"

	callbackToggleEnabled  = "filters:toggle:enabled"
	callbackOpenMain       = "filters:open:main"
	callbackOpenASN        = "filters:open:asn"
	callbackOpenRIR        = "filters:open:rir"
	callbackOpenSponsor    = "filters:open:sponsor"
	callbackCustomSponsor  = "filters:sponsors:custom"
	callbackTogglePresetSP = "filters:sponsors:preset:org-ml942-ripe"
	callbackClearAll       = "filters:clear_all"
	callbackClearASN       = "filters:clear_asn"
	callbackClearRIR       = "filters:clear_rir"
	callbackClearSponsor   = "filters:clear_sponsor"
)

const sponsorPresetLabel = "MoeDove LLC (ORG-ML942-RIPE)"
const sponsorPresetValue = "org-ml942-ripe"

var knownNIRs = []string{"CNNIC", "IDNIC", "IRINN", "JPNIC", "KRNIC", "TWNIC"}

func renderMenu(menu string, prefs preferences.UserPreferences) (string, *models.InlineKeyboardMarkup) {
	switch menu {
	case menuASN:
		return renderASNMenu(prefs)
	case menuRIR:
		return renderRIRMenu(prefs)
	case menuSponsor:
		return renderSponsorMenu(prefs)
	default:
		return renderMainMenu(prefs)
	}
}

func renderMainMenu(prefs preferences.UserPreferences) (string, *models.InlineKeyboardMarkup) {
	text := "Use the buttons below to update your ASN notification preferences."

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderPushToggleLabel(prefs.Enabled),
				CallbackData: callbackToggleEnabled,
			},
		},
		{
			{
				Text:         renderMainMenuStateLabel("ASN Byte", statusForSelection(len(prefs.ASNSizes), 2, false)),
				CallbackData: callbackOpenASN,
			},
			{
				Text:         renderMainMenuStateLabel("RIR (NIR)", statusForSelection(len(prefs.RIRs)+len(prefs.NIRs), 3+len(knownNIRs), false)),
				CallbackData: callbackOpenRIR,
			},
			{
				Text:         renderMainMenuStateLabel("Sponsor", statusForSelection(len(prefs.SponsoringOrgs), 0, true)),
				CallbackData: callbackOpenSponsor,
			},
		},
		{
			{
				Text:         "Clear All Filters",
				CallbackData: callbackClearAll,
			},
		},
	}

	return text, &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderASNMenu(prefs preferences.UserPreferences) (string, *models.InlineKeyboardMarkup) {
	text := "Select ASN Byte filters."

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderSelectableLabel("2-Byte", containsValue(prefs.ASNSizes, asnSize2B)),
				CallbackData: "filters:size:2b",
			},
		},
		{
			{
				Text:         renderSelectableLabel("4-Byte", containsValue(prefs.ASNSizes, asnSize4B)),
				CallbackData: "filters:size:4b",
			},
		},
		{
			{
				Text:         "Clear All",
				CallbackData: callbackClearASN,
			},
		},
		{
			{
				Text:         "Back",
				CallbackData: callbackOpenMain,
			},
		},
	}

	return text, &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderRIRMenu(prefs preferences.UserPreferences) (string, *models.InlineKeyboardMarkup) {
	text := "Select RIR and NIR filters."

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderSelectableLabel("ARIN", containsValue(prefs.RIRs, "ARIN")),
				CallbackData: "filters:rir:ARIN",
			},
			{
				Text:         renderSelectableLabel("APNIC", containsValue(prefs.RIRs, "APNIC")),
				CallbackData: "filters:rir:APNIC",
			},
			{
				Text:         renderSelectableLabel("RIPE", containsValue(prefs.RIRs, "RIPE")),
				CallbackData: "filters:rir:RIPE",
			},
		},
		{
			{
				Text:         renderSelectableLabel("CNNIC", containsValue(prefs.NIRs, "CNNIC")),
				CallbackData: "filters:nir:CNNIC",
			},
			{
				Text:         renderSelectableLabel("IDNIC", containsValue(prefs.NIRs, "IDNIC")),
				CallbackData: "filters:nir:IDNIC",
			},
			{
				Text:         renderSelectableLabel("IRINN", containsValue(prefs.NIRs, "IRINN")),
				CallbackData: "filters:nir:IRINN",
			},
		},
		{
			{
				Text:         renderSelectableLabel("JPNIC", containsValue(prefs.NIRs, "JPNIC")),
				CallbackData: "filters:nir:JPNIC",
			},
			{
				Text:         renderSelectableLabel("KRNIC", containsValue(prefs.NIRs, "KRNIC")),
				CallbackData: "filters:nir:KRNIC",
			},
			{
				Text:         renderSelectableLabel("TWNIC", containsValue(prefs.NIRs, "TWNIC")),
				CallbackData: "filters:nir:TWNIC",
			},
		},
		{
			{
				Text:         "Clear All",
				CallbackData: callbackClearRIR,
			},
		},
		{
			{
				Text:         "Back",
				CallbackData: callbackOpenMain,
			},
		},
	}

	return text, &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderSponsorMenu(prefs preferences.UserPreferences) (string, *models.InlineKeyboardMarkup) {
	text := "Select sponsor filters."
	if len(prefs.SponsoringOrgs) > 0 {
		text += "\n\nCurrent: " + strings.Join(prefs.SponsoringOrgs, ", ")
	}

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderSelectableLabel(sponsorPresetLabel, containsValue(prefs.SponsoringOrgs, sponsorPresetValue)),
				CallbackData: callbackTogglePresetSP,
			},
		},
		{
			{
				Text:         "Custom",
				CallbackData: callbackCustomSponsor,
			},
			{
				Text:         "Clear All",
				CallbackData: callbackClearSponsor,
			},
		},
		{
			{
				Text:         "Back",
				CallbackData: callbackOpenMain,
			},
		},
	}

	return text, &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func menuForAction(action string) string {
	switch {
	case action == callbackOpenMain, action == callbackToggleEnabled, action == callbackClearAll:
		return menuMain
	case action == callbackOpenASN, strings.HasPrefix(action, "filters:size:"), action == callbackClearASN:
		return menuASN
	case action == callbackOpenRIR, strings.HasPrefix(action, "filters:rir:"), strings.HasPrefix(action, "filters:nir:"), action == callbackClearRIR:
		return menuRIR
	case action == callbackOpenSponsor, action == callbackTogglePresetSP, action == callbackClearSponsor, action == callbackCustomSponsor:
		return menuSponsor
	default:
		return menuMain
	}
}

func applyFilterAction(prefs *preferences.UserPreferences, action string) (bool, error) {
	switch {
	case action == callbackToggleEnabled:
		prefs.Enabled = !prefs.Enabled
		return false, nil
	case action == callbackOpenMain, action == callbackOpenASN, action == callbackOpenRIR, action == callbackOpenSponsor:
		return false, nil
	case action == callbackCustomSponsor:
		return true, nil
	case action == callbackTogglePresetSP:
		prefs.SponsoringOrgs = toggleValue(prefs.SponsoringOrgs, sponsorPresetValue)
		return false, nil
	case action == callbackClearAll:
		prefs.ASNSizes = nil
		prefs.RIRs = nil
		prefs.NIRs = nil
		prefs.SponsoringOrgs = nil
		return false, nil
	case action == callbackClearASN:
		prefs.ASNSizes = nil
		return false, nil
	case action == callbackClearRIR:
		prefs.RIRs = nil
		prefs.NIRs = nil
		return false, nil
	case action == callbackClearSponsor:
		prefs.SponsoringOrgs = nil
		return false, nil
	case strings.HasPrefix(action, "filters:size:"):
		prefs.ASNSizes = toggleValue(prefs.ASNSizes, strings.TrimPrefix(action, "filters:size:"))
		return false, nil
	case strings.HasPrefix(action, "filters:rir:"):
		rir := strings.TrimPrefix(action, "filters:rir:")
		if rir == "APNIC" {
			if containsValue(prefs.RIRs, "APNIC") {
				prefs.RIRs = removeValue(prefs.RIRs, "APNIC")
				prefs.NIRs = nil
			} else {
				prefs.RIRs = toggleValue(prefs.RIRs, "APNIC")
				prefs.NIRs = append([]string{}, knownNIRs...)
			}
			return false, nil
		}
		prefs.RIRs = toggleValue(prefs.RIRs, rir)
		return false, nil
	case strings.HasPrefix(action, "filters:nir:"):
		prefs.NIRs = toggleValue(prefs.NIRs, strings.TrimPrefix(action, "filters:nir:"))
		if len(prefs.NIRs) > 0 && !containsValue(prefs.RIRs, "APNIC") {
			prefs.RIRs = toggleValue(prefs.RIRs, "APNIC")
		}
		return false, nil
	default:
		return false, fmt.Errorf("unsupported filter action %q", action)
	}
}

func parseSponsorInput(text string) []string {
	if text == "" {
		return nil
	}

	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})

	normalized := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, value := range fields {
		value = normalizeSponsorValue(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	if len(normalized) == 0 {
		return nil
	}

	sort.Strings(normalized)
	return normalized
}

func sponsorInputPrompt() string {
	return "Send sponsor values to add, separated by commas if needed.\n\nExample:\nORG-1-RIPE, ORG-2-RIPE, ORG-3-RIPE"
}

func renderPushToggleLabel(enabled bool) string {
	if enabled {
		return "🟢 PM Push Enabled"
	}
	return "⚪ PM Push Disabled"
}

func renderMainMenuStateLabel(label, state string) string {
	return state + " " + label
}

func renderSelectableLabel(label string, selected bool) string {
	if selected {
		return "🟢 " + label
	}
	return "⚪ " + label
}

func statusForSelection(selectedCount, totalCount int, orangeWhenAny bool) string {
	switch {
	case selectedCount == 0:
		return "⚪"
	case orangeWhenAny:
		return "🟠"
	case totalCount > 0 && selectedCount >= totalCount:
		return "🟢"
	default:
		return "🟠"
	}
}

func containsValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func toggleValue(values []string, target string) []string {
	filtered := make([]string, 0, len(values))
	found := false

	for _, value := range values {
		if value == target {
			found = true
			continue
		}
		filtered = append(filtered, value)
	}

	if !found {
		filtered = append(filtered, target)
	}

	if len(filtered) == 0 {
		return nil
	}

	sort.Strings(filtered)
	return filtered
}

func removeValue(values []string, target string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value == target {
			continue
		}
		filtered = append(filtered, value)
	}

	if len(filtered) == 0 {
		return nil
	}

	sort.Strings(filtered)
	return filtered
}
