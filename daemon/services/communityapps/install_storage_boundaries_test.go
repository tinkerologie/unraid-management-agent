package communityapps

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"golang.org/x/sys/unix"
)

func TestPrepareStorageRefusesChangesAtTheFileSystemBoundary(t *testing.T) {
	for _, name := range []string{"host root no longer opens", "competing creation", "published directory replaced"} {
		t.Run(name, func(t *testing.T) {
			service, _, preview := newWorkerFixture(t)
			plan, err := service.lookupPending(preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(service.hostRoot, plan.effective.configuration.Mounts[0].Source)
			openRoot, create := os.OpenRoot, service.createAppdata
			want := ErrPlanChanged
			switch name {
			case "host root no longer opens":
				openRoot = func(string) (*os.Root, error) { return nil, fs.ErrPermission }
				want = ErrStorageState
			case "competing creation":
				openRoot = func(path string) (*os.Root, error) {
					if err := os.Mkdir(source, 0o750); err != nil {
						return nil, err
					}
					if err := os.WriteFile(filepath.Join(source, "keep"), []byte("competing appdata"), 0o600); err != nil {
						return nil, err
					}
					return os.OpenRoot(path)
				}
			case "published directory replaced":
				create = func(root *os.Root, identity mountIdentity, src string, created map[string]mountIdentity) (mountIdentity, error) {
					id, err := service.createAppdata(root, identity, src, created)
					if err != nil {
						return id, err
					}
					if err := os.Rename(source, source+"-original"); err != nil {
						return id, err
					}
					return id, os.Mkdir(source, 0o750)
				}
			}
			mounts, err := service.prepareStorage(plan.effective, openRoot, create)
			if mounts != nil || !errors.Is(err, want) {
				t.Fatalf("unavailable or changed storage was approved: %+v, %v", mounts, err)
			}
			if name == "competing creation" {
				got, err := os.ReadFile(filepath.Join(source, "keep"))
				if err != nil || string(got) != "competing appdata" {
					t.Fatalf("exclusive publication changed competing appdata: %q, %v", got, err)
				}
			}
		})
	}
}

func openAppdataFixture(t *testing.T) (*Service, *os.Root, mountIdentity) {
	t.Helper()
	root := t.TempDir()
	const anchor = "/mnt/appdata"
	if err := os.MkdirAll(filepath.Join(root, anchor), 0o750); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, anchor))
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	identity := mountIdentity{anchor: anchor, device: fmt.Sprint(stat.Dev), inode: stat.Ino}
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	service := NewService(nil, ServiceOptions{HostRoot: root})
	t.Cleanup(service.Close)
	return service, dir, identity
}

func TestAppdataCreationRefusesAnUnverifiedAnchor(t *testing.T) {
	for _, symlink := range []bool{true, false} {
		t.Run(fmt.Sprintf("symlink=%t", symlink), func(t *testing.T) {
			service, root, id := openAppdataFixture(t)
			anchor := filepath.Join(service.hostRoot, id.anchor)
			if err := os.Rename(anchor, anchor+"-held"); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := os.Symlink(anchor+"-held", anchor); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(anchor, 0o750); err != nil {
				t.Fatal(err)
			}
			if _, err := service.createAppdata(root, id, id.anchor+"/new", make(map[string]mountIdentity)); !errors.Is(err, ErrPlanChanged) {
				t.Fatalf("changed ancestor was trusted: %v", err)
			}
			for _, parent := range []string{anchor, anchor + "-held"} {
				if _, err := os.Lstat(filepath.Join(parent, "new")); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("created a directory through an unverified anchor: %v", err)
				}
			}
		})
	}
}

func TestAppdataOwnershipFailureOnlyTouchesTheCreatedLeaf(t *testing.T) {
	service, root, id := openAppdataFixture(t)
	source := id.anchor + "/new"
	called := false
	_, err := service.createAppdataWithOwnership(root, id, source, make(map[string]mountIdentity),
		func(file *os.File, uid, gid int) error {
			called = true
			fdInfo, err := file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			leafInfo, err := os.Stat(filepath.Join(service.hostRoot, source))
			if err != nil || !os.SameFile(leafInfo, fdInfo) || uid != os.Geteuid() || gid != os.Getegid() {
				t.Fatalf("ownership operation received the wrong directory or identity: %v, %d:%d", err, uid, gid)
			}
			return fs.ErrPermission
		})
	if !called || !errors.Is(err, ErrStorageState) {
		t.Fatalf("refused ownership update did not stop the install: %v", err)
	}
	if info, err := os.Stat(filepath.Join(service.hostRoot, source)); err != nil || !info.IsDir() {
		t.Fatalf("ownership refusal discarded the staged appdata: %v", err)
	}
}

