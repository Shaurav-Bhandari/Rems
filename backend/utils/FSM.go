package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// TYPE CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────────

// State is the constraint for state type parameters.
// Any comparable type works: string, int, custom string alias, etc.
type State interface{ comparable }

// Event is the constraint for event type parameters.
type Event interface{ comparable }

// ─────────────────────────────────────────────────────────────────────────────
// ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrInvalidTransition is returned when no transition exists for the
	// current state + event combination.
	ErrInvalidTransition = errors.New("fsm: invalid transition")

	// ErrGuardRejected is returned when a guard function blocks a transition.
	ErrGuardRejected = errors.New("fsm: guard rejected transition")

	// ErrTerminalState is returned when Send is called on a machine in a
	// terminal state.
	ErrTerminalState = errors.New("fsm: machine is in a terminal state")

	// ErrCompilationFailed is returned when MustCompile detects definition errors.
	ErrCompilationFailed = errors.New("fsm: definition compilation failed")

	// ErrConcurrentTransition is returned when a transition is already in
	// progress on the same machine.
	ErrConcurrentTransition = errors.New("fsm: concurrent transition in progress")

	// ErrUnknownState is returned when a state referenced in a transition
	// definition was never registered via State().
	ErrUnknownState = errors.New("fsm: referenced state was never declared")
)

// TransitionError wraps a lower-level error with the full transition context
// so callers can inspect exactly what failed and why.
type TransitionError[S State, E Event] struct {
	From  S
	To    S
	Event E
	Phase TransitionPhase
	Cause error
}

func (te *TransitionError[S, E]) Error() string {
	return fmt.Sprintf("fsm: transition %v -[%v]→ %v failed at %s: %v",
		te.From, te.Event, te.To, te.Phase, te.Cause)
}

func (te *TransitionError[S, E]) Unwrap() error { return te.Cause }

// TransitionPhase identifies which hook in the pipeline failed.
type TransitionPhase string

const (
	PhaseGuard       TransitionPhase = "guard"
	PhaseBeforeExit  TransitionPhase = "before_exit"
	PhaseBeforeEnter TransitionPhase = "before_enter"
	PhaseEffect      TransitionPhase = "effect"
	PhaseAfterExit   TransitionPhase = "after_exit"
	PhaseAfterEnter  TransitionPhase = "after_enter"
)

// ─────────────────────────────────────────────────────────────────────────────
// EVENT ENVELOPE
// Wraps an event with metadata so guards and effects have full context.
// ─────────────────────────────────────────────────────────────────────────────

// EventEnvelope wraps the triggering event with request-level context.
type EventEnvelope[E Event] struct {
	// ID uniquely identifies this event occurrence for idempotency checking.
	ID string

	// Event is the domain event type being sent.
	Event E

	// Payload is arbitrary structured data attached by the caller.
	// Guards and effects can type-assert it for domain-specific logic.
	Payload any

	// ActorID is the UUID of the user or system component sending the event.
	// Used by effects that need to record who triggered a transition.
	ActorID uuid.UUID

	// TenantID scopes the event to a tenant for multi-tenant guards.
	TenantID uuid.UUID

	// OccurredAt is when the event was created — not when it was processed.
	OccurredAt time.Time

	// Metadata holds arbitrary key-value pairs (IP, device ID, trace ID, etc.)
	Metadata map[string]string
}

// NewEnvelope constructs a minimal EventEnvelope with a generated ID and timestamp.
func NewEnvelope[E Event](event E, payload any) EventEnvelope[E] {
	return EventEnvelope[E]{
		ID:         uuid.NewString(),
		Event:      event,
		Payload:    payload,
		OccurredAt: time.Now(),
		Metadata:   make(map[string]string),
	}
}

// WithActor attaches actor and tenant IDs to the envelope.
func (e EventEnvelope[E]) WithActor(actorID, tenantID uuid.UUID) EventEnvelope[E] {
	e.ActorID = actorID
	e.TenantID = tenantID
	return e
}

// WithMeta attaches a key-value metadata pair to the envelope.
func (e EventEnvelope[E]) WithMeta(key, value string) EventEnvelope[E] {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// ─────────────────────────────────────────────────────────────────────────────
// HOOKS
// All hooks receive the context object and the event envelope.
// Returning a non-nil error aborts the transition (except AfterExit/AfterEnter
// which are called post-commit and cannot abort).
// ─────────────────────────────────────────────────────────────────────────────

// GuardFunc is called before any state change occurs.
// Return nil to allow, return an error to block.
type GuardFunc[C any, E Event] func(ctx context.Context, obj C, event EventEnvelope[E]) error

// EffectFunc is called after guards pass but before the state is committed.
// Use this to mutate the context object (set timestamps, update counters, etc.)
type EffectFunc[C any, E Event] func(ctx context.Context, obj C, event EventEnvelope[E]) error

// HookFunc is used for BeforeExit, BeforeEnter, AfterExit, AfterEnter.
// It receives the full transition context.
type HookFunc[C any, S State, E Event] func(ctx context.Context, obj C, t TransitionRecord[S, E]) error

// ObserverFunc is a post-commit callback. It cannot abort the transition.
// Used for audit logging, metrics, notifications, cache invalidation.
type ObserverFunc[C any, S State, E Event] func(ctx context.Context, obj C, t TransitionRecord[S, E])

// ─────────────────────────────────────────────────────────────────────────────
// TRANSITION RECORD
// Written to history on every successful transition.
// ─────────────────────────────────────────────────────────────────────────────

// TransitionRecord is the immutable record of a completed transition.
type TransitionRecord[S State, E Event] struct {
	// SequenceNumber increments with each successful transition on this machine.
	SequenceNumber uint64

	From       S
	To         S
	Event      EventEnvelope[E]
	OccurredAt time.Time
	DurationMs int64 // how long the full pipeline took
}

// stateMeta holds configuration for a single state.
type stateMeta[C any, S State, E Event] struct {
	name        S
	isTerminal  bool
	isInitial   bool
	description string

	// onEntry fires every time this state is entered (after effect, before AfterEnter)
	onEntry []HookFunc[C, S, E]

	// onExit fires every time this state is exited (before effect, after BeforeExit)
	onExit []HookFunc[C, S, E]
}

// transitionDef is an internal, compiled representation of one allowed transition.
type transitionDef[C any, S State, E Event] struct {
	from  S
	event E
	to    S

	// guards are evaluated in order; first failure blocks the transition
	guards []GuardFunc[C, E]

	// effects are applied in order after all guards pass
	effects []EffectFunc[C, E]

	// beforeExit runs on the FROM state before exit hooks
	beforeExit []HookFunc[C, S, E]

	// beforeEnter runs on the TO state before entry hooks
	beforeEnter []HookFunc[C, S, E]

	// afterExit runs on the FROM state after the state has changed (non-aborting)
	afterExit []HookFunc[C, S, E]

	// afterEnter runs on the TO state after the state has changed (non-aborting)
	afterEnter []HookFunc[C, S, E]

	// description for documentation and visualization output
	description string
}

// TransitionOption configures a transition definition.
type TransitionOption[C any, S State, E Event] func(*transitionDef[C, S, E])

// Guard adds a guard function to the transition.
// Guards run in registration order; first failure wins.
func Guard[C any, S State, E Event](fn GuardFunc[C, E]) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.guards = append(t.guards, fn)
	}
}

// Effect adds a side-effect function to the transition.
// Effects run after all guards pass, before state is committed.
func Effect[C any, S State, E Event](fn EffectFunc[C, E]) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.effects = append(t.effects, fn)
	}
}

// BeforeExit adds a hook that runs before the FROM state's exit hooks.
func BeforeExit[C any, S State, E Event](fn HookFunc[C, S, E]) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.beforeExit = append(t.beforeExit, fn)
	}
}

// BeforeEnter adds a hook that runs before the TO state's entry hooks.
func BeforeEnter[C any, S State, E Event](fn HookFunc[C, S, E]) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.beforeEnter = append(t.beforeEnter, fn)
	}
}

// AfterExit adds a post-commit hook for the FROM state. Cannot abort.
func AfterExit[C any, S State, E Event](fn HookFunc[C, S, E]) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.afterExit = append(t.afterExit, fn)
	}
}

// AfterEnter adds a post-commit hook for the TO state. Cannot abort.
func AfterEnter[C any, S State, E Event](fn HookFunc[C, S, E]) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.afterEnter = append(t.afterEnter, fn)
	}
}

// Describe sets a human-readable description on the transition.
// Used for audit logs and the DOT graph exporter.
func Describe[C any, S State, E Event](desc string) TransitionOption[C, S, E] {
	return func(t *transitionDef[C, S, E]) {
		t.description = desc
	}
}

// StateOption configures a state definition.
type StateOption[C any, S State, E Event] func(*stateMeta[C, S, E])

// Terminal marks the state as a terminal (sink) state.
// Machines in terminal states reject all further events.
func Terminal[C any, S State, E Event]() StateOption[C, S, E] {
	return func(s *stateMeta[C, S, E]) { s.isTerminal = true }
}

// Initial marks this as the default initial state for machines created without
// an explicit starting state.
func Initial[C any, S State, E Event]() StateOption[C, S, E] {
	return func(s *stateMeta[C, S, E]) { s.isInitial = true }
}

// OnEntry registers a hook that fires every time this state is entered.
func OnEntry[C any, S State, E Event](fn HookFunc[C, S, E]) StateOption[C, S, E] {
	return func(s *stateMeta[C, S, E]) { s.onEntry = append(s.onEntry, fn) }
}

// OnExit registers a hook that fires every time this state is exited.
func OnExit[C any, S State, E Event](fn HookFunc[C, S, E]) StateOption[C, S, E] {
	return func(s *stateMeta[C, S, E]) { s.onExit = append(s.onExit, fn) }
}

