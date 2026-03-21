package telegram

import (
	"strconv"
	"strings"

	"github.com/realSunyz/irr-monitor/internal/preferences"
)

const (
	asnSize2B = "2b"
	asnSize4B = "4b"
)

type NotificationEvent struct {
	AutNum       *AutNum
	Message      string
	ASNSize      string
	RIR          string
	NIR          string
	SponsorTerms map[string]struct{}
}

func buildNotificationEvent(source string, autNum *AutNum) *NotificationEvent {
	event := &NotificationEvent{
		AutNum:       autNum,
		Message:      formatASNMessage(source, autNum),
		ASNSize:      deriveASNSize(autNum.ASN),
		RIR:          source,
		NIR:          getNIRName(autNum),
		SponsorTerms: buildSponsorTerms(source, autNum),
	}
	return event
}

func (e *NotificationEvent) MatchesPreferences(prefs preferences.UserPreferences) bool {
	if !prefs.Enabled {
		return false
	}
	if !hasAnyFilterSelected(prefs) {
		return false
	}

	if !matchesValueSet(prefs.ASNSizes, e.ASNSize) {
		return false
	}
	if !matchesRIRAndNIRPreferences(prefs, e) {
		return false
	}
	if !matchesSponsorTerms(prefs.SponsoringOrgs, e.SponsorTerms) {
		return false
	}

	return true
}

func deriveASNSize(asn string) string {
	asn = strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(asn), "AS"))
	if asn == "" {
		return ""
	}

	num, err := strconv.ParseInt(asn, 10, 64)
	if err != nil {
		return ""
	}
	if num <= 65535 {
		return asnSize2B
	}
	return asnSize4B
}

func buildSponsorTerms(source string, autNum *AutNum) map[string]struct{} {
	terms := make(map[string]struct{})

	addSponsorTerm(terms, autNum.SponsoringOrg)
	if isLIRAutNum(source, autNum) {
		addSponsorTerm(terms, "lir")
	}

	return terms
}

func addSponsorTerm(terms map[string]struct{}, value string) {
	value = preferences.NormalizeSponsoringOrg(value)
	if value == "" {
		return
	}
	terms[value] = struct{}{}
}

func isLIRAutNum(source string, autNum *AutNum) bool {
	if source != "APNIC" && source != "RIPE" {
		return false
	}
	if autNum.SponsoringOrg != "" {
		return false
	}
	if source == "APNIC" && getNIRName(autNum) != "" {
		return false
	}
	return preferences.NormalizeSponsoringOrg(autNum.OrgType) != "end-user"
}

func matchesValueSet(selected []string, value string) bool {
	if len(selected) == 0 {
		return true
	}
	if value == "" {
		return false
	}
	for _, candidate := range selected {
		if candidate == value {
			return true
		}
	}
	return false
}

func matchesSponsorTerms(selected []string, terms map[string]struct{}) bool {
	if len(selected) == 0 {
		return true
	}
	if len(terms) == 0 {
		return false
	}
	for _, candidate := range selected {
		if _, ok := terms[candidate]; ok {
			return true
		}
	}
	return false
}

func matchesRIRAndNIRPreferences(prefs preferences.UserPreferences, event *NotificationEvent) bool {
	if event.RIR != "APNIC" {
		if len(prefs.RIRs) == 0 {
			return len(prefs.NIRs) == 0
		}
		return matchesValueSet(prefs.RIRs, event.RIR)
	}

	hasAPNIC := containsValue(prefs.RIRs, "APNIC")
	selectedNIR := matchesValueSet(prefs.NIRs, event.NIR)

	switch {
	case len(prefs.RIRs) == 0 && len(prefs.NIRs) == 0:
		return true
	case hasAPNIC && len(prefs.NIRs) == 0:
		return event.NIR == ""
	case hasAPNIC && len(prefs.NIRs) > 0:
		return event.NIR == "" || selectedNIR
	case !hasAPNIC && len(prefs.NIRs) > 0:
		return event.NIR != "" && selectedNIR
	default:
		return false
	}
}
