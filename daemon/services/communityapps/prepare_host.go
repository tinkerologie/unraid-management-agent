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
	root       string
	appdata    string
	unraidPath string
	hostname   string
	timezone   string
	pidsLimit  int64
	mounts     []hostMount
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
			if !beneath("/mnt", point) {
				continue
			}
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
	if m.point == "/mnt" || m.root != "/" {
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
	return closest, closest.storage && !ambiguous
}

func (host installHost) resolveSource(source string) (Mount, mountIdentity, string) {
	if isDockerSocket(source) {
		return Mount{}, mountIdentity{}, "docker_socket_mount"
	}
	if !validBindPath(source) {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	source = path.Clean(source)
	if _, allowed := host.findMount(source); !allowed {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	resolved, anchor, info, err := host.existingAncestor(source)
	if err != nil || !validBindPath(resolved) {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	if isDockerSocket(resolved) {
		return Mount{}, mountIdentity{}, "docker_socket_mount"
	}
	m, allowed := host.findMount(resolved)
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !allowed || !ok || !beneath(m.point, anchor) {
		return Mount{}, mountIdentity{}, "unsafe_mount_source"
	}
	for _, other := range host.mounts {
		// Bind mounts carry nested mounts. Reject a source that would include
		// another mount the operator did not independently review.
		if other.point != resolved && beneath(resolved, other.point) {
			return Mount{}, mountIdentity{}, "nested_mount"
		}
	}
	return Mount{Source: resolved, CreateDirectory: resolved != anchor}, mountIdentity{
		anchor: anchor, mount: m, device: fmt.Sprint(stat.Dev), inode: stat.Ino,
	}, ""
}

func (host installHost) existingAncestor(source string) (string, string, fs.FileInfo, error) {
	current := filepath.Join(host.root, source)
	var missing []string
	for {
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && current != host.root {
			missing = append(missing, filepath.Base(current))
			current = filepath.Dir(current)
			continue
		}
		if err != nil {
			return "", "", nil, err
		}
		canonical, err := filepath.EvalSymlinks(current)
		if err != nil {
			return "", "", nil, err
		}
		relative, err := filepath.Rel(host.root, canonical)
		if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
			return "", "", nil, ErrStorageState
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			info, err = os.Stat(canonical)
			if err != nil {
				return "", "", nil, err
			}
		}
		if !info.IsDir() && (len(missing) != 0 || !info.Mode().IsRegular()) {
			return "", "", nil, ErrStorageState
		}
		anchor := path.Join("/", filepath.ToSlash(relative))
		slices.Reverse(missing)
		resolved := path.Join(append([]string{anchor}, missing...)...)
		return resolved, anchor, info, nil
	}
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

func (host installHost) savedBindings(name string) ([]string, error) {
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
		if entry.IsDir() {
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
	if err := checkDockerName(containers, config.Name); err != nil {
		return err
	}
	existing, err := host.savedBindings(config.Name)
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
	for i, wanted := range config.Mounts {
		if readOnlyMode(wanted.Mode) {
			continue
		}
		for _, source := range existing {
			if !host.bindsOverlap(wanted.Source, source) {
				continue
			}
			if !request.Paths[wanted.Target].AllowShared {
				return ErrSharedAppdata
			}
			config.Mounts[i].Shared = true
			break
		}
	}
	return nil
}

func (host installHost) bindsOverlap(wanted, source string) bool {
	existing := path.Clean(source)
	if resolved, _, _, err := host.existingAncestor(existing); err == nil {
		existing = resolved
	}
	if beneath(existing, wanted) || beneath(wanted, existing) {
		return true
	}
	// User shares aggregate matching relative paths on pools/array disks
	// through shfs, so equivalent aliases need not have matching inodes.
	left, leftOK := host.findMount(wanted)
	right, rightOK := host.findMount(existing)
	if !leftOK || !rightOK || left.point == right.point ||
		(left.fsType != "fuse.shfs" && right.fsType != "fuse.shfs") {
		return false
	}
	leftPath, leftOK := shareRelativePath(wanted)
	rightPath, rightOK := shareRelativePath(existing)
	return leftOK && rightOK && (beneath(leftPath, rightPath) || beneath(rightPath, leftPath))
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
