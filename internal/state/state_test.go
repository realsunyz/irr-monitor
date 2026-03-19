package state

import (
	"reflect"
	"testing"
)

func TestDecodeStateSupportsCurrentMapFormat(t *testing.T) {
	got, err := decodeState([]byte(`{"RIPE":123,"ARIN":456}`))
	if err != nil {
		t.Fatalf("decodeState returned error: %v", err)
	}

	want := map[string]int64{
		"RIPE": 123,
		"ARIN": 456,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected serials: got %#v want %#v", got, want)
	}
}

func TestDecodeStateSupportsWrappedFormat(t *testing.T) {
	got, err := decodeState([]byte(`{"serials":{"RIPE":123,"ARIN":456}}`))
	if err != nil {
		t.Fatalf("decodeState returned error: %v", err)
	}

	want := map[string]int64{
		"RIPE": 123,
		"ARIN": 456,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected serials: got %#v want %#v", got, want)
	}
}

func TestDecodeStateSupportsLegacySingleRIPEFormat(t *testing.T) {
	got, err := decodeState([]byte(`12345`))
	if err != nil {
		t.Fatalf("decodeState returned error: %v", err)
	}

	want := map[string]int64{
		"RIPE": 12345,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected serials: got %#v want %#v", got, want)
	}
}