// StateDesc sets a human-readable description for the state.
func StateDesc[C any, S State, E Event](desc string) StateOption[C, S, E] {
	return func(s *stateMeta[C, S, E]) { s.description = desc }
}

// DefinitionBuilder is the fluent builder for constructing an FSM definition.
// S = state type, E = event type, C = context object type (the domain entity).
type DefinitionBuilder[C any, S State, E Event] struct {
	states      map[S]*stateMeta[C, S, E]
	transitions []*transitionDef[C, S, E]
	observers   []ObserverFunc[C, S, E]

	// globalGuards run on every transition before transition-specific guards
	globalGuards []GuardFunc[C, E]

	// globalEffects run on every transition after all guards pass
	globalEffects []EffectFunc[C, E]

	// historyCapacity is the maximum number of transition records retained per machine
	historyCapacity int
}

// Define starts building an FSM definition.
// Type parameters must be provided explicitly:
//
//	fsm.Define[MyState, MyEvent, *MyEntity]()
func Define[C any, S State, E Event]() *DefinitionBuilder[C, S, E] {
	return &DefinitionBuilder[C, S, E]{
		states:          make(map[S]*stateMeta[C, S, E]),
		historyCapacity: 100,
	}
}

// State registers a state with optional configuration.
func (b *DefinitionBuilder[C, S, E]) State(name S, opts ...StateOption[C, S, E]) *DefinitionBuilder[C, S, E] {
	meta := &stateMeta[C, S, E]{name: name}
	for _, opt := range opts {
		opt(meta)
	}
	b.states[name] = meta
	return b
}

// Transition registers an allowed transition from → event → to.
// Multiple transitions from the same (from, event) pair are NOT allowed
// (this is a deterministic FSM, not an NFA). Compile() will catch this.
func (b *DefinitionBuilder[C, S, E]) Transition(
	from S, event E, to S,
	opts ...TransitionOption[C, S, E],
) *DefinitionBuilder[C, S, E] {
	t := &transitionDef[C, S, E]{from: from, event: event, to: to}
	for _, opt := range opts {
		opt(t)
	}
	b.transitions = append(b.transitions, t)
	return b
}

// GlobalGuard adds a guard that runs before every transition's specific guards.
// Use this for cross-cutting concerns like tenant isolation or soft-delete checks.
func (b *DefinitionBuilder[C, S, E]) GlobalGuard(fn GuardFunc[C, E]) *DefinitionBuilder[C, S, E] {
	b.globalGuards = append(b.globalGuards, fn)
	return b
}

// GlobalEffect adds an effect that runs after every successful guard check.
// Use this for cross-cutting mutations like updating UpdatedAt timestamps.
func (b *DefinitionBuilder[C, S, E]) GlobalEffect(fn EffectFunc[C, E]) *DefinitionBuilder[C, S, E] {
	b.globalEffects = append(b.globalEffects, fn)
	return b
}

// Observe registers a post-commit observer.
// Observers receive every successful transition and run asynchronously by default.
// They cannot abort or modify the transition.
func (b *DefinitionBuilder[C, S, E]) Observe(fn ObserverFunc[C, S, E]) *DefinitionBuilder[C, S, E] {
	b.observers = append(b.observers, fn)
	return b
}

// HistoryCapacity sets how many TransitionRecords each machine instance retains.
// Default is 100. Set to 0 for unlimited (memory-unbound — use with care).
func (b *DefinitionBuilder[C, S, E]) HistoryCapacity(n int) *DefinitionBuilder[C, S, E] {
	b.historyCapacity = n
	return b
}

// Compile validates the definition and returns a compiled FSMDefinition.
// Returns an error describing every validation failure found.
func (b *DefinitionBuilder[C, S, E]) Compile() (*FSMDefinition[C, S, E], error) {
	var errs []string

	// Build transition lookup: (from, event) → transitionDef
	index := make(map[transitionKey[S, E]]*transitionDef[C, S, E])

	for _, t := range b.transitions {
		// Validate from state is declared
		if _, ok := b.states[t.from]; !ok {
			errs = append(errs, fmt.Sprintf("transition from undeclared state %v", t.from))
		}
		// Validate to state is declared
		if _, ok := b.states[t.to]; !ok {
			errs = append(errs, fmt.Sprintf("transition to undeclared state %v", t.to))
		}

		key := transitionKey[S, E]{from: t.from, event: t.event}
		if _, exists := index[key]; exists {
			errs = append(errs, fmt.Sprintf(
				"duplicate transition: state %v + event %v (FSM is deterministic — use guards to differentiate)",
				t.from, t.event,
			))
		}
		index[key] = t
	}

	// Check that terminal states have no outgoing transitions
	for _, t := range b.transitions {
		if meta, ok := b.states[t.from]; ok && meta.isTerminal {
			errs = append(errs, fmt.Sprintf(
				"terminal state %v has outgoing transition on event %v — terminal states must be sinks",
				t.from, t.event,
			))
		}
	}

	// Warn if no initial state declared (not fatal — caller can pass initial state to New())
	hasInitial := false
	for _, meta := range b.states {
		if meta.isInitial {
			hasInitial = true
			break
		}
	}
	_ = hasInitial // suppress unused warning — it's informational only

	if len(errs) > 0 {
		return nil, fmt.Errorf("%w:\n  - %s", ErrCompilationFailed, strings.Join(errs, "\n  - "))
	}

	return &FSMDefinition[C, S, E]{
		states:          b.states,
		index:           index,
		observers:       b.observers,
		globalGuards:    b.globalGuards,
		globalEffects:   b.globalEffects,
		historyCapacity: b.historyCapacity,
	}, nil
}

// MustCompile is like Compile but panics on error.
// Use at package init time where a compilation failure is a programming error.
func (b *DefinitionBuilder[C, S, E]) MustCompile() *FSMDefinition[C, S, E] {
	def, err := b.Compile()
	if err != nil {
		panic(err)
	}
	return def
}

// ─────────────────────────────────────────────────────────────────────────────
// FSM DEFINITION (compiled, immutable, shareable across goroutines)
// ─────────────────────────────────────────────────────────────────────────────

// FSMDefinition is a compiled, immutable FSM definition.
// It is safe to share across goroutines. Create one per entity type at startup.
type FSMDefinition[C any, S State, E Event] struct {
	states          map[S]*stateMeta[C, S, E]
	index           map[transitionKey[S, E]]*transitionDef[C, S, E]
	observers       []ObserverFunc[C, S, E]
	globalGuards    []GuardFunc[C, E]
	globalEffects   []EffectFunc[C, E]
	historyCapacity int
}

// New creates a new Machine instance from this definition.
// obj is the domain entity the machine operates on.
// initialState is where the machine starts.
func (d *FSMDefinition[C, S, E]) New(obj C, initialState S) *Machine[C, S, E] {
	return &Machine[C, S, E]{
		def:     d,
		obj:     obj,
		current: initialState,
		history: make([]TransitionRecord[S, E], 0, min(d.historyCapacity, 32)),
		seq:     0,
	}
}

// CanTransition returns true if the given (from, event) pair has a registered
// transition — without evaluating guards. Use for UI hint generation.
func (d *FSMDefinition[C, S, E]) CanTransition(from S, event E) bool {
	_, ok := d.index[transitionKey[S, E]{from: from, event: event}]
	return ok
}

// AllowedEvents returns all events that have a registered (not necessarily
// guard-passing) transition from the given state.
func (d *FSMDefinition[C, S, E]) AllowedEvents(from S) []E {
	var events []E
	for key := range d.index {
		if key.from == from {
			events = append(events, key.event)
		}
	}
	return events
}

// IsTerminal returns whether the given state is terminal.
func (d *FSMDefinition[C, S, E]) IsTerminal(state S) bool {
	if meta, ok := d.states[state]; ok {
		return meta.isTerminal
	}
	return false
}

