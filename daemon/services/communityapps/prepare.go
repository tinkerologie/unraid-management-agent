package communityapps

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	digest "github.com/opencontainers/go-digest"

	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

const (
	planLifetime = 15 * time.Minute
	maxPlans     = 64
)

var (
	ErrInvalidPlanRequest = &Problem{Code: "invalid_plan_request", Message: "Provide a valid listing reference and typed overrides for declared settings."}
	ErrCAOnboarding       = &Problem{Code: "onboarding_required", Message: "Accept the Community Applications first-run warning in the Unraid Apps tab."}
	ErrNameConflict       = &Problem{Code: "name_conflict", Message: "A Docker container or saved Unraid user template already uses this name."}
	ErrPortConflict       = &Problem{Code: "port_conflict", Message: "A fixed port binding is already in use, duplicated, or unavailable on this host."}
	ErrSharedAppdata      = &Problem{Code: "shared_appdata", Message: "An existing installation overlaps a writable source. Specify a distinct path or explicitly allow the shared path for this mount."}
	ErrExposedMountParent = &Problem{Code: "exposed_mount_parent", Message: "A writable Docker bind exposes a parent of another approved source. Choose a different storage path."}
	ErrDockerState        = &Problem{Code: "docker_unavailable", Message: "Current Docker daemon state could not be checked."}
	ErrStorageState       = &Problem{Code: "storage_unavailable", Message: "Current Unraid storage or native Docker settings could not be verified."}
	ErrImageDigest        = &Problem{Code: "image_digest_unavailable", Message: "Docker could not verify the configured image's registry digest."}
	ErrPlanCapacity       = &Problem{Code: "plan_capacity", Message: "Too many pending install plans. Retry after an outstanding plan expires."}
	ErrPlanExpired        = &Problem{Code: "plan_expired", Message: "The pending plan is unavailable or expired. Prepare and review a new install plan."}
	ErrPlanApproval       = &Problem{Code: "invalid_plan_approval", Message: "Supply the exact plan approval token after reviewing the preview."}
	ErrPlanChanged        = &Problem{Code: "plan_changed", Message: "The listing or approved settings changed. Prepare and review a new install plan."}
	ErrPlanCredential     = &Problem{Code: "credential_required", Message: "Provide a valid transport credential to request a Community Applications install."}

	bridgeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,127}$`)
	envName    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,127}$`)
	fullID     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// PortOverride sets the host binding for a declared port. Keys use the
// container port and protocol, for example "80/tcp".
type PortOverride struct {
	HostPort    uint16 `json:"host_port"`
	BindAddress string `json:"bind_address,omitempty"`
}

// PathOverride selects one declared mount's source. A deliberate shared mount
// choice applies only to this explicitly supplied source.
type PathOverride struct {
	Source      string `json:"source"`
	AllowShared bool   `json:"allow_shared,omitempty"`
}

// PrepareRequest identifies a CA listing and provides per-install overrides.
type PrepareRequest struct {
	Reference   string                  `json:"reference"`
	Name        string                  `json:"name,omitempty"`
	Network     string                  `json:"network,omitempty"`
	Ports       map[string]PortOverride `json:"ports,omitempty"`
	Paths       map[string]PathOverride `json:"paths,omitempty"`
	Environment map[string]string       `json:"environment,omitempty"`
}

// PortBinding is a fixed address, host port, protocol and container port.
type PortBinding struct {
	BindAddress   string `json:"bind_address"`
	HostPort      uint16 `json:"host_port"`
	Protocol      string `json:"protocol"`
	ContainerPort uint16 `json:"container_port"`
}

// Mount shows the canonical source and changes the install would make.
type Mount struct {
	Source          string `json:"source"`
	Target          string `json:"target"`
	Mode            string `json:"mode"`
	CreateDirectory bool   `json:"create_directory"`
	Shared          bool   `json:"shared"`
}

// Bridge identifies the native network and the Docker object it resolves to.
type Bridge struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Driver   string `json:"driver"`
	Internal bool   `json:"internal"`
	IPv4     bool   `json:"ipv4"`
	IPv6     bool   `json:"ipv6"`
}

