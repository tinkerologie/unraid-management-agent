package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContainerUpdateInfo_Status(t *testing.T) {
	tests := []struct {
		name string
		info ContainerUpdateInfo
		want string
	}{
		{"available", ContainerUpdateInfo{CurrentDigest: "sha256:a", LatestDigest: "sha256:b", UpdateAvailable: true}, UpdateStatusAvailable},
		{"up to date", ContainerUpdateInfo{CurrentDigest: "sha256:a", LatestDigest: "sha256:a"}, UpdateStatusUpToDate},
		{"unknown when no latest digest", ContainerUpdateInfo{CurrentDigest: "sha256:a"}, UpdateStatusUnknown},
		{"unknown overrides available when no latest digest", ContainerUpdateInfo{UpdateAvailable: true}, UpdateStatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.Status(); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContainerInfo_RestartCountMarshals(t *testing.T) {
	c := ContainerInfo{Name: "plex", RestartCount: 3}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"restart_count":3`) {
		t.Errorf("expected restart_count:3, got %s", b)
	}
}

func TestContainerInfo_UpdateMarshalOmitsNilBool(t *testing.T) {
	c := ContainerInfo{Name: "plex", UpdateStatus: UpdateStatusUnknown}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, `"update_available"`) {
		t.Errorf("expected update_available omitted when nil, got %s", s)
	}
	if !strings.Contains(s, `"update_status":"unknown"`) {
		t.Errorf("expected update_status=unknown, got %s", s)
	}
}