// ExportDOT returns a Graphviz DOT representation of the state machine.
// Pipe to `dot -Tsvg` to generate a diagram.
// Terminal states are shown as double circles; initial states have an arrow.
func (d *FSMDefinition[C, S, E]) ExportDOT(title string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("digraph %q {\n", title))
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=circle fontname=\"Helvetica\"];\n")
	sb.WriteString("  __start__ [label=\"\" shape=point];\n\n")

	// Terminal states
	for state, meta := range d.states {
		if meta.isTerminal {
			sb.WriteString(fmt.Sprintf("  %q [shape=doublecircle];\n", fmt.Sprintf("%v", state)))
		}
		if meta.isInitial {
			sb.WriteString(fmt.Sprintf("  __start__ -> %q;\n", fmt.Sprintf("%v", state)))
		}
	}
	sb.WriteString("\n")

	// Transitions
	for _, t := range d.index {
		label := fmt.Sprintf("%v", t.event)
		if t.description != "" {
			label = fmt.Sprintf("%v\\n[%s]", t.event, t.description)
		}
		sb.WriteString(fmt.Sprintf("  %q -> %q [label=%q];\n",
			fmt.Sprintf("%v", t.from),
			fmt.Sprintf("%v", t.to),
			label,
		))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// MACHINE (stateful, per-entity instance)
// ─────────────────────────────────────────────────────────────────────────────

// Machine is a single running instance of an FSM tied to one domain entity.
// It is NOT goroutine-safe by design — coordinate access externally if needed.
// For concurrent use, wrap in a mutex or use the provided Synchronized() wrapper.
type Machine[C any, S State, E Event] struct {
	def      *FSMDefinition[C, S, E]
	obj      C
	current  S
	history  []TransitionRecord[S, E]
	seq      uint64
	inFlight bool // prevents re-entrant transitions
}

// State returns the current state of the machine.
func (m *Machine[C, S, E]) State() S { return m.current }

// Object returns the domain entity this machine is operating on.
func (m *Machine[C, S, E]) Object() C { return m.obj }

// IsTerminal returns true if the machine has reached a terminal state.
func (m *Machine[C, S, E]) IsTerminal() bool {
	return m.def.IsTerminal(m.current)
}

// History returns a snapshot of all recorded transitions in chronological order.
func (m *Machine[C, S, E]) History() []TransitionRecord[S, E] {
	snap := make([]TransitionRecord[S, E], len(m.history))
	copy(snap, m.history)
	return snap
}

// LastTransition returns the most recent TransitionRecord, or nil if no
// transitions have occurred yet.
func (m *Machine[C, S, E]) LastTransition() *TransitionRecord[S, E] {
	if len(m.history) == 0 {
		return nil
	}
	rec := m.history[len(m.history)-1]
	return &rec
}

// Can returns true if the event is registered from the current state AND
// all guards pass. This does NOT mutate state or context.
// Use this for permission checks and UI hints that need guard evaluation.
func (m *Machine[C, S, E]) Can(ctx context.Context, event E, envelope EventEnvelope[E]) bool {
	if m.IsTerminal() {
		return false
	}
	t, ok := m.def.index[transitionKey[S, E]{from: m.current, event: event}]
	if !ok {
		return false
	}
	for _, g := range m.def.globalGuards {
		if g(ctx, m.obj, envelope) != nil {
			return false
		}
	}
	for _, g := range t.guards {
		if g(ctx, m.obj, envelope) != nil {
			return false
		}
	}
	return true
}

// Send processes an event through the full transition pipeline.
// The transition pipeline in order:
//  1. Terminal check
//  2. Transition lookup
//  3. Global guards
//  4. Transition-specific guards
//  5. BeforeExit hooks (transition-level)
//  6. From-state OnExit hooks
//  7. BeforeEnter hooks (transition-level)
//  8. Global effects
//  9. Transition-specific effects
//
// 10. ── STATE COMMIT ──
// 11. To-state OnEntry hooks
// 12. AfterExit hooks (transition-level, non-aborting)
// 13. AfterEnter hooks (transition-level, non-aborting)
// 14. History recording
// 15. Observers (synchronous by default)
//
// Steps 1–9 can abort the transition by returning an error.
// Steps 11–15 cannot abort — errors are collected but the state change is permanent.
func (m *Machine[C, S, E]) Send(ctx context.Context, envelope EventEnvelope[E]) error {
	if m.inFlight {
		return ErrConcurrentTransition
	}
	m.inFlight = true
	defer func() { m.inFlight = false }()

	started := time.Now()

	// ── 1. Terminal check ──────────────────────────────────────────────────
	if m.IsTerminal() {
		return ErrTerminalState
	}

	// ── 2. Transition lookup ───────────────────────────────────────────────
	key := transitionKey[S, E]{from: m.current, event: envelope.Event}
	t, ok := m.def.index[key]
	if !ok {
		return fmt.Errorf("%w: state=%v event=%v", ErrInvalidTransition, m.current, envelope.Event)
	}

	wrapErr := func(phase TransitionPhase, err error) error {
		return &TransitionError[S, E]{
			From:  m.current,
			To:    t.to,
			Event: envelope.Event,
			Phase: phase,
			Cause: err,
		}
	}

	// ── 3. Global guards ───────────────────────────────────────────────────
	for _, g := range m.def.globalGuards {
		if err := g(ctx, m.obj, envelope); err != nil {
			return wrapErr(PhaseGuard, fmt.Errorf("%w: %v", ErrGuardRejected, err))
		}
	}

	// ── 4. Transition-specific guards ─────────────────────────────────────
	for _, g := range t.guards {
		if err := g(ctx, m.obj, envelope); err != nil {
			return wrapErr(PhaseGuard, fmt.Errorf("%w: %v", ErrGuardRejected, err))
		}
	}

	// Build the record now so hooks can reference it (To is the destination)
	record := TransitionRecord[S, E]{
		SequenceNumber: m.seq + 1,
		From:           m.current,
		To:             t.to,
		Event:          envelope,
		OccurredAt:     started,
	}

	// ── 5. Transition BeforeExit ───────────────────────────────────────────
	for _, h := range t.beforeExit {
		if err := h(ctx, m.obj, record); err != nil {
			return wrapErr(PhaseBeforeExit, err)
		}
	}

	// ── 6. From-state OnExit hooks ─────────────────────────────────────────
	if fromMeta, ok := m.def.states[m.current]; ok {
		for _, h := range fromMeta.onExit {
			if err := h(ctx, m.obj, record); err != nil {
				return wrapErr(PhaseBeforeExit, err)
			}
		}
	}

	// ── 7. Transition BeforeEnter ──────────────────────────────────────────
	for _, h := range t.beforeEnter {
		if err := h(ctx, m.obj, record); err != nil {
			return wrapErr(PhaseBeforeEnter, err)
		}
	}

	// ── 8. Global effects ──────────────────────────────────────────────────
	for _, ef := range m.def.globalEffects {
		if err := ef(ctx, m.obj, envelope); err != nil {
			return wrapErr(PhaseEffect, err)
		}
	}

	// ── 9. Transition-specific effects ────────────────────────────────────
	for _, ef := range t.effects {
		if err := ef(ctx, m.obj, envelope); err != nil {
			return wrapErr(PhaseEffect, err)
		}
	}

	// ── 10. STATE COMMIT ─────────────────────────────────────────────────
	// Past this point, the transition is permanent.
	m.current = t.to
	m.seq++
	record.DurationMs = time.Since(started).Milliseconds()

	// ── 11. To-state OnEntry hooks ────────────────────────────────────────
	// Errors here are logged but cannot roll back.
	if toMeta, ok := m.def.states[t.to]; ok {
		for _, h := range toMeta.onEntry {
			_ = h(ctx, m.obj, record) // non-aborting post-commit
		}
	}

	// ── 12. Transition AfterExit (non-aborting) ────────────────────────────
	for _, h := range t.afterExit {
		_ = h(ctx, m.obj, record)
	}

	// ── 13. Transition AfterEnter (non-aborting) ───────────────────────────
	for _, h := range t.afterEnter {
		_ = h(ctx, m.obj, record)
	}

	// ── 14. History ────────────────────────────────────────────────────────
	m.appendHistory(record)

	// ── 15. Observers ──────────────────────────────────────────────────────
	for _, obs := range m.def.observers {
		obs(ctx, m.obj, record)
	}

	return nil
}

func (m *Machine[C, S, E]) appendHistory(r TransitionRecord[S, E]) {
	cap := m.def.historyCapacity
	if cap == 0 {
		m.history = append(m.history, r)
		return
	}
	if len(m.history) >= cap {
		// Ring-buffer eviction: drop the oldest
		copy(m.history, m.history[1:])
		m.history = m.history[:len(m.history)-1]
	}
	m.history = append(m.history, r)
}

// ─────────────────────────────────────────────────────────────────────────────
// SYNCHRONIZED MACHINE WRAPPER
// For cases where the same machine is accessed from multiple goroutines.
// ─────────────────────────────────────────────────────────────────────────────

// SynchronizedMachine wraps a Machine with a mutex.
// All methods are goroutine-safe.
type SynchronizedMachine[C any, S State, E Event] struct {
	mu *sync.RWMutex
	m  *Machine[C, S, E]
}

// Synchronized wraps a machine to make it goroutine-safe.
func Synchronized[C any, S State, E Event](m *Machine[C, S, E]) *SynchronizedMachine[C, S, E] {
	return &SynchronizedMachine[C, S, E]{mu: &sync.RWMutex{}, m: m}
}

func (s *SynchronizedMachine[C, S, E]) Send(ctx context.Context, envelope EventEnvelope[E]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.Send(ctx, envelope)
}

func (s *SynchronizedMachine[C, S, E]) State() S {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m.State()
}

func (s *SynchronizedMachine[C, S, E]) IsTerminal() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m.IsTerminal()
}

func (s *SynchronizedMachine[C, S, E]) Can(ctx context.Context, event E, envelope EventEnvelope[E]) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m.Can(ctx, event, envelope)
}

func (s *SynchronizedMachine[C, S, E]) History() []TransitionRecord[S, E] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m.History()
}

// ─────────────────────────────────────────────────────────────────────────────
// BATCH PROCESSOR
// Apply the same event to a slice of machines, collecting per-item results.
// Useful for bulk status updates (e.g. cancel all pending orders for a table).
// ─────────────────────────────────────────────────────────────────────────────

// BatchResult holds the outcome for one item in a batch Send.
type BatchResult[S State, E Event] struct {
	Index int
	From  S
	To    S
	Err   error
}

// BatchSend sends the same event to a slice of machines sequentially.
// It never stops on error — all machines are attempted. Check Results for
// per-item outcomes. Returns true only if ALL transitions succeeded.
func BatchSend[C any, S State, E Event](
	ctx context.Context,
	machines []*Machine[C, S, E],
	envelope EventEnvelope[E],
) (allSucceeded bool, results []BatchResult[S, E]) {
	results = make([]BatchResult[S, E], len(machines))
	allSucceeded = true
	for i, m := range machines {
		from := m.State()
		err := m.Send(ctx, envelope)
		results[i] = BatchResult[S, E]{
			Index: i,
			From:  from,
			To:    m.State(),
			Err:   err,
		}
		if err != nil {
			allSucceeded = false
		}
	}
	return allSucceeded, results
}

// ─────────────────────────────────────────────────────────────────────────────
// RESTORE — reconstruct a machine from a stored state (for DB-backed entities)
// ─────────────────────────────────────────────────────────────────────────────

