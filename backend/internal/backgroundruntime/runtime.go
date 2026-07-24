package backgroundruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	StateActive     = "active"
	StateStandby    = "standby"
	StateActivating = "activating"
	StateFailed     = "failed"
	StateStopping   = "stopping"
)

var slotPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type task struct {
	name    string
	start   func() error
	started bool
}

type Runtime struct {
	mu sync.RWMutex
	// activationMu serializes a complete activation with the shutdown barrier.
	// Task start functions are expected to be bounded and only launch workers.
	activationMu sync.Mutex

	state     string
	slot      string
	statePath string
	lastError string
	tasks     []*task
}

type Snapshot struct {
	State     string `json:"state"`
	Slot      string `json:"slot,omitempty"`
	LastError string `json:"error,omitempty"`
}

var global = &Runtime{state: StateActive}

// ConfigureFromEnv puts managed deployment candidates in standby until the
// host deployer sends SIGUSR1. A persisted slot marker makes container restarts
// resume in active mode after a successful promotion.
func ConfigureFromEnv() error {
	standby := parseBool(os.Getenv("DEPLOYMENT_STANDBY"))
	slot := strings.TrimSpace(os.Getenv("DEPLOYMENT_SLOT"))
	statePath := strings.TrimSpace(os.Getenv("DEPLOYMENT_STATE_FILE"))
	if !standby {
		global.configure(StateActive, "", "")
		return nil
	}
	if !slotPattern.MatchString(slot) {
		return errors.New("DEPLOYMENT_SLOT is required and must contain only letters, numbers, dot, underscore, or hyphen")
	}
	if statePath == "" || !filepath.IsAbs(statePath) {
		return errors.New("DEPLOYMENT_STATE_FILE must be an absolute path")
	}

	state := StateStandby
	if data, err := os.ReadFile(statePath); err == nil && strings.TrimSpace(string(data)) == slot {
		state = StateActive
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read deployment state: %w", err)
	}
	global.configure(state, slot, statePath)
	return nil
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (r *Runtime) configure(state, slot, statePath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
	r.slot = slot
	r.statePath = statePath
	r.lastError = ""
	r.tasks = nil
}

// Register starts a process-wide background service immediately on ordinary
// instances, or queues it until promotion on a managed standby candidate.
func Register(name string, start func() error) error {
	if strings.TrimSpace(name) == "" || start == nil {
		return errors.New("background runtime task requires a name and start function")
	}
	return global.register(name, start)
}

func (r *Runtime) register(name string, start func() error) error {
	r.mu.Lock()
	t := &task{name: name, start: start}
	r.tasks = append(r.tasks, t)
	active := r.state == StateActive
	r.mu.Unlock()
	if !active {
		return nil
	}
	if err := runTask(t); err != nil {
		return err
	}
	r.mu.Lock()
	t.started = true
	r.mu.Unlock()
	return nil
}

func runTask(t *task) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("background task %s panicked: %v", t.name, recovered)
		}
	}()
	if err := t.start(); err != nil {
		return fmt.Errorf("start background task %s: %w", t.name, err)
	}
	return nil
}

// Activate verifies the host-owned slot marker and starts every deferred
// background service exactly once. It is safe to call repeatedly.
func Activate() error {
	return global.activate()
}

func (r *Runtime) activate() error {
	r.activationMu.Lock()
	defer r.activationMu.Unlock()

	r.mu.Lock()
	if r.state == StateActive {
		r.mu.Unlock()
		return nil
	}
	if r.state == StateStopping {
		r.mu.Unlock()
		return errors.New("background runtime is stopping")
	}
	if r.state == StateActivating {
		r.mu.Unlock()
		return errors.New("background runtime activation is already in progress")
	}
	r.state = StateActivating
	r.lastError = ""
	statePath := r.statePath
	slot := r.slot
	r.mu.Unlock()

	if err := verifyMarker(statePath, slot); err != nil {
		r.fail(err)
		return err
	}
	for {
		r.mu.Lock()
		var next *task
		for _, candidate := range r.tasks {
			if !candidate.started {
				next = candidate
				break
			}
		}
		if next == nil {
			r.state = StateActive
			r.lastError = ""
			r.mu.Unlock()
			return nil
		}
		r.mu.Unlock()
		if err := runTask(next); err != nil {
			r.fail(err)
			return err
		}
		r.mu.Lock()
		next.started = true
		r.mu.Unlock()
	}
}

// BeginShutdown prevents deferred background services from being activated
// after process shutdown has started. Already-active services keep running
// until the application cleanup phase stops them after HTTP draining.
func BeginShutdown() {
	global.beginShutdown()
}

func (r *Runtime) beginShutdown() {
	// Wait for an activation already in progress to finish so cleanup can never
	// race a service Start method. Normal task starts only launch workers.
	r.activationMu.Lock()
	defer r.activationMu.Unlock()
	r.mu.Lock()
	r.state = StateStopping
	r.lastError = ""
	r.mu.Unlock()
}

func (r *Runtime) fail(err error) {
	r.mu.Lock()
	r.state = StateFailed
	r.lastError = err.Error()
	r.mu.Unlock()
}

func verifyMarker(path, slot string) error {
	if path == "" {
		return errors.New("deployment state path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read deployment state marker: %w", err)
	}
	if strings.TrimSpace(string(data)) != slot {
		return fmt.Errorf("deployment state marker does not authorize slot %s", slot)
	}
	return nil
}

func Status() Snapshot {
	global.mu.RLock()
	defer global.mu.RUnlock()
	return Snapshot{State: global.state, Slot: global.slot, LastError: global.lastError}
}

// IsActive reports whether this process is authorized to start work that
// mutates shared deployment state. Managed candidates remain inactive until
// every deferred background service has started successfully.
func IsActive() bool {
	global.mu.RLock()
	defer global.mu.RUnlock()
	return global.state == StateActive
}
