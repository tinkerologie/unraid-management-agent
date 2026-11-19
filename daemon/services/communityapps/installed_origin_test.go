package communityapps_test

import (
	"context"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestInstalledCatalogLinkUsesOneUnambiguousPublicNativeTemplate(t *testing.T) {
	const link = "https://catalog.example.org/containers/example.xml"
	const native = "<Container><Name>example</Name><TemplateURL>" + link + "</TemplateURL></Container>"
	tests := []struct {
		name  string
		xml   string
		extra string
		label string
		want  string
	}{
		{name: "native managed", xml: native, label: "dockerman", want: link},
		{name: "unmanaged container", xml: native},
		{name: "duplicate template names", xml: native, extra: native, label: "dockerman"},
		{name: "missing origin", xml: "<Container><Name>example</Name></Container>", label: "dockerman"},
		{name: "multiple origin links", xml: "<Container><Name>example</Name><TemplateURL>" + link + "</TemplateURL><TemplateURL>" + link + "</TemplateURL></Container>", label: "dockerman"},
		{name: "query in link", xml: "<Container><Name>example</Name><TemplateURL>" + link + "?one=2</TemplateURL></Container>", label: "dockerman"},
		{name: "literal IP in link", xml: "<Container><Name>example</Name><TemplateURL>https://[::1]/catalog.xml</TemplateURL></Container>", label: "dockerman"},
		{name: "malformed XML", xml: "<Container><Name>example", label: "dockerman"},
		{name: "extra root element", xml: native + native, label: "dockerman"},
		{name: "extra text", xml: native + "text", label: "dockerman"},
		{name: "directive", xml: "<!DOCTYPE Container>" + native, label: "dockerman"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			communityappstest.WriteFile(t, root, recoveryTemplateDir+"/my-example.xml", tt.xml)
			communityappstest.WriteFile(t, root, recoveryTemplateDir+"/.ignored-directory/my-example.xml", native)
			communityappstest.WriteFile(t, root, recoveryTemplateDir+"/.ignored-file.xml", native)
			if tt.extra != "" {
				communityappstest.WriteFile(t, root, recoveryTemplateDir+"/more/duplicate.xml", tt.extra)
			}
			service := communityapps.NewService(nil, communityapps.ServiceOptions{HostRoot: root})
			t.Cleanup(service.Close)
			labels := map[string]string{"net.unraid.docker.managed": tt.label}
			if got := service.InstalledCatalogLink(t.Context(), "example", labels); got != tt.want {
				t.Fatalf("installed origin = %q, want %q", got, tt.want)
			}
		})
	}
	absent := communityapps.NewService(nil, communityapps.ServiceOptions{HostRoot: t.TempDir()})
	t.Cleanup(absent.Close)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := absent.InstalledCatalogLink(ctx, "example", map[string]string{"net.unraid.docker.managed": "dockerman"}); got != "" {
		t.Fatalf("cancelled lookup with missing native directory returned %q", got)
	}
}
