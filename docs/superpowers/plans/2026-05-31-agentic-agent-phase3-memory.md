# Agentic Agent — Phase 3 (Planning & Memory) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the agent memory and foresight: it recalls relevant past incidents to inform new sessions, decomposes goals into a visible plan, learns operator preferences and runbooks (suggest-not-mutate), and supports multi-turn operator chat.

**Architecture:** Builds on Phase 1+2 (merged to `main`). Adds a `daemon/services/agent/memory` package (JSON-persisted episodic incidents + semantic preferences). The Service gains recall-on-start and episodic-write-on-finish hooks around its existing `runLoop` entrypoints, a lightweight planner that decomposes a goal into plan steps stored on the session, suggest-not-mutate learning tools (proposals stay pending until the operator confirms; confirmed `auto_approve_tool` preferences let the policy gate raise autonomy), runbook proposals persisted via a new store in the `remediation` package (reusing its `Runbook` type), and multi-turn `SendMessage`. New REST + MCP surfaces expose memory/preferences/chat.

**Tech Stack:** Go 1.26, existing JSON-store pattern (`agent.Store`/`watchdog.Store`), `gorilla/mux`, official `modelcontextprotocol/go-sdk`. No new third-party deps.

**Reference spec:** `docs/superpowers/specs/2026-05-31-agentic-agent-design.md` (Phase 3 = "Planning & memory")
**Builds on:** Phase 1 (`docs/.../phase1-foundation.md`) + Phase 2 (`docs/.../phase2-autonomy.md`), both merged.

**Phase-3 scope:** episodic + semantic memory store; recall injection at session start; episodic write on finish; lightweight planner (goal → plan steps on the session); suggest-not-mutate learning (preferences + runbook proposals, pending until confirmed; confirmed `auto_approve_tool` raises the gate); multi-turn `/messages` chat; REST + MCP for memory/preferences/chat. **Out of scope (future):** vector/embeddings recall (keep keyword/tag), native-notification approvals, GUI page.

**Key existing facts (verified against `main`):**

- Agent Service (`daemon/services/agent/service.go`): fields `cfg,provider,tools,store,bc,mu,seq,hub,wakeMu,lastWake,activeAuto`. Three `runLoop(ctx,*dto.AgentSession)` entrypoints — `StartSession`, `startAutonomousSession`, `ApproveAction` — each seeds/extends `sess.Transcript`, runs `runLoop`, then `s.store.Put(sess)` + `s.store.Save()`. `nextID()` → `sess-N`. `Enabled()`, `GetSession`, `ListSessions`, `CancelSession`, `SweepExpiredApprovals`, `isForbidden`, `invokeTool`, `emit`, `finish`, `fail`.
- `loop.go`: `systemPrompt` const, `runLoop`, `transcriptToMessages`, `m2dto`, `appendTranscript`. The pause check is `mode := s.cfg.Autonomy[tier]; if mode != dto.ModeAuto { ... pause ... }`.
- DTOs (`daemon/dto/agent.go`): `AgentSession{ID,Goal,Status,Steps,Answer,Error,TokensUsed,StartedAt,EndedAt,PendingApproval,Transcript}`, `AgentStep{Index,Thought,ToolCalls,At}`, `AgentToolCall{Name,Args,RiskTier,Result,Error,At}`, `RiskTier`, `AutonomyMode`(`ModeAuto/Approve/Forbid`), statuses incl. `SessionCompleted/Failed/Cancelled/AwaitingApproval`, `AgentConfig{...,Autonomy,MaxIterations,MaxTokensPerSession,SessionDeadlineSecs,WakeDebounceSecs,WakeCooldownSecs,MaxConcurrentSessions,ApprovalTTLSecs,ForbidList}` + `DefaultAgentConfig()`.
- `agent.Store` (`store.go`): `NewStore(dir)`, `Put/Get/List/Save/Load`, `DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"`, JSON via `os.WriteFile` 0o600 with `// #nosec` annotations.
- `bootstrap.go`: `BuildService(cfg, configDir, state tools.StateProvider, docker tools.DockerActor, bc Broadcaster) (*Service,error)` builds provider, `store := NewStore(configDir)`, `store.Load()`, `reg := tools.BuildDefault(state,docker)`, `NewService(...)`.
- `tools.Registry`: `NewRegistry()`, `Register(Tool)`, `Get`, `Schemas()`; `Tool{Name,Description,Schema []byte,RiskTier,Invoke func(ctx,argsJSON)(string,error)}`; `BuildDefault(state,docker)`.
- API: `handlers_agent.go` (handlers + `SystemJSON/ArrayJSON/DockerJSON`), `server.go` routes `/agent/sessions[...]`, helpers `respondJSON`/`respondWithError`, `mux.Vars`. `BroadcastAgentEvent`.
- MCP: `server.go` `registerAgentTools()`, `SetAgent`, `agentSvc`, `textResult`/`jsonResult`/`ptr`, `dto.MCPEmptyArgs`, arg structs use double tags `json:"..." jsonschema:"..."`.
- `remediation` package: `Runbook{Name,Description,Steps []RunbookStep}`, `RunbookStep{Action,Target,Reason}`, `Runbooks() []Runbook` (static). No persistent store yet.

---

## File Structure

| File                                           | Responsibility                                                                                                                   | Action |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `daemon/dto/agent.go`                          | `PlanStep`, `AgentIncident`, `AgentPreference`+status, session `Plan`, config knobs                                              | Modify |
| `daemon/services/agent/memory/store.go`        | JSON-persisted episodic incidents + semantic preferences + recall                                                                | Create |
| `daemon/services/agent/recall.go`              | signature derivation + recall-context formatting; episodic-write distillation                                                    | Create |
| `daemon/services/agent/planner.go`             | goal → `[]PlanStep` via one LLM call                                                                                             | Create |
| `daemon/services/agent/service.go`             | wire memory: recall-on-start, finalize-on-finish, plan-on-start; `SendMessage`; preference confirm; policy consults active prefs | Modify |
| `daemon/services/agent/loop.go`                | policy gate consults active `auto_approve_tool` preferences                                                                      | Modify |
| `daemon/services/agent/learning_tools.go`      | `propose_preference` / `propose_runbook` agent tools (write pending)                                                             | Create |
| `daemon/services/agent/bootstrap.go`           | construct memory store + runbook store; register learning tools                                                                  | Modify |
| `daemon/services/remediation/runbook_store.go` | persistent proposed/confirmed runbooks (reuses `Runbook`)                                                                        | Create |
| `daemon/services/api/handlers_agent.go`        | `/messages`, `/memory`, `/preferences/{id}/confirm`                                                                              | Modify |
| `daemon/services/api/server.go`                | routes                                                                                                                           | Modify |
| `daemon/services/mcp/server.go`                | `agent_send_message`, `agent_get_memory`, `agent_confirm_preference` tools                                                       | Modify |
| `daemon/services/orchestrator.go`              | construct memory/runbook stores, pass into BuildService                                                                          | Modify |

---

## Task 1: Phase-3 DTOs + config knobs

**Goal:** Add the plan/memory/preference data types and config knobs with safe defaults.

**Files:**

- Modify: `daemon/dto/agent.go`
- Test: `daemon/dto/agent_test.go`

**Acceptance Criteria:**

