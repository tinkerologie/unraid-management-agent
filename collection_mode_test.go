package main

import (
	"os"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

// setCollectionModeEnv sets COLLECTION_MODE for the test, or unsets it when empty.
func setCollectionModeEnv(t *testing.T, v string) {
	t.Helper()
	t.Setenv("COLLECTION_MODE", v)
	if v == "" {
		if err := os.Unsetenv("COLLECTION_MODE"); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCollectionModePrecedence checks --collection-mode, COLLECTION_MODE and
// collection_mode in config.yml follow flag > env > file > default (#280).
func TestCollectionModePrecedence(t *testing.T) {
	dynamic, manual, bogus := domain.CollectionModeDynamic, domain.CollectionModeManual, "sometimes"
	tests := []struct {
		name string
		args []string
		env  string
		file *string
		want string
	}{
		{name: "default is manual", want: manual},
		{name: "file overrides default", file: &dynamic, want: dynamic},
		{name: "invalid file value is ignored", file: &bogus, want: manual},
		{name: "env beats file", env: manual, file: &dynamic, want: manual},
		{name: "env overrides default", env: dynamic, want: dynamic},
		{name: "flag beats env and file", args: []string{"--collection-mode=manual"}, env: dynamic, file: &dynamic, want: manual},
		{name: "flag overrides default", args: []string{"--collection-mode=dynamic"}, want: dynamic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved, savedExplicit := cli, explicitFlags
			t.Cleanup(func() { cli, explicitFlags = saved, savedExplicit })
			setCollectionModeEnv(t, tt.env)
			if _, _, err := parseCLI(append(tt.args, "boot"), kong.Exit(func(int) { t.Fatal("kong exited") })); err != nil {
				t.Fatal(err)
			}
			applyFileConfig(&domain.FileConfig{CollectionMode: tt.file})
			if cli.CollectionMode != tt.want {
				t.Errorf("CollectionMode = %q, want %q", cli.CollectionMode, tt.want)
			}
		})
	}
}

// TestCollectionModeRejectsInvalidValue checks Kong's enum rejects an
// unknown mode from the flag or the environment.
func TestCollectionModeRejectsInvalidValue(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		env  string
	}{
		{name: "flag", args: []string{"--collection-mode=fast"}},
		{name: "env", env: "fast"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			saved, savedExplicit := cli, explicitFlags
			t.Cleanup(func() { cli, explicitFlags = saved, savedExplicit })
			setCollectionModeEnv(t, tt.env)
			if _, _, err := parseCLI(append(tt.args, "boot"), kong.Exit(func(int) { t.Fatal("kong exited") })); err == nil {
				t.Error("parseCLI accepted an invalid collection mode")
			}
		})
	}
}
