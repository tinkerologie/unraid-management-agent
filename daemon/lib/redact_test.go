package lib

import (
	"testing"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "no sensitive data",
			input: "hello world",
			want:  "hello world",
		},
		// Password patterns
		{
			name:  "password= pattern",
			input: "password=mysecret123",
			want:  "password=[REDACTED]",
		},
		{
			name:  "Password: pattern",
			input: "Password: mysecret123",
			want:  "Password: [REDACTED]",
		},
		{
			name:  "PASSWORD= uppercase",
			input: "PASSWORD=hunter2",
			want:  "PASSWORD=[REDACTED]",
		},
		// Bearer tokens
		{
			name:  "bearer token",
			input: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			want:  "Bearer [REDACTED]",
		},
		{
			name:  "bearer token lowercase",
			input: "bearer my-secret-token",
			want:  "bearer [REDACTED]",
		},
		// Shoutrrr URLs
		{
			name:  "ntfy URL",
			input: "ntfy://ntfy.example.com/mytopic",
			want:  "ntfy://[REDACTED]",
		},
		{
			name:  "gotify URL",
			input: "gotify://gotify.example.com/message?token=secret123",
			want:  "gotify://[REDACTED]",
		},
		{
			name:  "discord URL",
			input: "discord://token@channel",
			want:  "discord://[REDACTED]",
		},
		{
			name:  "slack URL",
			input: "slack://hook:token@workspace/channel",
			want:  "slack://[REDACTED]",
		},
		{
			name:  "telegram URL",
			input: "telegram://bottoken@telegram?channels=chat",
			want:  "telegram://[REDACTED]",
		},
		// Webhook URLs
		{
			name:  "webhook URL with token param",
			input: "https://hooks.example.com/api?token=abc123def",
			want:  "https://hooks.example.com/api?token=[REDACTED]",
		},
		{
			name:  "webhook URL with key param",
			input: "https://api.example.com/hook?key=secretkey123",
			want:  "https://api.example.com/hook?key=[REDACTED]",
		},
		{
			name:  "webhook path",
			input: "https://hooks.example.com/webhook/abc123def456",
			want:  "https://hooks.example.com/webhook/[REDACTED]",
		},
		// CSRF tokens
		{
			name:  "csrf_token= pattern",
			input: "csrf_token=abc123def456",
			want:  "csrf_token=[REDACTED]",
		},
		{
			name:  "CSRF_TOKEN uppercase",
			input: "CSRF_TOKEN=mytoken",
			want:  "CSRF_TOKEN=[REDACTED]",
		},
		// Mixed content
		{
			name:  "password in context",
			input: "connecting with password=secret to server",
			want:  "connecting with password=[REDACTED] to server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.input)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactMap(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want map[string]any
	}{
		{
			name: "nil map",
			m:    nil,
			want: nil,
		},
		{
			name: "empty map",
			m:    map[string]any{},
			want: map[string]any{},
		},
		{
			name: "sensitive field name",
			m:    map[string]any{"password": "secret123", "username": "admin"},
			want: map[string]any{"password": "[REDACTED]", "username": "admin"},
		},
		{
			name: "api_key field",
			m:    map[string]any{"api_key": "key123", "name": "test"},
			want: map[string]any{"api_key": "[REDACTED]", "name": "test"},
		},
		{
			name: "token field case insensitive",
			m:    map[string]any{"AuthToken": "secret", "host": "localhost"},
			want: map[string]any{"AuthToken": "[REDACTED]", "host": "localhost"},
		},
		{
			name: "nested map",
			m: map[string]any{
				"config": map[string]any{
					"password": "secret",
					"host":     "localhost",
				},
			},
			want: map[string]any{
				"config": map[string]any{
					"password": "[REDACTED]",
					"host":     "localhost",
				},
			},
		},
		{
			name: "string value with sensitive pattern",
			m:    map[string]any{"log": "password=secret123 connected"},
			want: map[string]any{"log": "password=[REDACTED] connected"},
		},
		{
			name: "slice values",
			m: map[string]any{
				"channels": []any{
					"ntfy://ntfy.example.com/topic",
					"normal string",
				},
			},
			want: map[string]any{
				"channels": []any{
					"ntfy://[REDACTED]",
					"normal string",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactMap(tt.m)
			if tt.want == nil {
				if got != nil {
					t.Errorf("RedactMap() = %v, want nil", got)
				}
				return
			}
			assertMapsEqual(t, got, tt.want)
		})
	}
}