func TestAppdataStagingFailuresNeverPublishAnUnverifiedDirectory(t *testing.T) {
	for _, name := range []string{"parent is not a directory", "cannot reopen staged directory", "cannot read staged identity", "exclusive rename unavailable"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			parentPath := dir
			ops := appdataStageOperations{open: unix.Openat, stat: unix.Fstat, publish: renameExclusiveAt}
			refusedFD := -1
			switch name {
			case "parent is not a directory":
				parentPath = filepath.Join(dir, "not-directory")
				if err := os.WriteFile(parentPath, []byte("file sentinel"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "cannot reopen staged directory":
				ops.open = func(_ int, _ string, flags int, _ uint32) (int, error) {
					if flags&unix.O_NOFOLLOW == 0 {
						t.Error("stage open could follow a substituted symlink")
					}
					return -1, unix.EIO
				}
			case "cannot read staged identity":
				ops.stat = func(fd int, _ *unix.Stat_t) error {
					refusedFD = fd
					return unix.EIO
				}
			case "exclusive rename unavailable":
				ops.publish = func(int, string, string) error { return unix.ENOTSUP }
			}
			parent, err := os.Open(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = parent.Close() })
			file, id, err := stageAppdataWith(parent, "final", "/mnt/appdata/final", hostMount{}, ops)
			if file != nil || id != (mountIdentity{}) || !errors.Is(err, ErrStorageState) {
				t.Fatalf("failed operation produced an approved directory: %+v, %v", id, err)
			}
			if refusedFD != -1 {
				var ignored unix.Stat_t
				if err := unix.Fstat(refusedFD, &ignored); !errors.Is(err, unix.EBADF) {
					t.Fatalf("failed identity lookup did not release the descriptor: %v", err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if name == "parent is not a directory" {
				if len(entries) != 1 || entries[0].Name() != "not-directory" {
					t.Fatalf("mkdir changed an unrelated object: %v", entries)
				}
			} else if len(entries) != 0 {
				t.Fatalf("stage failure leaked an unverified directory: %v", entries)
			}
		})
	}
}

func TestFinalStartCheckRejectsChangedNativeAndDockerEvidence(t *testing.T) {
	for _, name := range []string{"native template missing", "shared appdata appeared", "Docker no longer reports a stopped container"} {
		t.Run(name, func(t *testing.T) {
			service, engine, preview := newWorkerFixture(t)
			plan, err := service.lookupPending(preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			mounts, err := service.prepareInstallStorage(plan.effective)
			if err != nil {
				t.Fatal(err)
			}
			receipt := installFixtureReceipt("publishing_template", AttemptRunning)
			receipt.Attempt.Configuration = plan.effective.configuration
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			inspected, err := engine.ContainerInspect(t.Context(), receipt.OwnedContainerID, client.ContainerInspectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			receipt.ImageID = inspected.Container.Image
			template := publishRecoveryTemplate(t, service, &receipt)
			var want *Problem
			switch name {
			case "native template missing":
				if err := os.Remove(template); err != nil {
					t.Fatal(err)
				}
				want = ErrNativeTemplate
			case "shared appdata appeared":
				m := plan.effective.configuration.Mounts[0]
				plan.request.Paths = map[string]PathOverride{m.Target: {Source: m.Source, AllowShared: true}}
				engine.Containers = []container.Summary{{
					Names: []string{"/another-app"}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: m.Source, RW: true}},
				}}
				want = ErrPlanChanged
			case "Docker no longer reports a stopped container":
				engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
					if c != nil {
						c.State.Status = "running"
					}
					return nil
				}
				want = ErrContainerStart
			}
			if err := service.checkBeforeStart(t.Context(), engine, receipt, plan, mounts); !errors.Is(err, want) {
				t.Fatalf("changed start preconditions did not block Docker start: %v", err)
			}
		})
	}
}

func TestCleanupPersistsUnexpectedOwnedContainerBeforeRemoval(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	receipt.CleanupPhase = "creating_container"
	receipt.EngineRejected = true
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	containerID := receipt.OwnedContainerID
	receipt.OwnedContainerID = ""
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	service.createReceiptTemp = createReadOnlyReceiptTemp
	if service.reconcileInstall(t.Context(), receipt) || len(engine.RemoveRequests()) != 0 {
		t.Fatal("cleanup removed the exact container without durably recording its ID")
	}
	service.createReceiptTemp = os.CreateTemp
	engine.BeforeRemove = func() error {
		path := filepath.Join(service.hostRoot, attemptDirectory, receipt.PlanID+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var current installReceipt
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
		if current.OwnedContainerID != containerID {
			t.Fatalf("cleanup attempted removal before recording the exact ID: %q", current.OwnedContainerID)
		}
		return nil
	}
	if !service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("cleanup could not resume after durable storage recovered")
	}
	if removes := engine.RemoveRequests(); len(removes) != 1 || removes[0].ID != containerID || removes[0].Options.Force {
		t.Fatalf("unexpected recovery mutations: %+v", removes)
	}
}
