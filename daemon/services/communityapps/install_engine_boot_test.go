package communityapps

import (
	"crypto/rand"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestLegacyRecoveryEstablishesItsFirstKnownBoot(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := NewService(nil, ServiceOptions{HostRoot: root})
	t.Cleanup(service.Close)
	receipt := installReceipt{
		Version: 1, PlanID: rand.Text(), Owner: rand.Text(),
		Attempt: Attempt{ID: rand.Text(), State: AttemptRunning, Phase: "creating_container", Configuration: Configuration{Name: "fixture"}},
	}
	if err := service.saveReceipt(receipt, true); err != nil {
		t.Fatal(err)
	}
	if err := service.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	communityappstest.WriteFile(t, root, engineBootIDPath, "invalid-boot-evidence")
	if service.oldEngineCallsDrained(&receipt) || receipt.EngineBootID != "" {
		t.Fatal("invalid boot evidence was accepted")
	}
	const initialBoot = "11111111-1111-1111-1111-111111111111"
	communityappstest.WriteFile(t, root, engineBootIDPath, initialBoot+"\n")
	if service.oldEngineCallsDrained(&receipt) || receipt.EngineBootID != initialBoot {
		t.Fatal("first observation did not establish a boot without settling the receipt")
	}
	fresh := NewService(nil, ServiceOptions{HostRoot: root})
	t.Cleanup(fresh.Close)
	if err := fresh.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	durable := fresh.receipts[receipt.PlanID]
	if durable.EngineBootID != initialBoot || fresh.oldEngineCallsDrained(&durable) {
		t.Fatal("lack of a boot change released a pending intent")
	}
	communityappstest.WriteFile(t, root, engineBootIDPath, "22222222-2222-2222-2222-222222222222\n")
	if !fresh.oldEngineCallsDrained(&durable) || !durable.EngineBootEnded {
		t.Fatal("a boot change did not drain the old Engine call")
	}
	durable.EngineBootID = "invalid-boot-evidence"
	if validRecoveryReceipt(durable) {
		t.Fatal("receipt with invalid terminal boot evidence was treated as valid")
	}
}