// Restore creates a Machine with a known current state and no history.
// Use this when loading an entity from the database where the current state
// is stored as a string field — you don't replay history, you trust the DB.
func (d *FSMDefinition[C, S, E]) Restore(obj C, currentState S) *Machine[C, S, E] {
	return &Machine[C, S, E]{
		def:     d,
		obj:     obj,
		current: currentState,
		history: make([]TransitionRecord[S, E], 0, min(d.historyCapacity, 8)),
		seq:     0,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// DOMAIN MACHINE DEFINITIONS
// Pre-built definitions for every stateful entity in the platform.
// Import and use these directly in your services.
// ─────────────────────────────────────────────────────────────────────────────

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  ORDER MACHINE                                                           │
// │                                                                          │
// │  pending → confirmed → preparing → ready → served                       │
// │       ↘               ↘           ↘        ↓                           │
// │        cancelled ◄─────────────────────────┘                           │
// └─────────────────────────────────────────────────────────────────────────┘

// OrderState mirrors models.Order status values.
type OrderState string

const (
	OrderStatePending   OrderState = "pending"
	OrderStateConfirmed OrderState = "confirmed"
	OrderStatePreparing OrderState = "preparing"
	OrderStateReady     OrderState = "ready"
	OrderStateServed    OrderState = "served"
	OrderStateCancelled OrderState = "cancelled"
	OrderStateRefunded  OrderState = "refunded"
)

// OrderEvent is the set of events that can drive an Order machine.
type OrderEvent string

const (
	OrderEventConfirm OrderEvent = "confirm" // pending → confirmed
	OrderEventPrepare OrderEvent = "prepare" // confirmed → preparing (KOT sent)
	OrderEventReady   OrderEvent = "ready"   // preparing → ready (kitchen done)
	OrderEventServe   OrderEvent = "serve"   // ready → served
	OrderEventCancel  OrderEvent = "cancel"  // any non-terminal → cancelled
	OrderEventRefund  OrderEvent = "refund"  // served → refunded
)

// OrderContext is the minimal interface an order entity must satisfy.
// This keeps the FSM decoupled from the concrete models.Order type.
type OrderContext interface {
	GetOrderID() string
	GetTotalAmount() float64
	SetStatus(s string)
	GetStatus() string
}

// OrderFSM is the compiled order state machine definition.
// Use OrderFSM.Restore(order, OrderState(order.Status)) in your service.
var OrderFSM = Define[OrderContext, OrderState, OrderEvent]().
	State(OrderStatePending,
		Initial[OrderContext, OrderState, OrderEvent](),
		StateDesc[OrderContext, OrderState, OrderEvent]("Order created, awaiting confirmation"),
	).
	State(OrderStateConfirmed,
		StateDesc[OrderContext, OrderState, OrderEvent]("Order confirmed, awaiting kitchen"),
	).
	State(OrderStatePreparing,
		StateDesc[OrderContext, OrderState, OrderEvent]("Kitchen is preparing the order"),
	).
	State(OrderStateReady,
		StateDesc[OrderContext, OrderState, OrderEvent]("Order ready for service"),
	).
	State(OrderStateServed,
		StateDesc[OrderContext, OrderState, OrderEvent]("Order delivered to customer"),
	).
	State(OrderStateCancelled,
		Terminal[OrderContext, OrderState, OrderEvent](),
		StateDesc[OrderContext, OrderState, OrderEvent]("Order cancelled"),
	).
	State(OrderStateRefunded,
		Terminal[OrderContext, OrderState, OrderEvent](),
		StateDesc[OrderContext, OrderState, OrderEvent]("Order refunded"),
	).
	// ── Transitions ──────────────────────────────────────────────────────────
	Transition(OrderStatePending, OrderEventConfirm, OrderStateConfirmed,
		Guard[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			if o.GetTotalAmount() <= 0 {
				return errors.New("cannot confirm order with zero or negative total")
			}
			return nil
		}),
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateConfirmed))
			return nil
		}),
		Describe[OrderContext, OrderState, OrderEvent]("Payment validated"),
	).
	Transition(OrderStateConfirmed, OrderEventPrepare, OrderStatePreparing,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStatePreparing))
			return nil
		}),
		Describe[OrderContext, OrderState, OrderEvent]("KOT sent to kitchen"),
	).
	Transition(OrderStatePreparing, OrderEventReady, OrderStateReady,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateReady))
			return nil
		}),
		Describe[OrderContext, OrderState, OrderEvent]("Kitchen marks complete"),
	).
	Transition(OrderStateReady, OrderEventServe, OrderStateServed,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateServed))
			return nil
		}),
		Describe[OrderContext, OrderState, OrderEvent]("Waiter delivers to table"),
	).
	Transition(OrderStateServed, OrderEventRefund, OrderStateRefunded,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateRefunded))
			return nil
		}),
		Describe[OrderContext, OrderState, OrderEvent]("Payment reversed"),
	).
	// Cancel is allowed from any non-terminal state
	Transition(OrderStatePending, OrderEventCancel, OrderStateCancelled,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateCancelled))
			return nil
		}),
	).
	Transition(OrderStateConfirmed, OrderEventCancel, OrderStateCancelled,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateCancelled))
			return nil
		}),
	).
	Transition(OrderStatePreparing, OrderEventCancel, OrderStateCancelled,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateCancelled))
			return nil
		}),
		Describe[OrderContext, OrderState, OrderEvent]("Requires manager approval"),
	).
	Transition(OrderStateReady, OrderEventCancel, OrderStateCancelled,
		Effect[OrderContext, OrderState, OrderEvent](func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
			o.SetStatus(string(OrderStateCancelled))
			return nil
		}),
	).
	// Global effect: keep DB Status field in sync on every transition
	GlobalEffect(func(ctx context.Context, o OrderContext, e EventEnvelope[OrderEvent]) error {
		// Status is set by each transition's specific effect above.
		// Add cross-cutting logic here (audit token, updated_at, etc.)
		return nil
	}).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  KOT MACHINE                                                             │
// │                                                                          │
// │  sent → in_progress → completed                                          │
// │    ↘         ↘                                                          │
// │     cancelled                                                            │
// └─────────────────────────────────────────────────────────────────────────┘

// KOTState mirrors models.KOTStatus.
type KOTState string

const (
	KOTStateSent       KOTState = "sent"
	KOTStateInProgress KOTState = "in_progress"
	KOTStateCompleted  KOTState = "completed"
	KOTStateCancelled  KOTState = "cancelled"
)

// KOTEvent drives the KOT machine.
type KOTEvent string

const (
	KOTEventStart    KOTEvent = "start"    // sent → in_progress
	KOTEventComplete KOTEvent = "complete" // in_progress → completed
	KOTEventCancel   KOTEvent = "cancel"   // sent | in_progress → cancelled
)

// KOTContext is the minimal interface a KOT entity must satisfy.
type KOTContext interface {
	GetKOTID() string
	SetStatus(s string)
	GetPriority() string
}

// KOTFSM is the compiled KOT state machine.
var KOTFSM = Define[KOTContext, KOTState, KOTEvent]().
	State(KOTStateSent, Initial[KOTContext, KOTState, KOTEvent]()).
	State(KOTStateInProgress).
	State(KOTStateCompleted, Terminal[KOTContext, KOTState, KOTEvent]()).
	State(KOTStateCancelled, Terminal[KOTContext, KOTState, KOTEvent]()).
	Transition(KOTStateSent, KOTEventStart, KOTStateInProgress,
		Effect[KOTContext, KOTState, KOTEvent](func(ctx context.Context, k KOTContext, e EventEnvelope[KOTEvent]) error {
			k.SetStatus(string(KOTStateInProgress))
			return nil
		}),
	).
	Transition(KOTStateInProgress, KOTEventComplete, KOTStateCompleted,
		Effect[KOTContext, KOTState, KOTEvent](func(ctx context.Context, k KOTContext, e EventEnvelope[KOTEvent]) error {
			k.SetStatus(string(KOTStateCompleted))
			return nil
		}),
	).
	Transition(KOTStateSent, KOTEventCancel, KOTStateCancelled,
		Effect[KOTContext, KOTState, KOTEvent](func(ctx context.Context, k KOTContext, e EventEnvelope[KOTEvent]) error {
			k.SetStatus(string(KOTStateCancelled))
			return nil
		}),
	).
	Transition(KOTStateInProgress, KOTEventCancel, KOTStateCancelled,
		Guard[KOTContext, KOTState, KOTEvent](func(ctx context.Context, k KOTContext, e EventEnvelope[KOTEvent]) error {
			// In-progress KOTs with urgent priority require explicit override
			if k.GetPriority() == "urgent" {
				if override, ok := e.Metadata["force_cancel"]; !ok || override != "true" {
					return errors.New("urgent KOT requires force_cancel=true metadata to cancel mid-preparation")
				}
			}
			return nil
		}),
		Effect[KOTContext, KOTState, KOTEvent](func(ctx context.Context, k KOTContext, e EventEnvelope[KOTEvent]) error {
			k.SetStatus(string(KOTStateCancelled))
			return nil
		}),
	).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  TABLE MACHINE                                                           │
// │                                                                          │
// │  available → occupied → cleaning → available                            │
// │       ↓          ↑                                                      │
// │    reserved ─────┘                                                      │
// │       ↓                                                                 │
// │  out_of_service (terminal)                                              │
// └─────────────────────────────────────────────────────────────────────────┘

// TableState mirrors models.TableStatus.
type TableState string

const (
	TableStateAvailable    TableState = "available"
	TableStateOccupied     TableState = "occupied"
	TableStateReserved     TableState = "reserved"
	TableStateCleaning     TableState = "cleaning"
	TableStateOutOfService TableState = "out_of_service"
)

// TableEvent drives the Table machine.
type TableEvent string