- [ ] `dto.PlanStep{Intent string, Tool string, Done bool}` exists; `dto.AgentSession` gains `Plan []PlanStep` (`json:"plan,omitempty"`).
- [ ] `dto.AgentIncident{ID,Signature,Goal,Outcome,Summary string, Actions []string, At time.Time}` exists with JSON tags.
- [ ] `dto.PreferenceStatus` (`PreferencePending`="pending", `PreferenceActive`="active") and `dto.AgentPreference{ID,Kind,Subject,Note string, Status PreferenceStatus, At time.Time}` exist.
- [ ] `dto.AgentConfig` gains `MemoryEnabled bool`, `MaxIncidents int`, `RecallTopK int`; `DefaultAgentConfig()` sets `MemoryEnabled:true`, `MaxIncidents:200`, `RecallTopK:3`.

**Verify:** `go test ./daemon/dto/ -run TestAgent -v && go build ./daemon/dto/` → PASS

**Steps:**

- [ ] **Step 1: Add test** to `daemon/dto/agent_test.go`:

```go
func TestDefaultAgentConfigPhase3Defaults(t *testing.T) {
 cfg := DefaultAgentConfig()
 if !cfg.MemoryEnabled {
  t.Error("memory should be enabled by default")
 }
 if cfg.MaxIncidents <= 0 || cfg.RecallTopK <= 0 {
  t.Error("MaxIncidents and RecallTopK must be positive")
 }
}

func TestAgentIncidentAndPreferenceRoundTrip(t *testing.T) {
 inc := AgentIncident{ID: "inc-1", Signature: "watchdog:Plex HTTP", Goal: "fix plex", Outcome: "completed", Summary: "restarted plex", Actions: []string{"restart_container"}}
 pref := AgentPreference{ID: "pref-1", Kind: "auto_approve_tool", Subject: "restart_container", Status: PreferencePending}
 for _, v := range []any{inc, pref} {
  b, err := json.Marshal(v)
  if err != nil {
   t.Fatalf("marshal: %v", err)
  }
  if len(b) == 0 {
   t.Fatal("empty marshal")
  }
 }
 sess := AgentSession{ID: "s1", Plan: []PlanStep{{Intent: "check disk", Tool: "get_system_info"}}}
 b, _ := json.Marshal(sess)
 var back AgentSession
 if err := json.Unmarshal(b, &back); err != nil || len(back.Plan) != 1 || back.Plan[0].Intent != "check disk" {
  t.Fatalf("plan round-trip failed: %+v err=%v", back.Plan, err)
 }
}
```

- [ ] **Step 2: Run** `go test ./daemon/dto/ -run TestAgent -v` → FAIL.

- [ ] **Step 3: Edit `daemon/dto/agent.go`.** Add types:

```go
// PlanStep is one step of a goal decomposition produced by the planner.
type PlanStep struct {
 Intent string `json:"intent"`
 Tool   string `json:"tool,omitempty"`
 Done   bool   `json:"done"`
}

// AgentIncident is an episodic memory record of a finished session.
type AgentIncident struct {
 ID        string    `json:"id"`
 Signature string    `json:"signature"` // recall key, e.g. "watchdog:Plex HTTP"
 Goal      string    `json:"goal"`
 Outcome   string    `json:"outcome"` // terminal session status
 Summary   string    `json:"summary"` // distilled answer/result
 Actions   []string  `json:"actions,omitempty"`
 At        time.Time `json:"at"`
}

// PreferenceStatus is the lifecycle of a learned preference.
type PreferenceStatus string

const (
 PreferencePending PreferenceStatus = "pending"
 PreferenceActive  PreferenceStatus = "active"
)

// AgentPreference is a learned, suggest-not-mutate operator preference.
// Kind "auto_approve_tool" + Subject=<tool name> lets the policy gate auto-approve
// that tool once the preference is Active (operator-confirmed).
type AgentPreference struct {
 ID      string           `json:"id"`
 Kind    string           `json:"kind"`
 Subject string           `json:"subject"`
 Note    string           `json:"note,omitempty"`
 Status  PreferenceStatus `json:"status"`
 At      time.Time        `json:"at"`
}
```

Add to `AgentSession` (after `Transcript`): `Plan []PlanStep \`json:"plan,omitempty"\``.

Add to `AgentConfig` (after `ForbidList`):

```go
 MemoryEnabled bool `json:"memory_enabled"`
 MaxIncidents  int  `json:"max_incidents"`
 RecallTopK    int  `json:"recall_top_k"`
```

In `DefaultAgentConfig()` literal add: `MemoryEnabled: true, MaxIncidents: 200, RecallTopK: 3,`.

- [ ] **Step 4: Run** `go test ./daemon/dto/ -run TestAgent -v` (PASS), `go build ./...`, gofmt/vet clean.

- [ ] **Step 5: Commit**

```bash
git add daemon/dto/agent.go daemon/dto/agent_test.go
git commit -m "feat(agent): phase-3 DTOs (plan steps, incidents, preferences) + memory config"
```

---

## Task 2: Memory store (episodic + semantic + recall)

**Goal:** A JSON-persisted store for incidents and preferences with a simple keyword/tag recall.

**Files:**

- Create: `daemon/services/agent/memory/store.go`
- Test: `daemon/services/agent/memory/store_test.go`

**Acceptance Criteria:**

- [ ] `NewStore(dir string, maxIncidents int) *Store`; empty dir uses `DefaultConfigDir`. Persists to `agent_memory.json`.
- [ ] `AddIncident(dto.AgentIncident)` (bounds to maxIncidents, newest kept), `ListIncidents() []dto.AgentIncident` (newest-first), `Recall(signature string, k int) []dto.AgentIncident` (token-overlap scored, top-k, only positive scores).
- [ ] `AddPreference(dto.AgentPreference)`, `ListPreferences()`, `ConfirmPreference(id string) error` (pending→active; error if not found), `ActivePreferences() []dto.AgentPreference`.
- [ ] `Save()`/`Load()` round-trip incidents+preferences; concurrency-safe (`sync.RWMutex`).

**Verify:** `go test ./daemon/services/agent/memory/ -v` → PASS

**Steps:**

- [ ] **Step 1: Test** `daemon/services/agent/memory/store_test.go`:

```go
package memory

import (
 "testing"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestMemoryRoundTripAndRecall(t *testing.T) {
 dir := t.TempDir()
 s := NewStore(dir, 100)
 s.AddIncident(dto.AgentIncident{ID: "i1", Signature: "watchdog:Plex HTTP", Summary: "restarted plex", At: time.Now()})
 s.AddIncident(dto.AgentIncident{ID: "i2", Signature: "alert:High CPU", Summary: "killed runaway", At: time.Now().Add(time.Second)})
 if err := s.Save(); err != nil {
  t.Fatalf("save: %v", err)
 }
 s2 := NewStore(dir, 100)
 if err := s2.Load(); err != nil {
  t.Fatalf("load: %v", err)
 }
 if len(s2.ListIncidents()) != 2 || s2.ListIncidents()[0].ID != "i2" {
  t.Fatalf("incidents reload/order wrong: %+v", s2.ListIncidents())
 }
 hits := s2.Recall("watchdog:Plex HTTP timeout", 3)
 if len(hits) == 0 || hits[0].ID != "i1" {
  t.Fatalf("recall should surface the Plex incident first: %+v", hits)
 }
 // Unrelated query → no false positives.
 if got := s2.Recall("zfs:pool degraded", 3); len(got) != 0 {
  t.Fatalf("expected no recall for unrelated signature, got %+v", got)
 }
}

func TestMemoryMaxIncidents(t *testing.T) {
 s := NewStore(t.TempDir(), 3)
 for i := 0; i < 10; i++ {
  s.AddIncident(dto.AgentIncident{ID: string(rune('a' + i)), Signature: "x", At: time.Now().Add(time.Duration(i) * time.Second)})
 }
 if len(s.ListIncidents()) != 3 {
  t.Fatalf("expected bounded to 3, got %d", len(s.ListIncidents()))
 }
}

func TestPreferenceConfirm(t *testing.T) {
 s := NewStore(t.TempDir(), 100)
 s.AddPreference(dto.AgentPreference{ID: "p1", Kind: "auto_approve_tool", Subject: "restart_container", Status: dto.PreferencePending})
 if len(s.ActivePreferences()) != 0 {
  t.Fatal("pending preference must not be active")
 }
 if err := s.ConfirmPreference("p1"); err != nil {
  t.Fatalf("confirm: %v", err)
 }
 if len(s.ActivePreferences()) != 1 {
  t.Fatal("confirmed preference should be active")
 }
 if err := s.ConfirmPreference("missing"); err == nil {
  t.Fatal("expected error confirming unknown preference")
 }
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement `daemon/services/agent/memory/store.go`:**

```go
// Package memory provides the agent's episodic (incident) and semantic
// (preference) memory with a simple keyword/tag recall.
package memory

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// DefaultConfigDir matches the other agent/watchdog stores.
const DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"

