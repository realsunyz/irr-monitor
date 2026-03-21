package telegram

import (
	"fmt"
	"html"
	"regexp"
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
	callbackClearRIR       = "filters:clear_rir"
	callbackClearSponsor   = "filters:clear_sponsor"
)

const sponsorPresetLabel = "MoeDove LLC (ORG-ML942-RIPE)"
const sponsorPresetValue = "org-ml942-ripe"

var knownNIRs = []string{"CNNIC", "IDNIC", "IRINN", "JPNIC", "KRNIC", "TWNIC"}
var sponsorValuePattern = regexp.MustCompile(`^(org-[a-z0-9][a-z0-9-]*-(ap|ripe)|lir)$`)

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
	lines := []string{
		"<b>ASN Notification</b>",
		"",
		"We will notify you when a new ASN meets the following criteria:",
		"",
		"<b>ASN Byte:</b> " + html.EscapeString(formatASNByteSummary(prefs)),
		"<b>RIR (NIR):</b> " + html.EscapeString(formatRIRSummary(prefs)),
		"<b>Sponsor:</b> " + html.EscapeString(formatSponsorSummary(prefs)),
	}
	if !hasAnyFilterSelected(prefs) {
		lines = append(lines,
			"",
			"<i>To enable this feature, you must select at least one filter. Otherwise, simply subscribe to the @New_ASNs channel.</i>",
		)
	}
	lines = append(lines,
		"",
		"Use the buttons below to update your preferences.",
	)

	text := strings.Join(lines, "\n")

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderPushToggleLabel(prefs.Enabled),
				CallbackData: callbackToggleEnabled,
			},
		},
		{
			{
				Text:         renderMainMenuStateLabel("ASN Byte", asnMenuState(prefs)),
				CallbackData: callbackOpenASN,
			},
			{
				Text:         renderMainMenuStateLabel("RIR (NIR)", rirMenuState(prefs)),
				CallbackData: callbackOpenRIR,
			},
			{
				Text:         renderMainMenuStateLabel("Sponsor", sponsorMenuState(prefs)),
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
	text := strings.Join([]string{
		"<b>ASN Notification &gt; ASN Byte Filter</b>",
		"",
		"If you select the 4-Byte filter, 2-Byte ASNs (i.e., 0 to 65,535) will not be notified.",
		"",
		"Current: " + html.EscapeString(formatASNByteSummary(prefs)),
	}, "\n")

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderSelectableLabel("2-Byte", asnOptionSelected(prefs, asnSize2B)),
				CallbackData: "filters:size:2b",
			},
			{
				Text:         renderSelectableLabel("4-Byte", asnOptionSelected(prefs, asnSize4B)),
				CallbackData: "filters:size:4b",
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
	text := strings.Join([]string{
		"<b>ASN Notification &gt; RIR Filter</b>",
		"",
		"CNNIC, IDNIC, IRINN, JPNIC, KRNIC, and TWNIC are the National Internet Registries (NIRs) within the APNIC region. You have the flexibility to choose whether to notify the ASN assigned by the NIR or only the ASN assigned by APNIC.",
		"",
		"Current: " + html.EscapeString(formatRIRSummary(prefs)),
	}, "\n")

	rows := [][]models.InlineKeyboardButton{
		{
			{
				Text:         renderSelectableStateLabel("APNIC", apnicButtonState(prefs)),
				CallbackData: "filters:rir:APNIC",
			},
			{
				Text:         renderSelectableStateLabel("ARIN", rirOptionState(prefs, "ARIN")),
				CallbackData: "filters:rir:ARIN",
			},
			{
				Text:         renderSelectableStateLabel("RIPE", rirOptionState(prefs, "RIPE")),
				CallbackData: "filters:rir:RIPE",
			},
		},
		{
			{
				Text:         renderSelectableStateLabel("CNNIC", nirOptionState(prefs, "CNNIC")),
				CallbackData: "filters:nir:CNNIC",
			},
			{
				Text:         renderSelectableStateLabel("IDNIC", nirOptionState(prefs, "IDNIC")),
				CallbackData: "filters:nir:IDNIC",
			},
			{
				Text:         renderSelectableStateLabel("IRINN", nirOptionState(prefs, "IRINN")),
				CallbackData: "filters:nir:IRINN",
			},
		},
		{
			{
				Text:         renderSelectableStateLabel("JPNIC", nirOptionState(prefs, "JPNIC")),
				CallbackData: "filters:nir:JPNIC",
			},
			{
				Text:         renderSelectableStateLabel("KRNIC", nirOptionState(prefs, "KRNIC")),
				CallbackData: "filters:nir:KRNIC",
			},
			{
				Text:         renderSelectableStateLabel("TWNIC", nirOptionState(prefs, "TWNIC")),
				CallbackData: "filters:nir:TWNIC",
			},
		},
		{
			{
				Text:         "Clear All",
				CallbackData: callbackClearRIR,
			},
			{
				Text:         "Back",
				CallbackData: callbackOpenMain,
			},
		},
	}

	return text, &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderSponsorMenu(prefs preferences.UserPreferences) (string, *models.InlineKeyboardMarkup) {
	text := strings.Join([]string{
		"<b>ASN Notification &gt; Sponsor Filter</b>",
		"",
		"This feature supports only ASNs assigned by APNIC and RIPE NCC.",
		"",
		"You can enter &quot;LIR&quot; to filter for ASNs owned by LIRs (i.e., those without a sponsoring-org field and not assigned to end users).",
		"",
		"The buttons below list some common LIRs. You can also click &quot;Custom&quot; to enter your own.",
		"",
		"Current: " + html.EscapeString(formatSponsorSummary(prefs)),
	}, "\n")

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
	case action == callbackOpenASN, strings.HasPrefix(action, "filters:size:"):
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
		if !prefs.Enabled && !hasAnyFilterSelected(*prefs) {
			return false, fmt.Errorf("You must select at least one filter.")
		}
		prefs.Enabled = !prefs.Enabled
		return false, nil
	case action == callbackOpenMain, action == callbackOpenASN, action == callbackOpenRIR, action == callbackOpenSponsor:
		return false, nil
	case action == callbackCustomSponsor:
		return true, nil
	case action == callbackTogglePresetSP:
		prefs.SponsoringOrgs = toggleValue(prefs.SponsoringOrgs, sponsorPresetValue)
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	case action == callbackClearAll:
		prefs.ASNSizes = nil
		prefs.RIRs = nil
		prefs.NIRs = nil
		prefs.SponsoringOrgs = nil
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	case action == callbackClearRIR:
		prefs.RIRs = nil
		prefs.NIRs = nil
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	case action == callbackClearSponsor:
		prefs.SponsoringOrgs = nil
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	case strings.HasPrefix(action, "filters:size:"):
		prefs.ASNSizes = toggleValue(prefs.ASNSizes, strings.TrimPrefix(action, "filters:size:"))
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	case strings.HasPrefix(action, "filters:rir:"):
		rir := strings.TrimPrefix(action, "filters:rir:")
		if rir == "APNIC" {
			if containsValue(prefs.RIRs, "APNIC") {
				prefs.RIRs = removeValue(prefs.RIRs, "APNIC")
			} else {
				prefs.RIRs = toggleValue(prefs.RIRs, "APNIC")
				if len(prefs.NIRs) == 0 {
					prefs.NIRs = append([]string{}, knownNIRs...)
				}
			}
			ensureEnabledStateMatchesFilters(prefs)
			return false, nil
		}
		prefs.RIRs = toggleValue(prefs.RIRs, rir)
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	case strings.HasPrefix(action, "filters:nir:"):
		prefs.NIRs = toggleValue(prefs.NIRs, strings.TrimPrefix(action, "filters:nir:"))
		ensureEnabledStateMatchesFilters(prefs)
		return false, nil
	default:
		return false, fmt.Errorf("unsupported filter action %q", action)
	}
}

func parseSponsorInput(text string) ([]string, error) {
	if text == "" {
		return nil, nil
	}

	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})

	normalized := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	invalid := make([]string, 0)
	for _, value := range fields {
		rawValue := strings.TrimSpace(value)
		value = preferences.NormalizeSponsoringOrg(rawValue)
		if rawValue == "" {
			continue
		}
		if !sponsorValuePattern.MatchString(value) {
			invalid = append(invalid, rawValue)
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid sponsor format: %s", strings.Join(invalid, ", "))
	}

	if len(normalized) == 0 {
		return nil, nil
	}

	return preferences.NormalizeSponsoringOrgs(normalized), nil
}

