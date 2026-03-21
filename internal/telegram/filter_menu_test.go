package telegram

import (
	"strings"
	"testing"

	"github.com/realSunyz/irr-monitor/internal/preferences"
)

func TestApplyFilterActionToggleAPNICSelectsDefaultNIRsAndPreservesThemWhenCleared(t *testing.T) {
	prefs := preferences.UserPreferences{}

	if _, err := applyFilterAction(&prefs, "filters:rir:APNIC"); err != nil {
		t.Fatalf("applyFilterAction returned error: %v", err)
	}

	if !containsValue(prefs.RIRs, "APNIC") {
		t.Fatalf("expected APNIC to be selected, got %v", prefs.RIRs)
	}

	for _, nir := range knownNIRs {
		if !containsValue(prefs.NIRs, nir) {
			t.Fatalf("expected NIR %s to be selected, got %v", nir, prefs.NIRs)
		}
	}

	if _, err := applyFilterAction(&prefs, "filters:rir:APNIC"); err != nil {
		t.Fatalf("applyFilterAction returned error on second toggle: %v", err)
	}

	if containsValue(prefs.RIRs, "APNIC") {
		t.Fatalf("expected APNIC to be cleared, got %v", prefs.RIRs)
	}
	for _, nir := range knownNIRs {
		if !containsValue(prefs.NIRs, nir) {
			t.Fatalf("expected NIR %s to remain selected, got %v", nir, prefs.NIRs)
		}
	}
}

func TestRenderSubmenusIncludeBackButton(t *testing.T) {
	menus := []struct {
		name string
		menu string
	}{
		{name: "asn", menu: menuASN},
		{name: "rir", menu: menuRIR},
		{name: "sponsor", menu: menuSponsor},
	}

	for _, tc := range menus {
		_, markup := renderMenu(tc.menu, preferences.UserPreferences{})
		if markup == nil || len(markup.InlineKeyboard) == 0 {
			t.Fatalf("%s menu did not render keyboard", tc.name)
		}

		lastRow := markup.InlineKeyboard[len(markup.InlineKeyboard)-1]
		if tc.menu == menuRIR {
			if len(lastRow) != 2 {
				t.Fatalf("%s menu expected two-button final row, got %d buttons", tc.name, len(lastRow))
			}
			if lastRow[0].Text != "Clear All" || lastRow[1].Text != "Back" {
				t.Fatalf("%s menu expected Clear All/Back row, got %#v", tc.name, lastRow)
			}
			if lastRow[0].CallbackData != callbackClearRIR || lastRow[1].CallbackData != callbackOpenMain {
				t.Fatalf("%s menu unexpected callbacks in final row: %#v", tc.name, lastRow)
			}
			continue
		}
		if tc.menu == menuSponsor {
			if len(lastRow) != 3 {
				t.Fatalf("%s menu expected three-button final row, got %d buttons", tc.name, len(lastRow))
			}
			if lastRow[0].Text != "Custom" || lastRow[1].Text != "Clear All" || lastRow[2].Text != "Back" {
				t.Fatalf("%s menu expected Custom/Clear All/Back row, got %#v", tc.name, lastRow)
			}
			if lastRow[0].CallbackData != callbackCustomSponsor || lastRow[1].CallbackData != callbackClearSponsor || lastRow[2].CallbackData != callbackOpenMain {
				t.Fatalf("%s menu unexpected callbacks in final row: %#v", tc.name, lastRow)
			}
			continue
		}

		if len(lastRow) != 1 {
			t.Fatalf("%s menu expected single-button back row, got %d buttons", tc.name, len(lastRow))
		}
		if lastRow[0].Text != "Back" {
			t.Fatalf("%s menu expected Back button, got %q", tc.name, lastRow[0].Text)
		}
		if lastRow[0].CallbackData != callbackOpenMain {
			t.Fatalf("%s menu expected callback %q, got %q", tc.name, callbackOpenMain, lastRow[0].CallbackData)
		}
	}
}

func TestRenderMainMenuSummaryText(t *testing.T) {
	text, _ := renderMenu(menuMain, preferences.UserPreferences{
		Enabled:        true,
		ASNSizes:       []string{asnSize2B},
		RIRs:           []string{"ARIN", "RIPE"},
		NIRs:           []string{"CNNIC", "IDNIC"},
		SponsoringOrgs: []string{"org-abc-ripe"},
	})

	for _, want := range []string{
		"<b>ASN Notification</b>",
		"<b>ASN Byte:</b> 2-Byte",
		"<b>RIR (NIR):</b> APNIC (CNNIC, IDNIC Only), ARIN, RIPE",
		"<b>Sponsor:</b> ORG-ABC-RIPE",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("main menu text %q does not contain %q", text, want)
		}
	}
}

