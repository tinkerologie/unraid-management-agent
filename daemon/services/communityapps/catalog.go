// Package communityapps reads the host's Community Applications catalog.
package communityapps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"gopkg.in/ini.v1"

	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

const (
	pluginDirectory = "boot/config/plugins/community.applications"
	pluginManifest  = pluginDirectory + ".plg"
	catalogPath     = "tmp/community.applications/tempFiles/templates_new.json"
	osVersionPath   = "etc/unraid-version"
	supportedBuild  = "2026.07.21"
	settingsPath    = pluginDirectory + "/community.applications.cfg"
	onboardingPath  = pluginDirectory + "/accepted"

	maxCatalogBytes  = 64 << 20
	maxSettingsBytes = 256 << 10
	maxResults       = 100
)

// Problem is the stable and safe error response shared by REST and MCP.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (p *Problem) Error() string { return p.Message }

// ProblemForError converts unexpected errors to a safe transport response.
func ProblemForError(err error) *Problem {
	var problem *Problem
	if errors.As(err, &problem) {
		return problem
	}
	return &Problem{Code: "catalog_request_failed", Message: "Community Applications request failed."}
}

var (
	ErrCatalogUnavailable = &Problem{
		Code: "catalog_unavailable", Message: "Community Applications catalog unavailable. Open the Unraid Apps tab to load it, then retry.",
	}
	ErrCatalogUnsupported = &Problem{
		Code: "catalog_unsupported", Message: "Community Applications catalog layout or build unsupported. This agent supports CA " + supportedBuild + " on Unraid 7.2 and later.",
	}
	ErrListingUnavailable = &Problem{
		Code: "listing_unavailable", Message: "The listing is absent from the current filtered catalog. Search Community Applications again.",
	}
	ErrInvalidSearch = &Problem{
		Code: "invalid_search", Message: "Search terms must be at most 256 bytes and contain no null bytes.",
	}
	ErrInvalidReference = &Problem{
		Code: "invalid_reference", Message: "Use a listing reference returned by Community Applications search.",
	}
)

var (
	buildEntity   = regexp.MustCompile(`<!ENTITY\s+version\s+["']([0-9]{4}\.[0-9]{2}\.[0-9]{2})["']\s*>`)
	unraidRelease = regexp.MustCompile(`^([0-9]+)\.([0-9]+)(?:\.|$)`)
)

// Catalog reads the installed plugin's host state without refreshing or editing it.
type Catalog struct {
	host     fs.FS
	mu       sync.Mutex
	fileInfo fs.FileInfo
	snapshot []selection
}

// NewCatalog reads Community Applications state from a filesystem rooted at the Unraid host root.
func NewCatalog(host fs.FS) *Catalog {
	return &Catalog{host: host}
}

// NewHostCatalog reads the local Unraid host's Community Applications catalog.
func NewHostCatalog() *Catalog {
	return NewCatalog(hostFiles{root: "/", owner: 0})
}

// The host uses only fixed paths. Reject links at the file itself, special
// files, and files modifiable by non-root users before reading their bytes.
type hostFiles struct {
	root  string
	owner int
}