func sponsorInputPrompt() string {
	return "Please enter the values for the sponsoring-org field, separated by commas if there are multiple entries.\n\nAllowed formats:\n- ORG-XXXXX-AP\n- ORG-XXXXX-RIPE\n- LIR"
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

func renderSelectableStateLabel(label, state string) string {
	return state + " " + label
}

func formatASNByteSummary(prefs preferences.UserPreferences) string {
	if asnFiltersAll(prefs) {
		return "All"
	}

	var labels []string
	if containsValue(prefs.ASNSizes, asnSize2B) {
		labels = append(labels, "2-Byte")
	}
	if containsValue(prefs.ASNSizes, asnSize4B) {
		labels = append(labels, "4-Byte")
	}
	return strings.Join(labels, ", ")
}

func formatSponsorSummary(prefs preferences.UserPreferences) string {
	if len(prefs.SponsoringOrgs) == 0 {
		return "All"
	}

	values := make([]string, 0, len(prefs.SponsoringOrgs))
	for _, sponsor := range prefs.SponsoringOrgs {
		values = append(values, strings.ToUpper(sponsor))
	}
	return strings.Join(values, ", ")
}

func formatRIRSummary(prefs preferences.UserPreferences) string {
	if rirFiltersAll(prefs) {
		return "All"
	}

	parts := make([]string, 0, 3)

	includeARIN := containsValue(prefs.RIRs, "ARIN")
	includeRIPE := containsValue(prefs.RIRs, "RIPE")

	if apnic := formatAPNICSummary(prefs); apnic != "" {
		parts = append(parts, apnic)
	}

	if includeARIN {
		parts = append(parts, "ARIN")
	}

	if includeRIPE {
		parts = append(parts, "RIPE")
	}

	return strings.Join(parts, ", ")
}

func formatAPNICSummary(prefs preferences.UserPreferences) string {
	hasAPNIC := containsValue(prefs.RIRs, "APNIC")
	selectedNIRs := selectedNIRsInOrder(prefs.NIRs)

	switch {
	case hasAPNIC && len(selectedNIRs) == 0:
		return "APNIC (Excluding all NIRs)"
	case hasAPNIC && len(selectedNIRs) == len(knownNIRs):
		return "APNIC (Including all NIRs)"
	case hasAPNIC && len(selectedNIRs) > 0:
		return "APNIC (Including " + strings.Join(selectedNIRs, ", ") + ")"
	case !hasAPNIC && len(selectedNIRs) == len(knownNIRs):
		return "APNIC (NIRs Only)"
	case !hasAPNIC && len(selectedNIRs) > 0:
		return "APNIC (" + strings.Join(selectedNIRs, ", ") + " Only)"
	default:
		return ""
	}
}

func selectedNIRsInOrder(selected []string) []string {
	values := make([]string, 0, len(selected))
	for _, nir := range knownNIRs {
		if containsValue(selected, nir) {
			values = append(values, nir)
		}
	}
	return values
}

func apnicButtonState(prefs preferences.UserPreferences) string {
	if rirFiltersAll(prefs) {
		return "⚪"
	}
	if containsValue(prefs.RIRs, "APNIC") {
		if len(prefs.NIRs) == 0 {
			return "🟠"
		}
		return "🟢"
	}
	return "⚪"
}

func asnFiltersAll(prefs preferences.UserPreferences) bool {
	return len(prefs.ASNSizes) == 0 || len(prefs.ASNSizes) == 2
}

func rirFiltersAll(prefs preferences.UserPreferences) bool {
	return (len(prefs.RIRs) == 0 && len(prefs.NIRs) == 0) ||
		(len(prefs.RIRs) == 3 && len(prefs.NIRs) == len(knownNIRs))
}

func hasAnyFilterSelected(prefs preferences.UserPreferences) bool {
	return len(prefs.ASNSizes) > 0 || len(prefs.RIRs) > 0 || len(prefs.NIRs) > 0 || len(prefs.SponsoringOrgs) > 0
}

func ensureEnabledStateMatchesFilters(prefs *preferences.UserPreferences) {
	if !hasAnyFilterSelected(*prefs) {
		prefs.Enabled = false
	}
}

func asnOptionSelected(prefs preferences.UserPreferences, value string) bool {
	if asnFiltersAll(prefs) {
		return false
	}
	return containsValue(prefs.ASNSizes, value)
}

func rirOptionState(prefs preferences.UserPreferences, value string) string {
	if rirFiltersAll(prefs) {
		return "⚪"
	}
	if containsValue(prefs.RIRs, value) {
		return "🟢"
	}
	return "⚪"
}

func nirOptionState(prefs preferences.UserPreferences, value string) string {
	if rirFiltersAll(prefs) {
		return "⚪"
	}
	if containsValue(prefs.NIRs, value) {
		return "🟢"
	}
	return "⚪"
}

func asnMenuState(prefs preferences.UserPreferences) string {
	if asnFiltersAll(prefs) {
		return "⚪"
	}
	if len(prefs.ASNSizes) > 0 {
		return "🟢"
	}
	return "⚪"
}

func rirMenuState(prefs preferences.UserPreferences) string {
	if rirFiltersAll(prefs) {
		return "⚪"
	}
	if len(prefs.RIRs) > 0 || len(prefs.NIRs) > 0 {
		return "🟢"
	}
	return "⚪"
}

func sponsorMenuState(prefs preferences.UserPreferences) string {
	if len(prefs.SponsoringOrgs) > 0 {
		return "🟢"
	}
	return "⚪"
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
