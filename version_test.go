package search

import (
	"strings"
	"testing"
	"time"
)

func TestVersionFileRoundTrip(t *testing.T) {
	built := time.Date(2026, 10, 1, 15, 4, 5, 0, time.UTC)
	until := time.Date(2026, 10, 2, 15, 4, 5, 0, time.UTC)
	docs := uint32(600)
	enabled := true
	hidden := false
	public := true
	full := VersionFile{
		ObjectKey: "search/org/01H/2026-10-01T15:04:05Z.bin",
		ETag:      "abc",
		Size:      92472,
		BuiltAt:   built,
		DocCount:  &docs,
		IsEnabled: &enabled,
		IsHidden:  &hidden,
		PublicOK:  &public,
		Encoding:  "gzip",
		Keep: []KeepEntry{{
			ObjectKey: "search/org/01H/2026-09-30T18:12:00Z.bin",
			Until:     until,
		}},
	}
	raw, err := full.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"encoding":"gzip"`) || !strings.Contains(text, `"is_hidden":false`) || !strings.Contains(text, `"doc_count":600`) {
		t.Fatalf("json %s", text)
	}
	back, err := ParseVersionFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.Encoding != "gzip" || len(back.Keep) != 1 || back.Keep[0].ObjectKey != full.Keep[0].ObjectKey || !back.Keep[0].Until.Equal(until) {
		t.Fatalf("keep %+v", back.Keep)
	}
	if back.DocCount == nil || *back.DocCount != 600 || back.IsHidden == nil || *back.IsHidden || back.IsEnabled == nil || !*back.IsEnabled || back.PublicOK == nil || !*back.PublicOK {
		t.Fatalf("flags %+v", back)
	}
	if !back.BuiltAt.Equal(built) || back.ObjectKey != full.ObjectKey || back.ETag != full.ETag || back.Size != full.Size {
		t.Fatalf("pointer %+v", back)
	}

	rawOnly := VersionFile{ObjectKey: "frame.bin", ETag: "e", Size: 10, BuiltAt: built}
	rawBytes, err := rawOnly.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawBytes), "encoding") {
		t.Fatalf("empty encoding was written: %s", rawBytes)
	}
	parsed, err := ParseVersionFile(rawBytes)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Encoding != "" || parsed.DocCount != nil || parsed.IsHidden != nil || len(parsed.Keep) != 0 {
		t.Fatalf("raw pointer %+v", parsed)
	}
	if _, err := ParseVersionFile([]byte(`{"object_key":"k","nope":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}