const (
	TableEventSeat    TableEvent = "seat"    // available → occupied
	TableEventReserve TableEvent = "reserve" // available → reserved
	TableEventArrive  TableEvent = "arrive"  // reserved → occupied
	TableEventRelease TableEvent = "release" // occupied → cleaning
	TableEventClean   TableEvent = "clean"   // cleaning → available
	TableEventDisable TableEvent = "disable" // any → out_of_service
)

// TableContext is the minimal interface for a Table entity.
type TableContext interface {
	GetTableID() int
	GetCapacity() int
	SetStatus(s string)
}

// TableFSM is the compiled table state machine.
var TableFSM = Define[TableContext, TableState, TableEvent]().
	State(TableStateAvailable, Initial[TableContext, TableState, TableEvent]()).
	State(TableStateOccupied).
	State(TableStateReserved).
	State(TableStateCleaning).
	State(TableStateOutOfService, Terminal[TableContext, TableState, TableEvent]()).
	Transition(TableStateAvailable, TableEventSeat, TableStateOccupied,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateOccupied))
			return nil
		}),
	).
	Transition(TableStateAvailable, TableEventReserve, TableStateReserved,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateReserved))
			return nil
		}),
	).
	Transition(TableStateReserved, TableEventArrive, TableStateOccupied,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateOccupied))
			return nil
		}),
	).
	Transition(TableStateOccupied, TableEventRelease, TableStateCleaning,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateCleaning))
			return nil
		}),
	).
	Transition(TableStateCleaning, TableEventClean, TableStateAvailable,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateAvailable))
			return nil
		}),
	).
	// Disable is allowed from any non-terminal state
	Transition(TableStateAvailable, TableEventDisable, TableStateOutOfService,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateOutOfService))
			return nil
		}),
	).
	Transition(TableStateOccupied, TableEventDisable, TableStateOutOfService,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateOutOfService))
			return nil
		}),
	).
	Transition(TableStateReserved, TableEventDisable, TableStateOutOfService,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateOutOfService))
			return nil
		}),
	).
	Transition(TableStateCleaning, TableEventDisable, TableStateOutOfService,
		Effect[TableContext, TableState, TableEvent](func(ctx context.Context, t TableContext, e EventEnvelope[TableEvent]) error {
			t.SetStatus(string(TableStateOutOfService))
			return nil
		}),
	).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  SUBSCRIPTION MACHINE                                                    │
// │                                                                          │
// │  trialing → active → past_due → cancelled                               │
// │               ↓         ↑                                               │
// │           suspended ────┘  ← payment failure                           │
// │               ↓                                                         │
// │           cancelled (terminal)                                          │
// └─────────────────────────────────────────────────────────────────────────┘

// SubscriptionState mirrors models.Subscription status values.
type SubscriptionState string

const (
	SubscriptionStateTrialing  SubscriptionState = "trialing"
	SubscriptionStateActive    SubscriptionState = "active"
	SubscriptionStatePastDue   SubscriptionState = "past_due"
	SubscriptionStateSuspended SubscriptionState = "suspended"
	SubscriptionStateCancelled SubscriptionState = "cancelled"
	SubscriptionStateExpired   SubscriptionState = "expired"
)

// SubscriptionEvent drives the Subscription machine.
type SubscriptionEvent string

const (
	SubscriptionEventActivate   SubscriptionEvent = "activate"   // trialing → active
	SubscriptionEventPayFail    SubscriptionEvent = "pay_fail"   // active → past_due
	SubscriptionEventPayOk      SubscriptionEvent = "pay_ok"     // past_due → active
	SubscriptionEventSuspend    SubscriptionEvent = "suspend"    // past_due → suspended
	SubscriptionEventReactivate SubscriptionEvent = "reactivate" // suspended → active
	SubscriptionEventCancel     SubscriptionEvent = "cancel"     // any → cancelled
	SubscriptionEventExpire     SubscriptionEvent = "expire"     // active → expired
)

// SubscriptionContext is the minimal interface for a Subscription entity.
type SubscriptionContext interface {
	GetSubscriptionID() string
	SetStatus(s string)
	GetAutoRenew() bool
}

// SubscriptionFSM is the compiled subscription state machine.
var SubscriptionFSM = Define[SubscriptionContext, SubscriptionState, SubscriptionEvent]().
	State(SubscriptionStateTrialing, Initial[SubscriptionContext, SubscriptionState, SubscriptionEvent]()).
	State(SubscriptionStateActive).
	State(SubscriptionStatePastDue).
	State(SubscriptionStateSuspended).
	State(SubscriptionStateCancelled, Terminal[SubscriptionContext, SubscriptionState, SubscriptionEvent]()).
	State(SubscriptionStateExpired, Terminal[SubscriptionContext, SubscriptionState, SubscriptionEvent]()).
	Transition(SubscriptionStateTrialing, SubscriptionEventActivate, SubscriptionStateActive,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateActive))
			return nil
		}),
	).
	Transition(SubscriptionStateActive, SubscriptionEventPayFail, SubscriptionStatePastDue,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStatePastDue))
			return nil
		}),
	).
	Transition(SubscriptionStatePastDue, SubscriptionEventPayOk, SubscriptionStateActive,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateActive))
			return nil
		}),
	).
	Transition(SubscriptionStatePastDue, SubscriptionEventSuspend, SubscriptionStateSuspended,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateSuspended))
			return nil
		}),
	).
	Transition(SubscriptionStateSuspended, SubscriptionEventReactivate, SubscriptionStateActive,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateActive))
			return nil
		}),
	).
	Transition(SubscriptionStateActive, SubscriptionEventExpire, SubscriptionStateExpired,
		Guard[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			if s.GetAutoRenew() {
				return errors.New("cannot expire subscription with auto-renew enabled — disable auto-renew first")
			}
			return nil
		}),
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateExpired))
			return nil
		}),
	).
	Transition(SubscriptionStateTrialing, SubscriptionEventCancel, SubscriptionStateCancelled,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateCancelled))
			return nil
		}),
	).
	Transition(SubscriptionStateActive, SubscriptionEventCancel, SubscriptionStateCancelled,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateCancelled))
			return nil
		}),
	).
	Transition(SubscriptionStatePastDue, SubscriptionEventCancel, SubscriptionStateCancelled,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateCancelled))
			return nil
		}),
	).
	Transition(SubscriptionStateSuspended, SubscriptionEventCancel, SubscriptionStateCancelled,
		Effect[SubscriptionContext, SubscriptionState, SubscriptionEvent](func(ctx context.Context, s SubscriptionContext, e EventEnvelope[SubscriptionEvent]) error {
			s.SetStatus(string(SubscriptionStateCancelled))
			return nil
		}),
	).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  USER ACCOUNT MACHINE                                                    │
// │                                                                          │
// │  pending_verification → active ⇌ locked                                │
// │                            ↓                                            │
// │                        suspended (terminal)                             │
// │                            ↓                                            │
// │                        deactivated (terminal)                           │
// └─────────────────────────────────────────────────────────────────────────┘

// UserAccountState represents the lifecycle state of a User account.
// This is derived from the combination of IsActive, LockedUntil,
// IsEmailVerified, and MustChangePassword fields on models.User.
type UserAccountState string

const (
	UserStatePendingVerification UserAccountState = "pending_verification"
	UserStateActive              UserAccountState = "active"
	UserStateLocked              UserAccountState = "locked" // failed login lockout
	UserStateMustChangePassword  UserAccountState = "must_change_password"
	UserStateSuspended           UserAccountState = "suspended"   // admin suspension
	UserStateDeactivated         UserAccountState = "deactivated" // soft delete
)

// UserAccountEvent drives the UserAccount machine.
type UserAccountEvent string

const (
	UserEventVerifyEmail         UserAccountEvent = "verify_email"     // pending → active
	UserEventLock                UserAccountEvent = "lock"             // active → locked
	UserEventUnlock              UserAccountEvent = "unlock"           // locked → active
	UserEventForcePasswordChange UserAccountEvent = "force_pw_change"  // active → must_change_password
	UserEventPasswordChanged     UserAccountEvent = "password_changed" // must_change_password → active
	UserEventSuspend             UserAccountEvent = "suspend"          // active → suspended
	UserEventReactivate          UserAccountEvent = "reactivate"       // suspended → active
	UserEventDeactivate          UserAccountEvent = "deactivate"       // any → deactivated
)

// UserAccountContext is the minimal interface for a User entity.
type UserAccountContext interface {
	GetUserID() string
	SetIsActive(v bool)
	SetMustChangePassword(v bool)
	GetFailedLoginAttempts() int
}

