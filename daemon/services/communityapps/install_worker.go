package communityapps

import (
	"context"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	digest "github.com/opencontainers/go-digest"
)

const installOwnerLabel = "net.unraid.management-agent.ca-install"

var errContainerIdentity = &Problem{Code: "container_identity_unverified", Message: "Docker reports a container with changed or ambiguous ownership evidence. Inspect the original installation in Docker Manager."}

func (s *Service) runInstall(receipt installReceipt, plan *pendingPlan) {
	defer func() {
		s.mu.Lock()
		delete(s.active, receipt.PlanID)
		s.wakeRecoveryLocked()
		s.mu.Unlock()
	}()
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	ctx, cancel := context.WithTimeout(s.lifecycle, 45*time.Minute)
	defer cancel()

	if ctx.Err() != nil {
		s.finishInstall(&receipt, ErrInstallStopped)
		return
	}
	if err := s.advanceInstall(&receipt, "validating"); err != nil {
		s.finishInstall(&receipt, err)
		return
	}
	docker, err := s.openInstaller()
	if err != nil {
		s.finishInstall(&receipt, ErrDockerState)
		return
	}
	defer func() { _ = docker.Close() }()
	err = s.executeInstall(ctx, docker, &receipt, plan)
	if installMutationMayExist(receipt.Attempt.Phase) {
		// A disconnected Engine call can report failure after taking effect.
		// Observe with a bounded context even during daemon shutdown.
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer checkCancel()
		s.settleInstall(checkCtx, docker, &receipt, err)
		return
	}
	s.finishInstall(&receipt, err)
}

func installMutationMayExist(phase string) bool {
	switch phase {
	case "creating_container", "publishing_template", "starting_container", "cleaning_up":
		return true
	default:
		return false
	}
}

func (s *Service) advanceInstall(receipt *installReceipt, phase string) error {
	next := *receipt
	next.Attempt.State, next.Attempt.Phase = AttemptRunning, phase
	if err := s.persistProgress(next); err != nil {
		return err
	}
	*receipt = next
	return nil
}

func (s *Service) beginInstallCleanup(receipt *installReceipt, cause error) error {
	if receipt.Attempt.Phase == "cleaning_up" {
		return nil
	}
	next := *receipt
	next.Attempt.Problem = ProblemForError(cause)
	next.CleanupPhase = receipt.Attempt.Phase
	if err := s.advanceInstall(&next, "cleaning_up"); err != nil {
		return err
	}
	*receipt = next
	return nil
}

func (s *Service) finishInstall(receipt *installReceipt, err error) bool {
	receipt.Attempt.Phase = "complete"
	if err == nil {
		receipt.Attempt.State = AttemptSucceeded
		receipt.Attempt.Problem = nil
		receipt.Attempt.ContainerID = receipt.OwnedContainerID
	} else {
		receipt.Attempt.State = AttemptFailed
		receipt.Attempt.Problem = ProblemForError(err)
	}
	// Retry transient writes. If the store remains unavailable, return its
	// last durable phase for restart reconciliation instead of inventing a
	// durable outcome.
	for range 2 {
		if s.persistProgress(*receipt) == nil {
			return true
		}
	}
	s.mu.Lock()
	s.pendingCommit[receipt.PlanID] = *receipt
	s.mu.Unlock()
	return false
}