// MemoryFile is the on-disk filename.
const MemoryFile = "agent_memory.json"

type persisted struct {
 Incidents   []dto.AgentIncident  `json:"incidents"`
 Preferences []dto.AgentPreference `json:"preferences"`
}

// Store holds episodic incidents and semantic preferences, persisted as JSON.
type Store struct {
 mu           sync.RWMutex
 incidents    []dto.AgentIncident
 preferences  []dto.AgentPreference
 maxIncidents int
 filePath     string
}

// NewStore creates a memory store. Empty dir uses DefaultConfigDir.
func NewStore(configDir string, maxIncidents int) *Store {
 if configDir == "" {
  configDir = DefaultConfigDir
 }
 if maxIncidents <= 0 {
  maxIncidents = 200
 }
 return &Store{maxIncidents: maxIncidents, filePath: filepath.Join(configDir, MemoryFile)}
}

// AddIncident records an incident, keeping only the newest maxIncidents.
func (s *Store) AddIncident(inc dto.AgentIncident) {
 s.mu.Lock()
 defer s.mu.Unlock()
 s.incidents = append(s.incidents, inc)
 sort.Slice(s.incidents, func(i, j int) bool { return s.incidents[i].At.After(s.incidents[j].At) })
 if len(s.incidents) > s.maxIncidents {
  s.incidents = s.incidents[:s.maxIncidents]
 }
}

// ListIncidents returns incidents newest-first.
func (s *Store) ListIncidents() []dto.AgentIncident {
 s.mu.RLock()
 defer s.mu.RUnlock()
 out := make([]dto.AgentIncident, len(s.incidents))
 copy(out, s.incidents)
 return out
}

// tokenize lowercases and splits on non-alphanumeric runs.
func tokenize(s string) map[string]bool {
 set := map[string]bool{}
 for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
  return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
 }) {
  if len(f) > 1 {
   set[f] = true
  }
 }
 return set
}

// Recall returns up to k incidents whose signature shares tokens with query,
// scored by token overlap (positive scores only), highest first.
func (s *Store) Recall(query string, k int) []dto.AgentIncident {
 s.mu.RLock()
 defer s.mu.RUnlock()
 q := tokenize(query)
 type scored struct {
  inc   dto.AgentIncident
  score int
 }
 var hits []scored
 for _, inc := range s.incidents {
  sigTokens := tokenize(inc.Signature)
  score := 0
  for t := range sigTokens {
   if q[t] {
    score++
   }
  }
  if score > 0 {
   hits = append(hits, scored{inc, score})
  }
 }
 sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
 out := make([]dto.AgentIncident, 0, k)
 for i := 0; i < len(hits) && i < k; i++ {
  out = append(out, hits[i].inc)
 }
 return out
}

// AddPreference appends a preference.
func (s *Store) AddPreference(p dto.AgentPreference) {
 s.mu.Lock()
 defer s.mu.Unlock()
 s.preferences = append(s.preferences, p)
}

// ListPreferences returns all preferences.
func (s *Store) ListPreferences() []dto.AgentPreference {
 s.mu.RLock()
 defer s.mu.RUnlock()
 out := make([]dto.AgentPreference, len(s.preferences))
 copy(out, s.preferences)
 return out
}

// ConfirmPreference flips a pending preference to active.
func (s *Store) ConfirmPreference(id string) error {
 s.mu.Lock()
 defer s.mu.Unlock()
 for i := range s.preferences {
  if s.preferences[i].ID == id {
   s.preferences[i].Status = dto.PreferenceActive
   return nil
  }
 }
 return fmt.Errorf("preference %q not found", id)
}

// ActivePreferences returns only active preferences.
func (s *Store) ActivePreferences() []dto.AgentPreference {
 s.mu.RLock()
 defer s.mu.RUnlock()
 var out []dto.AgentPreference
 for _, p := range s.preferences {
  if p.Status == dto.PreferenceActive {
   out = append(out, p)
  }
 }
 return out
}

// Save writes the store to disk.
func (s *Store) Save() error {
 s.mu.RLock()
 data := persisted{Incidents: s.incidents, Preferences: s.preferences}
 s.mu.RUnlock()
 if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
  return fmt.Errorf("creating agent config dir: %w", err)
 }
 b, err := json.MarshalIndent(data, "", "  ")
 if err != nil {
  return fmt.Errorf("marshal memory: %w", err)
 }
 if err := os.WriteFile(s.filePath, b, 0o600); err != nil {
  return fmt.Errorf("writing memory: %w", err)
 }
 return nil
}

// Load reads the store from disk; a missing file is not an error.
func (s *Store) Load() error {
 b, err := os.ReadFile(s.filePath)
 if err != nil {
  if os.IsNotExist(err) {
   logger.Info("Agent: no memory file yet, starting empty")
   return nil
  }
  return fmt.Errorf("reading memory: %w", err)
 }
 var data persisted
 if err := json.Unmarshal(b, &data); err != nil {
  return fmt.Errorf("unmarshal memory: %w", err)
 }
 s.mu.Lock()
 defer s.mu.Unlock()
 s.incidents = data.Incidents
 s.preferences = data.Preferences
 return nil
}
```

- [ ] **Step 4: Run** `go test ./daemon/services/agent/memory/ -v` (PASS), gofmt/vet/golangci clean.

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/memory/
git commit -m "feat(agent): episodic+semantic memory store with keyword recall"
```

---

## Task 3: Recall injection + episodic write (wire memory into Service)

**Goal:** Inject recalled context at session start and write an incident when a session finishes — without duplicating logic across the three entrypoints.

**Files:**

- Create: `daemon/services/agent/recall.go`
- Modify: `daemon/services/agent/service.go` (add `memory *memory.Store`; helpers; call from entrypoints)
- Test: `daemon/services/agent/recall_test.go`

**Acceptance Criteria:**

