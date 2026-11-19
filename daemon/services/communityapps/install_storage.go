package communityapps

import (
	"context"
	"errors"
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
	created := make(map[string]bool)
	after := make([]mountIdentity, len(mounts))
	for i, m := range mounts {
		if m.CreateDirectory {
			if err := s.createAppdata(root, before[i], m.Source, created); err != nil {
				return nil, err
			}
		}
		current, identity, reason := host.resolveSource(m.Source)
		if reason != "" || current.Source != m.Source || current.CreateDirectory || identity.mount != before[i].mount ||
			(!m.CreateDirectory && identity != before[i]) {
			return nil, ErrPlanChanged
		}
		after[i] = identity
	}
	return after, nil
}

func (s *Service) createAppdata(root *os.Root, identity mountIdentity, source string, created map[string]bool) error {
	// Open the approved existing ancestor separately so a symlink introduced
	// during creation cannot redirect Mkdir outside that directory.
	dir, err := root.OpenFile(strings.TrimPrefix(identity.anchor, "/"), unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ErrPlanChanged
	}
	defer func() { _ = dir.Close() }()
	anchor, err := dir.Stat()
	if err != nil || !protectedBindDirectory(anchor) || !hasMountIdentity(anchor, identity) {
		return ErrPlanChanged
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(source, identity.anchor), "/")
	absolute := identity.anchor
	for component := range strings.SplitSeq(relative, "/") {
		absolute = path.Join(absolute, component)
		newDirectory := !created[absolute]
		if newDirectory {
			if err := unix.Mkdirat(int(dir.Fd()), component, 0o755); err != nil {
				if errors.Is(err, fs.ErrExist) {
					return ErrPlanChanged
				}
				return ErrStorageState
			}
			created[absolute] = true
		}
		// A component may be renamed after Mkdir. A no-follow open
		// anchors all subsequent mkdir calls at a directory descriptor.
		fd, err := unix.Openat(int(dir.Fd()), component, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return ErrPlanChanged
		}
		child := os.NewFile(uintptr(fd), component)
		_ = dir.Close()
		dir = child
		if absolute != source {
			info, err := dir.Stat()
			if err != nil || !protectedBindDirectory(info) {
				return ErrPlanChanged
			}
		} else if newDirectory {
			// All ancestors are protected. Change only the newly created
			// leaf by descriptor to preserve native appdata ownership.
			uid, gid := appdataOwnership(s.hostRoot)
			if err := dir.Chown(uid, gid); err != nil {
				return ErrStorageState
			}
		}
	}
	return nil
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
