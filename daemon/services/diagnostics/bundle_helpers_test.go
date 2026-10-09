package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMemInfo(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string]int64
	}{
		{
			name: "typical meminfo",
			data: "MemTotal:       16384000 kB\nMemFree:         8192000 kB\nBuffers:          512000 kB\nCached:          1024000 kB\n",
			want: map[string]int64{
				"MemTotal": 16384000,
				"MemFree":  8192000,
				"Buffers":  512000,
				"Cached":   1024000,
			},
		},
		{
			name: "empty input",
			data: "",
			want: map[string]int64{},
		},
		{
			name: "lines without values are skipped",
			data: "MemTotal:\nMemFree: 100 kB\n",
			want: map[string]int64{"MemFree": 100},
		},
		{
			name: "non-numeric values are skipped",
			data: "MemTotal: notanumber kB\nMemFree: 200 kB\n",
			want: map[string]int64{"MemFree": 200},
		},
		{
			name: "no final newline",
			data: "MemTotal: 500 kB",
			want: map[string]int64{"MemTotal": 500},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMemInfo(tt.data)
			if len(got) != len(tt.want) {
				t.Fatalf("parseMemInfo() returned %d keys, want %d: %v", len(got), len(tt.want), got)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("parseMemInfo()[%q] = %d, want %d", k, got[k], want)
				}
			}
		})
	}
}

func TestReadLastNLines(t *testing.T) {
	t.Run("missing file returns nil", func(t *testing.T) {
		got := readLastNLines(filepath.Join(t.TempDir(), "does-not-exist.log"), 10)
		if got != nil {
			t.Errorf("expected nil for missing file, got %v", got)
		}
	})

	t.Run("empty file returns nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.log")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		got := readLastNLines(path, 10)
		if got != nil {
			t.Errorf("expected nil for empty file, got %v", got)
		}
	})

	t.Run("fewer lines than n returns all", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "few.log")
		if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		got := readLastNLines(path, 10)
		want := []string{"a", "b", "c"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("returns exactly last n in order", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "many.log")
		if err := os.WriteFile(path, []byte("1\n2\n3\n4\n5\n"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		got := readLastNLines(path, 3)
		want := []string{"3", "4", "5"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("no trailing newline still reads last line", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonewline.log")
		if err := os.WriteFile(path, []byte("x\ny\nz"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		got := readLastNLines(path, 2)
		want := []string{"y", "z"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("redacts sensitive data", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secret.log")
		content := "connecting password=hunter2 now\nBearer abc.def.ghi\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		got := readLastNLines(path, 10)
		for _, line := range got {
			if strings.Contains(line, "hunter2") {
				t.Errorf("password not redacted: %q", line)
			}
			if strings.Contains(line, "abc.def.ghi") {
				t.Errorf("bearer token not redacted: %q", line)
			}
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(got))
		}
		if !strings.Contains(got[0], "[REDACTED]") {
			t.Errorf("expected redaction marker in %q", got[0])
		}
	})
}