- [ ] `Service` gains a `memory *memory.Store` field; `NewService` accepts it (update signature) — see note on call sites.
- [ ] `signatureFor(goal string) string` derives a recall key; `recallContext(sig string) string` returns a human-readable "relevant past incidents" block (empty if none).
- [ ] At session start (StartSession + startAutonomousSession), when `cfg.MemoryEnabled` and recall is non-empty, a `system`-role transcript message with the recall block is appended after the user goal.
- [ ] When a session reaches a terminal state, `finalize(sess)` writes one `dto.AgentIncident` (signature, goal, outcome, summary=answer-or-error, actions=tool names) and persists memory. Called from all three entrypoints after `runLoop`.
- [ ] No incident is written for non-terminal (awaiting_approval) pauses.

**Verify:** `go test ./daemon/services/agent/ -run 'Recall|Finalize|Memory' -v` → PASS

**Steps:**

- [ ] **Step 1: Update `NewService` signature.** Add `mem *memory.Store` as the last param: `NewService(cfg, provider, reg, store, mem, bc)`. Update ALL call sites: `bootstrap.go` (Task adds mem), and every test that calls `NewService(...)` (search `NewService(` across the repo — loop_test.go, service_test.go, trigger_test.go, handlers_agent_test.go, mcp/server_test.go). For tests, pass `memory.NewStore(t.TempDir(), 0)` (import the memory package). Do this as the first edit so the package compiles, then proceed.

> Engineer note: updating the constructor signature touches several test files. Update them mechanically to pass a fresh `memory.NewStore(t.TempDir(), 0)`. This is expected churn, not scope creep.

- [ ] **Step 2: Add test** `daemon/services/agent/recall_test.go`:

```go
package agent

import (
 "context"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func TestFinalizeWritesIncident(t *testing.T) {
 p := llm.NewMockProvider(&llm.ChatResponse{Text: "All healthy.", OutputTokens: 2})
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 mem := memory.NewStore(t.TempDir(), 100)
 svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), mem, &capturingBroadcaster{})

 sess, _ := svc.StartSession(context.Background(), "is plex healthy?")
 if sess.Status != dto.SessionCompleted {
  t.Fatalf("precondition: %q", sess.Status)
 }
 incs := mem.ListIncidents()
 if len(incs) != 1 || incs[0].Goal != "is plex healthy?" || incs[0].Outcome != string(dto.SessionCompleted) {
  t.Fatalf("expected one completed incident, got %+v", incs)
 }
}

func TestRecallInjectedAtStart(t *testing.T) {
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 mem := memory.NewStore(t.TempDir(), 100)
 mem.AddIncident(dto.AgentIncident{ID: "i1", Signature: "plex healthy", Summary: "last time: restarted plex container"})
 // Provider that answers immediately; we inspect the request it received.
 p := llm.NewMockProvider(&llm.ChatResponse{Text: "ok", OutputTokens: 1})
 svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), mem, &capturingBroadcaster{})

 _, _ = svc.StartSession(context.Background(), "is plex healthy?")
 reqs := p.Requests()
 if len(reqs) == 0 {
  t.Fatal("no provider request")
 }
 found := false
 for _, m := range reqs[0].Messages {
  if m.Role == "system" && len(m.Content) > 0 {
   found = true
  }
 }
 if !found {
  t.Fatal("expected a recalled-context system message injected into the first request")
 }
}

func TestNoIncidentOnApprovalPause(t *testing.T) {
 p := llm.NewMockProvider(&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "t1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2})
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.NewRegistry()
 reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh, Invoke: func(_ context.Context, _ string) (string, error) { return "", nil }})
 mem := memory.NewStore(t.TempDir(), 100)
 svc := NewService(cfg, p, reg, NewStore(t.TempDir()), mem, &capturingBroadcaster{})
 sess, _ := svc.StartSession(context.Background(), "stop array")
 if sess.Status != dto.SessionAwaitingApproval {
  t.Fatalf("precondition: %q", sess.Status)
 }
 if len(mem.ListIncidents()) != 0 {
  t.Fatal("must not write an incident while awaiting approval")
 }
}
```

- [ ] **Step 3: Implement `daemon/services/agent/recall.go`:**

```go
package agent

import (
 "fmt"
 "strings"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

// signatureFor derives a coarse recall key from a goal/incident description.
// It keeps the leading "source:subsystem" prefix when present (autonomous goals
// embed it), else uses the whole goal text.
func signatureFor(goal string) string {
 g := strings.TrimSpace(goal)
 if i := strings.IndexAny(g, ".\n"); i > 0 {
  g = g[:i]
 }
 if len(g) > 120 {
  g = g[:120]
 }
 return g
}

// recallContext builds a system-message body summarizing relevant past incidents
// and active preferences. Returns "" when there is nothing useful to inject.
func (s *Service) recallContext(sig string) string {
 if s.memory == nil || !s.cfg.MemoryEnabled {
  return ""
 }
 var b strings.Builder
 if hits := s.memory.Recall(sig, s.cfg.RecallTopK); len(hits) > 0 {
  b.WriteString("Relevant past incidents (most similar first):\n")
  for _, h := range hits {
   fmt.Fprintf(&b, "- [%s] %s — %s\n", h.Outcome, h.Signature, h.Summary)
  }
 }
 if prefs := s.memory.ActivePreferences(); len(prefs) > 0 {
  b.WriteString("Operator preferences in effect:\n")
  for _, p := range prefs {
   fmt.Fprintf(&b, "- %s: %s (%s)\n", p.Kind, p.Subject, p.Note)
  }
 }
 return strings.TrimSpace(b.String())
}

// injectRecall appends a recalled-context system message to a fresh session, if any.
func (s *Service) injectRecall(sess *dto.AgentSession) {
 ctxText := s.recallContext(signatureFor(sess.Goal))
 if ctxText == "" {
  return
 }
 appendTranscript(sess, llm.Message{Role: "system", Content: ctxText})
}

// finalize writes an episodic incident for a terminal session and persists memory.
func (s *Service) finalize(sess *dto.AgentSession) {
 if s.memory == nil || !s.cfg.MemoryEnabled {
  return
 }
 switch sess.Status {
 case dto.SessionCompleted, dto.SessionFailed:
 default:
  return // not terminal (e.g. awaiting_approval) — nothing to record yet
 }
 summary := sess.Answer
 if summary == "" {
  summary = sess.Error
 }
 var actions []string
 for _, st := range sess.Steps {
  for _, tc := range st.ToolCalls {
   actions = append(actions, tc.Name)
  }
 }
 s.memory.AddIncident(dto.AgentIncident{
  ID:        "inc-" + sess.ID,
  Signature: signatureFor(sess.Goal),
  Goal:      sess.Goal,
  Outcome:   string(sess.Status),
  Summary:   summary,
  Actions:   actions,
  At:        sess.StartedAt,
 })
 if err := s.memory.Save(); err != nil {
  // best-effort; do not fail the session on a memory persistence error
  _ = err
 }
}
```

> Note: `finalize` uses `sess.StartedAt` for the timestamp (deterministic in tests; avoids `time.Now()` ordering flakiness). The memory `Save` error is intentionally swallowed (best-effort) — but log it via `logger.Warning` instead of `_ = err` to match codebase conventions; import `logger` and use `logger.Warning("Agent: memory save failed: %v", err)`.

- [ ] **Step 4: Wire into `service.go`.** Add field `memory *memory.Store` to `Service`; import the memory package. In `NewService`, store it. In `StartSession` and `startAutonomousSession`: after seeding the transcript with the user goal and BEFORE `s.emit(&sess,"session_started",...)`/`runLoop`, call `s.injectRecall(&sess)`. After `runLoop` (in StartSession, startAutonomousSession, AND ApproveAction), before/after `store.Put`, call `s.finalize(&sess)`.

