package platform

import (
	"reflect"
	"testing"
)

func TestMissingKeys(t *testing.T) {
	m := map[string]string{"a": "1", "b": ""}
	got := MissingKeys(m, "a", "b", "c")
	if !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("MissingKeys = %v, want [b c]", got)
	}
}