// EnvironmentField shows whether an environment variable has a nonempty value.
type EnvironmentField struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
}

// Configuration shows the settings that can be retained in an install result.
type Configuration struct {
	Listing     Listing            `json:"listing"`
	Name        string             `json:"name"`
	Image       string             `json:"image"`
	Digest      digest.Digest      `json:"digest,omitempty" swaggertype:"string"`
	Network     Bridge             `json:"network"`
	Ports       []PortBinding      `json:"ports"`
	Mounts      []Mount            `json:"mounts"`
	Environment []EnvironmentField `json:"environment"`
	PIDsLimit   int64              `json:"pids_limit,omitempty"`
	Privileged  bool               `json:"privileged"`
}

// Preview is the redacted configuration and opaque approval material. Only an
// eligible preview carries a plan ID and approval token.
type Preview struct {
	Configuration
	Eligible      bool       `json:"eligible"`
	Issues        []string   `json:"issues"`
	PlanID        string     `json:"plan_id,omitempty"`
	ApprovalToken string     `json:"approval_token,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type environmentSetting struct {
	name, value string
}

type installSettings struct {
	configuration   Configuration
	template        Template
	environment     []environmentSetting
	mountIdentities []mountIdentity
}

type pendingPlan struct {
	request   PrepareRequest
	preview   Preview
	effective installSettings
	expiry    *time.Timer
}

// PlanningDocker is the read-only Docker Engine surface used to prepare plans.
type PlanningDocker interface {
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	DistributionInspect(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error)
	Close() error
}

// ServiceOptions supplies the host filesystem and system boundaries for planning.
type ServiceOptions struct {
	HostRoot          string
	OpenInstallDocker func() (InstallDocker, error)
	OpenDocker        func() (PlanningDocker, error)
	PortAvailable     func(context.Context, PortBinding) error
	Now               func() time.Time
}

// Service owns pending plans shared by REST and MCP.
type Service struct {
	catalog           *Catalog
	hostRoot          string
	openDocker        func() (PlanningDocker, error)
	portAvailable     func(context.Context, PortBinding) error
	now               func() time.Time
	mu                sync.Mutex
	plans             map[string]*pendingPlan
	openInstaller     func() (InstallDocker, error)
	createReceiptTemp func(string, string) (*os.File, error)
	closed            bool
	recoveryStop      context.CancelFunc
	workers           sync.WaitGroup
	applyMu           sync.Mutex
	active            map[string]context.CancelFunc
	recoveryWake      chan struct{}
	receiptsLoaded    bool
	receiptSyncNeeded bool
	pendingCommit     map[string]installReceipt
	receipts          map[string]installReceipt
	attemptIDs        map[string]string
	unreadableClaims  map[string]struct{}
}

// NewService creates an install service using the supplied host dependencies.
func NewService(catalog *Catalog, options ServiceOptions) *Service {
	if options.HostRoot == "" {
		options.HostRoot = "/"
	}
	if root, err := filepath.EvalSymlinks(options.HostRoot); err == nil {
		options.HostRoot = root
	}
	if options.OpenDocker == nil {
		options.OpenDocker = func() (PlanningDocker, error) { return openLocalInstallDocker() }
	}
	if options.OpenInstallDocker == nil {
		options.OpenInstallDocker = func() (InstallDocker, error) { return openLocalInstallDocker() }
	}
	if options.PortAvailable == nil {
		options.PortAvailable = probePort
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{
		catalog: catalog, hostRoot: options.HostRoot, openDocker: options.OpenDocker,
		openInstaller: options.OpenInstallDocker, createReceiptTemp: os.CreateTemp,
		portAvailable: options.PortAvailable, now: options.Now, plans: make(map[string]*pendingPlan),
		active: make(map[string]context.CancelFunc), pendingCommit: make(map[string]installReceipt),
	}
}

func openLocalInstallDocker() (*client.Client, error) {
	docker, err := client.New(client.FromEnv)
	if err != nil {
		return nil, ErrDockerState
	}
	socket, local := strings.CutPrefix(docker.DaemonHost(), "unix://")
	if !local || !filepath.IsAbs(socket) {
		_ = docker.Close()
		return nil, ErrDockerState
	}
	return docker, nil
}

// NewHostService prepares plans using the local Unraid host and Docker Engine.
func NewHostService(catalog *Catalog) *Service {
	return NewService(catalog, ServiceOptions{})
}

// Prepare resolves effective settings without mutating Docker or host storage.
func (s *Service) Prepare(ctx context.Context, request PrepareRequest) (Preview, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	effective, issues, err := s.resolve(ctx, request, "")
	if err != nil {
		return Preview{}, err
	}
	preview := Preview{Configuration: effective.configuration, Issues: issues, Eligible: len(issues) == 0}
	if preview.Eligible {
		if err := s.storePending(request, &preview, effective); err != nil {
			return Preview{}, err
		}
	}
	return clonePreview(preview), nil
}

func (s *Service) storePending(request PrepareRequest, preview *Preview, effective installSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrInstallStopped
	}
	now := s.now().UTC()
	for id, plan := range s.plans {
		if !now.Before(*plan.preview.ExpiresAt) {
			s.dropPendingLocked(id)
		}
	}
	if len(s.plans) >= maxPlans {
		return ErrPlanCapacity
	}
	request.Environment = maps.Clone(request.Environment)
	request.Ports = maps.Clone(request.Ports)
	request.Paths = maps.Clone(request.Paths)
	expiration := now.Add(planLifetime)
	preview.PlanID, preview.ApprovalToken, preview.ExpiresAt = rand.Text(), rand.Text(), &expiration
	plan := &pendingPlan{request: request, preview: *preview, effective: effective}
	s.plans[preview.PlanID] = plan
	id := preview.PlanID
	plan.expiry = time.AfterFunc(planLifetime, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.dropPendingLocked(id)
	})
	return nil
}

func (s *Service) dropPendingLocked(id string) {
	if plan := s.plans[id]; plan != nil {
		plan.expiry.Stop()
		delete(s.plans, id)
	}
}

func clonePreview(preview Preview) Preview {
	preview.Ports = slices.Clone(preview.Ports)
	preview.Mounts = slices.Clone(preview.Mounts)
	preview.Environment = slices.Clone(preview.Environment)
	preview.Issues = slices.Clone(preview.Issues)
	if preview.ExpiresAt != nil {
		expiration := *preview.ExpiresAt
		preview.ExpiresAt = &expiration
	}
	return preview
}

// ValidateApproval checks a pending approval and its live settings. An
// executor must atomically claim the validated plan before accepting it.
func (s *Service) ValidateApproval(ctx context.Context, planID, approvalToken string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	plan, err := s.lookupPending(planID, approvalToken)
	if err != nil {
		return err
	}
	if err := s.revalidate(ctx, plan); err != nil {
		return err
	}
	_, err = s.lookupPending(planID, approvalToken)
	return err
}

func (s *Service) lookupPending(planID, approvalToken string) (*pendingPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrInstallStopped
	}
	plan := s.plans[planID]
	if plan == nil || !s.now().Before(*plan.preview.ExpiresAt) {
		s.dropPendingLocked(planID)
		return nil, ErrPlanExpired
	}
	if subtle.ConstantTimeCompare([]byte(approvalToken), []byte(plan.preview.ApprovalToken)) != 1 {
		return nil, ErrPlanApproval
	}
	return plan, nil
}

// revalidate recomputes the selected material template and live effective
// settings. Unrelated catalog rows and presentation fields are immaterial.
func (s *Service) revalidate(ctx context.Context, plan *pendingPlan) error {
	return s.revalidateSettings(ctx, plan, "")
}

func (s *Service) revalidateForInstall(ctx context.Context, plan *pendingPlan) error {
	return s.revalidateSettings(ctx, plan, plan.effective.configuration.Digest)
}

func (s *Service) revalidateSettings(ctx context.Context, plan *pendingPlan, approvedDigest digest.Digest) error {
	current, issues, err := s.resolve(ctx, plan.request, approvedDigest)
	if err != nil {
		return err
	}
	if len(issues) != 0 || !sameInstallSettings(current, plan.effective) {
		return ErrPlanChanged
	}
	return nil
}

func sameInstallSettings(current, approved installSettings) bool {
	// Presentation metadata is carried into the saved template without
	// changing the runtime approval. Icon and WebUI also set Docker labels.
	for _, settings := range []*installSettings{&current, &approved} {
		t := &settings.template
		t.Registry, t.Support, t.Project, t.Overview, t.Category = "", "", "", "", ""
		t.Config = slices.Clone(t.Config)
		for i := range t.Config {
			t.Config[i].Name, t.Config[i].Description, t.Config[i].Display = "", "", ""
		}
	}
	return reflect.DeepEqual(current, approved)
}

func (s *Service) resolve(ctx context.Context, request PrepareRequest, approvedDigest digest.Digest) (installSettings, []string, error) {
	result, err := s.catalog.Resolve(ctx, request.Reference)
	if err != nil {
		return installSettings{}, nil, err
	}
	if !result.OnboardingAccepted {
		return installSettings{}, nil, ErrCAOnboarding
	}
	settings := newInstallSettings(result, request)
	if !validNativeName(settings.configuration.Name) {
		return installSettings{}, nil, ErrInvalidPlanRequest
	}
	issues := append(append([]string{}, result.PolicyIssues...), result.Template.RuntimeIssues...)
	settings.configuration.Privileged = slices.Contains(issues, "privileged")
	if safeNativeURL(result.Template.CatalogLink) == "" {
		issues = append(issues, "missing_native_catalog_link")
	}
	if len(issues) != 0 {
		for _, field := range result.Template.Config {
			if field.Kind != ConfigPath {
				continue
			}
			override, overridden := request.Paths[field.Target]
			if !overridden && field.mountsDockerSocket() ||
				overridden && (isDockerSocket(override.Source) || isDockerSocket(field.Target)) {
				issues = append(issues, "docker_socket_mount")
				break
			}
		}
		return settings, issues, nil
	}
	if err := validateOverrides(result.Template.Config, request); err != nil {
		return installSettings{}, nil, err
	}
	host, err := readInstallHost(s.hostRoot)
	if err != nil {
		return installSettings{}, nil, err
	}
	settings.configuration.PIDsLimit = host.pidsLimit
	if issues = settings.resolveFields(host, request); len(issues) != 0 {
		return settings, issues, nil
	}
	return s.resolveDocker(ctx, settings, host, request, approvedDigest)
}

func validNativeName(name string) bool {
	return len(name) <= 128 && lib.ValidateContainerRef(name) == nil
}

func newInstallSettings(result ResolvedTemplate, request PrepareRequest) installSettings {
	name := request.Name
	if name == "" {
		name = result.Template.Name
	}
	networkName := request.Network
	if networkName == "" {
		networkName = result.Template.Network
	}
	return installSettings{
		configuration: Configuration{
			Listing: result.Listing, Name: name, Image: result.Template.Image, Network: Bridge{Name: networkName},
			Ports: []PortBinding{}, Mounts: []Mount{}, Environment: []EnvironmentField{},
		},
		template: result.Template,
	}
}

func (s *Service) resolveDocker(ctx context.Context, settings installSettings, host installHost, request PrepareRequest, approvedDigest digest.Digest) (installSettings, []string, error) {
	config := &settings.configuration
	if !bridgeName.MatchString(config.Network.Name) {
		return settings, []string{"unsupported_network"}, nil
	}
	docker, err := s.openDocker()
	if err != nil {
		return installSettings{}, nil, ErrDockerState
	}
	defer func() { _ = docker.Close() }()
	bridge, err := inspectBridge(ctx, docker, config.Network.Name, config.Ports)
	if err != nil {
		return settings, nil, err
	}
	if bridge.ID == "" {
		return settings, []string{"unsupported_network"}, nil
	}
	config.Network = bridge
	if err := checkDuplicateBindings(config.Ports); err != nil {
		return installSettings{}, nil, err
	}
	containers, err := docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return installSettings{}, nil, ErrDockerState
	}
	items, err := withNamedVolumeBinds(ctx, docker, config.Mounts, containers.Items)
	if err != nil {
		return installSettings{}, nil, err
	}
	if err := host.checkConflicts(config, request, items); err != nil {
		return installSettings{}, nil, err
	}
	if err := s.checkApprovedPorts(ctx, *config); err != nil {
		return installSettings{}, nil, err
	}
	config.Digest = approvedDigest
	if approvedDigest == "" {
		_, config.Digest, err = resolveImageDigest(ctx, docker, config.Image)
	}
	if err != nil {
		return installSettings{}, nil, err
	}
	return settings, []string{}, nil
}

func inspectBridge(ctx context.Context, docker PlanningDocker, name string, ports []PortBinding) (Bridge, error) {
	bridge, err := docker.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil && !errdefs.IsNotFound(err) {
		return Bridge{}, ErrDockerState
	}
	network := bridge.Network
	if network.Name != name || network.Driver != "bridge" || network.Scope != "local" ||
		network.Internal && len(ports) != 0 || network.ConfigOnly || !fullID.MatchString(network.ID) ||
		!assignBindAddresses(ports, network.Network) || !supportsFixedBindings(ports, network.Network) {
		return Bridge{}, nil
	}
	return Bridge{
		Name: network.Name, ID: network.ID, Driver: network.Driver, Internal: network.Internal,
		IPv4: hasNetworkFamily(network.Network, false), IPv6: hasNetworkFamily(network.Network, true),
	}, nil
}

func resolveImageDigest(ctx context.Context, docker PlanningDocker, image string) (reference.Named, digest.Digest, error) {
	if len(image) > 512 {
		return nil, "", ErrImageDigest
	}
	normalized, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return nil, "", ErrImageDigest
	}
	info, err := docker.DistributionInspect(ctx, image, client.DistributionInspectOptions{})
	if err != nil || info.Descriptor.Digest.Validate() != nil || info.Descriptor.Digest.Algorithm() != digest.SHA256 {
		return nil, "", ErrImageDigest
	}
	if pinned, ok := normalized.(reference.Digested); ok && pinned.Digest() != info.Descriptor.Digest {
		return nil, "", ErrImageDigest
	}
	return normalized, info.Descriptor.Digest, nil
}

func checkDockerName(containers []container.Summary, wanted string) error {
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.EqualFold(strings.TrimPrefix(name, "/"), wanted) {
				return ErrNameConflict
			}
		}
	}
	return nil
}

func validateOverrides(fields []TemplateConfig, request PrepareRequest) error {
	if len(fields) > 256 || len(request.Ports)+len(request.Paths)+len(request.Environment) > 256 {
		return ErrInvalidPlanRequest
	}
	kinds := make(map[string]ConfigKind, len(fields))
	total := 0
	for _, field := range fields {
		total += len(field.Name) + len(field.Target) + len(field.Mode) + len(field.Default) + len(field.Value)
		key := field.Target
		if field.Kind == ConfigPort {
			key += "/" + strings.ToLower(field.Mode)
		}
		kinds[key] = field.Kind
	}
	if total > 128<<10 {
		return ErrInvalidPlanRequest
	}
	for key, port := range request.Ports {
		if kinds[key] != ConfigPort || port.HostPort == 0 {
			return ErrInvalidPlanRequest
		}
		if port.BindAddress != "" {
			address, err := netip.ParseAddr(port.BindAddress)
			if err != nil || !validBindAddress(address) {
				return ErrInvalidPlanRequest
			}
		}
	}
	for key, path := range request.Paths {
		if kinds[key] != ConfigPath || !validBindPath(path.Source) {
			return ErrInvalidPlanRequest
		}
	}
	size := 0
	for key, value := range request.Environment {
		size += len(value)
		if kinds[key] != ConfigEnvironment || len(value) > 4096 {
			return ErrInvalidPlanRequest
		}
	}
	if size > 64<<10 {
		return ErrInvalidPlanRequest
	}
	return nil
}
