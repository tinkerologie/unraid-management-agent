package collectors

import "testing"

func TestValidateRequiredKeys(t *testing.T) {
	ok, reason := validateRequiredKeys(map[string]string{"mdState": "STARTED"}, "mdState")
	if !ok || reason != "" {
		t.Fatalf("expected ok, got ok=%v reason=%q", ok, reason)
	}
	ok, reason = validateRequiredKeys(map[string]string{}, "mdState", "mdNumDisks")
	if ok || reason == "" {
		t.Fatalf("expected failure with reason, got ok=%v reason=%q", ok, reason)
	}
}