func (s *Service) executeInstall(ctx context.Context, docker InstallDocker, receipt *installReceipt, plan *pendingPlan) error {
	if err := s.revalidateForInstall(ctx, plan); err != nil {
		return err
	}
	config := plan.effective.configuration
	if err := s.advanceInstall(receipt, "pulling_image"); err != nil {
		return err
	}
	imageID, err := pullApprovedImage(ctx, docker, config)
	if err != nil {
		return err
	}
	receipt.ImageID = imageID
	if err := s.revalidateForInstall(ctx, plan); err != nil {
		return err
	}
	if err := s.advanceInstall(receipt, "preparing_storage"); err != nil {
		return err
	}
	mounts, err := s.prepareInstallStorage(plan.effective)
	if err != nil {
		return err
	}
	if err := s.advanceInstall(receipt, "creating_container"); err != nil {
		return err
	}
	created, err := docker.ContainerCreate(ctx, containerOptions(plan.effective, receipt.Owner))
	if err != nil {
		return engineInstallError(err, ErrContainerCreate)
	}
	if !fullID.MatchString(created.ID) {
		return ErrContainerCreate
	}
	receipt.OwnedContainerID = created.ID
	if err := s.persistProgress(*receipt); err != nil {
		return err
	}
	stopped, found, err := ownedContainer(ctx, docker, *receipt)
	if err != nil || !found || stopped.Image != receipt.ImageID || stopped.State == nil ||
		stopped.State.Status != "created" || hasStarted(stopped) {
		return ErrDigestChanged
	}
	template, err := prepareNativeTemplate(plan.effective, receipt, s.now().Unix())
	if err != nil {
		return err
	}
	if err := s.advanceInstall(receipt, "publishing_template"); err != nil {
		return err
	}
	if err := s.publishNativeTemplate(template, *receipt); err != nil {
		return err
	}
	if err := s.checkBeforeStart(ctx, docker, *receipt, plan, mounts); err != nil {
		return err
	}
	if err := s.advanceInstall(receipt, "starting_container"); err != nil {
		return err
	}
	if _, err := docker.ContainerStart(ctx, receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
		return engineInstallError(err, ErrContainerStart)
	}
	return nil
}

func pullApprovedImage(ctx context.Context, docker InstallDocker, config Configuration) (string, error) {
	current, err := resolveImageDigest(ctx, docker, config.Image)
	if err != nil {
		return "", err
	}
	if current != config.Digest {
		return "", ErrDigestChanged
	}
	pull, err := docker.ImagePull(ctx, config.Image, client.ImagePullOptions{})
	if err != nil {
		return "", ErrImagePull
	}
	defer func() { _ = pull.Close() }()
	var receivedDigest bool
	for msg, streamErr := range pull.JSONMessages(ctx) {
		if streamErr != nil || msg.Error != nil {
			return "", ErrImagePull
		}
		if d, found := strings.CutPrefix(msg.Status, "Digest: "); found && msg.ID == "" {
			if d != config.Digest.String() {
				return "", ErrDigestChanged
			}
			receivedDigest = true
		}
	}
	if !receivedDigest || ctx.Err() != nil {
		return "", ErrImagePull
	}

	name, err := reference.ParseNormalizedNamed(config.Image)
	if err != nil {
		return "", ErrImagePull
	}
	canonical, err := reference.WithDigest(reference.TrimNamed(name), config.Digest)
	if err != nil {
		return "", ErrImagePull
	}
	result, err := docker.ImageInspect(ctx, canonical.String())
	if err != nil || !validImageID(result.ID) {
		return "", ErrImagePull
	}
	associated := slices.ContainsFunc(result.RepoDigests, func(repoDigest string) bool {
		ref, err := reference.ParseNormalizedNamed(repoDigest)
		return err == nil && ref.String() == canonical.String()
	})
	if result.Descriptor != nil {
		associated = result.Descriptor.Digest == config.Digest
	}
	if !associated {
		return "", ErrImagePull
	}
	return result.ID, nil
}

func validImageID(id string) bool {
	value := digest.Digest(id)
	return value.Validate() == nil && value.Algorithm() == digest.SHA256
}

func containerOptions(settings installSettings, owner string) client.ContainerCreateOptions {
	c := settings.configuration
	labels := map[string]string{dockerManagedLabel: dockerManagedValue, installOwnerLabel: owner}
	if settings.template.WebUI != "" {
		labels["net.unraid.docker.webui"] = settings.template.WebUI
	}
	if settings.template.Icon != "" {
		labels["net.unraid.docker.icon"] = settings.template.Icon
	}
	config := &container.Config{
		Image: c.Image, Labels: labels, ExposedPorts: make(network.PortSet, len(c.Ports)),
	}
	host := &container.HostConfig{
		NetworkMode: container.NetworkMode(c.Network.Name), PortBindings: make(network.PortMap, len(c.Ports)),
		Resources: container.Resources{PidsLimit: &c.PIDsLimit},
	}
	for _, env := range settings.environment {
		config.Env = append(config.Env, env.name+"="+env.value)
	}
	for _, p := range c.Ports {
		port := network.MustParsePort(strconv.Itoa(int(p.ContainerPort)) + "/" + p.Protocol)
		config.ExposedPorts[port] = struct{}{}
		host.PortBindings[port] = append(host.PortBindings[port], network.PortBinding{
			HostIP: netip.MustParseAddr(p.BindAddress), HostPort: strconv.Itoa(int(p.HostPort)),
		})
	}
	for _, m := range c.Mounts {
		host.Mounts = append(host.Mounts, mount.Mount{
			Type: mount.TypeBind, Source: m.Source, Target: m.Target, ReadOnly: readOnlyMode(m.Mode),
		})
	}
	return client.ContainerCreateOptions{
		Name: c.Name, Config: config, HostConfig: host,
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{c.Network.Name: {NetworkID: c.Network.ID}},
		},
	}
}

