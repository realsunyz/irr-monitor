package nrtm

import (
	"testing"
)

func TestParseRPSLObject_AutNum(t *testing.T) {
	input := `aut-num:        AS12345
as-name:        EXAMPLE-AS
descr:          Example Network
country:        NL
org:            ORG-DUMY-RIPE
admin-c:        DUMY-RIPE
tech-c:         DUMY-RIPE
mnt-by:         EXAMPLE-MNT
created:        2026-01-01T00:00:00Z
last-modified:  2026-01-01T00:00:00Z
source:         RIPE`

	obj := ParseRPSLObject(input)
	if obj == nil {
		t.Fatal("Expected object, got nil")
	}

	if obj.Type != "aut-num" {
		t.Errorf("Expected type 'aut-num', got '%s'", obj.Type)
	}

	if !obj.IsAutNum() {
		t.Error("Expected IsAutNum() to return true")
	}

	autNum := obj.ToAutNum()
	if autNum == nil {
		t.Fatal("Expected AutNum, got nil")
	}

	if autNum.ASN != "AS12345" {
		t.Errorf("Expected ASN 'AS12345', got '%s'", autNum.ASN)
	}

	if autNum.AsName != "EXAMPLE-AS" {
		t.Errorf("Expected AsName 'EXAMPLE-AS', got '%s'", autNum.AsName)
	}

	if autNum.Country != "NL" {
		t.Errorf("Expected Country 'NL', got '%s'", autNum.Country)
	}

	if autNum.Source != "RIPE" {
		t.Errorf("Expected Source 'RIPE', got '%s'", autNum.Source)
	}
}

func TestParseRPSLObject_EmptyInput(t *testing.T) {
	obj := ParseRPSLObject("")
	if obj != nil {
		t.Error("Expected nil for empty input")
	}
}

func TestParseRPSLObject_NotAutNum(t *testing.T) {
	input := `inetnum:        192.0.2.0 - 192.0.2.255
netname:        EXAMPLE-NET
descr:          Example Network
country:        US
source:         RIPE`

	obj := ParseRPSLObject(input)
	if obj == nil {
		t.Fatal("Expected object, got nil")
	}

	if obj.Type != "inetnum" {
		t.Errorf("Expected type 'inetnum', got '%s'", obj.Type)
	}

	if obj.IsAutNum() {
		t.Error("Expected IsAutNum() to return false for inetnum")
	}

	if obj.ToAutNum() != nil {
		t.Error("Expected ToAutNum() to return nil for non-aut-num")
	}
}
