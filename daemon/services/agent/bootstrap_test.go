package agent

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestLoadConfigDefaultsWhenMissing(t *testing.T) {
	cfg := LoadConfig(t.TempDir())
	if cfg.Enabled {
		t.Fatal("expected disabled default config")
	}
}

func TestBuildServiceEnabledBranches(t *testing.T) {
	cases := []struct {
		name      string
		enabled   bool
		provider  string
		apiKey    string
		wantSvc   bool
		wantErr   bool
		errSubstr string
	}{
		{name: "disabled", enabled: false, wantSvc: false, wantErr: false},
		{name: "anthropic with key", enabled: true, provider: "anthropic", apiKey: "sk-test", wantSvc: true, wantErr: false},
		{name: "anthropic no key", enabled: true, provider: "anthropic", apiKey: "", wantSvc: false, wantErr: true, errSubstr: "is not set"},
		{name: "unsupported provider", enabled: true, provider: "bogus", apiKey: "sk-test", wantSvc: false, wantErr: true, errSubstr: "unsupported agent provider"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Ensure the env key never leaks into the test expectations.
			t.Setenv("UMA_AGENT_API_KEY", "")
			cfg := dto.DefaultAgentConfig()
			cfg.Enabled = tc.enabled
			if tc.provider != "" {
				cfg.Provider = tc.provider
			}
			cfg.APIKey = tc.apiKey

			svc, err := BuildService(cfg, t.TempDir(), nil, nil, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (svc=%v)", svc)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.wantSvc && svc == nil {
				t.Fatal("expected non-nil service")
			}
			if !tc.wantSvc && svc != nil {
				t.Fatal("expected nil service")
			}
		})
	}
}