func TestRenderSubmenuCurrentText(t *testing.T) {
	sponsorText, _ := renderMenu(menuSponsor, preferences.UserPreferences{
		SponsoringOrgs: []string{"org-abc-ripe"},
	})
	if !strings.Contains(sponsorText, "<b>ASN Notification &gt; Sponsor Filter</b>") ||
		!strings.Contains(sponsorText, "This feature supports only ASNs assigned by APNIC and RIPE NCC.") ||
		!strings.Contains(sponsorText, "You can enter &quot;LIR&quot; to filter for ASNs owned by LIRs") ||
		!strings.Contains(sponsorText, "The buttons below list some common LIRs. You can also click &quot;Custom&quot; to enter your own.") ||
		!strings.Contains(sponsorText, "Current: ORG-ABC-RIPE") {
		t.Fatalf("unexpected sponsor text: %q", sponsorText)
	}

	asnText, _ := renderMenu(menuASN, preferences.UserPreferences{
		ASNSizes: []string{asnSize2B, asnSize4B},
	})
	if !strings.Contains(asnText, "<b>ASN Notification &gt; ASN Byte Filter</b>") ||
		!strings.Contains(asnText, "If you select the 4-Byte filter, 2-Byte ASNs (i.e., 0 to 65,535) will not be notified.") ||
		!strings.Contains(asnText, "Current: All") {
		t.Fatalf("unexpected asn text: %q", asnText)
	}

	rirText, _ := renderMenu(menuRIR, preferences.UserPreferences{})
	if !strings.Contains(rirText, "<b>ASN Notification &gt; RIR Filter</b>") ||
		!strings.Contains(rirText, "CNNIC, IDNIC, IRINN, JPNIC, KRNIC, and TWNIC are the National Internet Registries (NIRs) within the APNIC region.") ||
		!strings.Contains(rirText, "Current: All") {
		t.Fatalf("unexpected rir text: %q", rirText)
	}
}

func TestRenderRIRMenuShowsOrangeAPNICForNIROnly(t *testing.T) {
	_, markup := renderMenu(menuRIR, preferences.UserPreferences{
		NIRs: []string{"CNNIC", "IDNIC"},
	})

	apnicText := markup.InlineKeyboard[0][0].Text
	if apnicText != "⚪ APNIC" {
		t.Fatalf("APNIC button text = %q, want %q", apnicText, "⚪ APNIC")
	}
}

func TestRenderMainMenuShowsAllForUnspecifiedFilters(t *testing.T) {
	text, markup := renderMenu(menuMain, preferences.UserPreferences{})

	for _, want := range []string{
		"<b>ASN Byte:</b> All",
		"<b>RIR (NIR):</b> All",
		"<b>Sponsor:</b> All",
		"<i>To enable this feature, you must select at least one filter. Otherwise, simply subscribe to the @New_ASNs channel.</i>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("main menu text %q does not contain %q", text, want)
		}
	}

	if markup.InlineKeyboard[1][0].Text != "⚪ ASN Byte" {
		t.Fatalf("unexpected ASN button text: %q", markup.InlineKeyboard[1][0].Text)
	}
	if markup.InlineKeyboard[1][1].Text != "⚪ RIR (NIR)" {
		t.Fatalf("unexpected RIR button text: %q", markup.InlineKeyboard[1][1].Text)
	}
}

func TestSponsorInputPromptText(t *testing.T) {
	prompt := sponsorInputPrompt()

	for _, want := range []string{
		"Please enter the values for the sponsoring-org field, separated by commas if there are multiple entries.",
		"Allowed formats:",
		"- ORG-XXXXX-AP",
		"- ORG-XXXXX-RIPE",
		"- LIR",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt %q does not contain %q", prompt, want)
		}
	}
}

func TestRenderASNMenuPutsBothOptionsOnOneRowAndWhitesOutAll(t *testing.T) {
	_, markup := renderMenu(menuASN, preferences.UserPreferences{
		ASNSizes: []string{asnSize2B, asnSize4B},
	})

	firstRow := markup.InlineKeyboard[0]
	if len(firstRow) != 2 {
		t.Fatalf("expected ASN first row to have 2 buttons, got %d", len(firstRow))
	}
	if firstRow[0].Text != "⚪ 2-Byte" || firstRow[1].Text != "⚪ 4-Byte" {
		t.Fatalf("unexpected ASN row texts: %#v", firstRow)
	}
	if len(markup.InlineKeyboard) < 2 || markup.InlineKeyboard[1][0].Text != "Back" {
		t.Fatalf("expected Back row immediately after ASN options, got %#v", markup.InlineKeyboard)
	}
}

func TestRenderMainMenuShowsGreenASNButtonWhenAnyASNFilterSelected(t *testing.T) {
	_, markup := renderMenu(menuMain, preferences.UserPreferences{
		ASNSizes: []string{asnSize4B},
	})

	if markup.InlineKeyboard[1][0].Text != "🟢 ASN Byte" {
		t.Fatalf("unexpected ASN button text: %q", markup.InlineKeyboard[1][0].Text)
	}
}