func engineInstallError(err error, fallback *Problem) error {
	// Only fixed, allowlisted diagnostics cross the boundary. Docker errors
	// and warnings can echo container configuration, including secrets.
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "port is already allocated") || strings.Contains(message, "address already in use") {
		return ErrPortConflict
	}
	if errdefs.IsConflict(err) && (strings.Contains(message, "container name") || strings.Contains(message, "already in use")) {
		return ErrNameConflict
	}
	return fallback
}

func ownedContainer(ctx context.Context, docker InstallDocker, receipt installReceipt) (container.InspectResponse, bool, error) {
	id := receipt.OwnedContainerID
	if id == "" {
		var err error
		id, err = findCreatedContainer(ctx, docker, receipt)
		if err != nil || id == "" {
			return container.InspectResponse{}, false, err
		}
	}
	result, err := docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return container.InspectResponse{}, false, nil
	}
	if err != nil {
		return container.InspectResponse{}, false, ErrDockerState
	}
	c := result.Container
	if !fullID.MatchString(c.ID) {
		return c, false, ErrDockerState
	}
	if c.ID != id {
		return c, false, nil
	}
	if c.Config == nil || c.Name != "/"+receipt.Attempt.Configuration.Name ||
		c.Config.Labels[installOwnerLabel] != receipt.Owner || c.Config.Image != receipt.Attempt.Configuration.Image ||
		(receipt.OwnedContainerID == "" && c.Config.Labels["io.tinkerologie.uma.update-attempt"] != "") {
		return c, false, errContainerIdentity
	}
	return c, true, nil
}

func findCreatedContainer(ctx context.Context, docker InstallDocker, receipt installReceipt) (string, error) {
	result, err := docker.ContainerList(ctx, client.ContainerListOptions{
		All: true, Filters: make(client.Filters).Add("label", installOwnerLabel+"="+receipt.Owner),
	})
	if err != nil {
		return "", ErrDockerState
	}
	var id string
	for _, c := range result.Items {
		// The updater copies labels to its replacement. An unrelated
		// container with the expected name is also never ownership proof.
		if c.Labels[installOwnerLabel] != receipt.Owner || c.Labels["io.tinkerologie.uma.update-attempt"] != "" {
			continue
		}
		if id != "" || !fullID.MatchString(c.ID) {
			return "", errContainerIdentity
		}
		id = c.ID
	}
	return id, nil
}

func hasStarted(container container.InspectResponse) bool {
	if container.State == nil {
		return false
	}
	started, err := time.Parse(time.RFC3339Nano, container.State.StartedAt)
	return err == nil && !started.IsZero()
}

// completedInstall distinguishes positive completion, known partial state,
// and an unavailable ownership check. Only the known partial can be cleaned up.
func (s *Service) completedInstall(ctx context.Context, docker InstallDocker, receipt installReceipt) (bool, error) {
	if receipt.OwnedContainerID == "" || !validImageID(receipt.ImageID) {
		return false, nil
	}
	c, owned, err := ownedContainer(ctx, docker, receipt)
	if err == errContainerIdentity {
		return false, nil
	}
	if err != nil || !owned {
		return false, err
	}
	return s.startedWithTemplate(c, receipt)
}

func (s *Service) startedWithTemplate(c container.InspectResponse, receipt installReceipt) (bool, error) {
	if c.Image != receipt.ImageID {
		return false, nil
	}
	if c.State == nil {
		return false, ErrDockerState
	}
	started, err := time.Parse(time.RFC3339Nano, c.State.StartedAt)
	if err != nil {
		return false, ErrDockerState
	}
	if started.IsZero() {
		return false, nil
	}
	return s.ownsPublishedTemplate(receipt)
}

