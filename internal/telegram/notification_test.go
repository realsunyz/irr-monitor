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

	wantTerms := map[string]struct{}{
		"org-example": {},
	}
	if !reflect.DeepEqual(event.SponsorTerms, wantTerms) {
		t.Fatalf("SponsorTerms = %#v, want %#v", event.SponsorTerms, wantTerms)
	}
}

func TestNotificationEventMatchesPreferences(t *testing.T) {
	t.Parallel()

	event := buildNotificationEvent("APNIC", &AutNum{
		ASN:           "AS64512",
		AsName:        "EXAMPLE",
		MntBy:         "MAINT-CNNIC-AP",
		SponsoringOrg: "ORG-CNNIC-AP",
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
			name: "enabled user with no filters does not match",
			prefs: preferences.UserPreferences{
				Enabled: true,
			},
			want: false,
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
			name: "4-byte filter excludes 2-byte asns",
			prefs: preferences.UserPreferences{
				Enabled:  true,
				ASNSizes: []string{"4b"},
			},
			want: false,
		},
		{
			name: "sponsor terms match sponsoring org only",
			prefs: preferences.UserPreferences{
				Enabled:        true,
				SponsoringOrgs: []string{"org-cnnic-ap"},
			},
			want: true,
		},
		{
			name: "sponsor terms do not match mnt-by or nir names",
			prefs: preferences.UserPreferences{
				Enabled:        true,
				SponsoringOrgs: []string{"cnnic"},
			},
			want: false,
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

func TestNotificationEventMatchesPreferencesForAPNICModes(t *testing.T) {
	t.Parallel()

	cnnicEvent := buildNotificationEvent("APNIC", &AutNum{
		ASN:   "AS64512",
		MntBy: "MAINT-CNNIC-AP",
	})
	directAPNICEvent := buildNotificationEvent("APNIC", &AutNum{
		ASN:   "AS64513",
		MntBy: "MAINT-EXAMPLE-AP",
	})

	tests := []struct {
		name  string
		prefs preferences.UserPreferences
		event *NotificationEvent
		want  bool
	}{
		{
			name: "nir only matches selected nir",
			prefs: preferences.UserPreferences{
				Enabled: true,
				NIRs:    []string{"CNNIC"},
			},
			event: cnnicEvent,
			want:  true,
		},
		{
			name: "nir only excludes direct apnic",
			prefs: preferences.UserPreferences{
				Enabled: true,
				NIRs:    []string{"CNNIC"},
			},
			event: directAPNICEvent,
			want:  false,
		},
		{
			name: "apnic without nirs excludes all nirs",
			prefs: preferences.UserPreferences{
				Enabled: true,
				RIRs:    []string{"APNIC"},
			},
			event: cnnicEvent,
			want:  false,
		},
		{
			name: "apnic without nirs includes direct apnic",
			prefs: preferences.UserPreferences{
				Enabled: true,
				RIRs:    []string{"APNIC"},
			},
			event: directAPNICEvent,
			want:  true,
		},
		{
			name: "apnic with partial nirs includes direct apnic and selected nir",
			prefs: preferences.UserPreferences{
				Enabled: true,
				RIRs:    []string{"APNIC"},
				NIRs:    []string{"CNNIC"},
			},
			event: directAPNICEvent,
			want:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.event.MatchesPreferences(tt.prefs); got != tt.want {
				t.Fatalf("MatchesPreferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSponsorInputReplacesWithNormalizedValues(t *testing.T) {
	t.Parallel()

	got, err := parseSponsorInput(" ORG-ml942-ripe,\nORG-abc123-ap,\nlir,\norg-ml942-ripe ")
	if err != nil {
		t.Fatalf("parseSponsorInput() error = %v", err)
	}
	want := []string{"lir", "org-abc123-ap", "org-ml942-ripe"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSponsorInput() = %#v, want %#v", got, want)
	}
}

func TestParseSponsorInputRejectsInvalidFormats(t *testing.T) {
	t.Parallel()

	if _, err := parseSponsorInput("Example Sponsor, ORG-OK-RIPE"); err == nil {
		t.Fatalf("expected invalid sponsor format error")
	}
}

func TestBuildNotificationEventAddsLIRSponsorTerm(t *testing.T) {
	t.Parallel()

	event := buildNotificationEvent("RIPE", &AutNum{
		ASN:     "AS64512",
		OrgType: "LIR",
	})

	if _, ok := event.SponsorTerms["lir"]; !ok {
		t.Fatalf("expected lir sponsor term in %#v", event.SponsorTerms)
	}
}

func TestBuildNotificationEventExcludesAPNICNIRFromLIRSponsorTerm(t *testing.T) {
	t.Parallel()

	event := buildNotificationEvent("APNIC", &AutNum{
		ASN:     "AS64512",
		MntBy:   "MAINT-CNNIC-AP",
		OrgType: "LIR",
	})

	if _, ok := event.SponsorTerms["lir"]; ok {
		t.Fatalf("did not expect lir sponsor term in %#v", event.SponsorTerms)
	}
}

func TestApplyFilterActionTogglesValues(t *testing.T) {
	t.Parallel()

	prefs := preferences.UserPreferences{}

	if _, err := applyFilterAction(&prefs, "filters:size:2b"); err != nil {
		t.Fatalf("size toggle error = %v", err)
	}
	if _, err := applyFilterAction(&prefs, "filters:rir:ARIN"); err != nil {
		t.Fatalf("rir toggle error = %v", err)
	}
	if _, err := applyFilterAction(&prefs, "filters:nir:JPNIC"); err != nil {
		t.Fatalf("nir toggle error = %v", err)
	}
	if _, err := applyFilterAction(&prefs, callbackToggleEnabled); err != nil {
		t.Fatalf("toggle enabled error = %v", err)
	}
	if !prefs.Enabled {
		t.Fatalf("expected Enabled to be true")
	}

	want := preferences.UserPreferences{
		Enabled:  true,
		ASNSizes: []string{"2b"},
		RIRs:     []string{"ARIN"},
		NIRs:     []string{"JPNIC"},
	}
	prefs.Normalize()
	if !reflect.DeepEqual(prefs, want) {
		t.Fatalf("prefs = %#v, want %#v", prefs, want)
	}
}
