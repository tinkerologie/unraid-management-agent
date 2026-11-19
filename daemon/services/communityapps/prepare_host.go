package communityapps

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"golang.org/x/sys/unix"
	"gopkg.in/ini.v1"
)

const (
	userTemplatesDir = "boot/config/plugins/dockerMan/templates-user"
	defaultAppdata   = "/mnt/user/appdata"
)

type hostMount struct {
	point, id, fsType, source, root string
	storage                         bool
}

type mountIdentity struct {
	anchor string
	mount  hostMount
	device string
	inode  uint64
}

type installHost struct {
	root          string
	appdata       string
	unraidPath    string
	hostname      string
	timezone      string
	pidsLimit     int64
	mounts        []hostMount
	blockedLookup map[string]bool
}

func readInstallHost(root string) (installHost, error) {
	host := installHost{root: root, appdata: defaultAppdata, pidsLimit: 2048}
	settings, err := readHostINI(root, "boot/config/docker.cfg")
	if err != nil {
		return installHost{}, err
	}
	if appdata := settings.Key("DOCKER_APP_CONFIG_PATH").String(); appdata != "" {
		host.appdata = strings.TrimRight(appdata, "/")
	}
	host.unraidPath = strings.TrimRight(settings.Key("DOCKER_APP_UNRAID_PATH").String(), "/")
	if value := settings.Key("DOCKER_PID_LIMIT").String(); value != "" {
		host.pidsLimit, err = strconv.ParseInt(value, 10, 64)
		if err != nil || host.pidsLimit == 0 || host.pidsLimit < -1 {
			return installHost{}, ErrStorageState
		}
	}
	emhttp, err := readHostINI(root, "var/local/emhttp/var.ini")
	if err != nil {
		return installHost{}, err
	}
	host.timezone, host.hostname = emhttp.Key("timeZone").String(), emhttp.Key("NAME").String()
	if !nativeValue(host.timezone) || !nativeValue(host.hostname) || strings.ContainsAny(host.hostname+host.timezone, "\"`$\\") {
		return installHost{}, ErrStorageState
	}
	host.mounts, err = readMountInfo(root)
	host.blockedLookup = make(map[string]bool)
	for _, mount := range host.mounts {
		if !localLookupFilesystem(mount.fsType) {
			host.blockedLookup[mount.point] = true
		}
	}
	return host, err
}

