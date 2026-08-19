package audit

import "testing"

func TestSanitizeMetadataDropsUnsafeAndTruncatesValues(t *testing.T) {
	long := "  "
	for i := 0; i < 600; i++ {
		long += "x"
	}
	got := SanitizeMetadata(map[string]string{" good ": " value ", "bad\nkey": "x", "": "empty", "long": long})
	if got["good"] != "value" {
		t.Fatalf("good metadata = %+v", got)
	}
	if _, ok := got["bad\nkey"]; ok {
		t.Fatal("unsafe key retained")
	}
	if len(got["long"]) != 512 {
		t.Fatalf("long value length = %d", len(got["long"]))
	}
}
