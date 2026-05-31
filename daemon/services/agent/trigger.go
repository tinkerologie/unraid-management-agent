package agent

import (
	"context"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// sweepInterval is how often Start checks for expired approvals.
const sweepInterval = 30 * time.Second

// Start subscribes to agent_wake events and runs until ctx is cancelled.
// No-op when the agent is disabled or no event bus is wired.
func (s *Service) Start(ctx context.Context) {
	if !s.Enabled() || s.hub == nil {
		logger.Info("Agent: autonomous triggers not started (disabled or no event bus)")
		return
	}
	ch := s.hub.SubTopics(constants.TopicAgentWake)
	defer s.hub.Unsub(ch, constants.TopicAgentWake.Name)

	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	logger.Success("Agent: autonomous trigger listening on %q", constants.TopicAgentWake.Name)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Agent: autonomous trigger stopped")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Agent sweeper", r)
					}
				}()
				s.SweepExpiredApprovals(ctx, time.Now())
			}()
		case msg := <-ch:
			ev, ok := msg.(dto.AgentWakeEvent)
			if !ok {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Agent wake handler", r)
					}
				}()
				s.handleWake(ctx, ev)
			}()
		}
	}
}

// handleWake applies dedup/debounce/cooldown/concurrency policy and, if admitted,
// runs an autonomous session synchronously. Returns true if a session was spawned.
func (s *Service) handleWake(ctx context.Context, ev dto.AgentWakeEvent) bool {
	now := time.Now()
	debounce := time.Duration(s.cfg.WakeDebounceSecs) * time.Second
	cooldown := time.Duration(s.cfg.WakeCooldownSecs) * time.Second

	s.wakeMu.Lock()
	last, seen := s.lastWake[ev.Subsystem]
	if seen && now.Sub(last) < debounce {
		s.wakeMu.Unlock()
		logger.Debug("Agent: wake for %q debounced", ev.Subsystem)
		return false
	}
	if seen && now.Sub(last) < cooldown {
		s.wakeMu.Unlock()
		logger.Debug("Agent: wake for %q in cooldown", ev.Subsystem)
		return false
	}
	if s.activeAuto >= s.cfg.MaxConcurrentSessions {
		s.wakeMu.Unlock()
		logger.Warning("Agent: wake for %q dropped — %d autonomous sessions running (cap=%d)",
			ev.Subsystem, s.activeAuto, s.cfg.MaxConcurrentSessions)
		return false
	}
	s.lastWake[ev.Subsystem] = now
	s.activeAuto++
	s.wakeMu.Unlock()

	logger.Info("Agent: waking on %s incident (%s)", ev.Subsystem, ev.Title)
	s.startAutonomousSession(ctx, ev)
	return true
}
