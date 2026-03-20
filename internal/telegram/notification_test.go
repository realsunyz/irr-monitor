package telegram

import (
	"reflect"
	"testing"

	"github.com/realSunyz/irr-monitor/internal/preferences"
)

func TestBuildNotificationEventDerivesMetadata(t *testing.T) {
	t.Parallel()

	event := buildNotificationEvent("APNIC", &AutNum{
		ASN:               "AS65551",
		AsName:            "EXAMPLE",
		MntBy:             "MAINT-JPNIC",
		SponsoringOrg:     "ORG-EXAMPLE",
		SponsoringOrgName: "Example Sponsor",
	})

	if event.ASNSize != asnSize4B {
		t.Fatalf("ASNSize = %q, want %q", event.ASNSize, asnSize4B)
	}
	if event.RIR != "APNIC" {
		t.Fatalf("RIR = %q, want APNIC", event.RIR)
	}
	if event.NIR != "JPNIC" {
		t.Fatalf("NIR = %q, want JPNIC", event.NIR)
	}

	for _, term := range []string{"org-example", "example sponsor", "jpnic", "maint-jpnic"} {
		if _, ok := event.SponsorTerms[term]; !ok {
			t.Fatalf("expected sponsor term %q in %#v", term, event.SponsorTerms)
		}
	}
}

func TestNotificationEventMatchesPreferences(t *testing.T) {
	t.Parallel()

	event := buildNotificationEvent("APNIC", &AutNum{
		ASN:    "AS64512",
		AsName: "EXAMPLE",
		MntBy:  "MAINT-CNNIC-AP",
	})

	tests := []struct {
		name  string
		prefs preferences.UserPreferences
		want  bool
	}{
		{
			name: "disabled user does not match",
			prefs: preferences.UserPreferences{
				Enabled: false,
			},
			want: false,
		},
		{
			name: "enabled user with no filters matches all",
			prefs: preferences.UserPreferences{
				Enabled: true,
			},
			want: true,
		},
		{
			name: "asn size matches",
			prefs: preferences.UserPreferences{
				Enabled:  true,
				ASNSizes: []string{"2b"},
			},
			want: true,
		},
		{
			name: "sponsor terms use exact normalized match",
			prefs: preferences.UserPreferences{
				Enabled:        true,
				SponsoringOrgs: []string{"cnnic"},
			},
			want: true,
		},
		{
			name: "combined dimensions use and-across",
			prefs: preferences.UserPreferences{
				Enabled: true,
				RIRs:    []string{"APNIC"},
				NIRs:    []string{"CNNIC"},
			},
			want: true,
		},
		{
			name: "combined dimensions fail when one dimension misses",
			prefs: preferences.UserPreferences{
				Enabled: true,
				RIRs:    []string{"APNIC"},
				NIRs:    []string{"JPNIC"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := event.MatchesPreferences(tt.prefs); got != tt.want {
				t.Fatalf("MatchesPreferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSponsorInputReplacesWithNormalizedValues(t *testing.T) {
	t.Parallel()

	got := parseSponsorInput(" Example Sponsor,\nEXAMPLE sponsor,\nmaint-jpnic ")
	want := []string{"example sponsor", "maint-jpnic"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSponsorInput() = %#v, want %#v", got, want)
	}
}

func TestApplyFilterActionTogglesValues(t *testing.T) {
	t.Parallel()

	prefs := preferences.UserPreferences{}

	if _, err := applyFilterAction(&prefs, callbackToggleEnabled); err != nil {
		t.Fatalf("toggle enabled error = %v", err)
	}
	if !prefs.Enabled {
		t.Fatalf("expected Enabled to be true")
	}

	if _, err := applyFilterAction(&prefs, "filters:size:2b"); err != nil {
		t.Fatalf("size toggle error = %v", err)
	}
	if _, err := applyFilterAction(&prefs, "filters:rir:ARIN"); err != nil {
		t.Fatalf("rir toggle error = %v", err)
	}
	if _, err := applyFilterAction(&prefs, "filters:nir:JPNIC"); err != nil {
		t.Fatalf("nir toggle error = %v", err)
	}

	want := preferences.UserPreferences{
		Enabled:  true,
		ASNSizes: []string{"2b"},
		RIRs:     []string{"APNIC", "ARIN"},
		NIRs:     []string{"JPNIC"},
	}
	prefs.Normalize()
	if !reflect.DeepEqual(prefs, want) {
		t.Fatalf("prefs = %#v, want %#v", prefs, want)
	}
}
