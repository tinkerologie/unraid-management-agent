package communityapps

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"regexp"
	"slices"
	"time"

	"github.com/moby/moby/client"
)

type AttemptState string

const (
	AttemptPending   AttemptState = "pending"
	AttemptRunning   AttemptState = "running"
	AttemptSucceeded AttemptState = "succeeded"
	AttemptFailed    AttemptState = "failed"

	maxActiveAttempts = 16
)

var (
	ErrAttemptNotFound        = &Problem{Code: "attempt_not_found", Message: "The requested Community Applications install attempt was not found."}
	ErrAttemptStorage         = &Problem{Code: "attempt_storage_unavailable", Message: "The durable install receipt could not be read or saved."}
	ErrInstallPlanTooLarge    = &Problem{Code: "install_plan_too_large", Message: "The approved configuration is too large to track durably. Use Unraid Docker Manager to install this application."}
	ErrInstallCapacity        = &Problem{Code: "install_capacity", Message: "Too many active Community Applications install attempts. Retry shortly."}
	ErrInstallStopped         = &Problem{Code: "install_worker_unavailable", Message: "The Community Applications install worker is stopping."}
	ErrDigestChanged          = &Problem{Code: "image_digest_changed", Message: "The configured image no longer resolves to the approved digest. Prepare and review a new install plan."}
	ErrImagePull              = &Problem{Code: "image_pull_failed", Message: "Docker could not pull the approved image or prove its digest."}
	ErrContainerCreate        = &Problem{Code: "container_create_failed", Message: "Docker could not create the planned container."}
	ErrContainerStart         = &Problem{Code: "container_start_failed", Message: "Docker could not verify that the planned container started."}
	ErrStartedTemplateMissing = &Problem{Code: "started_template_missing", Message: "Docker started the installed container, but its native user template is missing or is no longer a regular file. The container was preserved. Check Docker Manager and restore the user template if needed."}
	ErrNativeTemplate         = &Problem{Code: "native_template_failed", Message: "The Unraid user template could not be saved."}

	opaqueID = regexp.MustCompile(`^[A-Z2-7]{26}$`)
)

// Attempt is the redacted, durable progress or outcome of one approved install.
type Attempt struct {
	ID              string         `json:"attempt_id"`
	State           AttemptState   `json:"state"`
	Phase           string         `json:"phase"`
	Configuration   Configuration  `json:"configuration"`
	ContainerID     string         `json:"container_id,omitempty"`
	Problem         *Problem       `json:"problem,omitempty"`
	CleanupFailures []string       `json:"cleanup_failures,omitempty"`
	CleanupDetails  []CleanupIssue `json:"cleanup_details,omitempty"`
}

// CleanupIssue identifies a remaining or unverified resource using public, safe identifiers.
type CleanupIssue struct {
	Code              string `json:"code"`
	Resource          string `json:"resource"`
	ContainerID       string `json:"container_id,omitempty"`
	TemplateFile      string `json:"template_file,omitempty"`
	OwnershipVerified bool   `json:"ownership_verified"`
}

// ConfirmRequest carries the opaque approval token returned with the preview.
type ConfirmRequest struct {
	ApprovalToken string `json:"approval_token"`
}