Concretely, in each entrypoint replace the tail:

```go
 s.runLoop(ctx, &sess)
 s.finalize(&sess)
 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
 }
```

For `StartSession`/`startAutonomousSession`, add `s.injectRecall(&sess)` right after the `sess.Transcript = []dto.AgentMessage{{Role:"user",Content:goal}}` line.

- [ ] **Step 5: Run** `go test ./daemon/services/agent/... -run 'Recall|Finalize|Memory|Loop|Approve|Trigger|Sweep' -count=1` (all pass), full agent package, golangci clean, `go build ./...`.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/agent/recall.go daemon/services/agent/service.go daemon/services/agent/recall_test.go daemon/services/agent/loop_test.go daemon/services/agent/service_test.go daemon/services/agent/trigger_test.go
git commit -m "feat(agent): recall past incidents at start; write episodic memory on finish"
```

(Include whichever test files you had to update for the NewService signature.)

---

## Task 4: Planner (goal → plan steps)

**Goal:** Decompose a goal into a short ordered plan stored on the session for visibility (and to steer the loop), via one extra LLM call.

**Files:**

- Create: `daemon/services/agent/planner.go`
- Modify: `daemon/services/agent/service.go` (call planner at start when enabled)
- Test: `daemon/services/agent/planner_test.go`

**Acceptance Criteria:**

- [ ] `plan(ctx, goal string) []dto.PlanStep` asks the provider for a JSON array of steps and parses it; returns nil on any error/parse failure (planning is best-effort, never fails the session).
- [ ] When planning yields steps, they are stored on `sess.Plan` and a brief plan summary is appended to the transcript as a `system` message so the loop is plan-aware.
- [ ] A mock provider returning a JSON plan populates `sess.Plan`; a mock returning garbage leaves `sess.Plan` empty and the session still runs.

**Verify:** `go test ./daemon/services/agent/ -run Plan -v` → PASS

**Steps:**

- [ ] **Step 1: Test** `daemon/services/agent/planner_test.go`:

```go
package agent

