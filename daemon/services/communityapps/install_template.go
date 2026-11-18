package communityapps

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type nativeTemplate struct {
	XMLName       xml.Name       `xml:"Container"`
	Version       string         `xml:"version,attr"`
	Name          string         `xml:"Name"`
	Repository    string         `xml:"Repository"`
	Registry      string         `xml:"Registry"`
	Network       string         `xml:"Network"`
	Shell         string         `xml:"Shell"`
	Privileged    bool           `xml:"Privileged"`
	Support       string         `xml:"Support"`
	Project       string         `xml:"Project"`
	Overview      string         `xml:"Overview"`
	Category      string         `xml:"Category"`
	WebUI         string         `xml:"WebUI"`
	TemplateURL   string         `xml:"TemplateURL"`
	Icon          string         `xml:"Icon"`
	DateInstalled int64          `xml:"DateInstalled"`
	Config        []nativeConfig `xml:"Config"`
}

type nativeConfig struct {
	Name        string `xml:"Name,attr"`
	Target      string `xml:"Target,attr"`
	Default     string `xml:"Default,attr"`
	Mode        string `xml:"Mode,attr"`
	Description string `xml:"Description,attr"`
	Type        string `xml:"Type,attr"`
	Display     string `xml:"Display,attr"`
	Required    bool   `xml:"Required,attr"`
	Mask        bool   `xml:"Mask,attr"`
	Value       string `xml:",chardata"`
}

func makeNativeTemplate(settings installSettings, installed int64) nativeTemplate {
	t := settings.template
	c := settings.configuration
	native := nativeTemplate{
		Version: "2", Name: c.Name, Repository: c.Image, Network: c.Network.Name,
		Registry: t.Registry, Shell: "sh", Support: t.Support, Project: t.Project,
		Overview: t.Overview, Category: t.Category, WebUI: t.WebUI, Icon: t.Icon,
		TemplateURL: t.CatalogLink, DateInstalled: installed,
	}
	for _, f := range t.Config {
		n := nativeConfig{
			Name: safeNativeAttribute(f.Name), Target: f.Target, Required: f.Required, Mask: f.Masked,
			Description: f.Description, Display: f.Display,
		}
		if n.Name == "" {
			n.Name = f.Target
		}
		switch f.Kind {
		case ConfigPort:
			for _, p := range c.Ports {
				if n.Target == strconv.Itoa(int(p.ContainerPort)) && strings.EqualFold(f.Mode, p.Protocol) {
					n.Type, n.Mode = "Port", p.Protocol
					n.Value = net.JoinHostPort(p.BindAddress, strconv.Itoa(int(p.HostPort)))
					break
				}
			}
		case ConfigPath:
			n.Type, n.Mode = "Path", "rw"
			for _, m := range c.Mounts {
				if n.Target == m.Target {
					n.Mode, n.Value = m.Mode, m.Source
					break
				}
			}
		case ConfigEnvironment:
			n.Type = "Variable"
			for _, env := range settings.environment {
				if env.name == n.Target {
					n.Value = env.value
					break
				}
			}
		}
		native.Config = append(native.Config, n)
	}
	return native
}

func (*Service) finalTemplateName(receipt installReceipt) string {
	return "my-" + receipt.Attempt.Configuration.Name + ".xml"
}

func (*Service) stagedTemplateName(receipt installReceipt) string {
	return ".uma-ca-" + receipt.Owner + ".tmp"
}

const maxNativeTemplate = 512 << 10

func templateMarker(owner, key string) string {
	return "<!-- Unraid Management Agent CA install " + owner + " " + key + " -->\n"
}

func prepareNativeTemplate(settings installSettings, receipt *installReceipt, installed int64) ([]byte, error) {
	body, err := xml.MarshalIndent(makeNativeTemplate(settings, installed), "", "  ")
	if err != nil || len(body) >= maxNativeTemplate {
		return nil, ErrNativeTemplate
	}
	body = append(body, '\n')
	key := rand.Text()
	receipt.TemplateKeyHash = sha256.Sum256([]byte(key))
	receipt.TemplateMAC = nativeTemplateMAC(key, body)
	return append([]byte(xml.Header+templateMarker(receipt.Owner, key)), body...), nil
}

func nativeTemplateMAC(key string, body []byte) [sha256.Size]byte {
	auth := hmac.New(sha256.New, []byte(key))
	_, _ = auth.Write(body)
	var result [sha256.Size]byte
	auth.Sum(result[:0])
	return result
}

func (s *Service) publishNativeTemplate(template []byte, receipt installReceipt) error {
	dir := filepath.Join(s.hostRoot, userTemplatesDir)
	if err := ensureInstallDirectory(s.hostRoot, userTemplatesDir, 0o755); err != nil {
		return ErrNativeTemplate
	}
	if _, err := (installHost{root: s.hostRoot}).savedBindings(receipt.Attempt.Configuration.Name, ""); err != nil {
		return err
	}

	stage := filepath.Join(dir, s.stagedTemplateName(receipt))
	// #nosec G304 -- Staging name is a random, validated owner identifier in the fixed native templates directory.
	file, err := os.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return ErrNativeTemplate
	}
	_, err = file.Write(template)
	if err == nil {
		err = file.Sync()
	}
	info, _ := file.Stat()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		if current, statErr := os.Lstat(stage); statErr == nil && info != nil && os.SameFile(info, current) {
			_ = os.Remove(stage)
		}
		return ErrNativeTemplate
	}
	// Durably name the staging file before attempting exclusive publication.
	if err := syncDirectory(dir); err != nil {
		return ErrNativeTemplate
	}
	target := filepath.Join(dir, s.finalTemplateName(receipt))
	if err := renameExclusive(stage, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrNameConflict
		}
		return ErrNativeTemplate
	}
	if err := syncDirectory(dir); err != nil {
		return ErrNativeTemplate
	}
	return nil
}