func (host hostFiles) Open(name string) (fs.File, error) {
	switch name {
	case pluginManifest, catalogPath, osVersionPath, settingsPath, onboardingPath:
	default:
		return nil, fs.ErrInvalid
	}
	// #nosec G304 -- Only the fixed catalog files above can be opened.
	file, err := os.OpenFile(filepath.Join(host.root, name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if ok && int(owner.Uid) == host.owner && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 {
			return file, nil
		}
	}
	_ = file.Close()
	return nil, fs.ErrPermission
}

// Listing identifies a CA catalog template and variant, independent of any
// installation name or Docker container ID.
type Listing struct {
	Reference string `json:"reference"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	Publisher string `json:"publisher,omitempty"`
	Variant   string `json:"variant,omitempty"`
}

// SearchResult contains the first page of matching Docker listings. Total
// reports the number of matches before applying the response size bound.
type SearchResult struct {
	CAVersion string    `json:"ca_version"`
	Listings  []Listing `json:"listings"`
	Total     int       `json:"total"`
}

// Inspection contains the published template's editable fields and what can
// already be established about install eligibility without preparing a plan.
type Inspection struct {
	CAVersion          string          `json:"ca_version"`
	Listing            Listing         `json:"listing"`
	Network            string          `json:"network"`
	Fields             []TemplateField `json:"fields"`
	OnboardingAccepted bool            `json:"onboarding_accepted"`
	EligibilityIssues  []string        `json:"eligibility_issues"`
}

// ResolvedTemplate allows the install service to use the exact published
// template and check current host policy. Public callers use Inspection.
type ResolvedTemplate struct {
	Listing            Listing
	Template           Template
	OnboardingAccepted bool
	PolicyIssues       []string
}

type catalogEntry struct {
	id, name, image, publisher, repository, path, templateURL, variant string
	searchTerms                                                        [5]string
	branches                                                           []string
	displayable                                                        bool
	compatible                                                         bool
	blacklisted                                                        bool
	deprecated                                                         bool
	featured                                                           bool
	hasRequirements                                                    bool
	private                                                            bool
	uninstallOnly                                                      bool
	hidden                                                             bool
	template                                                           Template
}

type selection struct {
	entry     *catalogEntry
	parent    *catalogEntry
	reference string
}

func (s selection) listing() Listing {
	publisher := s.entry.publisher
	if publisher == "" {
		publisher = s.parent.publisher
	}
	return Listing{
		Reference: s.reference, Name: s.entry.name, Image: s.entry.image,
		Publisher: publisher, Variant: s.entry.variant,
	}
}

type catalogPolicy struct {
	hideDeprecated   bool
	hideIncompatible bool
}

func (p catalogPolicy) shows(entry *catalogEntry) bool {
	return !entry.hidden && !entry.blacklisted && (!entry.deprecated || !p.hideDeprecated) &&
		(!p.hideIncompatible || entry.compatible || entry.featured)
}

func (s selection) visible(policy catalogPolicy) bool {
	return policy.shows(s.parent) && policy.shows(s.entry)
}

// Search finds Docker listings currently displayed by the host's CA policy.
func (c *Catalog) Search(ctx context.Context, query string) (SearchResult, error) {
	if lib.ValidateMaxLength(query, "search", 256) != nil || strings.ContainsRune(query, 0) {
		return SearchResult{}, ErrInvalidSearch
	}
	entries, policy, err := c.load(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	result := SearchResult{CAVersion: supportedBuild, Listings: make([]Listing, 0)}
	for _, entry := range entries {
		if !entry.visible(policy) || !entry.matches(query) {
			continue
		}
		result.Total++
		if len(result.Listings) < maxResults {
			result.Listings = append(result.Listings, entry.listing())
		}
	}
	return result, nil
}

func (s selection) matches(query string) bool {
	return query == "" || strings.Contains(s.parent.searchTerms[0], query) ||
		slices.ContainsFunc(s.entry.searchTerms[:], func(term string) bool {
			return strings.Contains(term, query)
		})
}

// Inspect returns the selected listing and its redacted published settings.
func (c *Catalog) Inspect(ctx context.Context, reference string) (Inspection, error) {
	resolved, err := c.Resolve(ctx, reference)
	if err != nil {
		return Inspection{}, err
	}
	issues := append(make([]string, 0), resolved.PolicyIssues...)
	issues = append(issues, resolved.Template.RuntimeIssues...)
	if !resolved.OnboardingAccepted {
		issues = append(issues, "accept_warning_in_unraid_apps")
	}
	fields := make([]TemplateField, 0, len(resolved.Template.Config))
	for _, config := range resolved.Template.Config {
		fields = append(fields, config.publicField())
		if config.mountsDockerSocket() && !slices.Contains(issues, "docker_socket_mount") {
			issues = append(issues, "docker_socket_mount")
		}
	}
	return Inspection{
		CAVersion: supportedBuild, Listing: resolved.Listing, Network: resolved.Template.Network,
		Fields: fields, OnboardingAccepted: resolved.OnboardingAccepted, EligibilityIssues: issues,
	}, nil
}

// Resolve selects the same variant from the current catalog and policy, even
// when the position of a row in CA's cache changed since search.
func (c *Catalog) Resolve(ctx context.Context, reference string) (ResolvedTemplate, error) {
	if len(reference) != sha256.Size*2 || strings.ToLower(reference) != reference {
		return ResolvedTemplate{}, ErrInvalidReference
	}
	if _, err := hex.DecodeString(reference); err != nil {
		return ResolvedTemplate{}, ErrInvalidReference
	}
	entries, policy, err := c.load(ctx)
	if err != nil {
		return ResolvedTemplate{}, err
	}
	for _, candidate := range entries {
		if candidate.reference != reference || !candidate.visible(policy) {
			continue
		}
		_, err := fs.Stat(c.host, onboardingPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ResolvedTemplate{}, ErrCatalogUnavailable
		}
		template := candidate.entry.template
		template.Config = slices.Clone(template.Config)
		template.RuntimeIssues = slices.Clone(template.RuntimeIssues)
		return ResolvedTemplate{
			Listing: candidate.listing(), Template: template, OnboardingAccepted: err == nil,
			PolicyIssues: candidate.policyIssues(),
		}, nil
	}
	return ResolvedTemplate{}, ErrListingUnavailable
}

func (s selection) policyIssues() []string {
	var issues []string
	if !s.entry.compatible || !s.parent.compatible {
		issues = append(issues, "incompatible_unraid_version")
	}
	if s.entry.uninstallOnly || s.parent.uninstallOnly {
		issues = append(issues, "uninstall_only")
	}
	if s.entry.hasRequirements || s.parent.hasRequirements {
		issues = append(issues, "publisher_requirements")
	}
	if s.entry.private || s.parent.private {
		issues = append(issues, "private_template")
	}
	return issues
}

func (c *Catalog) load(ctx context.Context) ([]selection, catalogPolicy, error) {
	if err := ctx.Err(); err != nil {
		return nil, catalogPolicy{}, err
	}
	buildManifest, err := c.readSettings(pluginManifest)
	if err != nil {
		return nil, catalogPolicy{}, ErrCatalogUnavailable
	}
	match := buildEntity.FindSubmatch(buildManifest)
	if len(match) != 2 || string(match[1]) != supportedBuild || !c.supportedUnraid() {
		return nil, catalogPolicy{}, ErrCatalogUnsupported
	}
	entries, err := c.loadEntries(ctx)
	if err != nil {
		return nil, catalogPolicy{}, err
	}
	policy, err := c.loadPolicy()
	return entries, policy, err
}

func (c *Catalog) supportedUnraid() bool {
	data, err := c.readSettings(osVersionPath)
	if err != nil {
		return false
	}
	version, err := ini.Load(data)
	if err != nil {
		return false
	}
	match := unraidRelease.FindStringSubmatch(version.Section("").Key("version").String())
	if len(match) != 3 {
		return false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(match[2])
	return err == nil && (major > 7 || major == 7 && minor >= 2)
}

func (c *Catalog) readSettings(name string) ([]byte, error) {
	file, err := c.host.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return readBounded(file, maxSettingsBytes)
}

func (c *Catalog) loadPolicy() (catalogPolicy, error) {
	policy := catalogPolicy{hideDeprecated: true, hideIncompatible: true}
	data, err := c.readSettings(settingsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return policy, nil
	}
	if err != nil {
		return catalogPolicy{}, ErrCatalogUnavailable
	}
	config, err := ini.Load(data)
	if err != nil {
		return catalogPolicy{}, ErrCatalogUnavailable
	}
	settings := config.Section("")
	if key, err := settings.GetKey("hideDeprecated"); err == nil {
		policy.hideDeprecated = key.String() == "true"
	}
	if key, err := settings.GetKey("hideIncompatible"); err == nil {
		policy.hideIncompatible = key.String() == "true"
	}
	return policy, nil
}

func readBounded(file fs.File, maxSize int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, errCacheEncoding
	}
	return data, nil
}

func (c *Catalog) loadEntries(ctx context.Context) ([]selection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := c.host.Open(catalogPath)
	if err != nil {
		c.fileInfo, c.snapshot = nil, nil
		return nil, ErrCatalogUnavailable
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, ErrCatalogUnavailable
	}
	if info.Size() > maxCatalogBytes {
		return nil, ErrCatalogUnsupported
	}
	if old := c.fileInfo; old != nil && os.SameFile(old, info) &&
		old.Size() == info.Size() && old.ModTime().Equal(info.ModTime()) {
		return c.snapshot, nil
	}
	c.fileInfo, c.snapshot = nil, nil
	data, err := readBounded(file, maxCatalogBytes)
	if errors.Is(err, errCacheEncoding) {
		return nil, ErrCatalogUnsupported
	}
	if err != nil {
		return nil, ErrCatalogUnavailable
	}
	entries, err := decodeCAEntries(ctx, data)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, ErrCatalogUnsupported
	}
	selections := selectDockerListings(entries)
	c.fileInfo, c.snapshot = info, selections
	return selections, nil
}

func readEntry(row phpArray) (catalogEntry, error) {
	name := row.text("Name")
	if name == "" {
		return catalogEntry{}, errCacheEncoding
	}
	if row.flag("Plugin") || row.text("PluginURL") != "" || row.text("Language") != "" || row.flag("LanguagePack") {
		return catalogEntry{}, nil
	}
	image := row.text("Repository")
	if image == "" {
		return catalogEntry{}, errCacheEncoding
	}
	for _, key := range []string{"Displayable", "Compatible", "Blacklist", "Deprecated"} {
		if _, ok := row.get(key).(bool); !ok {
			return catalogEntry{}, errCacheEncoding
		}
	}
	entry := catalogEntry{
		id: row.text("ID"), name: name, image: image, publisher: row.text("Author"),
		repository: row.text("RepoName"), path: row.text("Path"), templateURL: row.text("TemplateURL"),
		variant: row.text("BranchName"), displayable: row.flag("Displayable"), compatible: row.flag("Compatible"),
		blacklisted: row.flag("Blacklist"), deprecated: row.flag("Deprecated"), featured: row.flag("Featured"),
		hasRequirements: hasTemplateSetting(row, "Requires") || hasTemplateSetting(row, "RequiresFile"),
		private:         row.flag("Private"), uninstallOnly: row.flag("UninstallOnly"), hidden: row.flag("hideFromCA"),
	}
	for _, field := range row.array("BranchID") {
		if id, ok := field.value.(int64); ok {
			entry.branches = append(entry.branches, strconv.FormatInt(id, 10))
		}
	}
	entry.template = readTemplate(row, Listing{Name: name, Image: image})
	entry.searchTerms = [5]string{
		strings.ToLower(entry.name), strings.ToLower(entry.image), strings.ToLower(entry.publisher),
		strings.ToLower(entry.repository), strings.ToLower(entry.variant),
	}
	return entry, nil
}

func selectDockerListings(entries []catalogEntry) []selection {
	branches := make(map[string]*catalogEntry)
	for i := range entries {
		entry := &entries[i]
		if entry.variant != "" && entry.id != "" {
			// Refuse to resolve ambiguous branch IDs within a cache.
			if _, duplicate := branches[entry.id]; duplicate {
				branches[entry.id] = nil
			} else {
				branches[entry.id] = entry
			}
		}
	}
	candidates := make([]selection, 0, len(entries))
	for i := range entries {
		parent := &entries[i]
		if !parent.displayable || parent.variant != "" {
			continue
		}
		candidates = append(candidates, parent.selectEntry(parent))
		for _, id := range parent.branches {
			if branch := branches[id]; branch != nil {
				candidates = append(candidates, parent.selectEntry(branch))
			}
		}
	}
	counts := make(map[string]int, len(candidates))
	for _, candidate := range candidates {
		counts[candidate.reference]++
	}
	// An identity collision is ambiguous even when only one row is visible.
	// Omitting both rows prevents a policy change from retargeting an install.
	return slices.DeleteFunc(candidates, func(candidate selection) bool { return counts[candidate.reference] != 1 })
}

func (parent *catalogEntry) selectEntry(entry *catalogEntry) selection {
	source := parent.templateURL
	if source == "" {
		source = parent.path
	}
	digest := sha256.New()
	for _, value := range []string{parent.repository, parent.publisher, parent.name, source, entry.variant} {
		// Length-prefix fields to keep identity independent of field delimiters.
		_, _ = io.WriteString(digest, strconv.Itoa(len(value))+":"+value)
	}
	return selection{entry: entry, parent: parent, reference: hex.EncodeToString(digest.Sum(nil))}
}