import (
 "context"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func TestPlanParsesSteps(t *testing.T) {
 p := llm.NewMockProvider(
  &llm.ChatResponse{Text: `[{"intent":"check array","tool":"get_array_status"},{"intent":"answer"}]`, OutputTokens: 5},
  &llm.ChatResponse{Text: "done", OutputTokens: 1},
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
 sess, _ := svc.StartSession(context.Background(), "is the array ok?")
 if len(sess.Plan) != 2 || sess.Plan[0].Intent != "check array" {
  t.Fatalf("plan not populated: %+v", sess.Plan)
 }
}

func TestPlanGarbageIgnored(t *testing.T) {
 p := llm.NewMockProvider(
  &llm.ChatResponse{Text: "not json at all", OutputTokens: 2},
  &llm.ChatResponse{Text: "done", OutputTokens: 1},
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
 sess, _ := svc.StartSession(context.Background(), "anything")
 if len(sess.Plan) != 0 {
  t.Fatalf("garbage plan should be ignored, got %+v", sess.Plan)
 }
 if sess.Status != dto.SessionCompleted {
  t.Fatalf("session should still complete, got %q", sess.Status)
 }
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement `daemon/services/agent/planner.go`:**

````go
package agent

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

const plannerPrompt = `You are planning how to achieve an operator's goal on an Unraid server. ` +
 `Reply with ONLY a JSON array of 1-6 steps, each {"intent": "...", "tool": "<tool name or empty>"}. ` +
 `No prose, no code fences — just the JSON array.`

// plan asks the provider for a short ordered plan. Best-effort: returns nil on
// any error or parse failure (the session proceeds without a plan).
func (s *Service) plan(ctx context.Context, goal string) []dto.PlanStep {
 resp, err := s.provider.Chat(ctx, llm.ChatRequest{
  System:    plannerPrompt,
  Messages:  []llm.Message{{Role: "user", Content: goal}},
  MaxTokens: 512,
 })
 if err != nil || resp == nil {
  return nil
 }
 text := strings.TrimSpace(resp.Text)
 // Tolerate ```json fences.
 text = strings.TrimPrefix(text, "```json")
 text = strings.TrimPrefix(text, "```")
 text = strings.TrimSuffix(text, "```")
 text = strings.TrimSpace(text)
 var steps []dto.PlanStep
 if err := json.Unmarshal([]byte(text), &steps); err != nil {
  return nil
 }
 return steps
}

// planSummary renders a plan as a compact system message.
func planSummary(steps []dto.PlanStep) string {
 var b strings.Builder
 b.WriteString("Your plan for this goal:\n")
 for i, st := range steps {
  fmt.Fprintf(&b, "%d. %s", i+1, st.Intent)
  if st.Tool != "" {
   fmt.Fprintf(&b, " (tool: %s)", st.Tool)
  }
  b.WriteByte('\n')
 }
 return strings.TrimSpace(b.String())
}
````

- [ ] **Step 4: Wire into `StartSession`** (only StartSession — autonomous wakes already have a focused goal; keep planning to operator-initiated sessions to bound token use). After `injectRecall(&sess)` and before `runLoop`:

```go
 if steps := s.plan(ctx, goal); len(steps) > 0 {
  sess.Plan = steps
  appendTranscript(&sess, llm.Message{Role: "system", Content: planSummary(steps)})
 }
```

- [ ] **Step 5: Run** `go test ./daemon/services/agent/ -run Plan -v` (PASS) + full package (the extra planner Chat call consumes one mock response — UPDATE any StartSession-based tests that now need an extra leading mock response: the planner calls `provider.Chat` once before the loop. For tests that assert specific loop behavior, prepend a planner response `&llm.ChatResponse{Text:"[]"}` to their mock script OR set `cfg.MemoryEnabled` etc. — simplest: prepend an empty-plan response `&llm.ChatResponse{Text: "[]", OutputTokens: 1}` to the mock scripts of existing StartSession tests so the planner consumes it and returns no steps. Audit every `StartSession` test and adjust its mock script.)

> Engineer note: this is the trickiest integration point. The planner adds ONE provider call at the start of `StartSession`. Every existing test that drives `StartSession` with a scripted `MockProvider` must prepend one planner response. Use `&llm.ChatResponse{Text: "[]"}` (empty plan, ignored) as the first scripted response in those tests. `startAutonomousSession` and `ApproveAction` do NOT call the planner, so their tests are unaffected. Run the full package and fix each failing test by prepending the planner response.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/agent/planner.go daemon/services/agent/service.go daemon/services/agent/planner_test.go daemon/services/agent/loop_test.go daemon/services/agent/service_test.go daemon/services/agent/recall_test.go
git commit -m "feat(agent): goal-decomposition planner stored on the session"
```

---

## Task 5: Suggest-not-mutate learning + runbook proposals

**Goal:** Let the agent propose operator preferences and runbooks (stored pending, never auto-applied); confirmed `auto_approve_tool` preferences let the policy gate auto-approve that tool. Add a persistent runbook store in the `remediation` package.

**Files:**

- Create: `daemon/services/remediation/runbook_store.go`
- Create: `daemon/services/agent/learning_tools.go`
- Modify: `daemon/services/agent/service.go` (`ConfirmPreference`; `RegisterLearningTools`), `daemon/services/agent/loop.go` (policy consults active prefs)
- Test: `daemon/services/remediation/runbook_store_test.go`, `daemon/services/agent/learning_test.go`

**Acceptance Criteria:**

- [ ] `remediation.RunbookStore` (JSON-persisted): `NewRunbookStore(dir)`, `Add(Runbook)`, `List() []Runbook`, `Save/Load`. Stores agent-proposed runbooks alongside (not replacing) the static `Runbooks()`.
- [ ] Agent tools `propose_preference` (args: kind, subject, note) and `propose_runbook` (args: name, description) registered on the tool registry; both `RiskReadOnly` (they only write a PENDING proposal — no system change). Invoking them adds a pending preference / a runbook to the stores and returns a confirmation string telling the operator to confirm it.
- [ ] `Service.ConfirmPreference(id) error` flips a pending preference active (delegates to memory store).
- [ ] Policy gate: in `runLoop`, a tool whose tier is `ModeApprove` is AUTO-approved (no pause) when an ACTIVE `auto_approve_tool` preference exists for that tool name. Forbid-list still wins. Read-only and existing auto tiers unchanged.
- [ ] A pending `auto_approve_tool` preference does NOT auto-approve (suggest-not-mutate).

**Verify:** `go test ./daemon/services/remediation/ -run RunbookStore -v && go test ./daemon/services/agent/ -run 'Learning|Pref|AutoApprove' -v` → PASS

**Steps:**

- [ ] **Step 1: `remediation/runbook_store.go`** — JSON store mirroring `agent.Store` pattern (RWMutex, `os.MkdirAll`, `os.WriteFile` 0o600 with `// #nosec G301/G306`, missing-file-not-error Load). Persists to `agent_runbooks.json` under `DefaultConfigDir` (add a `DefaultConfigDir` const if the package lacks one — it likely does; define it). `Add(rb Runbook)`, `List() []Runbook`. Test `runbook_store_test.go`: add → save → reload → List contains it.

- [ ] **Step 2: `learning_tools.go`** — `RegisterLearningTools(reg *tools.Registry)` method on Service that registers two `RiskReadOnly` tools:

```go
package agent

import (
 "context"
 "encoding/json"
 "fmt"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/remediation"
)

// RegisterLearningTools adds suggest-not-mutate learning tools to the registry.
func (s *Service) RegisterLearningTools(reg *tools.Registry) {
 reg.Register(tools.Tool{
  Name:        "propose_preference",
  Description: "Propose an operator preference for review (e.g. always auto-approve restarting a specific container). Stored PENDING — never takes effect until the operator confirms it.",
  RiskTier:    dto.RiskReadOnly,
  Schema:      []byte(`{"type":"object","properties":{"kind":{"type":"string"},"subject":{"type":"string"},"note":{"type":"string"}},"required":["kind","subject"]}`),
  Invoke: func(_ context.Context, argsJSON string) (string, error) {
   var a struct{ Kind, Subject, Note string }
   if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
    return "", fmt.Errorf("parse args: %w", err)
   }
   if s.memory == nil {
    return "Memory disabled; cannot store preference.", nil
   }
   id := fmt.Sprintf("pref-%d", s.nextPrefSeq())
   s.memory.AddPreference(dto.AgentPreference{ID: id, Kind: a.Kind, Subject: a.Subject, Note: a.Note, Status: dto.PreferencePending})
   _ = s.memory.Save()
   return fmt.Sprintf("Proposed preference %s (%s: %s) — PENDING operator confirmation; it will not take effect until confirmed.", id, a.Kind, a.Subject), nil
  },
 })
 reg.Register(tools.Tool{
  Name:        "propose_runbook",
  Description: "Propose a named remediation runbook for review. Stored for the operator; does not execute anything.",
  RiskTier:    dto.RiskReadOnly,
  Schema:      []byte(`{"type":"object","properties":{"name":{"type":"string"},"description":{"type":"string"}},"required":["name","description"]}`),
  Invoke: func(_ context.Context, argsJSON string) (string, error) {
   var a struct{ Name, Description string }
   if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
    return "", fmt.Errorf("parse args: %w", err)
   }
   if s.runbooks == nil {
    return "Runbook store unavailable.", nil
   }
   s.runbooks.Add(remediation.Runbook{Name: a.Name, Description: a.Description})
   _ = s.runbooks.Save()
   return fmt.Sprintf("Proposed runbook %q for operator review.", a.Name), nil
  },
 })
}
```

Add a `nextPrefSeq()` helper on Service (mutex-guarded counter `prefSeq int`), and a `runbooks *remediation.RunbookStore` field on Service (set via `NewService` — extend the signature OR a `SetRunbookStore` setter; prefer a setter to avoid another constructor change: `func (s *Service) SetRunbookStore(rs *remediation.RunbookStore) { s.runbooks = rs }`). Add `ConfirmPreference`:

```go
// ConfirmPreference activates a pending learned preference.
func (s *Service) ConfirmPreference(id string) error {
 if s.memory == nil {
  return fmt.Errorf("memory disabled")
 }
 err := s.memory.ConfirmPreference(id)
 if err == nil {
  _ = s.memory.Save()
 }
 return err
}
```

- [ ] **Step 3: Policy gate consults active prefs.** In `loop.go`, change the gate:

```go
   mode := s.cfg.Autonomy[tier]
   if mode != dto.ModeAuto && s.autoApprovedByPreference(call.Name) {
    mode = dto.ModeAuto
   }
   if mode != dto.ModeAuto {
    ... // existing pause
   }
```

Add to `service.go` (or recall.go):

```go
// autoApprovedByPreference reports whether an active auto_approve_tool preference
// covers the named tool. Forbidden tools are never covered (checked earlier).
func (s *Service) autoApprovedByPreference(toolName string) bool {
 if s.memory == nil {
  return false
 }
 for _, p := range s.memory.ActivePreferences() {
  if p.Kind == "auto_approve_tool" && p.Subject == toolName {
   return true
  }
 }
 return false
}
```

(The forbid-list check precedes this in the loop, so a forbidden tool is never reached here.)

- [ ] **Step 4: Tests** `learning_test.go`:

  - `TestProposePreferenceIsPending`: register learning tools, invoke `propose_preference` via the registry, assert one PENDING preference and zero active.
  - `TestActivePreferenceAutoApproves`: add+confirm an `auto_approve_tool` pref for `stop_array`; script a mock to call `stop_array` (RiskHigh/ModeApprove); assert the session does NOT pause (auto-approved) and the tool executed.
  - `TestPendingPreferenceDoesNotAutoApprove`: same but pref left pending → session pauses awaiting approval.

- [ ] **Step 5: Run** both verify commands (PASS), full agent+remediation packages, golangci clean, build.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/remediation/runbook_store.go daemon/services/remediation/runbook_store_test.go daemon/services/agent/learning_tools.go daemon/services/agent/service.go daemon/services/agent/loop.go daemon/services/agent/learning_test.go
git commit -m "feat(agent): suggest-not-mutate learning (preferences + runbook proposals); confirmed prefs raise the gate"
```

---

## Task 6: Multi-turn operator chat (SendMessage)

**Goal:** Continue a finished session with a follow-up operator message.

**Files:**

- Modify: `daemon/services/agent/service.go`
- Test: `daemon/services/agent/service_test.go`

**Acceptance Criteria:**

- [ ] `SendMessage(ctx, sessionID, message string) (dto.AgentSession, error)`: appends a `user` transcript message, sets status running, runs the loop, finalizes, persists.
- [ ] Only `SessionCompleted`/`SessionFailed` sessions can be continued; `awaiting_approval` returns an error (must approve/deny first); not-found returns an error.
- [ ] An empty message returns an error.

**Verify:** `go test ./daemon/services/agent/ -run SendMessage -v` → PASS

**Steps:**

- [ ] **Step 1: Test:**

```go
func TestSendMessageContinuesSession(t *testing.T) {
 p := llm.NewMockProvider(
  &llm.ChatResponse{Text: "[]"},                       // planner
  &llm.ChatResponse{Text: "Plex is healthy.", OutputTokens: 2}, // first answer
  &llm.ChatResponse{Text: "Yes, it restarted 2h ago.", OutputTokens: 2}, // follow-up answer
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
 sess, _ := svc.StartSession(context.Background(), "is plex healthy?")
 if sess.Status != dto.SessionCompleted {
  t.Fatalf("precondition: %q", sess.Status)
 }
 out, err := svc.SendMessage(context.Background(), sess.ID, "did it restart recently?")
 if err != nil {
  t.Fatalf("send: %v", err)
 }
 if out.Status != dto.SessionCompleted || out.Answer != "Yes, it restarted 2h ago." {
  t.Fatalf("follow-up not handled: %q / %q", out.Status, out.Answer)
 }
}

func TestSendMessageRejectsAwaitingOrEmpty(t *testing.T) {
 p := llm.NewMockProvider(&llm.ChatResponse{Text: "[]"}, &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "t1", Name: "stop_array", Args: "{}"}}})
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.NewRegistry()
 reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh, Invoke: func(_ context.Context, _ string) (string, error) { return "", nil }})
 svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
 sess, _ := svc.StartSession(context.Background(), "stop array")
 if _, err := svc.SendMessage(context.Background(), sess.ID, "hi"); err == nil {
  t.Fatal("expected error continuing an awaiting_approval session")
 }
 // empty message on a fresh completed session
 p2 := llm.NewMockProvider(&llm.ChatResponse{Text: "[]"}, &llm.ChatResponse{Text: "done"})
 svc2 := NewService(cfg, p2, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
 s2, _ := svc2.StartSession(context.Background(), "x")
 if _, err := svc2.SendMessage(context.Background(), s2.ID, "  "); err == nil {
  t.Fatal("expected error on empty message")
 }
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement in `service.go`:**

```go
// SendMessage continues a finished session with a follow-up operator message.
func (s *Service) SendMessage(ctx context.Context, sessionID, message string) (dto.AgentSession, error) {
 if strings.TrimSpace(message) == "" {
  return dto.AgentSession{}, errors.New("message must not be empty")
 }
 sess, ok := s.store.Get(sessionID)
 if !ok {
  return dto.AgentSession{}, fmt.Errorf("session %q not found", sessionID)
 }
 if sess.Status != dto.SessionCompleted && sess.Status != dto.SessionFailed {
  return dto.AgentSession{}, fmt.Errorf("session %q cannot be continued in state %q", sessionID, sess.Status)
 }
 sess.Status = dto.SessionRunning
 sess.Answer = ""
 sess.Error = ""
 sess.EndedAt = nil
 appendTranscript(&sess, llm.Message{Role: "user", Content: message})
 s.emit(&sess, "message_received", nil)
 s.runLoop(ctx, &sess)
 s.finalize(&sess)
 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
 }
 return sess, nil
}
```

- [ ] **Step 4: Run** `go test ./daemon/services/agent/ -run SendMessage -v` (PASS) + full package, golangci clean.

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/service.go daemon/services/agent/service_test.go
git commit -m "feat(agent): multi-turn SendMessage to continue a finished session"
```

---

## Task 7: REST + MCP surface + orchestrator wiring

**Goal:** Expose memory, preference confirmation, and chat over REST + MCP, and construct/wire the memory + runbook stores in the orchestrator.

**Files:**

- Modify: `daemon/services/agent/bootstrap.go`, `daemon/services/orchestrator.go`
- Modify: `daemon/services/api/handlers_agent.go`, `daemon/services/api/server.go`
- Modify: `daemon/services/mcp/server.go`
- Test: `daemon/services/api/handlers_agent_test.go`, `daemon/services/mcp/server_test.go`

**Acceptance Criteria:**

- [ ] `bootstrap.BuildService` constructs `memory.NewStore(configDir, cfg.MaxIncidents)` (+ `Load()`), passes it to `NewService`, constructs a `remediation.NewRunbookStore(configDir)` (+ `Load()`) and calls `svc.SetRunbookStore(...)` and `svc.RegisterLearningTools(reg)`.
- [ ] REST: `POST /api/v1/agent/sessions/{id}/messages` (`{"message":"..."}`) → continued session; `GET /api/v1/agent/memory` → `{incidents, preferences}`; `POST /api/v1/agent/preferences/{id}/confirm` → ok/err. 503 when disabled; 400 on bad input.
- [ ] MCP: `agent_send_message` (session_id, message), `agent_get_memory` (read-only), `agent_confirm_preference` (preference_id).
- [ ] New Service accessors: `Memory() (incidents []dto.AgentIncident, prefs []dto.AgentPreference)` (or two methods) for the handlers.
- [ ] `go build ./...` + api/mcp/agent packages pass.

**Verify:** `go test ./daemon/services/api/ -run Agent -v && go test ./daemon/services/mcp/ -run Agent -v && go build ./...` → PASS

**Steps:**

- [ ] **Step 1: Service accessors** in `service.go`:

```go
// MemoryIncidents returns recorded incidents (newest-first); nil when memory disabled.
func (s *Service) MemoryIncidents() []dto.AgentIncident {
 if s.memory == nil { return nil }
 return s.memory.ListIncidents()
}
// MemoryPreferences returns all learned preferences; nil when memory disabled.
func (s *Service) MemoryPreferences() []dto.AgentPreference {
 if s.memory == nil { return nil }
 return s.memory.ListPreferences()
}
```

- [ ] **Step 2: REST handlers** in `handlers_agent.go` — follow the EXACT pattern of the existing `handleAgentApprove`/`handleAgentCancel` (503-when-disabled, JSON decode, `respondWithError`, `respondJSON`, Swagger annotations, Tag "Agent"):
  - `handleAgentSendMessage`: decode `{message}`, call `s.agentSvc.SendMessage(r.Context(), id, body.Message)`, map error→400, return session.
  - `handleAgentMemory`: return `map[string]any{"incidents": s.agentSvc.MemoryIncidents(), "preferences": s.agentSvc.MemoryPreferences()}` (503 if `agentSvc==nil`).
  - `handleAgentConfirmPreference`: `id := mux.Vars(r)["id"]`, call `s.agentSvc.ConfirmPreference(id)`, map error→400, return `dto.Response{Success:true,...}`.
    Routes in `server.go` `setupRoutes()`:

```go
 api.HandleFunc("/agent/sessions/{id}/messages", s.handleAgentSendMessage).Methods("POST")
 api.HandleFunc("/agent/memory", s.handleAgentMemory).Methods("GET")
 api.HandleFunc("/agent/preferences/{id}/confirm", s.handleAgentConfirmPreference).Methods("POST")
```

Tests in `handlers_agent_test.go`: start a session (mock: planner `[]` + answer), then `POST /messages` (mock needs a 3rd response) → 200; `GET /memory` → 200 with `incidents`; confirm a preference (seed one via the service's memory) → 200.

- [ ] **Step 3: MCP tools** in `registerAgentTools()` (mcp/server.go) — three more `mcp.AddTool` calls mirroring the existing ones:

  - `agent_send_message` (args `session_id`, `message`) → `s.agentSvc.SendMessage`.
  - `agent_get_memory` (no args, `ReadOnlyHint`) → `jsonResult(map{"incidents":..., "preferences":...})`.
  - `agent_confirm_preference` (arg `preference_id`) → `s.agentSvc.ConfirmPreference`.
    Test in `server_test.go`: assert these tools register and `agent_get_memory` returns without panic when agent set.

- [ ] **Step 4: bootstrap + orchestrator wiring.**
      In `bootstrap.go` `BuildService`, after building `reg` and before `NewService`:

```go
 mem := memory.NewStore(configDir, cfg.MaxIncidents)
 if err := mem.Load(); err != nil {
  logger.Warning("Agent: failed to load memory: %v", err)
 }
 svc := NewService(cfg, provider, reg, store, mem, bc)
 rbStore := remediation.NewRunbookStore(configDir)
 if err := rbStore.Load(); err != nil {
  logger.Warning("Agent: failed to load runbooks: %v", err)
 }
 svc.SetRunbookStore(rbStore)
 svc.RegisterLearningTools(reg)
 return svc, nil
```

(Replace the existing `return NewService(...), nil` tail accordingly; import `memory` and `remediation`.) The orchestrator needs no new args — `BuildService` already receives `configDir=""`.

- [ ] **Step 5: Run** all three verify commands + full suite + golangci clean + build.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/agent/bootstrap.go daemon/services/agent/service.go daemon/services/api/ daemon/services/mcp/server.go daemon/services/mcp/server_test.go
git commit -m "feat(agent): REST+MCP for chat/memory/preferences; wire memory+runbook stores"
```

---

## Task 8: Docs, CHANGELOG, CodeRabbit review, and on-Unraid verification

**Goal:** **USER-ORDERED GATE — NON-SKIPPABLE.** This task was requested by the user in the current conversation. It MUST NOT be closed by walking around it, by declaring it "verified inline", or by substituting a cheaper check. Close only after every item in `acceptanceCriteria` has been re-validated independently, with output captured.

Document Phase 3, run the CodeRabbit CLI review and address findings, then build/test and deploy to Unraid and verify memory/planning/learning/chat work and the plugin is stable.

**Files:** Modify `CHANGELOG.md`, `docs/integrations/agent.md`; regenerate Swagger.

**Acceptance Criteria:**

- [ ] `CHANGELOG.md` Phase-3 entry under **Added** (recall/episodic memory, planner, suggest-not-mutate learning + runbook proposals, multi-turn chat, REST/MCP memory surfaces).
- [ ] `docs/integrations/agent.md` documents memory/recall, the planner, the learning/confirmation workflow, multi-turn chat, the new config (`memory_enabled`, `max_incidents`, `recall_top_k`), and the new REST/MCP endpoints.
- [ ] `make pre-commit-run` passes (clear stale golangci cache first if needed); `make test` (race) passes.
- [ ] CodeRabbit CLI review run on the branch diff; actionable findings fixed or noted out-of-scope with reasoning.
- [ ] Plugin builds + deploys to Unraid via Ansible (`build,deploy,verify`); no panics; verify suite passes (no regression).
- [ ] Enabled-path on a real provider (OpenRouter, e.g. model `z-ai/glm-4.5-air:free`, key from `scripts/.env` `OPENR_ROUTER_API_KEY` → `UMA_AGENT_API_KEY`): a session produces a non-empty `plan`, completes, and a second `SendMessage` follow-up returns an answer; an episodic incident is recorded; a `propose_preference` proposal is stored PENDING and only auto-approves after confirm. (May be exercised via a gated local integration test using the real provider, mirroring Phase 2's approach.)

**Verify:** `make pre-commit-run && make test`; `coderabbit review --agent --base-commit <phase3-base>`; `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify`; local real-provider integration exercising plan + memory + chat + suggest-not-mutate.

**Steps:**

- [ ] Update CHANGELOG + docs + Swagger annotations; `make swagger`.
- [ ] `make pre-commit-run` (clear golangci cache if paths look stale) + `make test`.
- [ ] CodeRabbit CLI review; fix actionable findings; re-run until clean or only style/deferred remain.
- [ ] Deploy via Ansible; tail log for panics; confirm verify suite passes.
- [ ] Real-provider enabled-path: gated local integration test (OpenRouter) exercising plan population, completion, a `SendMessage` follow-up, an episodic incident write, and a pending→confirm preference auto-approve cycle. Capture output.
- [ ] Commit docs.

```json:metadata
{"files": ["CHANGELOG.md", "docs/integrations/agent.md"], "verifyCommand": "make pre-commit-run && make test && ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["CHANGELOG Phase-3 Added entry", "agent.md documents memory/planner/learning/chat + new config + endpoints", "make pre-commit-run passes", "make test passes", "CodeRabbit review run + findings addressed", "builds+deploys to Unraid; no panics; verify suite passes", "real-provider: session yields plan + completes + SendMessage follow-up answers + incident recorded + pending->confirm preference auto-approves"], "userGate": true, "tags": ["user-gate"], "gateScope": "all"}
```

---

## Self-Review Notes

- **Spec coverage (Phase 3 = Planning & memory):** episodic + semantic memory store (Task 2); recall injection at start + episodic write on finish (Task 3); planner (Task 4); suggest-not-mutate learning incl. runbook proposals + policy consulting confirmed prefs (Task 5); multi-turn chat (Task 6); REST/MCP surfaces + wiring (Task 7); docs+review+verify (Task 8). Reuses `remediation.Runbook` via a new persistent `RunbookStore` (Task 5) rather than a parallel type — honoring the spec's "reuse runbooks" decision. Recall is keyword/tag (no embeddings) per spec.
- **Type consistency:** `dto.PlanStep`/`AgentIncident`/`AgentPreference`/`PreferenceStatus` (Task 1) used unchanged in Tasks 2-7. `memory.Store` API (Task 2) consumed by recall.go/service.go (Task 3,5), learning_tools.go (Task 5), accessors + handlers (Task 7). `NewService(cfg,provider,reg,store,mem,bc)` signature change (Task 3) is applied to every call site (tests + bootstrap). `SetRunbookStore`/`RegisterLearningTools`/`ConfirmPreference`/`SendMessage`/`MemoryIncidents`/`MemoryPreferences`/`autoApprovedByPreference`/`nextPrefSeq` are introduced once and reused consistently.
- **Import-cycle check:** `agent` imports `agent/memory`, `agent/tools`, `agent/llm`, `remediation`, `dto`, `domain`, `constants`, `logger`, `controllers` — none import `agent`. `remediation` imports `dto` only (RunbookStore adds `os`/`sync`/`encoding/json`). `memory` imports `dto`+`logger`. No cycles.
- **Biggest integration risk (called out in Task 4):** the planner adds one provider call at the start of `StartSession`, so every existing `StartSession`-driven mock test must prepend an empty-plan response `&llm.ChatResponse{Text:"[]"}`. The plan flags this explicitly and lists the affected files. `startAutonomousSession`/`ApproveAction` do not plan, so their tests are unaffected.
- **Placeholders:** none. "Follow the existing pattern" references point at committed Phase-1/2 code (handlers_agent.go, registerAgentTools, store.go), not at other plan tasks.