func (s *Service) ownsNativeFile(receipt installReceipt, staged bool) (bool, error) {
	name := s.finalTemplateName(receipt)
	if staged {
		name = s.stagedTemplateName(receipt)
	}
	return s.nativeFileOwnership(filepath.Join(s.hostRoot, userTemplatesDir, name), receipt, staged)
}

func (*Service) nativeFileOwnership(filename string, receipt installReceipt, staged bool) (bool, error) {
	info, err := os.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.Mode().IsRegular() {
		return false, nil
	}
	if err != nil {
		return false, ErrNativeTemplate
	}
	// #nosec G304 -- Template or random staging basename is derived from a validated receipt.
	file, err := os.OpenFile(filename, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false, ErrNativeTemplate
	}
	defer func() { _ = file.Close() }()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return false, ErrNativeTemplate
	}
	reader := bufio.NewReader(io.LimitReader(file, maxNativeTemplate+257))
	// Bound the initial read for unrelated and partially written files.
	header, headerErr := reader.ReadSlice('\n')
	if headerErr != nil && !errors.Is(headerErr, io.EOF) && !errors.Is(headerErr, bufio.ErrBufferFull) {
		return false, ErrNativeTemplate
	}
	if !bytes.Equal(header, []byte(xml.Header)) {
		return false, nil
	}
	marker, markerErr := reader.ReadSlice('\n')
	if markerErr != nil && !errors.Is(markerErr, io.EOF) && !errors.Is(markerErr, bufio.ErrBufferFull) {
		return false, ErrNativeTemplate
	}
	prefix := "<!-- Unraid Management Agent CA install " + receipt.Owner + " "
	key, found := strings.CutPrefix(string(marker), prefix)
	key, valid := strings.CutSuffix(key, " -->\n")
	if !found || !valid || !opaqueID.MatchString(key) || sha256.Sum256([]byte(key)) != receipt.TemplateKeyHash {
		return false, nil
	}
	if staged {
		// An owned staging file may be truncated by interrupted writes. The
		// key fingerprint verifies its identity independently of XML content.
		return true, nil
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return false, ErrNativeTemplate
	}
	mac := nativeTemplateMAC(key, body)
	return len(body) < maxNativeTemplate && hmac.Equal(mac[:], receipt.TemplateMAC[:]), nil
}

func (s *Service) removeOwnedNativeFile(receipt installReceipt, staged bool) bool {
	dir := filepath.Join(s.hostRoot, userTemplatesDir)
	name := s.finalTemplateName(receipt)
	suffix := ".published"
	if staged {
		name = s.stagedTemplateName(receipt)
		suffix = ".staged"
	}
	target := filepath.Join(dir, name)
	quarantine := filepath.Join(dir, ".uma-ca-"+receipt.Owner+suffix+"-cleanup")

	if _, err := os.Lstat(quarantine); err == nil {
		// Cleanup may have been interrupted after the exclusive rename.
		if !s.finishNativeRemoval(quarantine, target, receipt, staged) {
			return false
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	owned, err := s.nativeFileOwnership(target, receipt, staged)
	if err != nil || !owned {
		return err == nil
	}
	if err := renameExclusive(target, quarantine); err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return s.finishNativeRemoval(quarantine, target, receipt, staged)
}

func (s *Service) finishNativeRemoval(quarantine, target string, receipt installReceipt, staged bool) bool {
	// Native edits can preserve the original inode. Verify the contents
	// again after removing the artifact from the native writer's path.
	owned, err := s.nativeFileOwnership(quarantine, receipt, staged)
	if err != nil || !owned {
		restored := renameExclusive(quarantine, target) == nil && syncDirectory(filepath.Dir(target)) == nil
		return err == nil && restored
	}
	if err := os.Remove(quarantine); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return syncDirectory(filepath.Dir(target)) == nil
}

func safeNativeAttribute(value string) string {
	if len(value) > 512 || strings.ContainsAny(value, "\"'&") || !nativeValue(value) {
		return ""
	}
	return value
}

var nativePortPlaceholder = regexp.MustCompile(`\[PORT:[0-9]{1,5}\]`)

func safeNativeURL(value string) string {
	if len(value) > 2048 || strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "\"'<>`") || !nativeValue(value) {
		return ""
	}
	// Docker Manager permits [IP] and [PORT:<port>] placeholders in WebUI.
	resolved := strings.ReplaceAll(value, "[IP]", "127.0.0.1")
	resolved = nativePortPlaceholder.ReplaceAllString(resolved, "12345")
	parsed, err := url.Parse(resolved)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return value
}
