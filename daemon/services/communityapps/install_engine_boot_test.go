package communityapps

import (
	"crypto/rand"
	"io/fs"
	"os"
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
	service.createReceiptTemp = func(string, string) (*os.File, error) { return nil, fs.ErrPermission }
	if service.oldEngineCallsDrained(&receipt) || receipt.EngineBootID != "" {
		t.Fatal("the first observed boot became usable before it was saved")
	}
	service.createReceiptTemp = os.CreateTemp
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
	if !fresh.oldEngineCallsDrained(&durable) {
		t.Fatal("a boot change did not drain the old Engine call")
	}
}

func TestLegacyCreateCleanupResolvesAbsenceAcrossHostBoot(t *testing.T) {
	root := t.TempDir()
	docker := &communityappstest.InstallEngine{}
	options := ServiceOptions{
		HostRoot:          root,
		OpenInstallDocker: func() (InstallDocker, error) { return docker, nil },
	}
	service := NewService(nil, options)
	t.Cleanup(service.Close)
	receipt := installReceipt{
		Version: 1, PlanID: rand.Text(), Owner: rand.Text(), ImageID: communityappstest.ImageID,
		Attempt: Attempt{
			ID: rand.Text(), State: AttemptRunning, Phase: "cleaning_up",
			Configuration: Configuration{Name: "fixture", Image: "example/fixture:1"},
		},
	}
	if err := service.saveReceipt(receipt, true); err != nil {
		t.Fatal(err)
	}
	if err := service.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	const initialBoot = "11111111-1111-1111-1111-111111111111"
	communityappstest.WriteFile(t, root, engineBootIDPath, initialBoot+"\n")
	if service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("first observation of a boot released an unresolved legacy create")
	}
	if pending, err := service.GetAttempt(receipt.Attempt.ID); err != nil || pending.State != AttemptRunning ||
		pending.Problem == nil || pending.Problem.Code != "container_create_unverified" {
		t.Fatalf("legacy pending status = %+v, %v", pending, err)
	}

	rebooted := NewService(nil, options)
	t.Cleanup(rebooted.Close)
	if err := rebooted.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	durable := rebooted.receipts[receipt.PlanID]
	if durable.EngineBootID != initialBoot || durable.OwnedContainerID != "" {
		t.Fatalf("expected a pending legacy claim with saved boot evidence, got %+v", durable)
	}
	communityappstest.WriteFile(t, root, engineBootIDPath, "22222222-2222-2222-2222-222222222222\n")
	if !rebooted.reconcileInstall(t.Context(), durable) {
		t.Fatal("known absence across a host boot did not settle the legacy create")
	}
	if final, err := rebooted.GetAttempt(receipt.Attempt.ID); err != nil || final.State != AttemptFailed ||
		final.Problem == nil || final.Problem.Code != "install_interrupted" || final.ContainerID != "" {
		t.Fatalf("unexpected legacy boot outcome: %+v, %v", final, err)
	}
	if creates, _, starts, _, _ := docker.Observation(); creates != 0 || len(starts) != 0 || len(docker.RemoveRequests()) != 0 {
		t.Fatal("boot recovery issued a Docker mutation for an absent legacy create")
	}
}
