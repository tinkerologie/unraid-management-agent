package lib

import (
	"strings"
	"testing"
)

func TestValidateAPIToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
		errWant string
	}{
		{
			name:  "empty token is valid and means authentication is disabled",
			token: "",
		},
		{
			name:  "ordinary token is accepted",
			token: "s3cr3t-token",
		},
		{
			// sanitize_str in the plugin start script used to strip these, which
			// silently corrupted the credential. They must survive validation.
			name:  "shell metacharacters are accepted",
			token: "tok$en\"with`meta\\chars'",
		},
		{
			name:  "base64 padding and slashes are accepted",
			token: "aGVsbG8gd29ybGQ/Zm9vK2Jhcg==",
		},
		{
			name:    "whitespace-only token is rejected",
			token:   "   ",
			wantErr: true,
			errWant: "whitespace-only",
		},
		{
			name:    "tab-only token is rejected",
			token:   "\t",
			wantErr: true,
			errWant: "whitespace-only",
		},
		{
			// The middleware trims the configured token, so a padded value would
			// authenticate as the trimmed form — a different credential than the
			// operator configured.
			name:    "trailing whitespace is rejected",
			token:   "admin ",
			wantErr: true,
			errWant: "leading or trailing whitespace",
		},
		{
			name:    "leading whitespace is rejected",
			token:   " admin",
			wantErr: true,
			errWant: "leading or trailing whitespace",
		},
		{
			name:    "leading tab is rejected",
			token:   "\tadmin",
			wantErr: true,
			errWant: "leading or trailing whitespace",
		},
		{
			name:    "embedded newline is rejected",
			token:   "abc\ndef",
			wantErr: true,
			errWant: "control character",
		},
		{
			name:    "embedded NUL is rejected",
			token:   "abc\x00def",
			wantErr: true,
			errWant: "control character",
		},
		{
			name:    "DEL is rejected",
			token:   "abc\x7fdef",
			wantErr: true,
			errWant: "control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIToken(tt.token)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateAPIToken(%q) = nil, want error", tt.token)
				}
				if !strings.Contains(err.Error(), tt.errWant) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errWant)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateAPIToken(%q) = %v, want nil", tt.token, err)
			}
		})
	}
}
