package communityapps

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeCAEntriesRejectsOverlargeArrays(t *testing.T) {
	const pluginFields = `s:4:"Name";s:6:"Plugin";s:6:"Plugin";b:1;`
	for _, tc := range []struct{ name, data string }{
		{"top-level entry limit", "a:50001:{" + strings.Repeat("i:0;a:2:{"+pluginFields+"}", 50001) + "}"},
		{"nested field limit", "a:1:{i:0;a:3:{" + pluginFields + `s:5:"Extra";a:8193:{` + strings.Repeat("i:0;N;", 8193) + "}}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeCAEntries(t.Context(), []byte(tc.data)); !errors.Is(err, errCacheEncoding) {
				t.Fatalf("overlarge serialized array returned %v, want encoding error", err)
			}
		})
	}
}

func TestPHPDecoderReadsFloat(t *testing.T) {
	decoder := phpDecoder{input: `a:1:{s:5:"score";d:1.25;}`}
	value, err := decoder.value(0)
	fields, ok := value.(phpArray)
	if err != nil || !ok || fields.get("score") != 1.25 || decoder.input != "" {
		t.Fatalf("decoder returned %#v, %v and trailing %q", value, err, decoder.input)
	}
}