func TestRedactStruct(t *testing.T) {
	type Config struct {
		Host     string `json:"host"`
		Password string `json:"password"`
		Port     int    `json:"port"`
	}

	type NestedConfig struct {
		Name   string `json:"name"`
		Secret string `json:"secret"`
		Inner  Config `json:"inner"`
	}

	t.Run("struct with sensitive field", func(t *testing.T) {
		cfg := Config{Host: "localhost", Password: "secret123", Port: 8080}
		result := RedactStruct(cfg)
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map[string]any")
		}
		if m["password"] != "[REDACTED]" {
			t.Errorf("password = %v, want [REDACTED]", m["password"])
		}
		if m["host"] != "localhost" {
			t.Errorf("host = %v, want localhost", m["host"])
		}
	})

	t.Run("nested struct", func(t *testing.T) {
		cfg := NestedConfig{
			Name:   "test",
			Secret: "topsecret",
			Inner:  Config{Host: "db", Password: "pass", Port: 5432},
		}
		result := RedactStruct(cfg)
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map[string]any")
		}
		if m["secret"] != "[REDACTED]" {
			t.Errorf("secret = %v, want [REDACTED]", m["secret"])
		}
		inner, ok := m["inner"].(map[string]any)
		if !ok {
			t.Fatal("expected inner to be map[string]any")
		}
		if inner["password"] != "[REDACTED]" {
			t.Errorf("inner.password = %v, want [REDACTED]", inner["password"])
		}
	})

	t.Run("nil value", func(t *testing.T) {
		result := RedactStruct(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("pointer to struct", func(t *testing.T) {
		cfg := &Config{Host: "localhost", Password: "secret", Port: 80}
		result := RedactStruct(cfg)
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map[string]any")
		}
		if m["password"] != "[REDACTED]" {
			t.Errorf("password = %v, want [REDACTED]", m["password"])
		}
	})
}

func TestIsSensitiveField(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"password", true},
		{"Password", true},
		{"PASSWORD", true},
		{"token", true},
		{"access_token", true},
		{"secret", true},
		{"api_key", true},
		{"credential", true},
		{"host", false},
		{"port", false},
		{"username", false},
		{"name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSensitiveField(tt.name); got != tt.want {
				t.Errorf("isSensitiveField(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// assertMapsEqual is a test helper for deep comparison of map[string]any.
func assertMapsEqual(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, wantV := range want {
		gotV, ok := got[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		switch wv := wantV.(type) {
		case string:
			if gv, ok := gotV.(string); !ok || gv != wv {
				t.Errorf("key %q = %v, want %v", k, gotV, wantV)
			}
		case map[string]any:
			gv, ok := gotV.(map[string]any)
			if !ok {
				t.Errorf("key %q expected map, got %T", k, gotV)
				continue
			}
			assertMapsEqual(t, gv, wv)
		case []any:
			gv, ok := gotV.([]any)
			if !ok {
				t.Errorf("key %q expected slice, got %T", k, gotV)
				continue
			}
			if len(gv) != len(wv) {
				t.Errorf("key %q slice length = %d, want %d", k, len(gv), len(wv))
				continue
			}
			for i, wItem := range wv {
				if ws, ok := wItem.(string); ok {
					if gs, ok := gv[i].(string); !ok || gs != ws {
						t.Errorf("key %q[%d] = %v, want %v", k, i, gv[i], wItem)
					}
				}
			}
		}
	}
}