// UserAccountFSM is the compiled user lifecycle state machine.
var UserAccountFSM = Define[UserAccountContext, UserAccountState, UserAccountEvent]().
	State(UserStatePendingVerification,
		Initial[UserAccountContext, UserAccountState, UserAccountEvent](),
		StateDesc[UserAccountContext, UserAccountState, UserAccountEvent]("Awaiting email verification"),
	).
	State(UserStateActive,
		StateDesc[UserAccountContext, UserAccountState, UserAccountEvent]("Fully operational account"),
	).
	State(UserStateLocked,
		StateDesc[UserAccountContext, UserAccountState, UserAccountEvent]("Temporarily locked due to failed logins"),
	).
	State(UserStateMustChangePassword,
		StateDesc[UserAccountContext, UserAccountState, UserAccountEvent]("Password reset required before access"),
	).
	State(UserStateSuspended,
		StateDesc[UserAccountContext, UserAccountState, UserAccountEvent]("Admin-suspended, no access"),
	).
	State(UserStateDeactivated,
		Terminal[UserAccountContext, UserAccountState, UserAccountEvent](),
		StateDesc[UserAccountContext, UserAccountState, UserAccountEvent]("Soft-deleted, permanently inaccessible"),
	).
	Transition(UserStatePendingVerification, UserEventVerifyEmail, UserStateActive,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(true)
			return nil
		}),
		Describe[UserAccountContext, UserAccountState, UserAccountEvent]("Email token validated"),
	).
	Transition(UserStateActive, UserEventLock, UserStateLocked,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(false)
			return nil
		}),
		Describe[UserAccountContext, UserAccountState, UserAccountEvent]("Exceeded max failed logins"),
	).
	Transition(UserStateLocked, UserEventUnlock, UserStateActive,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(true)
			return nil
		}),
		Describe[UserAccountContext, UserAccountState, UserAccountEvent]("Lockout period expired or admin override"),
	).
	Transition(UserStateActive, UserEventForcePasswordChange, UserStateMustChangePassword,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetMustChangePassword(true)
			return nil
		}),
	).
	Transition(UserStateMustChangePassword, UserEventPasswordChanged, UserStateActive,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetMustChangePassword(false)
			u.SetIsActive(true)
			return nil
		}),
	).
	Transition(UserStateActive, UserEventSuspend, UserStateSuspended,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(false)
			return nil
		}),
		Describe[UserAccountContext, UserAccountState, UserAccountEvent]("Manual admin action required"),
	).
	Transition(UserStateSuspended, UserEventReactivate, UserStateActive,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(true)
			return nil
		}),
	).
	// Deactivate allowed from any living state
	Transition(UserStateActive, UserEventDeactivate, UserStateDeactivated,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(false)
			return nil
		}),
	).
	Transition(UserStateLocked, UserEventDeactivate, UserStateDeactivated,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(false)
			return nil
		}),
	).
	Transition(UserStateSuspended, UserEventDeactivate, UserStateDeactivated,
		Effect[UserAccountContext, UserAccountState, UserAccountEvent](func(ctx context.Context, u UserAccountContext, e EventEnvelope[UserAccountEvent]) error {
			u.SetIsActive(false)
			return nil
		}),
	).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  PURCHASE ORDER MACHINE                                                  │
// │                                                                          │
// │  draft → submitted → approved → partially_received → received           │
// │    ↘         ↘          ↘                                              │
// │     cancelled (terminal)                                                │
// └─────────────────────────────────────────────────────────────────────────┘

// PurchaseOrderState mirrors models.PurchaseOrder status values.
type PurchaseOrderState string

const (
	POStateDraft             PurchaseOrderState = "draft"
	POStateSubmitted         PurchaseOrderState = "submitted"
	POStateApproved          PurchaseOrderState = "approved"
	POStatePartiallyReceived PurchaseOrderState = "partially_received"
	POStateReceived          PurchaseOrderState = "received"
	POStateCancelled         PurchaseOrderState = "cancelled"
)

// PurchaseOrderEvent drives the PurchaseOrder machine.
type PurchaseOrderEvent string

const (
	POEventSubmit         PurchaseOrderEvent = "submit"
	POEventApprove        PurchaseOrderEvent = "approve"
	POEventReceivePartial PurchaseOrderEvent = "receive_partial"
	POEventReceiveFull    PurchaseOrderEvent = "receive_full"
	POEventCancel         PurchaseOrderEvent = "cancel"
)

// PurchaseOrderContext is the minimal interface for a PurchaseOrder entity.
type PurchaseOrderContext interface {
	GetPOID() string
	GetTotalAmount() float64
	SetStatus(s string)
}

// PurchaseOrderFSM is the compiled purchase order state machine.
var PurchaseOrderFSM = Define[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent]().
	State(POStateDraft, Initial[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent]()).
	State(POStateSubmitted).
	State(POStateApproved).
	State(POStatePartiallyReceived).
	State(POStateReceived, Terminal[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent]()).
	State(POStateCancelled, Terminal[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent]()).
	Transition(POStateDraft, POEventSubmit, POStateSubmitted,
		Guard[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			if po.GetTotalAmount() <= 0 {
				return errors.New("cannot submit purchase order with zero total")
			}
			return nil
		}),
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStateSubmitted))
			return nil
		}),
	).
	Transition(POStateSubmitted, POEventApprove, POStateApproved,
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStateApproved))
			return nil
		}),
	).
	Transition(POStateApproved, POEventReceivePartial, POStatePartiallyReceived,
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStatePartiallyReceived))
			return nil
		}),
	).
	Transition(POStatePartiallyReceived, POEventReceiveFull, POStateReceived,
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStateReceived))
			return nil
		}),
	).
	Transition(POStateApproved, POEventReceiveFull, POStateReceived,
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStateReceived))
			return nil
		}),
	).
	Transition(POStateDraft, POEventCancel, POStateCancelled,
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStateCancelled))
			return nil
		}),
	).
	Transition(POStateSubmitted, POEventCancel, POStateCancelled,
		Effect[PurchaseOrderContext, PurchaseOrderState, PurchaseOrderEvent](func(ctx context.Context, po PurchaseOrderContext, e EventEnvelope[PurchaseOrderEvent]) error {
			po.SetStatus(string(POStateCancelled))
			return nil
		}),
	).
	MustCompile()

// ─────────────────────────────────────────────────────────────────────────────
// INTERNAL HELPERS
// ─────────────────────────────────────────────────────────────────────────────

// transitionKey is the map key for the transition index.
type transitionKey[S State, E Event] struct {
	from  S
	event E
}

// min is a local implementation for pre-Go 1.21 compatibility.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── Append this block to the bottom of utils/FSM.go ─────────────────────────
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │  PAYMENT MACHINE                                                         │
// │                                                                          │
// │  pending_qr ──(qr_scanned)──► awaiting_confirm                         │
// │      │                               │                                  │
// │      │                      (payment_received)                          │
// │      │                               │                                  │
// │      │                               ▼                                  │
// │      │                           confirmed ──(refund)──► refunded       │
// │      │                               │                                  │
// │      │                            (fail)                                │
// │      │                               │                                  │
// │      └──(expire / fail)──────────► failed (terminal)                   │
// └─────────────────────────────────────────────────────────────────────────┘
//
// State semantics:
//   pending_qr        — QR has been generated; waiting for customer to scan
//   awaiting_confirm  — Fonepay callback received; waiting for our server-side
//                       verification call to complete
//   confirmed         — provider.VerifyPayment returned Success=true
//   failed            — verification failed, QR expired, or explicit cancel
//   refunded          — confirmed payment was later reversed

// PaymentState mirrors models.PaymentRecord.Status values.
type PaymentState string

const (
	PaymentStatePendingQR       PaymentState = "pending_qr"
	PaymentStateAwaitingConfirm PaymentState = "awaiting_confirm"
	PaymentStateConfirmed       PaymentState = "confirmed"
	PaymentStateFailed          PaymentState = "failed"
	PaymentStateRefunded        PaymentState = "refunded"
)

// PaymentEvent drives the Payment machine.
type PaymentEvent string

const (
	// PaymentEventQRScanned fires when Fonepay sends the callback indicating
	// the customer has scanned and submitted payment.
	PaymentEventQRScanned PaymentEvent = "qr_scanned"

	// PaymentEventReceived fires after our server-side VerifyPayment call
	// returns Success=true from Fonepay.
	PaymentEventReceived PaymentEvent = "payment_received"

	// PaymentEventFail fires when VerifyPayment returns Success=false,
	// or when a manual failure is recorded (e.g., QR expired).
	PaymentEventFail PaymentEvent = "fail"

	// PaymentEventExpire fires when the QR TTL has elapsed without a scan.
	// Treated as fail — transitions pending_qr → failed.
	PaymentEventExpire PaymentEvent = "expire"

	// PaymentEventRefund fires when a confirmed payment is reversed.
	PaymentEventRefund PaymentEvent = "refund"
)

// PaymentContext is the minimal interface a PaymentRecord entity must satisfy.
type PaymentContext interface {
	GetPaymentID() string
	SetStatus(s string)
	GetAmount() float64
	// GetFailureReason returns the failure message if status is "failed".
	GetFailureReason() string
	// SetFailureReason records the reason when transitioning to "failed".
	SetFailureReason(reason string)
}