func (s *Service) cleanupInstall(ctx context.Context, docker InstallDocker, receipt *installReceipt) (bool, bool) {
	if ctx.Err() != nil {
		return false, false
	}
	containerIssue, confirmedStart, retry := s.removeOwnedContainer(ctx, docker, receipt)
	if retry || ctx.Err() != nil {
		return false, false
	}
	if confirmedStart {
		return true, true
	}
	provenTemplate := receipt.CleanupPhase != "creating_container" && hasTemplateProof(*receipt)
	var preserveFinal bool
	if provenTemplate {
		// The native template may now be in use by a replacement. A lookup
		// by name can only veto template removal; it never authorizes it.
		replacement, err := replacementAtInstallName(ctx, docker, *receipt)
		if err != nil {
			return false, false
		}
		preserveFinal = replacement
	}
	if containerIssue.Code != "" {
		receipt.Attempt.CleanupDetails = append(receipt.Attempt.CleanupDetails, containerIssue)
	}
	if provenTemplate {
		if !preserveFinal {
			if issue := s.removeOwnedNativeFile(*receipt, false); issue.Code != "" {
				receipt.Attempt.CleanupDetails = append(receipt.Attempt.CleanupDetails, issue)
			}
		}
		if issue := s.removeOwnedNativeFile(*receipt, true); issue.Code != "" {
			receipt.Attempt.CleanupDetails = append(receipt.Attempt.CleanupDetails, issue)
		}
	}
	for _, issue := range receipt.Attempt.CleanupDetails {
		receipt.Attempt.CleanupFailures = append(receipt.Attempt.CleanupFailures, issue.Code)
	}
	return true, false
}

func replacementAtInstallName(ctx context.Context, docker InstallDocker, receipt installReceipt) (bool, error) {
	inspected, err := docker.ContainerInspect(ctx, receipt.Attempt.Configuration.Name, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil || !fullID.MatchString(inspected.Container.ID) {
		return false, ErrDockerState
	}
	return inspected.Container.ID != receipt.OwnedContainerID, nil
}

// removeOwnedContainer reports any residual resource, a late verified start,
// or an observation that needs to be retried before further cleanup.
func (s *Service) removeOwnedContainer(ctx context.Context, docker InstallDocker, receipt *installReceipt) (CleanupIssue, bool, bool) {
	issue := CleanupIssue{Resource: "container", ContainerID: receipt.OwnedContainerID}
	c, owned, err := ownedContainer(ctx, docker, *receipt)
	switch err {
	case errContainerIdentity:
		issue.Code = errContainerIdentity.Code
		return issue, false, false
	case nil:
		if !owned {
			return CleanupIssue{}, false, false
		}
	default:
		return CleanupIssue{}, false, true
	}
	issue.ContainerID, issue.OwnershipVerified = c.ID, true
	if receipt.OwnedContainerID == "" {
		next := *receipt
		next.OwnedContainerID = c.ID
		if err := s.persistProgress(next); err != nil {
			return CleanupIssue{}, false, true
		}
		*receipt = next
	}
	if receipt.CleanupPhase == "starting_container" {
		if complete, err := s.startedWithTemplate(c, *receipt); complete || err != nil {
			return CleanupIssue{}, complete, err != nil
		}
	}
	// Refuse a force-removal when Docker has no evidence of a start.
	// A concurrently running Start can then fail removal safely.
	_, err = docker.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: hasStarted(c)})
	if err == nil || errdefs.IsNotFound(err) {
		return CleanupIssue{}, false, false
	}
	after, stillOwned, inspectErr := ownedContainer(ctx, docker, *receipt)
	if inspectErr == nil && !stillOwned {
		return CleanupIssue{}, false, false
	}
	if receipt.CleanupPhase == "starting_container" && stillOwned {
		if complete, err := s.startedWithTemplate(after, *receipt); complete || err != nil {
			return CleanupIssue{}, complete, err != nil
		}
	}
	issue.Code = "remove_container"
	issue.OwnershipVerified = stillOwned
	return issue, false, false
}
