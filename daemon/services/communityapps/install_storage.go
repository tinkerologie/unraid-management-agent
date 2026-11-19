package communityapps

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/moby/moby/client"
	"golang.org/x/sys/unix"
)

func (s *Service) prepareInstallStorage(settings installSettings) ([]mountIdentity, error) {
	host, err := readInstallHost(s.hostRoot)
	if err != nil {
		return nil, err
	}
	mounts := settings.configuration.Mounts
	before := settings.mountIdentities
	for i, m := range mounts {
		current, identity, reason := host.resolveSource(m.Source)
		if reason != "" || identity != before[i] || current.CreateDirectory != m.CreateDirectory {
			return nil, ErrPlanChanged
		}
	}
	// #nosec G304 -- The configured host root and all anchors are validated against the host's storage mounts.
	root, err := os.OpenRoot(s.hostRoot)
	if err != nil {
		return nil, ErrStorageState
	}
	defer func() { _ = root.Close() }()
	created := make(map[string]mountIdentity)
	after := make([]mountIdentity, len(mounts))
	for i, m := range mounts {
		expected := before[i]
		if m.CreateDirectory {
			expected, err = s.createAppdata(root, expected, m.Source, created)
			if err != nil {
				return nil, err
			}
		}
		current, identity, reason := host.resolveSource(m.Source)
		if reason != "" || current.Source != m.Source || current.CreateDirectory || identity != expected {
			return nil, ErrPlanChanged
		}
		after[i] = identity
	}
	return after, nil
}

func (s *Service) createAppdata(root *os.Root, identity mountIdentity, source string, created map[string]mountIdentity) (mountIdentity, error) {
	// Open the approved existing ancestor separately so a symlink introduced
	// during creation cannot redirect Mkdir outside that directory.
	dir, err := root.OpenFile(strings.TrimPrefix(identity.anchor, "/"), unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return mountIdentity{}, ErrPlanChanged
	}
	defer func() { _ = dir.Close() }()
	anchor, err := dir.Stat()
	if err != nil || !protectedBindDirectory(anchor) || !hasMountIdentity(anchor, identity) {
		return mountIdentity{}, ErrPlanChanged
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(source, identity.anchor), "/")
	absolute := identity.anchor
	for component := range strings.SplitSeq(relative, "/") {
		absolute = path.Join(absolute, component)
		expected, reused := created[absolute]
		newDirectory := !reused
		var child *os.File
		if newDirectory {
			child, expected, err = stageAppdataComponent(dir, component, absolute, identity.mount)
			if err != nil {
				return mountIdentity{}, err
			}
			created[absolute] = expected
		} else {
			fd, err := unix.Openat(int(dir.Fd()), component, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return mountIdentity{}, ErrPlanChanged
			}
			child = os.NewFile(uintptr(fd), component)
		}
		_ = dir.Close()
		dir = child
		if absolute != source {
			info, err := dir.Stat()
			if err != nil || !protectedBindDirectory(info) || !hasMountIdentity(info, expected) {
				return mountIdentity{}, ErrPlanChanged
			}
		} else if newDirectory {
			// All ancestors are protected. Change only the newly created
			// leaf by descriptor to preserve native appdata ownership.
			uid, gid := appdataOwnership(s.hostRoot)
			if err := dir.Chown(uid, gid); err != nil {
				return mountIdentity{}, ErrStorageState
			}
		}
	}
	return created[source], nil
}

func stageAppdataComponent(parent *os.File, name, source string, storage hostMount) (*os.File, mountIdentity, error) {
	dir := int(parent.Fd())
	temporary := ".uma-ca-appdata-" + rand.Text()
	if err := unix.Mkdirat(dir, temporary, 0o755); err != nil {
		return nil, mountIdentity{}, ErrStorageState
	}
	defer func() { _ = unix.Unlinkat(dir, temporary, unix.AT_REMOVEDIR) }()
	fd, err := unix.Openat(dir, temporary, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, mountIdentity{}, ErrStorageState
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, mountIdentity{}, ErrStorageState
	}
	// The identity is observed before the directory is published. An
	// independent directory at the approved path cannot become our leaf.
	id := mountIdentity{anchor: source, mount: storage, device: fmt.Sprint(stat.Dev), inode: stat.Ino}
	if err := renameExclusiveAt(dir, temporary, name); err != nil {
		_ = unix.Close(fd)
		if errors.Is(err, fs.ErrExist) {
			return nil, mountIdentity{}, ErrPlanChanged
		}
		return nil, mountIdentity{}, ErrStorageState
	}
	return os.NewFile(uintptr(fd), name), id, nil
}

func appdataOwnership(root string) (int, int) {
	if root == "/" {
		return 99, 100 // Unraid's nobody:users
	}
	return os.Geteuid(), os.Getegid()
}

func (s *Service) checkBeforeStart(ctx context.Context, docker InstallDocker, receipt installReceipt, plan *pendingPlan, mounts []mountIdentity) error {
	config := plan.effective.configuration
	owned, err := s.ownsPublishedTemplate(receipt)
	if err != nil || !owned {
		return ErrNativeTemplate
	}

	bridge, err := inspectBridge(ctx, docker, config.Network.Name, config.Ports)
	if err != nil {
		return err
	}
	if bridge != config.Network {
		return ErrPlanChanged
	}
	host, err := readInstallHost(s.hostRoot)
	if err != nil {
		return err
	}
	if host.pidsLimit != config.PIDsLimit {
		return ErrPlanChanged
	}
	for i, m := range config.Mounts {
		current, identity, reason := host.resolveSource(m.Source)
		if reason != "" || current.Source != m.Source || current.CreateDirectory || identity != mounts[i] {
			return ErrPlanChanged
		}
	}
	containers, err := docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return ErrDockerState
	}
	for i, c := range containers.Items {
		if c.ID == receipt.OwnedContainerID {
			containers.Items = append(containers.Items[:i:i], containers.Items[i+1:]...)
			break
		}
	}
	config.Mounts = slices.Clone(config.Mounts)
	for i := range config.Mounts {
		config.Mounts[i].Shared = false
	}
	if err := host.checkConflictsExcept(&config, plan.request, containers.Items, filepath.Join(s.hostRoot, userTemplatesDir, s.finalTemplateName(receipt))); err != nil {
		return err
	}
	if !slices.Equal(config.Mounts, plan.effective.configuration.Mounts) {
		return ErrPlanChanged
	}
	if err := s.checkApprovedPorts(ctx, config); err != nil {
		return err
	}
	c, owned, err := ownedContainer(ctx, docker, receipt)
	if err != nil || !owned || c.Image != receipt.ImageID || c.State == nil || c.State.Status != "created" || hasStarted(c) {
		return ErrContainerStart
	}
	return nil
}
