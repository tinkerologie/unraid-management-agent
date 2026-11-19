package communityapps

import (
	"context"
	"strings"
	"time"
)

var (
	ErrCreateUnverified    = &Problem{Code: "container_create_unverified", Message: "Docker has not confirmed whether it created this container. Check Docker Manager and keep polling this attempt. Its name stays reserved until the outcome can be verified."}
	ErrStartUnverified     = &Problem{Code: "container_start_unverified", Message: "Docker has not confirmed whether the installed container started. Check Docker Manager and keep polling this attempt. Its name stays reserved while the outcome is unresolved."}
	ErrInstallInterrupted  = &Problem{Code: "install_interrupted", Message: "The install was interrupted. Prepare and approve a new plan after reviewing any remaining resources."}
	ErrInstallRecovery     = &Problem{Code: "install_recovery_pending", Message: "An install for this name is still in progress or awaiting verification. Check that Docker and the Unraid flash are available, then poll the original attempt."}
	ErrInstallReceiptProof = &Problem{Code: "install_receipt_unverified", Message: "The interrupted attempt lacks valid ownership evidence. Check Docker and the Unraid user templates for this install before preparing a new plan."}
)

// StartRecovery starts the daemon-owned recovery worker. Readers and
// standalone service instances can access receipts without applying recovery.
func (s *Service) StartRecovery(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recoveryWake != nil || s.closed || ctx.Err() != nil {
		return
	}
	recoveryCtx, cancel := context.WithCancel(ctx)
	s.recoveryStop = cancel
	context.AfterFunc(recoveryCtx, s.stopWork)
	s.recoveryWake = make(chan struct{}, 1)
	s.workers.Go(func() { s.recoverInstalls(recoveryCtx) })
}

func (s *Service) wakeRecoveryLocked() {
	select {
	case s.recoveryWake <- struct{}{}:
	default:
	}
}