func TestRenderMainMenuShowsGreenRIRAndSponsorButtonsWhenConfigured(t *testing.T) {
	_, markup := renderMenu(menuMain, preferences.UserPreferences{
		NIRs:           []string{"CNNIC"},
		SponsoringOrgs: []string{"org-abc-ripe"},
	})

	if markup.InlineKeyboard[1][1].Text != "🟢 RIR (NIR)" {
		t.Fatalf("unexpected RIR button text: %q", markup.InlineKeyboard[1][1].Text)
	}
	if markup.InlineKeyboard[1][2].Text != "🟢 Sponsor" {
		t.Fatalf("unexpected Sponsor button text: %q", markup.InlineKeyboard[1][2].Text)
	}
}

func TestRenderRIRSummaryUsesNIROnlyWhenAllNIRsSelectedWithoutAPNIC(t *testing.T) {
	text, markup := renderMenu(menuRIR, preferences.UserPreferences{
		NIRs: append([]string{}, knownNIRs...),
	})

	if !strings.Contains(text, "Current: APNIC (NIRs Only)") {
		t.Fatalf("unexpected RIR text: %q", text)
	}
	if markup.InlineKeyboard[0][0].Text != "⚪ APNIC" {
		t.Fatalf("unexpected APNIC button text: %q", markup.InlineKeyboard[0][0].Text)
	}
}

func TestRenderRIRMenuOrdersTopRowAsAPNICARINRIPE(t *testing.T) {
	_, markup := renderMenu(menuRIR, preferences.UserPreferences{})

	firstRow := markup.InlineKeyboard[0]
	got := []string{firstRow[0].Text, firstRow[1].Text, firstRow[2].Text}
	want := []string{"⚪ APNIC", "⚪ ARIN", "⚪ RIPE"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top row[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRenderRIRMenuShowsOrangeAPNICForExcludingAllNIRs(t *testing.T) {
	_, markup := renderMenu(menuRIR, preferences.UserPreferences{
		RIRs: []string{"APNIC"},
	})

	if markup.InlineKeyboard[0][0].Text != "🟠 APNIC" {
		t.Fatalf("unexpected APNIC button text: %q", markup.InlineKeyboard[0][0].Text)
	}
}

func TestApplyFilterActionAllowsNIROnlyState(t *testing.T) {
	prefs := preferences.UserPreferences{}

	if _, err := applyFilterAction(&prefs, "filters:nir:CNNIC"); err != nil {
		t.Fatalf("applyFilterAction returned error: %v", err)
	}
	if containsValue(prefs.RIRs, "APNIC") {
		t.Fatalf("expected APNIC to remain unchecked, got %v", prefs.RIRs)
	}

	if _, err := applyFilterAction(&prefs, "filters:rir:APNIC"); err != nil {
		t.Fatalf("applyFilterAction returned error: %v", err)
	}
	if !containsValue(prefs.RIRs, "APNIC") {
		t.Fatalf("expected APNIC to be selected, got %v", prefs.RIRs)
	}
	if !containsValue(prefs.NIRs, "CNNIC") {
		t.Fatalf("expected CNNIC to remain selected, got %v", prefs.NIRs)
	}

	if _, err := applyFilterAction(&prefs, "filters:rir:APNIC"); err != nil {
		t.Fatalf("applyFilterAction returned error on second toggle: %v", err)
	}
	if containsValue(prefs.RIRs, "APNIC") {
		t.Fatalf("expected APNIC to be cleared, got %v", prefs.RIRs)
	}
	if !containsValue(prefs.NIRs, "CNNIC") {
		t.Fatalf("expected CNNIC to remain selected after clearing APNIC, got %v", prefs.NIRs)
	}
}

func TestApplyFilterActionCannotEnableWithoutFilters(t *testing.T) {
	prefs := preferences.UserPreferences{}

	if _, err := applyFilterAction(&prefs, callbackToggleEnabled); err == nil {
		t.Fatalf("expected enabling push without filters to fail")
	} else if err.Error() != "You must select at least one filter." {
		t.Fatalf("unexpected error message: %v", err)
	}
	if prefs.Enabled {
		t.Fatalf("expected push to remain disabled")
	}
}

func TestApplyFilterActionClearAllDisablesPush(t *testing.T) {
	prefs := preferences.UserPreferences{
		Enabled:        true,
		ASNSizes:       []string{asnSize4B},
		RIRs:           []string{"ARIN"},
		SponsoringOrgs: []string{"org-abc-ripe"},
	}

	if _, err := applyFilterAction(&prefs, callbackClearAll); err != nil {
		t.Fatalf("clear all error = %v", err)
	}
	if prefs.Enabled {
		t.Fatalf("expected push to be disabled after clearing all filters")
	}
	if hasAnyFilterSelected(prefs) {
		t.Fatalf("expected all filters to be cleared, got %#v", prefs)
	}
}