// InstallDocker is the Docker Engine surface for installing an approved plan.
type InstallDocker interface {
	PlanningDocker
	ImagePull(context.Context, string, client.ImagePullOptions) (client.ImagePullResponse, error)
	ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

// Confirm claims one plan exactly once. A successful receipt is durable before
// Confirm hands the plan to the daemon-owned installation worker.
func (s *Service) Confirm(ctx context.Context, planID, approvalToken string) (Attempt, error) {
	if !opaqueID.MatchString(planID) {
		return Attempt{}, ErrPlanExpired
	}
	if !opaqueID.MatchString(approvalToken) {
		return Attempt{}, ErrPlanApproval
	}

	if attempt, found, err := s.replayConfirmation(planID, approvalToken); found || err != nil {
		return attempt, err
	}
	plan, err := s.lookupPending(planID, approvalToken)
	if err == nil {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = s.revalidateForInstall(checkCtx, plan)
		cancel()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if attempt, found, replayErr := s.replayConfirmationLocked(planID, approvalToken); found || replayErr != nil {
		return attempt, replayErr
	}
	if err != nil {
		return Attempt{}, err
	}
	if s.closed {
		return Attempt{}, ErrInstallStopped
	}
	if current := s.plans[planID]; current != plan || !s.now().Before(*plan.preview.ExpiresAt) {
		return Attempt{}, ErrPlanExpired
	}
	if len(s.active) >= maxActiveAttempts {
		return Attempt{}, ErrInstallCapacity
	}
	if ctx.Err() != nil {
		return Attempt{}, ErrInstallStopped
	}
	if s.installNameReserved(plan.effective.configuration.Name) {
		return Attempt{}, ErrInstallRecovery
	}

	receipt := installReceipt{
		Version:      installReceiptVersion,
		PlanID:       planID,
		ApprovalHash: sha256.Sum256([]byte(approvalToken)),
		Owner:        rand.Text(),
		Attempt: Attempt{
			ID: rand.Text(), State: AttemptPending, Phase: "queued", Configuration: plan.effective.configuration,
		},
	}
	if err := s.saveReceipt(receipt, true); err != nil {
		s.wakeRecoveryLocked()
		return Attempt{}, err
	}
	s.indexReceipt(receipt)
	s.dropPendingLocked(planID)
	workCtx, cancel := context.WithCancel(context.Background())
	s.active[planID] = cancel
	s.workers.Go(func() {
		defer cancel()
		s.runInstall(workCtx, receipt, plan)
	})
	return s.attemptStatusLocked(receipt), nil
}

func (s *Service) replayConfirmation(planID, token string) (Attempt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replayConfirmationLocked(planID, token)
}

func (s *Service) replayConfirmationLocked(planID, token string) (Attempt, bool, error) {
	if err := s.loadReceipts(); err != nil {
		return Attempt{}, false, err
	}
	receipt, exists := s.receipts[planID]
	if !exists {
		return Attempt{}, false, nil
	}
	hash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(receipt.ApprovalHash[:], hash[:]) != 1 {
		return Attempt{}, true, ErrPlanApproval
	}
	return s.attemptStatusLocked(receipt), true, nil
}

// GetAttempt returns the latest redacted status of a confirmed install.
func (s *Service) GetAttempt(id string) (Attempt, error) {
	if !opaqueID.MatchString(id) {
		return Attempt{}, ErrAttemptNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadReceipts(); err != nil {
		return Attempt{}, err
	}
	receipt, found := s.receipts[s.attemptIDs[id]]
	if !found {
		return Attempt{}, ErrAttemptNotFound
	}
	return s.attemptStatusLocked(receipt), nil
}

func cloneAttempt(attempt Attempt) Attempt {
	attempt.Configuration.Ports = slices.Clone(attempt.Configuration.Ports)
	attempt.Configuration.Mounts = slices.Clone(attempt.Configuration.Mounts)
	attempt.Configuration.Environment = slices.Clone(attempt.Configuration.Environment)
	attempt.CleanupFailures = slices.Clone(attempt.CleanupFailures)
	attempt.CleanupDetails = slices.Clone(attempt.CleanupDetails)
	if attempt.Problem != nil {
		problem := *attempt.Problem
		attempt.Problem = &problem
	}
	return attempt
}

// Close cancels the worker lifecycle and waits for accepted work to settle.
func (s *Service) Close() {
	s.stopWork()
	s.workers.Wait()
}

func (s *Service) stopWork() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.recoveryStop != nil {
		s.recoveryStop()
	}
	for _, cancel := range s.active {
		cancel()
	}
	for id := range s.plans {
		s.dropPendingLocked(id)
	}
}