func readHostINI(root, filename string) (*ini.Section, error) {
	var data []byte
	err := readInstallFile(filepath.Join(root, filename), maxSettingsBytes, func(reader io.Reader) error {
		var err error
		data, err = io.ReadAll(reader)
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, ErrStorageState
	}
	cfg, err := ini.Load(data)
	if err != nil {
		return nil, ErrStorageState
	}
	return cfg.Section(""), nil
}

func readInstallFile(filename string, limit int64, read func(io.Reader) error) error {
	// #nosec G304 -- Paths are fixed Unraid state files or entries of the fixed native templates directory.
	file, err := os.OpenFile(filename, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || info.Mode().Perm()&0o022 != 0 {
		return ErrStorageState
	}
	reader := &io.LimitedReader{R: file, N: limit + 1}
	if err = read(reader); err != nil {
		return err
	}
	if reader.N <= 0 {
		return ErrStorageState
	}
	return nil
}

var unescapeMountInfo = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, "\\")

func readMountInfo(root string) ([]hostMount, error) {
	var mounts []hostMount
	err := readInstallFile(filepath.Join(root, "proc/self/mountinfo"), 2<<20, func(reader io.Reader) error {
		lines := bufio.NewScanner(reader)
		for lines.Scan() {
			before, after, ok := strings.Cut(lines.Text(), " - ")
			left, right := strings.Fields(before), strings.Fields(after)
			if !ok || len(left) < 6 || len(right) < 3 {
				return ErrStorageState
			}
			point := unescapeMountInfo.Replace(left[4])
			m := hostMount{
				point: point, id: left[0] + ":" + left[2], fsType: right[0],
				source: unescapeMountInfo.Replace(right[1]), root: unescapeMountInfo.Replace(left[3]),
			}
			m.storage = isStorageMount(m)
			mounts = append(mounts, m)
		}
		return lines.Err()
	})
	if err != nil {
		return nil, ErrStorageState
	}
	for i, m := range mounts {
		if m.storage || m.fsType != "zfs" || m.root != "/" {
			continue
		}
		for _, pool := range mounts {
			if !pool.storage || pool.fsType != m.fsType || pool.point == m.point || !beneath(pool.point, m.point) {
				continue
			}
			// Only a genuine child dataset whose name matches its path
			// below the mounted pool may extend the storage root.
			if m.source == pool.source+strings.TrimPrefix(m.point, pool.point) {
				mounts[i].storage = true
				break
			}
		}
	}
	return mounts, nil
}

func isStorageMount(m hostMount) bool {
	if m.point == "/mnt" || !beneath("/mnt", m.point) || m.root != "/" {
		// Whole datasets, including child ZFS datasets, have / as their root.
		// Reject arbitrary bind mounts imported into an allowed-looking location.
		return false
	}
	parts := strings.Split(strings.TrimPrefix(m.point, "/mnt/"), "/")
	switch parts[0] {
	case "user", "user0":
		return len(parts) == 1 && m.fsType == "fuse.shfs" && m.source == "shfs"
	case "disks", "remotes", "addons":
		if len(parts) != 2 {
			return false
		}
	default:
		if len(parts) != 1 {
			return false
		}
	}
	switch m.fsType {
	case "xfs", "btrfs", "zfs", "ext4", "ext3", "ext2", "reiserfs", "ntfs3", "exfat", "vfat", "cifs", "nfs", "nfs4", "fuseblk":
		return true
	default:
		return false
	}
}

func beneath(parent, candidate string) bool {
	return candidate == parent || strings.HasPrefix(candidate, strings.TrimSuffix(parent, "/")+"/")
}

func validBindPath(value string) bool {
	return len(value) <= 4096 && strings.HasPrefix(value, "/") && value != "/" &&
		(value == path.Clean(value) || value == path.Clean(value)+"/") &&
		!strings.ContainsAny(value, ":\\") && nativeValue(value)
}

// Native Docker Manager decodes XML, applies xmlSecurity, and collapses
// repeated whitespace after composing its shell command.
func nativeValue(value string) bool {
	if !utf8.ValidString(value) || strings.Contains(value, "  ") ||
		strings.ContainsAny(value, "<\uFFFE\uFFFF") || html.UnescapeString(value) != value {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func (host installHost) findMount(filename string) (hostMount, bool) {
	mount, found := host.closestMount(filename)
	return mount, found && mount.storage
}

func (host installHost) closestMount(filename string) (hostMount, bool) {
	var closest hostMount
	ambiguous := false
	for _, m := range host.mounts {
		if !beneath(m.point, filename) || len(m.point) < len(closest.point) {
			continue
		}
		if len(m.point) > len(closest.point) {
			closest, ambiguous = m, false
		} else {
			ambiguous = true
		}
	}
	return closest, closest.point != "" && !ambiguous
}

func (host installHost) resolveSource(source string) (Mount, mountIdentity, string) {
	if isDockerSocket(source) {
		return Mount{}, mountIdentity{}, "docker_socket_mount"
	}
	if !validBindPath(source) {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	source = path.Clean(source)
	originalMount, allowed := host.findMount(source)
	if !allowed {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	if !localBindFilesystem(originalMount.fsType) {
		return Mount{}, mountIdentity{}, "unprotected_mount_source"
	}
	resolved, anchor, info, err := host.existingAncestor(source)
	if err != nil || !validBindPath(resolved) {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	if isDockerSocket(resolved) {
		return Mount{}, mountIdentity{}, "docker_socket_mount"
	}
	m, allowed := host.findMount(resolved)
	if !allowed || !beneath(m.point, anchor) {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	// Docker stores and reopens path strings during startup and restart.
	// Only accept local filesystems with meaningful Unix ownership and a
	// path whose entries unprivileged processes cannot replace. Pooled
	// shfs and network paths can change through another backing location.
	parent := path.Dir(resolved)
	if resolved != anchor {
		parent = anchor
	}
	if !localBindFilesystem(m.fsType) || !protectedBindParents(host.root, parent) {
		return Mount{}, mountIdentity{}, "unprotected_mount_source"
	}
	for _, other := range host.mounts {
		// Bind mounts carry nested mounts. Reject a source that would include
		// another mount the operator did not independently review.
		if other.point != resolved && beneath(resolved, other.point) {
			return Mount{}, mountIdentity{}, "nested_mount"
		}
	}
	return Mount{Source: resolved, CreateDirectory: resolved != anchor}, mountIdentity{
		anchor: anchor, mount: m, device: fmt.Sprint(info.Dev), inode: info.Ino,
	}, ""
}

func localBindFilesystem(fsType string) bool {
	switch fsType {
	case "xfs", "btrfs", "zfs", "ext4", "ext3", "ext2", "reiserfs":
		return true
	default:
		return false
	}
}

func localLookupFilesystem(fsType string) bool {
	switch fsType {
	case "rootfs", "overlay", "ramfs", "tmpfs", "proc", "sysfs", "devtmpfs", "vfat", "ntfs3", "exfat", "squashfs":
		return true
	default:
		return localBindFilesystem(fsType)
	}
}

// Check from the trusted root down, so every verified directory entry is
// protected by its own parent. The source itself can remain app-writable.
func protectedBindParents(root, parent string) bool {
	current := root
	for component := range strings.SplitSeq(strings.TrimPrefix(parent, "/"), "/") {
		info, err := os.Lstat(current)
		if err != nil || !protectedBindDirectory(info) {
			return false
		}
		current = filepath.Join(current, component)
	}
	info, err := os.Lstat(current)
	return err == nil && protectedBindDirectory(info)
}

func protectedBindDirectory(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.IsDir() && int(stat.Uid) == os.Geteuid() && info.Mode().Perm()&0o022 == 0
}

type bindLookupOps struct {
	statRoot func(int, *unix.Stat_t) error
	statAt   func(int, string, *unix.Stat_t, int) error
	openAt   func(int, string, int, uint32) (int, error)
}

func (host installHost) existingAncestor(source string) (string, string, unix.Stat_t, error) {
	return host.lookupAncestor(source, bindLookupOps{statRoot: unix.Fstat, statAt: unix.Fstatat, openAt: unix.Openat})
}

func (host installHost) lookupAncestor(source string, ops bindLookupOps) (string, string, unix.Stat_t, error) {
	var stat unix.Stat_t
	if !path.IsAbs(source) || host.blockedLookup["/"] {
		return "", "", stat, ErrStorageState
	}
	// #nosec G304 -- The service uses the configured host root.
	root, err := os.Open(host.root)
	if err != nil {
		return "", "", stat, err
	}
	defer func() { _ = root.Close() }()
	rootFD := int(root.Fd())
	if err := ops.statRoot(rootFD, &stat); err != nil {
		return "", "", stat, err
	}
	dir := rootFD
	defer func() {
		if dir != rootFD {
			_ = unix.Close(dir)
		}
	}()
	anchor, info := "/", stat
	remaining := strings.TrimPrefix(source, "/")
	required := 0
	for links := 0; remaining != ""; {
		component, tail, _ := strings.Cut(remaining, "/")
		next := path.Join(anchor, component)
		// A lookup below an NFS, FUSE or automount point can block even
		// after a request deadline. Consult mountinfo before each lookup,
		// including after following a symlink out of the original mount.
		if host.blockedLookup[next] {
			return "", "", stat, ErrStorageState
		}
		var entry unix.Stat_t
		err := ops.statAt(dir, component, &entry, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(err, fs.ErrNotExist) {
			if required > 0 {
				return "", "", stat, ErrStorageState
			}
			return path.Join(anchor, remaining), anchor, info, nil
		}
		if err != nil {
			return "", "", stat, err
		}
		kind := entry.Mode & unix.S_IFMT
		required = max(0, required-1)
		if kind == unix.S_IFLNK {
			links++
			var buffer [4096]byte
			size, err := unix.Readlinkat(dir, component, buffer[:])
			if err != nil || links > 40 || size == len(buffer) {
				return "", "", stat, ErrStorageState
			}
			target := string(buffer[:size])
			if !path.IsAbs(target) {
				target = path.Join(anchor, target)
			}
			target = path.Clean(target)
			// A link must resolve to an existing target. Only the
			// trailing path beneath that target may require creation.
			if target != "/" {
				required += 1 + strings.Count(strings.TrimPrefix(target, "/"), "/")
			}
			remaining = strings.TrimPrefix(path.Join(target, tail), "/")
			if dir != rootFD {
				_ = unix.Close(dir)
				dir = rootFD
			}
			anchor, info = "/", stat
			continue
		}
		if tail == "" && (kind == unix.S_IFDIR || kind == unix.S_IFREG) {
			return next, next, entry, nil
		}
		if kind != unix.S_IFDIR {
			return "", "", stat, ErrStorageState
		}
		child, err := ops.openAt(dir, component, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return "", "", stat, err
		}
		if dir != rootFD {
			_ = unix.Close(dir)
		}
		dir = child
		anchor, info, remaining = next, entry, tail
	}
	return anchor, anchor, info, nil
}

func (host installHost) defaultSource(target, name, value string) string {
	if strings.HasPrefix(value, defaultAppdata+"/") {
		value = host.appdata + strings.TrimPrefix(value, defaultAppdata)
	}
	if strings.EqualFold(target, "/config") && (value == "" || beneath(host.appdata, value)) {
		return path.Join(host.appdata, name)
	}
	if strings.EqualFold(target, "/unraid") && host.unraidPath != "" {
		return host.unraidPath
	}
	return value
}

type savedTemplate struct {
	XMLName xml.Name `xml:"Container"`
	Name    string   `xml:"Name"`
	Config  []struct {
		Type    string `xml:"Type,attr"`
		Mode    string `xml:"Mode,attr"`
		Default string `xml:"Default,attr"`
		Source  string `xml:",chardata"`
	} `xml:"Config"`
	Legacy []struct {
		Source string `xml:"HostDir"`
		Mode   string `xml:"Mode"`
	} `xml:"Data>Volume"`
}

func (host installHost) savedBindings(name, ignoredTemplate string) ([]string, error) {
	dir := filepath.Join(host.root, userTemplatesDir)
	bindings := []string{}
	count := 0
	err := filepath.WalkDir(dir, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if filename == dir && errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return ErrStorageState
		}
		if filename == ignoredTemplate || entry.IsDir() {
			return nil
		}
		if filename == dir {
			return ErrStorageState
		}
		if strings.EqualFold(entry.Name(), "my-"+name+".xml") && filepath.Dir(filename) == dir {
			return ErrNameConflict
		}
		if filepath.Ext(entry.Name()) != ".xml" || strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		count++
		if count > 1024 {
			return ErrStorageState
		}
		var saved savedTemplate
		err := readInstallFile(filename, 1<<20, func(reader io.Reader) error {
			return xml.NewDecoder(reader).Decode(&saved)
		})
		if err != nil {
			return ErrStorageState
		}
		if strings.EqualFold(saved.Name, name) {
			return ErrNameConflict
		}
		for _, field := range saved.Config {
			source := field.Source
			if source == "" {
				source = field.Default
			}
			if strings.EqualFold(field.Type, "Path") && source != "" && !readOnlyMode(field.Mode) {
				bindings = append(bindings, source)
			}
		}
		for _, field := range saved.Legacy {
			if field.Source != "" && !readOnlyMode(field.Mode) {
				bindings = append(bindings, field.Source)
			}
		}
		return nil
	})
	return bindings, err
}

func readOnlyMode(mode string) bool {
	return slices.Contains(strings.Split(mode, ","), "ro")
}

func (host installHost) checkConflicts(config *Configuration, request PrepareRequest, containers []container.Summary) error {
	return host.checkConflictsExcept(config, request, containers, "")
}

func (host installHost) checkConflictsExcept(config *Configuration, request PrepareRequest, containers []container.Summary, ignoredTemplate string) error {
	if err := checkDockerName(containers, config.Name); err != nil {
		return err
	}
	existing, err := host.savedBindings(config.Name, ignoredTemplate)
	if err != nil {
		return err
	}
	for _, c := range containers {
		for _, p := range c.Ports {
			for _, wanted := range config.Ports {
				if portOverlaps(wanted, p.IP, p.PublicPort, p.Type) {
					return ErrPortConflict
				}
			}
		}
		for _, m := range c.Mounts {
			if m.RW && m.Type == mount.TypeBind && m.Source != "" {
				existing = append(existing, m.Source)
			}
		}
	}
	for i, source := range existing {
		existing[i] = host.canonicalBindSource(source)
	}
	for i, wanted := range config.Mounts {
		for _, other := range config.Mounts {
			if (!readOnlyMode(other.Mode) || other.CreateDirectory) && other.Source != wanted.Source && beneath(other.Source, wanted.Source) {
				return ErrExposedMountParent
			}
		}
		shared := false
		for _, source := range existing {
			overlap, parent := host.bindOverlap(wanted.Source, source)
			if parent {
				return ErrExposedMountParent
			}
			shared = shared || overlap
		}
		if shared && !readOnlyMode(wanted.Mode) {
			if !request.Paths[wanted.Target].AllowShared {
				return ErrSharedAppdata
			}
			config.Mounts[i].Shared = true
		}
	}
	return nil
}

func (host installHost) canonicalBindSource(source string) string {
	existing := path.Clean(source)
	if resolved, _, _, err := host.existingAncestor(existing); err == nil {
		existing = resolved
	}
	return existing
}

// A writable ancestor exposes directory entries to the other workload.
// Keeping the same source or mounting its children shares contents only.
func (host installHost) bindOverlap(wanted, existing string) (overlap, parent bool) {
	if beneath(existing, wanted) {
		return true, existing != wanted
	}
	if beneath(wanted, existing) {
		return true, false
	}
	// A second mount can expose the same directory tree outside /mnt.
	// Compare paths relative to each mount's filesystem root.
	left, leftFound := host.closestMount(wanted)
	right, rightFound := host.closestMount(existing)
	if leftFound && rightFound && left.point != right.point {
		_, leftDevice, leftOK := strings.Cut(left.id, ":")
		_, rightDevice, rightOK := strings.Cut(right.id, ":")
		if leftOK && rightOK && leftDevice == rightDevice {
			leftPath := path.Join(left.root, strings.TrimPrefix(wanted, left.point))
			rightPath := path.Join(right.root, strings.TrimPrefix(existing, right.point))
			if beneath(rightPath, leftPath) {
				return true, rightPath != leftPath
			}
			if beneath(leftPath, rightPath) {
				return true, false
			}
		}
	}
	// User shares aggregate matching relative paths on pools/array disks
	// through shfs, so equivalent aliases need not have matching inodes.
	if !leftFound || !rightFound || !left.storage || !right.storage || left.point == right.point ||
		(left.fsType != "fuse.shfs" && right.fsType != "fuse.shfs") {
		return false, false
	}
	leftPath, leftOK := shareRelativePath(wanted)
	rightPath, rightOK := shareRelativePath(existing)
	if !leftOK || !rightOK {
		return false, false
	}
	if beneath(rightPath, leftPath) {
		return true, rightPath != leftPath
	}
	return beneath(leftPath, rightPath), false
}

func shareRelativePath(filename string) (string, bool) {
	root, relative, _ := strings.Cut(strings.TrimPrefix(filename, "/mnt/"), "/")
	switch root {
	case "disks", "remotes", "addons":
		return "", false
	default:
		return "/" + relative, true
	}
}

func (s *Service) checkApprovedPorts(ctx context.Context, config Configuration) error {
	for _, wanted := range config.Ports {
		if err := s.portAvailable(ctx, wanted); err != nil {
			return ErrPortConflict
		}
	}
	return nil
}

func hasMountIdentity(info fs.FileInfo, identity mountIdentity) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && fmt.Sprint(stat.Dev) == identity.device && stat.Ino == identity.inode
}