func (s *Service) recoverInstalls(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		pending, err := s.interruptedInstalls()
		retry := err != nil
		for _, receipt := range pending {
			if !s.reconcileInstall(ctx, receipt) {
				retry = true
			}
		}
		var next <-chan time.Time
		var timer *time.Timer
		if retry {
			timer = time.NewTimer(delay)
			next = timer.C
		}
		select {
		case <-ctx.Done():
		case <-s.recoveryWake:
			delay = time.Second
		case <-next:
			delay = min(2*delay, 30*time.Second)
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (s *Service) interruptedInstalls() ([]installReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadReceipts(); err != nil {
		return nil, err
	}
	var pending []installReceipt
	for id, receipt := range s.receipts {
		if isTerminal(receipt.Attempt.State) || s.active[id] != nil {
			continue
		}
		if result, ready := s.pendingCommit[id]; ready {
			receipt = result
		}
		pending = append(pending, receipt)
	}
	if len(s.unreadableClaims) != 0 {
		// Retry unreadable claims without delaying independently verified ones.
		return pending, ErrAttemptStorage
	}
	return pending, nil
}

func isTerminal(state AttemptState) bool {
	return state == AttemptSucceeded || state == AttemptFailed
}

func (s *Service) installNameReserved(name string) bool {
	for _, receipt := range s.receipts {
		if !isTerminal(receipt.Attempt.State) && strings.EqualFold(receipt.Attempt.Configuration.Name, name) {
			return true
		}
	}
	return false
}

func (s *Service) attemptStatusLocked(receipt installReceipt) Attempt {
	result := cloneAttempt(receipt.Attempt)
	if s.active[receipt.PlanID] == nil && !isTerminal(result.State) {
		if uncertainCreate(receipt) {
			result.Problem = ProblemForError(ErrCreateUnverified)
		} else if uncertainStart(receipt) && !isTerminal(s.pendingCommit[receipt.PlanID].Attempt.State) {
			result.Problem = ProblemForError(ErrStartUnverified)
		} else if s.recoveryWake != nil && result.Problem == nil {
			result.Problem = ProblemForError(ErrInstallRecovery)
		}
	}
	return result
}

// reconcileInstall never replays Create or Start. Its return value indicates
// whether the attempt is settled and has no work left to retry.
func (s *Service) reconcileInstall(ctx context.Context, receipt installReceipt) bool {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return false
	}
	if isTerminal(receipt.Attempt.State) {
		return s.persistProgress(receipt) == nil
	}
	if !validRecoveryReceipt(receipt) {
		return s.finishInstall(&receipt, ErrInstallReceiptProof)
	}
	if !installMutationMayExist(receipt.Attempt.Phase) {
		return s.finishInstall(&receipt, ErrInstallInterrupted)
	}
	docker, err := s.openInstaller()
	if err != nil {
		return false
	}
	defer func() { _ = docker.Close() }()

	var cause error = ErrInstallInterrupted
	if receipt.Attempt.Problem != nil {
		cause = receipt.Attempt.Problem
	}
	return s.settleInstall(ctx, docker, &receipt, cause)
}

func (s *Service) settleInstall(ctx context.Context, docker InstallDocker, receipt *installReceipt, cause error) bool {
	if mutationPhase(*receipt) == "starting_container" {
		observed, err := s.observeInstallStart(ctx, docker, *receipt)
		if err == errContainerIdentity {
			return s.finishInstall(receipt, err)
		}
		if err != nil {
			return false
		}
		switch observed {
		case startVerified, startTemplateMissing:
			return s.finishObservedStart(receipt, observed)
		case startContainerMissing:
			if !receipt.EngineRejected {
				// The native template was published before start intent. It
				// may be retained from an ordinary removal or replacement.
				return s.finishInstall(receipt, ErrInstallInterrupted)
			}
		}
		// A short-lived app can start and exit after Inspect reports a
		// zero start time. Even non-force removal would then delete it.
		if uncertainStart(*receipt) {
			if !s.oldEngineCallsDrained(receipt) {
				return false
			}
			// A new boot drains the request, but Docker's last start
			// timestamp might not have survived it. Preserve the result.
			receipt.Attempt.ContainerID = receipt.OwnedContainerID
			return s.finishInstall(receipt, ErrInstallInterrupted)
		}
	}
	if uncertainCreate(*receipt) && cause != ErrNameConflict {
		created, found, err := ownedContainer(ctx, docker, *receipt)
		switch {
		case found:
			next := *receipt
			next.OwnedContainerID = created.ID
			if s.oldEngineCallsDrained(&next) {
				next.Attempt.ContainerID = created.ID
				return s.finishInstall(&next, ErrInstallInterrupted)
			}
			if s.persistProgress(next) != nil {
				return false
			}
			*receipt = next
		case err != errContainerIdentity:
			if err == nil && s.oldEngineCallsDrained(receipt) {
				return s.finishInstall(receipt, ErrInstallInterrupted)
			}
			if receipt.Attempt.Problem == nil && cause != ErrInstallInterrupted {
				receipt.Attempt.Problem = ProblemForError(cause)
				_ = s.persistProgress(*receipt)
			}
			return false
		}
	}
	if err := s.beginInstallCleanup(receipt, cause); err != nil {
		return false
	}
	ready, observed := s.cleanupInstall(ctx, docker, receipt)
	if !ready {
		return false
	}
	if observed != startUnobserved {
		return s.finishObservedStart(receipt, observed)
	}
	return s.finishInstall(receipt, cause)
}

func uncertainCreate(receipt installReceipt) bool {
	return receipt.OwnedContainerID == "" && mutationPhase(receipt) == "creating_container" &&
		!receipt.EngineRejected &&
		(receipt.Attempt.Problem == nil || receipt.Attempt.Problem.Code != ErrNameConflict.Code)
}

func uncertainStart(receipt installReceipt) bool {
	return mutationPhase(receipt) == "starting_container" && !receipt.EngineRejected
}

func mutationPhase(receipt installReceipt) string {
	if receipt.Attempt.Phase != "cleaning_up" {
		return receipt.Attempt.Phase
	}
	if receipt.CleanupPhase != "" || receipt.Version != 1 {
		return receipt.CleanupPhase
	}
	// Version 1 cleanup did not persist the preceding phase. Treat a
	// possible start or unacknowledged create as unresolved.
	if receipt.OwnedContainerID != "" && hasTemplateProof(receipt) {
		return "starting_container"
	}
	return "creating_container"
}

func validRecoveryReceipt(r installReceipt) bool {
	phase := r.Attempt.Phase
	if phase == "queued" {
		return r.Attempt.State == AttemptPending
	}
	if r.Attempt.State != AttemptRunning || (r.OwnedContainerID != "" && !fullID.MatchString(r.OwnedContainerID)) {
		return false
	}
	switch phase {
	case "validating", "pulling_image", "preparing_storage":
		return true
	case "creating_container":
		return validImageID(r.ImageID) && r.Attempt.Configuration.Image != ""
	case "cleaning_up":
		switch r.CleanupPhase {
		case "":
			return r.Version == 1 && validImageID(r.ImageID) && r.Attempt.Configuration.Image != ""
		case "creating_container", "publishing_template", "starting_container":
			r.Attempt.Phase, r.CleanupPhase = r.CleanupPhase, ""
			return validRecoveryReceipt(r)
		default:
			return false
		}
	case "publishing_template", "starting_container":
		return r.OwnedContainerID != "" && validImageID(r.ImageID) && r.Attempt.Configuration.Image != "" &&
			hasTemplateProof(r)
	default:
		return false
	}
}

func hasTemplateProof(receipt installReceipt) bool {
	return receipt.TemplateKeyHash != [32]byte{} && receipt.TemplateMAC != [32]byte{}
}