// PaymentFSM is the compiled payment state machine.
var PaymentFSM = Define[PaymentContext, PaymentState, PaymentEvent]().
	State(PaymentStatePendingQR, Initial[PaymentContext, PaymentState, PaymentEvent]()).
	State(PaymentStateAwaitingConfirm).
	State(PaymentStateConfirmed).
	State(PaymentStateFailed, Terminal[PaymentContext, PaymentState, PaymentEvent]()).
	State(PaymentStateRefunded, Terminal[PaymentContext, PaymentState, PaymentEvent]()).

	// ── pending_qr → awaiting_confirm ─────────────────────────────────────
	// Fires when Fonepay sends the callback with EncodedParams.
	Transition(PaymentStatePendingQR, PaymentEventQRScanned, PaymentStateAwaitingConfirm,
		Describe[PaymentContext, PaymentState, PaymentEvent]("Fonepay callback received — verifying"),
		Effect[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			p.SetStatus(string(PaymentStateAwaitingConfirm))
			return nil
		}),
	).

	// ── awaiting_confirm → confirmed ──────────────────────────────────────
	// Fires when provider.VerifyPayment returns Success=true.
	Transition(PaymentStateAwaitingConfirm, PaymentEventReceived, PaymentStateConfirmed,
		Describe[PaymentContext, PaymentState, PaymentEvent]("Payment verified by Fonepay"),
		Effect[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			p.SetStatus(string(PaymentStateConfirmed))
			return nil
		}),
	).

	// ── awaiting_confirm → failed ─────────────────────────────────────────
	// Fires when VerifyPayment returns Success=false.
	Transition(PaymentStateAwaitingConfirm, PaymentEventFail, PaymentStateFailed,
		Describe[PaymentContext, PaymentState, PaymentEvent]("Fonepay verification returned failure"),
		Effect[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			p.SetStatus(string(PaymentStateFailed))
			if reason, ok := e.Metadata["failure_reason"]; ok {
				p.SetFailureReason(reason)
			}
			return nil
		}),
	).

	// ── pending_qr → failed (expire) ──────────────────────────────────────
	// Fires when the 30-minute QR TTL elapses.
	Transition(PaymentStatePendingQR, PaymentEventExpire, PaymentStateFailed,
		Describe[PaymentContext, PaymentState, PaymentEvent]("QR expired without scan"),
		Effect[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			p.SetStatus(string(PaymentStateFailed))
			p.SetFailureReason("QR code expired")
			return nil
		}),
	).

	// ── pending_qr → failed (explicit fail) ───────────────────────────────
	Transition(PaymentStatePendingQR, PaymentEventFail, PaymentStateFailed,
		Describe[PaymentContext, PaymentState, PaymentEvent]("Payment explicitly failed or cancelled"),
		Effect[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			p.SetStatus(string(PaymentStateFailed))
			if reason, ok := e.Metadata["failure_reason"]; ok {
				p.SetFailureReason(reason)
			}
			return nil
		}),
	).

	// ── confirmed → refunded ──────────────────────────────────────────────
	Transition(PaymentStateConfirmed, PaymentEventRefund, PaymentStateRefunded,
		Describe[PaymentContext, PaymentState, PaymentEvent]("Confirmed payment refunded"),
		Guard[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			// Require explicit manager override key in envelope metadata
			if v, ok := e.Metadata["manager_approved"]; !ok || v != "true" {
				return errors.New("refund requires manager_approved=true in envelope metadata")
			}
			return nil
		}),
		Effect[PaymentContext, PaymentState, PaymentEvent](func(ctx context.Context, p PaymentContext, e EventEnvelope[PaymentEvent]) error {
			p.SetStatus(string(PaymentStateRefunded))
			return nil
		}),
	).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  INVENTORY ITEM MACHINE                                                  │
// │                                                                          │
// │  active ──(stock_low)──► low_stock ──(stock_depleted)──► out_of_stock  │
// │    ↑                        ↑                                │          │
// │    └──(stock_replenished)───┘──────(stock_replenished)───────┘          │
// │    │                        │                                │          │
// │    └──(discontinue)─►  discontinued  ◄──(discontinue)────────┘          │
// │                        ◄──(discontinue)── low_stock                     │
// └─────────────────────────────────────────────────────────────────────────┘

// InventoryStockState represents the lifecycle state of an InventoryItem.
// This state is derived from CurrentQuantity vs ReorderPoint at restore time
// (no persistent DB column required — same approach as PurchaseOrderFSM).
type InventoryStockState string

const (
	InventoryStateActive       InventoryStockState = "active"
	InventoryStateLowStock     InventoryStockState = "low_stock"
	InventoryStateOutOfStock   InventoryStockState = "out_of_stock"
	InventoryStateDiscontinued InventoryStockState = "discontinued"
)

// InventoryStockEvent drives the InventoryItem machine.
type InventoryStockEvent string

const (
	InventoryEventStockLow         InventoryStockEvent = "stock_low"
	InventoryEventStockDepleted    InventoryStockEvent = "stock_depleted"
	InventoryEventStockReplenished InventoryStockEvent = "stock_replenished"
	InventoryEventDiscontinue      InventoryStockEvent = "discontinue"
	InventoryEventReactivate       InventoryStockEvent = "reactivate"
)

// InventoryItemContext is the minimal interface an InventoryItem entity
// must satisfy for the FSM to operate on it.
type InventoryItemContext interface {
	GetItemID() string
	GetCurrentQuantity() float64
	GetReorderPoint() *float64
	SetStockStatus(s string)
	GetStockStatus() string
}

// InventoryItemFSM is the compiled inventory item state machine.
// Use InventoryItemFSM.Restore(adapter, derivedState) after loading from DB.
var InventoryItemFSM = Define[InventoryItemContext, InventoryStockState, InventoryStockEvent]().
	State(InventoryStateActive,
		Initial[InventoryItemContext, InventoryStockState, InventoryStockEvent](),
		StateDesc[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Item is in stock above reorder point"),
	).
	State(InventoryStateLowStock,
		StateDesc[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Stock at or below reorder point — replenishment needed"),
	).
	State(InventoryStateOutOfStock,
		StateDesc[InventoryItemContext, InventoryStockState, InventoryStockEvent]("No stock remaining"),
	).
	State(InventoryStateDiscontinued,
		Terminal[InventoryItemContext, InventoryStockState, InventoryStockEvent](),
		StateDesc[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Item permanently removed from active inventory"),
	).
	// ── active → low_stock ──────────────────────────────────────────────────
	Transition(InventoryStateActive, InventoryEventStockLow, InventoryStateLowStock,
		Guard[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			rp := item.GetReorderPoint()
			if rp == nil {
				return errors.New("cannot transition to low_stock without a reorder point")
			}
			if item.GetCurrentQuantity() > *rp {
				return fmt.Errorf("quantity %.3f still above reorder point %.3f", item.GetCurrentQuantity(), *rp)
			}
			if item.GetCurrentQuantity() <= 0 {
				return errors.New("quantity is zero — should fire stock_depleted instead")
			}
			return nil
		}),
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateLowStock))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Stock dropped to or below reorder point"),
	).
	// ── active → out_of_stock ───────────────────────────────────────────────
	Transition(InventoryStateActive, InventoryEventStockDepleted, InventoryStateOutOfStock,
		Guard[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			if item.GetCurrentQuantity() > 0 {
				return fmt.Errorf("quantity %.3f is not zero — cannot deplete", item.GetCurrentQuantity())
			}
			return nil
		}),
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateOutOfStock))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Stock completely depleted"),
	).
	// ── low_stock → out_of_stock ────────────────────────────────────────────
	Transition(InventoryStateLowStock, InventoryEventStockDepleted, InventoryStateOutOfStock,
		Guard[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			if item.GetCurrentQuantity() > 0 {
				return fmt.Errorf("quantity %.3f is not zero — cannot deplete", item.GetCurrentQuantity())
			}
			return nil
		}),
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateOutOfStock))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Low stock fully consumed"),
	).
	// ── low_stock → active (replenished) ────────────────────────────────────
	Transition(InventoryStateLowStock, InventoryEventStockReplenished, InventoryStateActive,
		Guard[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			rp := item.GetReorderPoint()
			if rp != nil && item.GetCurrentQuantity() <= *rp {
				return fmt.Errorf("quantity %.3f still at or below reorder point %.3f", item.GetCurrentQuantity(), *rp)
			}
			return nil
		}),
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateActive))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Stock replenished above reorder point"),
	).
	// ── out_of_stock → active (replenished) ─────────────────────────────────
	Transition(InventoryStateOutOfStock, InventoryEventStockReplenished, InventoryStateActive,
		Guard[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			if item.GetCurrentQuantity() <= 0 {
				return errors.New("cannot replenish — quantity is still zero")
			}
			rp := item.GetReorderPoint()
			if rp != nil && item.GetCurrentQuantity() <= *rp {
				return fmt.Errorf("quantity %.3f at or below reorder point %.3f — use stock_low instead", item.GetCurrentQuantity(), *rp)
			}
			return nil
		}),
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateActive))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Out-of-stock item fully restocked"),
	).
	// ── out_of_stock → low_stock (partial replenish) ────────────────────────
	Transition(InventoryStateOutOfStock, InventoryEventStockLow, InventoryStateLowStock,
		Guard[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			if item.GetCurrentQuantity() <= 0 {
				return errors.New("quantity still zero — cannot move to low_stock")
			}
			rp := item.GetReorderPoint()
			if rp != nil && item.GetCurrentQuantity() > *rp {
				return fmt.Errorf("quantity %.3f above reorder point %.3f — use stock_replenished", item.GetCurrentQuantity(), *rp)
			}
			return nil
		}),
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateLowStock))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Partial restock — still below reorder point"),
	).
	// ── discontinue from any non-terminal state ─────────────────────────────
	Transition(InventoryStateActive, InventoryEventDiscontinue, InventoryStateDiscontinued,
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateDiscontinued))
			return nil
		}),
		Describe[InventoryItemContext, InventoryStockState, InventoryStockEvent]("Item discontinued"),
	).
	Transition(InventoryStateLowStock, InventoryEventDiscontinue, InventoryStateDiscontinued,
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateDiscontinued))
			return nil
		}),
	).
	Transition(InventoryStateOutOfStock, InventoryEventDiscontinue, InventoryStateDiscontinued,
		Effect[InventoryItemContext, InventoryStockState, InventoryStockEvent](func(ctx context.Context, item InventoryItemContext, e EventEnvelope[InventoryStockEvent]) error {
			item.SetStockStatus(string(InventoryStateDiscontinued))
			return nil
		}),
	).
	MustCompile()

// DeriveInventoryState computes the FSM state from an InventoryItem's current
// quantity and reorder point. Use this when restoring a machine from the DB.
func DeriveInventoryState(currentQty float64, reorderPoint *float64) InventoryStockState {
	if currentQty <= 0 {
		return InventoryStateOutOfStock
	}
	if reorderPoint != nil && currentQty <= *reorderPoint {
		return InventoryStateLowStock
	}
	return InventoryStateActive
}

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  ANALYTICS PIPELINE MACHINE                                              │
// │                                                                          │
// │  idle → collecting → aggregating → enriching → delivering → completed  │
// │    │         │            │            │           │                     │
// │    └─────────┴────────────┴────────────┴───────────┴──► failed          │
// └─────────────────────────────────────────────────────────────────────────┘

// AnalyticsPipelineState represents the lifecycle of an analytics pipeline run.
type AnalyticsPipelineState string

