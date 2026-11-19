package communityapps_test

import (
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestInstalledCatalogLinkRejectsMismatchedNativeXML(t *testing.T) {
	root := t.TempDir()
	const template = "<Container><Name>example</Name><TemplateURL>https://catalog.example.org/example.xml</TemplateURL></Container>"
	service := communityapps.NewService(nil, communityapps.ServiceOptions{HostRoot: root})
	t.Cleanup(service.Close)
	for _, native := range []string{"<Unexpected/>", template + "<unfinished"} {
		communityappstest.WriteFile(t, root, recoveryTemplateDir+"/my-example.xml", native)
		if link := service.InstalledCatalogLink(t.Context(), "example", map[string]string{"net.unraid.docker.managed": "dockerman"}); link != "" {
			t.Fatalf("malformed XML returned provenance %q", link)
		}
	}
}
