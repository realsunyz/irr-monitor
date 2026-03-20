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

	if !matchesValueSet(prefs.ASNSizes, e.ASNSize) {
		return false
	}
	if !matchesValueSet(prefs.RIRs, e.RIR) {
		return false
	}
	if !matchesValueSet(prefs.NIRs, e.NIR) {
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
	addSponsorTerm(terms, autNum.SponsoringOrgName)

	if source == "APNIC" {
		if nir := getNIRName(autNum); nir != "" {
			addSponsorTerm(terms, nir)
		}
		addSponsorTerm(terms, autNum.MntBy)
	}

	return terms
}

func addSponsorTerm(terms map[string]struct{}, value string) {
	value = normalizeSponsorValue(value)
	if value == "" {
		return
	}
	terms[value] = struct{}{}
}

func normalizeSponsorValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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