const (
	APStateIdle        AnalyticsPipelineState = "idle"
	APStateCollecting  AnalyticsPipelineState = "collecting"
	APStateAggregating AnalyticsPipelineState = "aggregating"
	APStateEnriching   AnalyticsPipelineState = "enriching"
	APStateDelivering  AnalyticsPipelineState = "delivering"
	APStateCompleted   AnalyticsPipelineState = "completed"
	APStateFailed      AnalyticsPipelineState = "failed"
)

// AnalyticsPipelineEvent drives the analytics pipeline machine.
type AnalyticsPipelineEvent string

const (
	APEventStartCollect AnalyticsPipelineEvent = "start_collect"
	APEventCollected    AnalyticsPipelineEvent = "collected"
	APEventAggregated   AnalyticsPipelineEvent = "aggregated"
	APEventEnriched     AnalyticsPipelineEvent = "enriched"
	APEventDelivered    AnalyticsPipelineEvent = "delivered"
	APEventFail         AnalyticsPipelineEvent = "fail"
)

// AnalyticsPipelineContext is the minimal interface for a pipeline run entity.
type AnalyticsPipelineContext interface {
	GetPipelineID() string
	SetStage(s string)
	GetRecordCount() int
	SetStartedAt(t time.Time)
	SetCompletedAt(t time.Time)
	SetError(err string)
}

// AnalyticsPipelineFSM is the compiled analytics pipeline state machine.
var AnalyticsPipelineFSM = Define[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]().
	State(APStateIdle, Initial[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]()).
	State(APStateCollecting,
		StateDesc[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Fetching raw data from source tables"),
	).
	State(APStateAggregating,
		StateDesc[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Computing aggregations and rollups"),
	).
	State(APStateEnriching,
		StateDesc[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Computing derived metrics and percentages"),
	).
	State(APStateDelivering,
		StateDesc[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Writing results to cache or response"),
	).
	State(APStateCompleted,
		Terminal[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](),
		StateDesc[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Pipeline run finished successfully"),
	).
	State(APStateFailed,
		Terminal[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](),
		StateDesc[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Pipeline run failed"),
	).
	// ── idle → collecting ───────────────────────────────────────────────────
	Transition(APStateIdle, APEventStartCollect, APStateCollecting,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateCollecting))
			p.SetStartedAt(time.Now())
			return nil
		}),
		Describe[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent]("Pipeline run initiated"),
	).
	// ── collecting → aggregating ────────────────────────────────────────────
	Transition(APStateCollecting, APEventCollected, APStateAggregating,
		Guard[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			if p.GetRecordCount() == 0 {
				return errors.New("cannot aggregate: no records were collected")
			}
			return nil
		}),
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateAggregating))
			return nil
		}),
	).
	// ── aggregating → enriching ─────────────────────────────────────────────
	Transition(APStateAggregating, APEventAggregated, APStateEnriching,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateEnriching))
			return nil
		}),
	).
	// ── enriching → delivering ──────────────────────────────────────────────
	Transition(APStateEnriching, APEventEnriched, APStateDelivering,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateDelivering))
			return nil
		}),
	).
	// ── delivering → completed ──────────────────────────────────────────────
	Transition(APStateDelivering, APEventDelivered, APStateCompleted,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateCompleted))
			p.SetCompletedAt(time.Now())
			return nil
		}),
	).
	// ── fail from any in-flight state ───────────────────────────────────────
	Transition(APStateCollecting, APEventFail, APStateFailed,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(APStateAggregating, APEventFail, APStateFailed,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(APStateEnriching, APEventFail, APStateFailed,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(APStateDelivering, APEventFail, APStateFailed,
		Effect[AnalyticsPipelineContext, AnalyticsPipelineState, AnalyticsPipelineEvent](func(ctx context.Context, p AnalyticsPipelineContext, e EventEnvelope[AnalyticsPipelineEvent]) error {
			p.SetStage(string(APStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	MustCompile()

// ┌─────────────────────────────────────────────────────────────────────────┐
// │  FORECAST PIPELINE MACHINE                                               │
// │                                                                          │
// │  idle → ingesting → feature_engineering → model_training → scoring     │
// │           → post_processing → publishing → completed                    │
// │    │        │             │             │        │           │           │
// │    └────────┴─────────────┴─────────────┴────────┴───────────┴─► failed │
// └─────────────────────────────────────────────────────────────────────────┘

// ForecastPipelineState represents the lifecycle of a forecast pipeline run.
type ForecastPipelineState string

const (
	FPStateIdle               ForecastPipelineState = "idle"
	FPStateIngesting          ForecastPipelineState = "ingesting"
	FPStateFeatureEngineering ForecastPipelineState = "feature_engineering"
	FPStateModelTraining      ForecastPipelineState = "model_training"
	FPStateScoring            ForecastPipelineState = "scoring"
	FPStatePostProcessing     ForecastPipelineState = "post_processing"
	FPStatePublishing         ForecastPipelineState = "publishing"
	FPStateCompleted          ForecastPipelineState = "completed"
	FPStateFailed             ForecastPipelineState = "failed"
)

// ForecastPipelineEvent drives the forecast pipeline machine.
type ForecastPipelineEvent string

const (
	FPEventStartIngest   ForecastPipelineEvent = "start_ingest"
	FPEventIngested      ForecastPipelineEvent = "ingested"
	FPEventFeaturesReady ForecastPipelineEvent = "features_ready"
	FPEventModelTrained  ForecastPipelineEvent = "model_trained"
	FPEventScored        ForecastPipelineEvent = "scored"
	FPEventPostProcessed ForecastPipelineEvent = "post_processed"
	FPEventPublished     ForecastPipelineEvent = "published"
	FPEventFail          ForecastPipelineEvent = "fail"
)

// ForecastPipelineContext is the minimal interface for a forecast pipeline run.
type ForecastPipelineContext interface {
	GetPipelineID() string
	SetStage(s string)
	GetDataPointCount() int
	GetModelAccuracy() float64
	SetStartedAt(t time.Time)
	SetCompletedAt(t time.Time)
	SetError(err string)
}

// ForecastPipelineFSM is the compiled forecast pipeline state machine.
var ForecastPipelineFSM = Define[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]().
	State(FPStateIdle, Initial[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]()).
	State(FPStateIngesting,
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Loading historical data from orders and stock movements"),
	).
	State(FPStateFeatureEngineering,
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Computing features: seasonality indices, day-of-week weights, trend components"),
	).
	State(FPStateModelTraining,
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Fitting exponential smoothing model with seasonal decomposition"),
	).
	State(FPStateScoring,
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Generating point forecasts and confidence intervals"),
	).
	State(FPStatePostProcessing,
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Applying business rules, floor/ceiling constraints, rounding"),
	).
	State(FPStatePublishing,
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Writing forecast results to cache and response"),
	).
	State(FPStateCompleted,
		Terminal[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](),
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Forecast pipeline completed successfully"),
	).
	State(FPStateFailed,
		Terminal[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](),
		StateDesc[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent]("Forecast pipeline failed"),
	).
	// ── idle → ingesting ────────────────────────────────────────────────────
	Transition(FPStateIdle, FPEventStartIngest, FPStateIngesting,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateIngesting))
			p.SetStartedAt(time.Now())
			return nil
		}),
	).
	// ── ingesting → feature_engineering ─────────────────────────────────────
	Transition(FPStateIngesting, FPEventIngested, FPStateFeatureEngineering,
		Guard[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			if p.GetDataPointCount() < 7 {
				return fmt.Errorf("insufficient data: need at least 7 data points, got %d", p.GetDataPointCount())
			}
			return nil
		}),
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFeatureEngineering))
			return nil
		}),
	).
	// ── feature_engineering → model_training ────────────────────────────────
	Transition(FPStateFeatureEngineering, FPEventFeaturesReady, FPStateModelTraining,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateModelTraining))
			return nil
		}),
	).
	// ── model_training → scoring ────────────────────────────────────────────
	Transition(FPStateModelTraining, FPEventModelTrained, FPStateScoring,
		Guard[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			if p.GetModelAccuracy() < 0.0 {
				return fmt.Errorf("model accuracy %.4f is negative — training diverged", p.GetModelAccuracy())
			}
			return nil
		}),
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateScoring))
			return nil
		}),
	).
	// ── scoring → post_processing ───────────────────────────────────────────
	Transition(FPStateScoring, FPEventScored, FPStatePostProcessing,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStatePostProcessing))
			return nil
		}),
	).
	// ── post_processing → publishing ────────────────────────────────────────
	Transition(FPStatePostProcessing, FPEventPostProcessed, FPStatePublishing,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStatePublishing))
			return nil
		}),
	).
	// ── publishing → completed ──────────────────────────────────────────────
	Transition(FPStatePublishing, FPEventPublished, FPStateCompleted,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateCompleted))
			p.SetCompletedAt(time.Now())
			return nil
		}),
	).
	// ── fail from any in-flight state ───────────────────────────────────────
	Transition(FPStateIngesting, FPEventFail, FPStateFailed,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(FPStateFeatureEngineering, FPEventFail, FPStateFailed,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(FPStateModelTraining, FPEventFail, FPStateFailed,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(FPStateScoring, FPEventFail, FPStateFailed,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(FPStatePostProcessing, FPEventFail, FPStateFailed,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	Transition(FPStatePublishing, FPEventFail, FPStateFailed,
		Effect[ForecastPipelineContext, ForecastPipelineState, ForecastPipelineEvent](func(ctx context.Context, p ForecastPipelineContext, e EventEnvelope[ForecastPipelineEvent]) error {
			p.SetStage(string(FPStateFailed))
			if reason, ok := e.Metadata["error"]; ok {
				p.SetError(reason)
			}
			return nil
		}),
	).
	MustCompile()
