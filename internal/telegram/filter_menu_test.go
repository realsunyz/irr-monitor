package telegram

import (
	"testing"

	"github.com/realSunyz/irr-monitor/internal/preferences"
)

func TestApplyFilterActionToggleAPNICSelectsAndClearsNIRs(t *testing.T) {
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
	if len(prefs.NIRs) != 0 {
		t.Fatalf("expected NIRs to be cleared, got %v", prefs.NIRs)
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
