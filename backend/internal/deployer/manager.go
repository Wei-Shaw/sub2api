package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	versionPattern   = regexp.MustCompile(`^[0-9][0-9A-Za-z.-]{0,63}$`)
	requestIDPattern = regexp.MustCompile(`^[0-9A-Za-z._:-]{8,128}$`)
	versionOutputRE  = regexp.MustCompile(`(?m)Sub2API\s+([^\s]+)`)
	containerIDRE    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const (
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
)

const startupHealthRetryInterval = 250 * time.Millisecond

const (
	maxJobHistory = 32
	jobHistoryTTL = 30 * 24 * time.Hour
)

var (
	ErrJobRunning        = errors.New("a deployment job is already running")
	ErrRequestConflict   = errors.New("request id was already used for a different deployment")
	ErrVersionConflict   = errors.New("active version does not match expected current version")
	ErrJobNotFound       = errors.New("deployment job not found")
	ErrDeployerDegraded  = errors.New("deployer is degraded and requires operator reconciliation")
	ErrDrainUnobservable = errors.New("previous application does not expose drain blockers")
	ErrDrainTimeout      = errors.New("previous application drain timed out")
)

type Manager struct {
	cfg        Config
	runner     CommandRunner
	httpClient *http.Client
	now        func() time.Time

	mu    sync.RWMutex
	state State
}

type containerRef struct {
	Name string
	ID   string
}

func (j *Job) oldContainerRef() containerRef {
	return containerRef{Name: j.OldContainer, ID: j.OldContainerID}
}

func (j *Job) candidateContainerRef() containerRef {
	return containerRef{Name: j.CandidateContainer, ID: j.CandidateContainerID}
}

func NewManager(cfg Config, runner CommandRunner) (*Manager, error) {
	if runner == nil {
		runner = ExecRunner{}
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		cfg:        cfg,
		runner:     runner,
		httpClient: &http.Client{Timeout: 4 * time.Second},
		now:        time.Now,
		state:      state,
	}
	if err := validateStateContainerIdentities(state); err != nil {
		return nil, err
	}
	startupCtx, cancel := context.WithTimeout(context.Background(), m.cfg.HealthTimeout.Duration)
	defer cancel()
	if err := m.bindLegacyContainerIDs(startupCtx); err != nil {
		return nil, err
	}
	if err := m.bootstrap(startupCtx); err != nil {
		return nil, err
	}
	if err := m.recoverInterruptedJob(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Health() Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	running := m.state.Job != nil && m.state.Job.Status == JobStatusRunning
	degraded := m.state.Degraded
	status := "ok"
	if degraded {
		status = "unhealthy"
	}
	return Health{
		Status:          status,
		Version:         "2.0.0",
		ActiveSlot:      m.state.ActiveSlot,
		ActiveContainer: m.state.ActiveContainer,
		ActivePort:      m.state.ActivePort,
		ActiveVersion:   m.state.ActiveVersion,
		JobRunning:      running,
		Degraded:        degraded,
		DegradedReason:  m.state.DegradedReason,
	}
}

func (m *Manager) Job(id string) (*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state.Job != nil && (id == "" || m.state.Job.ID == id) {
		job := *m.state.Job
		return &job, nil
	}
	if id != "" {
		for i := range m.state.JobHistory {
			if m.state.JobHistory[i].ID == id {
				job := m.state.JobHistory[i]
				return &job, nil
			}
		}
	}
	if m.state.Job == nil || id != "" {
		return nil, ErrJobNotFound
	}
	return nil, ErrJobNotFound
}

func (m *Manager) Reconcile(ctx context.Context, slotName string) error {
	return m.ReconcileWithOptions(ctx, slotName, false)
}

func (m *Manager) ReconcileWithOptions(ctx context.Context, slotName string, allowUnobservableDrain bool) error {
	if err := RequireDaemonStopped(m.cfg.SocketPath); err != nil {
		return err
	}
	slotName = strings.TrimSpace(slotName)
	slot, ok := m.slotByName(slotName)
	if !ok {
		return fmt.Errorf("deployment slot %q is not configured", slotName)
	}

	m.mu.RLock()
	if !m.state.Degraded {
		m.mu.RUnlock()
		return errors.New("deployer is not degraded")
	}
	container := m.containerForSlotLocked(slotName)
	oldActiveContainer := m.state.ActiveContainer
	oldActiveContainerID := m.state.ActiveContainerID
	oldActiveSlot := m.state.ActiveSlot
	oldActivePort := m.state.ActivePort
	oldActiveVersion := m.state.ActiveVersion
	oldActiveImage := m.state.ActiveImage
	var interruptedJob *Job
	if m.state.Job != nil {
		job := *m.state.Job
		interruptedJob = &job
	}
	m.mu.RUnlock()
	if container.Name == "" || container.ID == "" {
		return fmt.Errorf("no known container belongs to deployment slot %q", slotName)
	}

	if err := m.markDegraded("operator reconciliation in progress for slot " + slotName); err != nil {
		return fmt.Errorf("persist reconciliation intent: %w", err)
	}
	if _, found, err := m.inspectContainerRef(ctx, container); err != nil || !found {
		return fmt.Errorf("selected container identity is not valid: %w", err)
	}
	if err := m.containerHealthy(ctx, container, slot.Port); err != nil {
		return fmt.Errorf("selected container is not healthy: %w", err)
	}
	health, err := m.applicationHealth(ctx, slot.Port)
	if err != nil {
		return fmt.Errorf("read selected container runtime state: %w", err)
	}
	runtimeSlot := strings.TrimSpace(health.DeploymentRuntime.Slot)
	allowEmpty := container.Name == m.cfg.InitialContainer
	if runtimeSlot != slotName && (!allowEmpty || runtimeSlot != "") {
		return fmt.Errorf("selected container reports deployment runtime slot %q, expected %q", runtimeSlot, slotName)
	}
	if err := m.validateManagedRoute(ctx, slot.Port); err != nil {
		return fmt.Errorf("selected slot is not the configured Nginx route: %w", err)
	}
	if err := m.verifyRoutedHealth(ctx, slotName, allowEmpty); err != nil {
		return fmt.Errorf("selected slot is not serving through Nginx: %w", err)
	}
	var handedOffOld containerRef
	if interruptedJob != nil && interruptedJob.CandidateSlot == slotName && interruptedJob.TrafficState == TrafficStateCandidate && interruptedJob.OldContainer != "" {
		if err := m.prepareOldContainerHandoff(ctx, interruptedJob.ID); err != nil {
			return fmt.Errorf("persist previous container handoff before reconciliation: %w", err)
		}
		currentJob, err := m.Job(interruptedJob.ID)
		if err != nil {
			return err
		}
		if err := m.completeOldContainerHandoff(ctx, currentJob, slot.Port, slot.Name, allowEmpty, allowUnobservableDrain); err != nil {
			return fmt.Errorf("previous container is not safe to hand off: %w", err)
		}
		handedOffOld = currentJob.oldContainerRef()
	}

	for _, other := range m.otherKnownContainers(slotName, container) {
		if other == handedOffOld {
			continue
		}
		if err := m.stopContainer(ctx, other); err != nil {
			return fmt.Errorf("stop non-serving container %s: %w", other.Name, err)
		}
	}
	if err := m.ensureBackgroundActive(ctx, container, slot.Port, slot.Name); err != nil {
		return fmt.Errorf("activate selected deployment runtime: %w", err)
	}
	if err := m.verifyRoutedHealth(ctx, slotName, allowEmpty); err != nil {
		return fmt.Errorf("selected slot failed routed verification after activation: %w", err)
	}
	version, err := m.inspectContainerVersion(ctx, container.ID)
	if err != nil {
		return fmt.Errorf("inspect selected container version: %w", err)
	}
	image, err := m.inspectContainerImage(ctx, container.ID)
	if err != nil {
		return fmt.Errorf("inspect selected container image: %w", err)
	}
	if strings.TrimSpace(image) == "" {
		return errors.New("selected container image is empty")
	}
	line := m.cfg.ImageEnvironment + "=" + image + "\n"
	if err := atomicWrite(m.cfg.ImageStatePath, []byte(line), 0600); err != nil {
		return fmt.Errorf("persist reconciled image state: %w", err)
	}

	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneState(m.state)
	if oldActiveContainer != "" && oldActiveContainer != container.Name {
		next.PreviousSlot = oldActiveSlot
		next.PreviousContainer = oldActiveContainer
		next.PreviousContainerID = oldActiveContainerID
		next.PreviousPort = oldActivePort
		next.PreviousVersion = oldActiveVersion
		next.PreviousImage = oldActiveImage
	}
	next.ActiveSlot = slot.Name
	next.ActiveContainer = container.Name
	next.ActiveContainerID = container.ID
	next.ActivePort = slot.Port
	next.ActiveVersion = version
	next.ActiveImage = image
	next.Degraded = false
	next.DegradedReason = ""
	next.UpdatedAt = now
	if next.Job != nil && next.Job.Status != JobStatusSucceeded && next.Job.Status != JobStatusFailed {
		completedCandidate := next.Job.Status == JobStatusDegraded && next.Job.CandidateSlot == slot.Name && next.Job.TrafficState == TrafficStateCandidate
		if completedCandidate {
			next.Job.Status = JobStatusSucceeded
			next.Job.Stage = StageCompleted
			next.Job.Message = "Operator completed deployment reconciliation to slot " + slot.Name
			next.Job.CleanupWarning = next.Job.Error
			next.Job.Error = ""
			next.Job.BackgroundActivated = true
		} else {
			next.Job.Status = JobStatusFailed
			next.Job.Stage = StageFailed
			next.Job.Message = "Operator reconciled deployment to slot " + slot.Name
		}
		if next.Job.CandidateSlot == slot.Name {
			next.Job.TrafficState = TrafficStateCandidate
			next.Job.TrafficSwitched = true
		} else {
			next.Job.TrafficState = TrafficStateOld
			next.Job.TrafficSwitched = false
		}
		next.Job.UpdatedAt = now
		next.Job.FinishedAt = &now
	}
	if err := saveState(m.cfg.StatePath, next); err != nil {
		m.latchDegradedInMemoryLocked("reconciled runtime but could not persist reconciled state: " + err.Error())
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) Start(req DeployRequest) (*Job, error) {
	req.Action = strings.TrimSpace(req.Action)
	req.TargetVersion = strings.TrimPrefix(strings.TrimSpace(req.TargetVersion), "v")
	req.ExpectedCurrentVersion = strings.TrimPrefix(strings.TrimSpace(req.ExpectedCurrentVersion), "v")
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.Action != "update" && req.Action != "rollback" {
		return nil, errors.New("action must be update or rollback")
	}
	if !versionPattern.MatchString(req.TargetVersion) {
		return nil, errors.New("invalid target version")
	}
	if !requestIDPattern.MatchString(req.RequestID) {
		return nil, errors.New("invalid request id")
	}

	m.mu.Lock()
	if previous := findJobByRequestID(m.state, req.RequestID); previous != nil {
		if !jobMatchesRequest(previous, req) {
			m.mu.Unlock()
			return nil, ErrRequestConflict
		}
		job := *previous
		m.mu.Unlock()
		return &job, nil
	}
	if m.state.Degraded {
		m.mu.Unlock()
		return nil, ErrDeployerDegraded
	}
	if m.state.Job != nil && m.state.Job.Status == JobStatusRunning {
		m.mu.Unlock()
		return nil, ErrJobRunning
	}
	if req.ExpectedCurrentVersion != "" && m.state.ActiveVersion != "" && req.ExpectedCurrentVersion != strings.TrimPrefix(m.state.ActiveVersion, "v") {
		m.mu.Unlock()
		return nil, ErrVersionConflict
	}
	now := m.now().UTC()
	job := &Job{
		ID:              req.RequestID,
		Action:          req.Action,
		TargetVersion:   req.TargetVersion,
		ExpectedCurrent: req.ExpectedCurrentVersion,
		Status:          JobStatusRunning,
		Stage:           StagePulling,
		Message:         "Pulling target image",
		FromVersion:     m.state.ActiveVersion,
		FromImage:       m.state.ActiveImage,
		OldContainer:    m.state.ActiveContainer,
		OldContainerID:  m.state.ActiveContainerID,
		OldSlot:         m.state.ActiveSlot,
		TrafficState:    TrafficStateOld,
		CreatedAt:       now,
		StartedAt:       now,
		UpdatedAt:       now,
	}
	next := cloneState(m.state)
	archiveTerminalJob(&next, now)
	next.Job = job
	next.UpdatedAt = now
	if err := saveState(m.cfg.StatePath, next); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.state = next
	copy := *job
	m.mu.Unlock()

	go m.execute(req.RequestID)
	return &copy, nil
}

func (m *Manager) bootstrap(ctx context.Context) error {
	if m.state.ActiveContainer != "" && m.state.ActivePort != 0 {
		if m.state.Job != nil && (m.state.Job.Status == JobStatusRollbackFailed || m.state.Job.Status == JobStatusDegraded) && !m.state.Degraded {
			if err := m.markDegraded("automatic rollback previously failed: " + m.state.Job.RollbackError); err != nil {
				return err
			}
		}
		if m.state.Degraded {
			return nil
		}
		if m.state.Job != nil && m.state.Job.Status == JobStatusRunning {
			return nil
		}
		if err := m.validateManagedRoute(ctx, m.state.ActivePort); err != nil {
			if persistErr := m.markDegraded("startup route reconciliation failed: " + err.Error()); persistErr != nil {
				return persistErr
			}
			return nil
		}
		if err := m.waitForActiveDeploymentOnStartup(ctx); err != nil {
			if persistErr := m.markDegraded("startup routed health reconciliation failed: " + err.Error()); persistErr != nil {
				return persistErr
			}
		}
		return nil
	}
	upstreamData, readErr := os.ReadFile(m.cfg.NginxUpstreamPath)
	port, err := readManagedUpstreamPort(upstreamData, m.cfg.NginxUpstreamName)
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		port = m.cfg.Slots[0].Port
		if writeErr := m.writeUpstream(port); writeErr != nil {
			return fmt.Errorf("initialize nginx upstream: %w", writeErr)
		}
	}
	slot, ok := m.slotByPort(port)
	if !ok {
		return fmt.Errorf("nginx upstream port %d is not a configured deployment slot", port)
	}
	initial, found, inspectErr := m.inspectContainerRef(ctx, containerRef{Name: m.cfg.InitialContainer})
	if inspectErr != nil {
		return fmt.Errorf("inspect initial container %q: %w", m.cfg.InitialContainer, inspectErr)
	}
	if !found {
		return fmt.Errorf("initial container %q does not exist", m.cfg.InitialContainer)
	}
	image, imageErr := m.inspectContainerImage(ctx, initial.ID)
	if imageErr != nil {
		return fmt.Errorf("inspect initial container image: %w", imageErr)
	}
	if strings.TrimSpace(image) == "" {
		return errors.New("initial container image is empty")
	}
	version, err := m.inspectContainerVersion(ctx, initial.ID)
	if err != nil {
		return fmt.Errorf("inspect initial container version: %w", err)
	}
	configuredVersion := strings.TrimPrefix(strings.TrimSpace(m.cfg.InitialVersion), "v")
	if configuredVersion != "" && version != configuredVersion {
		return fmt.Errorf("initial container version mismatch: configured %s, got %s", configuredVersion, version)
	}
	if err := m.validateManagedRoute(ctx, port); err != nil {
		return fmt.Errorf("validate initial managed route: %w", err)
	}
	if err := m.verifyRoutedHealth(ctx, slot.Name, true); err != nil {
		return fmt.Errorf("verify initial routed health: %w", err)
	}
	next := cloneState(m.state)
	next.ActiveSlot = slot.Name
	next.ActiveContainer = m.cfg.InitialContainer
	next.ActiveContainerID = initial.ID
	next.ActivePort = slot.Port
	next.ActiveImage = image
	next.ActiveVersion = version
	next.UpdatedAt = m.now().UTC()
	if err := saveState(m.cfg.StatePath, next); err != nil {
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) waitForActiveDeploymentOnStartup(ctx context.Context) error {
	m.mu.RLock()
	activeContainer := containerRef{Name: m.state.ActiveContainer, ID: m.state.ActiveContainerID}
	activePort := m.state.ActivePort
	expectedSlot := m.state.ActiveSlot
	allowEmpty := activeContainer.Name == m.cfg.InitialContainer
	m.mu.RUnlock()

	var lastErr error
	for {
		if err := m.validateManagedRoute(ctx, activePort); err != nil {
			return fmt.Errorf("active Nginx route became unknown or changed: %w", err)
		}
		if err := m.containerHealthy(ctx, activeContainer, activePort); err != nil {
			lastErr = fmt.Errorf("active container is not ready: %w", err)
		} else {
			health, err := m.fetchApplicationHealth(ctx, m.cfg.NginxProbeURL, m.cfg.NginxProbeHost)
			if err != nil {
				lastErr = fmt.Errorf("active route is not ready: %w", err)
			} else {
				actualSlot := strings.TrimSpace(health.DeploymentRuntime.Slot)
				if (expectedSlot != "" && actualSlot == expectedSlot) || (allowEmpty && actualSlot == "") {
					return nil
				}
				return fmt.Errorf("routed deployment runtime slot is %q, expected %q", actualSlot, expectedSlot)
			}
		}
		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return fmt.Errorf("active deployment health timeout: %w", lastErr)
		case <-time.After(startupHealthRetryInterval):
		}
	}
}

func (m *Manager) recoverInterruptedJob() error {
	m.mu.RLock()
	if m.state.Degraded || m.state.Job == nil || m.state.Job.Status != JobStatusRunning {
		m.mu.RUnlock()
		return nil
	}
	job := *m.state.Job
	m.mu.RUnlock()
	normalizeLegacyTrafficState(&job)

	ctx, cancel := context.WithTimeout(context.Background(), m.recoveryTimeout())
	defer cancel()
	var recoveryErr error
	if job.Stage != StageRollingBack && job.TrafficState == TrafficStateCandidate && job.CandidateContainer != "" {
		allowEmptyCandidateSlot := job.CandidateContainer == m.cfg.InitialContainer
		recoveryErr = m.waitForCandidate(ctx, job.candidateContainerRef(), job.CandidatePort, job.TargetVersion, false, job.CandidateSlot, job.CandidateContainer == m.cfg.InitialContainer)
		oldAlreadyStopped := false
		if recoveryErr == nil && job.HandoffPrepared {
			if bindingErr := validateHandoffBinding(&job); bindingErr != nil {
				return m.degradeDrain(job.ID, fmt.Errorf("invalid previous container handoff after restart: %w", bindingErr))
			}
			_, oldRunning, runningErr := m.inspectContainerRunning(ctx, job.oldContainerRef())
			if runningErr != nil {
				return m.degradeDrain(job.ID, fmt.Errorf("inspect journal-bound previous container after restart: %w", runningErr))
			}
			oldAlreadyStopped = !oldRunning
		}
		if recoveryErr == nil && !oldAlreadyStopped {
			recoveryErr = m.stabilize(ctx, job.candidateContainerRef(), job.CandidatePort)
		}
		if recoveryErr == nil {
			recoveryErr = m.validateManagedRoute(ctx, job.CandidatePort)
		}
		if recoveryErr == nil {
			recoveryErr = m.verifyRoutedHealth(ctx, job.CandidateSlot, allowEmptyCandidateSlot)
		}
		if recoveryErr == nil {
			recoveryErr = m.prepareOldContainerHandoff(ctx, job.ID)
			if recoveryErr != nil {
				return m.degradeDrain(job.ID, fmt.Errorf("could not establish a durable previous container handoff after restart: %w", recoveryErr))
			}
		}
		if recoveryErr == nil {
			currentJob, err := m.Job(job.ID)
			if err != nil {
				return err
			}
			job = *currentJob
			recoveryErr = m.completeOldContainerHandoff(ctx, &job, job.CandidatePort, job.CandidateSlot, allowEmptyCandidateSlot, false)
			if recoveryErr != nil {
				return m.degradeDrain(job.ID, fmt.Errorf("could not safely complete the previous container handoff after restart: %w", recoveryErr))
			}
		}
		if recoveryErr == nil {
			recoveryErr = m.updateJob(job.ID, StageActivating, "Activating candidate background services after recovery", nil)
			if recoveryErr != nil {
				return m.degradeDrain(job.ID, fmt.Errorf("previous container handoff completed but activation intent could not be persisted after restart: %w", recoveryErr))
			}
		}
		if recoveryErr == nil {
			recoveryErr = m.ensureBackgroundActive(ctx, job.candidateContainerRef(), job.CandidatePort, job.CandidateSlot)
			if recoveryErr == nil {
				job.BackgroundActivated = true
				recoveryErr = m.updateJob(job.ID, StageActivating, "Candidate background services are active", func(current *Job) {
					current.BackgroundActivated = true
				})
			}
		}
		if recoveryErr == nil {
			current, err := m.Job(job.ID)
			if err != nil {
				return err
			}
			recoveryErr = m.persistSuccessfulDeployment(current)
		}
		if recoveryErr == nil {
			return m.finishRecoveredJob(job, JobStatusSucceeded, "Recovered deployment after deployer restart", "")
		}
	}

	rollbackErr := m.restoreOldDeployment(ctx, &job, true)
	if rollbackErr != nil {
		if recoveryErr != nil {
			rollbackErr = fmt.Errorf("candidate recovery failed: %v; previous deployment restoration failed: %w", recoveryErr, rollbackErr)
		}
		return m.finishRecoveredJob(job, JobStatusRollbackFailed, "Recovery could not restore the previous deployment", rollbackErr.Error())
	}
	job.RollbackPerformed = trafficMayHaveSwitched(&job)
	jobError := "deployer restarted during deployment"
	if recoveryErr != nil {
		jobError += "; candidate recovery failed: " + recoveryErr.Error()
	}
	return m.finishRecoveredJob(job, JobStatusFailed, "Interrupted deployment was rolled back", jobError)
}

func (m *Manager) execute(jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	job, err := m.Job(jobID)
	if err != nil {
		return
	}
	if !job.OldSlotCaptured {
		oldPort := m.activePortForJob(job)
		health, healthErr := m.applicationHealth(ctx, oldPort)
		if healthErr != nil {
			_ = m.fail(jobID, fmt.Errorf("capture previous deployment runtime slot: %w", healthErr))
			return
		}
		oldRuntimeSlot := strings.TrimSpace(health.DeploymentRuntime.Slot)
		if oldRuntimeSlot != job.OldSlot && (job.OldContainer != m.cfg.InitialContainer || oldRuntimeSlot != "") {
			_ = m.fail(jobID, fmt.Errorf("previous deployment runtime slot mismatch: expected %q, got %q", job.OldSlot, oldRuntimeSlot))
			return
		}
		if err := m.updateJob(jobID, job.Stage, job.Message, func(current *Job) {
			current.OldRuntimeSlot = oldRuntimeSlot
			current.OldSlotCaptured = true
		}); err != nil {
			_ = m.fail(jobID, fmt.Errorf("persist previous deployment runtime slot: %w", err))
			return
		}
		job, err = m.Job(jobID)
		if err != nil {
			return
		}
	}
	if job.Action == "rollback" {
		candidate, ok, retainedErr := m.retainedRollbackCandidate(ctx, job.TargetVersion)
		if retainedErr != nil {
			_ = m.fail(jobID, fmt.Errorf("inspect retained rollback candidate: %w", retainedErr))
			return
		}
		if ok {
			m.executeRetainedRollback(ctx, jobID, candidate)
			return
		}
	}
	taggedImage := m.cfg.ImageRepository + ":" + job.TargetVersion
	if err := m.updateJob(jobID, StagePulling, "Pulling target image", func(j *Job) { j.TargetImage = taggedImage }); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist image pull intent: %w", err))
		return
	}
	if _, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "pull", taggedImage); err != nil {
		_ = m.fail(jobID, fmt.Errorf("pull target image: %w", err))
		return
	}
	digest, err := m.resolveDigest(ctx, taggedImage)
	if err != nil {
		_ = m.fail(jobID, err)
		return
	}
	targetImage := m.cfg.ImageRepository + "@" + digest
	if err := m.verifyImageLabels(ctx, targetImage); err != nil {
		_ = m.fail(jobID, err)
		return
	}
	candidate, err := m.inactiveSlot(job.OldSlot)
	if err != nil {
		_ = m.fail(jobID, err)
		return
	}
	if err := m.updateJob(jobID, StagePreparing, "Preparing inactive deployment slot", func(j *Job) {
		j.TargetImage = targetImage
		j.TargetDigest = digest
		j.CandidateSlot = candidate.Name
		j.CandidateContainer = candidate.Name
		j.CandidatePort = candidate.Port
	}); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist candidate preparation intent: %w", err))
		return
	}
	job, err = m.Job(jobID)
	if err != nil {
		return
	}
	if err := m.prepareInactiveSlot(ctx, job, candidate); err != nil {
		_ = m.fail(jobID, fmt.Errorf("prepare candidate slot: %w", err))
		return
	}

	if err := m.updateJob(jobID, StageStartingCandidate, "Starting candidate container", nil); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist candidate start intent: %w", err))
		return
	}
	if err := m.startCandidate(ctx, jobID, candidate, targetImage); err != nil {
		_ = m.fail(jobID, err)
		return
	}
	m.finishCandidateDeployment(ctx, jobID, candidate, true)
}

type retainedCandidate struct {
	slot      Slot
	container containerRef
	version   string
	image     string
}

func (m *Manager) retainedRollbackCandidate(ctx context.Context, targetVersion string) (retainedCandidate, bool, error) {
	m.mu.RLock()
	previousSlot := m.state.PreviousSlot
	previousContainer := m.state.PreviousContainer
	previousContainerID := m.state.PreviousContainerID
	previousPort := m.state.PreviousPort
	previousVersion := m.state.PreviousVersion
	previousImage := m.state.PreviousImage
	m.mu.RUnlock()
	if strings.TrimPrefix(previousVersion, "v") != strings.TrimPrefix(targetVersion, "v") || previousContainer == "" || previousPort == 0 {
		return retainedCandidate{}, false, nil
	}
	previousRef := containerRef{Name: previousContainer, ID: previousContainerID}
	_, found, inspectErr := m.inspectContainerRef(ctx, previousRef)
	if inspectErr != nil {
		return retainedCandidate{}, false, inspectErr
	}
	if !found {
		return retainedCandidate{}, false, nil
	}
	if strings.TrimSpace(previousImage) == "" {
		var err error
		previousImage, err = m.inspectContainerImage(ctx, previousRef.ID)
		if err != nil || strings.TrimSpace(previousImage) == "" {
			return retainedCandidate{}, false, err
		}
	}
	return retainedCandidate{
		slot:      Slot{Name: previousSlot, Host: "127.0.0.1", Port: previousPort},
		container: previousRef,
		version:   previousVersion,
		image:     previousImage,
	}, true, nil
}

func (m *Manager) executeRetainedRollback(ctx context.Context, jobID string, candidate retainedCandidate) {
	if err := m.updateJob(jobID, StagePreparing, "Preparing retained rollback container", func(j *Job) {
		j.TargetImage = candidate.image
		j.CandidateSlot = candidate.slot.Name
		j.CandidateContainer = candidate.container.Name
		j.CandidateContainerID = candidate.container.ID
		j.CandidatePort = candidate.slot.Port
	}); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist retained rollback preparation: %w", err))
		return
	}
	if err := m.updateJob(jobID, StageStartingCandidate, "Starting retained rollback container", nil); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist retained rollback start intent: %w", err))
		return
	}
	if err := m.startContainer(ctx, candidate.container); err != nil {
		_ = m.fail(jobID, fmt.Errorf("start retained rollback container: %w", err))
		return
	}
	m.finishCandidateDeployment(ctx, jobID, candidate.slot, false)
}

func (m *Manager) finishCandidateDeployment(ctx context.Context, jobID string, candidate Slot, requireStandby bool) {
	job, err := m.Job(jobID)
	if err != nil {
		return
	}
	if err := m.updateJob(jobID, StageCheckingCandidate, "Checking candidate health, version, slot, and standby state", nil); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist candidate check intent: %w", err))
		return
	}
	if err := m.waitForCandidate(ctx, job.candidateContainerRef(), candidate.Port, job.TargetVersion, requireStandby, candidate.Name, job.CandidateContainer == m.cfg.InitialContainer); err != nil {
		_ = m.fail(jobID, err)
		return
	}

	if err := m.updateJob(jobID, StageSwitchingTraffic, "Switching Nginx traffic to candidate", func(current *Job) {
		current.TrafficState = TrafficStateSwitchPending
	}); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist traffic switch intent: %w", err))
		return
	}
	previousSlot, allowEmptyPrevious := m.expectedOldRuntimeSlot(job)
	allowEmptyCandidateSlot := job.CandidateContainer == m.cfg.InitialContainer
	route, switchErr := m.switchTraffic(ctx,
		trafficEndpoint{Port: candidate.Port, Slot: candidate.Name, AllowEmpty: allowEmptyCandidateSlot},
		trafficEndpoint{Port: m.activePortForJob(job), Slot: previousSlot, AllowEmpty: allowEmptyPrevious},
	)
	nextTrafficState := m.trafficStateForRoute(job, route)
	if switchErr != nil {
		if err := m.updateJob(jobID, StageSwitchingTraffic, "Nginx traffic switch did not complete", func(current *Job) {
			current.TrafficState = nextTrafficState
			current.TrafficSwitched = nextTrafficState == TrafficStateCandidate
		}); err != nil {
			switchErr = fmt.Errorf("%v; persist observed traffic route: %w", switchErr, err)
		}
		_ = m.fail(jobID, switchErr)
		return
	}
	if nextTrafficState != TrafficStateCandidate {
		_ = m.fail(jobID, errors.New("nginx reload returned success without a confirmed candidate route"))
		return
	}
	if err := m.updateJob(jobID, StageStabilizing, "Observing the new deployment while the previous worker runtime remains active", func(j *Job) {
		j.TrafficState = TrafficStateCandidate
		j.TrafficSwitched = true
	}); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist confirmed candidate route: %w", err))
		return
	}
	if err := m.validateManagedRoute(ctx, candidate.Port); err != nil {
		_ = m.fail(jobID, fmt.Errorf("validate candidate nginx route: %w", err))
		return
	}
	if err := m.verifyRoutedHealth(ctx, candidate.Name, allowEmptyCandidateSlot); err != nil {
		_ = m.fail(jobID, fmt.Errorf("verify candidate through nginx: %w", err))
		return
	}
	if err := m.stabilize(ctx, job.candidateContainerRef(), candidate.Port); err != nil {
		_ = m.fail(jobID, err)
		return
	}

	if err := m.prepareOldContainerHandoff(ctx, jobID); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist previous container handoff intent: %w", err))
		return
	}
	job, err = m.Job(jobID)
	if err != nil {
		return
	}
	if err := m.completeOldContainerHandoff(ctx, job, candidate.Port, candidate.Name, allowEmptyCandidateSlot, false); err != nil {
		_ = m.degradeDrain(jobID, fmt.Errorf("candidate remains routed but the previous container handoff could not be completed safely: %w", err))
		return
	}
	if err := m.updateJob(jobID, StageActivating, "Activating candidate background services", nil); err != nil {
		_ = m.degradeDrain(jobID, fmt.Errorf("previous container stopped but background activation intent could not be persisted: %w", err))
		return
	}
	if err := m.ensureBackgroundActive(ctx, job.candidateContainerRef(), candidate.Port, candidate.Name); err != nil {
		_ = m.fail(jobID, fmt.Errorf("activate candidate background services: %w", err))
		return
	}
	if err := m.updateJob(jobID, StageActivating, "Candidate background services are active", func(j *Job) {
		j.BackgroundActivated = true
	}); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist background activation result: %w", err))
		return
	}
	job, err = m.Job(jobID)
	if err != nil {
		return
	}
	if err := m.persistSuccessfulDeployment(job); err != nil {
		_ = m.fail(jobID, fmt.Errorf("persist deployed image: %w", err))
		return
	}
	if err := m.complete(jobID, "Deployment completed", ""); err != nil {
		return
	}
}

func (m *Manager) fail(jobID string, cause error) error {
	job, err := m.Job(jobID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.recoveryTimeout())
	defer cancel()
	rollbackErr := m.restoreOldDeployment(ctx, job, false)
	status := JobStatusFailed
	message := "Deployment failed; previous version remains active"
	if rollbackErr != nil {
		status = JobStatusRollbackFailed
		message = "Deployment failed and automatic rollback also failed"
	}
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Job != nil && m.state.Job.ID == jobID {
		next := cloneState(m.state)
		next.Job.Status = status
		next.Job.Stage = StageFailed
		next.Job.Message = message
		next.Job.Error = cause.Error()
		next.Job.RollbackPerformed = rollbackErr == nil && trafficMayHaveSwitched(job)
		if rollbackErr != nil {
			next.Job.RollbackError = rollbackErr.Error()
			next.Degraded = true
			next.DegradedReason = message + ": " + rollbackErr.Error()
		}
		next.Job.UpdatedAt = now
		next.Job.FinishedAt = &now
		next.UpdatedAt = now
		if err := saveState(m.cfg.StatePath, next); err != nil {
			m.latchTerminalPersistenceFailureLocked(jobID, "could not persist failed deployment state: "+err.Error())
			return err
		}
		m.state = next
	}
	return nil
}

func (m *Manager) degradeDrain(jobID string, cause error) error {
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Job == nil || m.state.Job.ID != jobID {
		return ErrJobNotFound
	}
	next := cloneState(m.state)
	next.Job.Status = JobStatusDegraded
	next.Job.Stage = StageDraining
	next.Job.Message = "Deployment is serving in degraded mode pending connection drain reconciliation"
	next.Job.Error = cause.Error()
	next.Job.UpdatedAt = now
	next.Job.FinishedAt = &now
	next.Degraded = true
	next.DegradedReason = cause.Error()
	next.UpdatedAt = now
	if err := saveState(m.cfg.StatePath, next); err != nil {
		m.latchTerminalPersistenceFailureLocked(jobID, "could not persist drain-degraded deployment state: "+err.Error())
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) restoreOldDeployment(ctx context.Context, job *Job, forceTrafficRestore bool) error {
	normalizeLegacyTrafficState(job)
	var failures []string
	needsTrafficRestore := job.TrafficState != TrafficStateOld || forceTrafficRestore
	trafficRestored := !needsTrafficRestore
	oldPort := m.activePortForJob(job)
	expectedOldSlot, allowEmptyOldSlot := m.expectedOldRuntimeSlot(job)
	if needsTrafficRestore {
		if err := m.updateJob(job.ID, StageRollingBack, "Restoring traffic to the previous container", func(current *Job) {
			current.TrafficState = TrafficStateRestorePending
		}); err != nil {
			return fmt.Errorf("persist traffic restoration intent: %w", err)
		}
		if err := m.startContainer(ctx, job.oldContainerRef()); err != nil {
			failures = append(failures, "start previous container: "+err.Error())
		} else if err := m.waitForCandidate(ctx, job.oldContainerRef(), oldPort, job.FromVersion, false, expectedOldSlot, allowEmptyOldSlot); err != nil {
			failures = append(failures, "previous container unhealthy: "+err.Error())
		} else {
			route, switchErr := m.switchTraffic(ctx,
				trafficEndpoint{Port: oldPort, Slot: expectedOldSlot, AllowEmpty: allowEmptyOldSlot},
				trafficEndpoint{Port: job.CandidatePort, Slot: job.CandidateSlot},
			)
			observedState := m.trafficStateForRoute(job, route)
			if observedState != TrafficStateOld {
				observedState = TrafficStateUnknown
			}
			if err := m.updateJob(job.ID, StageRollingBack, "Recorded Nginx traffic restoration result", func(current *Job) {
				current.TrafficState = observedState
				current.TrafficSwitched = observedState == TrafficStateCandidate
			}); err != nil {
				failures = append(failures, "persist restored traffic state: "+err.Error())
			} else if observedState != TrafficStateOld {
				if switchErr == nil {
					switchErr = errors.New("nginx route after restoration is unknown")
				}
				failures = append(failures, "restore nginx upstream: "+switchErr.Error())
			} else {
				trafficRestored = true
			}
		}
	}
	if trafficRestored {
		if err := m.validateManagedRoute(ctx, oldPort); err != nil {
			trafficRestored = false
			failures = append(failures, "confirm restored nginx route: "+err.Error())
		} else if err := m.verifyRoutedHealth(ctx, expectedOldSlot, allowEmptyOldSlot); err != nil {
			trafficRestored = false
			failures = append(failures, "confirm restored routed health: "+err.Error())
		}
	}
	if trafficRestored {
		if job.CandidateContainer != "" && job.CandidateContainer != job.OldContainer {
			if err := m.stopContainer(ctx, job.candidateContainerRef()); err != nil {
				failures = append(failures, "stop candidate: "+err.Error())
			}
		}
		if len(failures) == 0 {
			if err := m.ensureBackgroundActive(ctx, job.oldContainerRef(), oldPort, job.OldSlot); err != nil {
				failures = append(failures, "activate previous background services: "+err.Error())
			}
		}
		if len(failures) == 0 && job.FromImage != "" {
			line := m.cfg.ImageEnvironment + "=" + job.FromImage + "\n"
			if err := atomicWrite(m.cfg.ImageStatePath, []byte(line), 0600); err != nil {
				failures = append(failures, "restore image state: "+err.Error())
			}
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func normalizeLegacyTrafficState(job *Job) {
	if job.TrafficState != "" {
		return
	}
	if job.TrafficSwitched {
		job.TrafficState = TrafficStateCandidate
		return
	}
	switch job.Stage {
	case StageSwitchingTraffic, StageStabilizing, StageDraining, StageActivating, StageRollingBack:
		job.TrafficState = TrafficStateUnknown
	default:
		job.TrafficState = TrafficStateOld
	}
}

func trafficMayHaveSwitched(job *Job) bool {
	copy := *job
	normalizeLegacyTrafficState(&copy)
	return copy.TrafficState != TrafficStateOld
}

func (m *Manager) trafficStateForRoute(job *Job, route trafficRoute) string {
	if !route.Known {
		return TrafficStateUnknown
	}
	if route.Port == job.CandidatePort && job.CandidatePort != 0 {
		return TrafficStateCandidate
	}
	if route.Port == m.activePortForJob(job) {
		return TrafficStateOld
	}
	return TrafficStateUnknown
}

func (m *Manager) expectedOldRuntimeSlot(job *Job) (string, bool) {
	if job.OldSlotCaptured {
		return job.OldRuntimeSlot, job.OldRuntimeSlot == ""
	}
	if job.OldContainer == m.cfg.InitialContainer {
		return "", true
	}
	return job.OldSlot, false
}

func (m *Manager) recoveryTimeout() time.Duration {
	timeout := 2*m.cfg.HealthTimeout.Duration + 2*m.cfg.StopTimeout.Duration + m.cfg.DrainTimeout.Duration + time.Minute
	if timeout < 5*time.Minute {
		return 5 * time.Minute
	}
	return timeout
}

func (m *Manager) persistSuccessfulDeployment(job *Job) error {
	line := m.cfg.ImageEnvironment + "=" + job.TargetImage + "\n"
	if err := atomicWrite(m.cfg.ImageStatePath, []byte(line), 0600); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.state
	if next.ActiveContainer != job.CandidateContainer {
		next.PreviousSlot = next.ActiveSlot
		next.PreviousContainer = next.ActiveContainer
		next.PreviousContainerID = next.ActiveContainerID
		next.PreviousPort = next.ActivePort
		next.PreviousVersion = next.ActiveVersion
		next.PreviousImage = next.ActiveImage
	}
	next.ActiveSlot = job.CandidateSlot
	next.ActiveContainer = job.CandidateContainer
	next.ActiveContainerID = job.CandidateContainerID
	next.ActivePort = job.CandidatePort
	next.ActiveVersion = job.TargetVersion
	next.ActiveImage = job.TargetImage
	next.UpdatedAt = m.now().UTC()
	if err := saveState(m.cfg.StatePath, next); err != nil {
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) complete(jobID, message, cleanupWarning string) error {
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Job == nil || m.state.Job.ID != jobID {
		return ErrJobNotFound
	}
	next := cloneState(m.state)
	next.Job.Status = JobStatusSucceeded
	next.Job.Stage = StageCompleted
	next.Job.Message = message
	next.Job.CleanupWarning = cleanupWarning
	next.Job.UpdatedAt = now
	next.Job.FinishedAt = &now
	next.UpdatedAt = now
	if err := saveState(m.cfg.StatePath, next); err != nil {
		m.latchTerminalPersistenceFailureLocked(jobID, "deployment became active but completion state could not be persisted: "+err.Error())
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) finishRecoveredJob(job Job, status, message, jobError string) error {
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Job == nil || m.state.Job.ID != job.ID {
		return ErrJobNotFound
	}
	next := cloneState(m.state)
	next.Job.Status = status
	next.Job.Stage = map[bool]string{true: StageCompleted, false: StageFailed}[status == JobStatusSucceeded]
	next.Job.Message = message
	next.Job.Error = jobError
	next.Job.BackgroundActivated = job.BackgroundActivated
	next.Job.RollbackPerformed = job.RollbackPerformed
	next.Job.UpdatedAt = now
	next.Job.FinishedAt = &now
	next.UpdatedAt = now
	if status == JobStatusRollbackFailed {
		next.Degraded = true
		next.DegradedReason = message
		if jobError != "" {
			next.DegradedReason += ": " + jobError
		}
	}
	if err := saveState(m.cfg.StatePath, next); err != nil {
		m.latchTerminalPersistenceFailureLocked(job.ID, "could not persist recovered deployment terminal state: "+err.Error())
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) updateJob(jobID, stage, message string, mutate func(*Job)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Job == nil || m.state.Job.ID != jobID || m.state.Job.Status != JobStatusRunning {
		return ErrJobNotFound
	}
	next := cloneState(m.state)
	next.Job.Stage = stage
	next.Job.Message = message
	next.Job.UpdatedAt = m.now().UTC()
	if mutate != nil {
		mutate(next.Job)
	}
	next.UpdatedAt = next.Job.UpdatedAt
	if err := saveState(m.cfg.StatePath, next); err != nil {
		return err
	}
	m.state = next
	return nil
}

func cloneState(state State) State {
	next := state
	if state.Job != nil {
		job := *state.Job
		next.Job = &job
	}
	if state.JobHistory != nil {
		next.JobHistory = append([]Job(nil), state.JobHistory...)
	}
	return next
}

func findJobByRequestID(state State, requestID string) *Job {
	if state.Job != nil && state.Job.ID == requestID {
		return state.Job
	}
	for i := range state.JobHistory {
		if state.JobHistory[i].ID == requestID {
			return &state.JobHistory[i]
		}
	}
	return nil
}

func jobMatchesRequest(job *Job, req DeployRequest) bool {
	return job.Action == req.Action &&
		job.TargetVersion == req.TargetVersion &&
		job.ExpectedCurrent == req.ExpectedCurrentVersion
}

func archiveTerminalJob(state *State, now time.Time) {
	if state.Job != nil && state.Job.Status != JobStatusRunning {
		state.JobHistory = append([]Job{*state.Job}, state.JobHistory...)
	}
	cutoff := now.Add(-jobHistoryTTL)
	kept := state.JobHistory[:0]
	for _, job := range state.JobHistory {
		if len(kept) >= maxJobHistory {
			break
		}
		if job.UpdatedAt.Before(cutoff) {
			continue
		}
		kept = append(kept, job)
	}
	state.JobHistory = kept
}

func (m *Manager) markDegraded(reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneState(m.state)
	next.Degraded = true
	next.DegradedReason = strings.TrimSpace(reason)
	next.UpdatedAt = m.now().UTC()
	if err := saveState(m.cfg.StatePath, next); err != nil {
		m.latchDegradedInMemoryLocked(reason + "; degraded latch persistence failed: " + err.Error())
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) latchDegradedInMemoryLocked(reason string) {
	m.state.Degraded = true
	m.state.DegradedReason = strings.TrimSpace(reason)
	m.state.UpdatedAt = m.now().UTC()
}

func (m *Manager) latchTerminalPersistenceFailureLocked(jobID, reason string) {
	m.latchDegradedInMemoryLocked(reason)
	if m.state.Job == nil || m.state.Job.ID != jobID {
		return
	}
	now := m.now().UTC()
	m.state.Job.Status = JobStatusDegraded
	m.state.Job.Stage = StageFailed
	m.state.Job.Message = "Deployment state persistence failed; operator reconciliation is required"
	m.state.Job.Error = reason
	m.state.Job.UpdatedAt = now
	m.state.Job.FinishedAt = &now
}

func RequireDaemonStopped(socketPath string) error {
	info, statErr := os.Lstat(socketPath)
	if errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("inspect deployer socket %s: %w", socketPath, statErr)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("cannot verify that the deployer is stopped: %s is not a Unix socket", socketPath)
	}
	connection, err := net.DialTimeout("unix", socketPath, 250*time.Millisecond)
	if err != nil {
		if isConnectionRefused(err) || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("cannot verify that the deployer daemon is stopped on %s: %w", socketPath, err)
	}
	_ = connection.Close()
	return fmt.Errorf("deployer daemon is still accepting connections on %s; stop the service before reconciliation", socketPath)
}

func (m *Manager) resolveDigest(ctx context.Context, taggedImage string) (string, error) {
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "image", "inspect", "--format", "{{json .RepoDigests}}", taggedImage)
	if err != nil {
		return "", fmt.Errorf("inspect target image: %w", err)
	}
	var repoDigests []string
	if err := json.Unmarshal([]byte(output), &repoDigests); err != nil {
		return "", fmt.Errorf("decode image digests: %w", err)
	}
	prefix := m.cfg.ImageRepository + "@"
	for _, value := range repoDigests {
		if strings.HasPrefix(value, prefix) {
			digest := strings.TrimPrefix(value, prefix)
			if strings.HasPrefix(digest, "sha256:") && len(digest) == 71 {
				return digest, nil
			}
		}
	}
	return "", errors.New("target image has no immutable digest for the configured repository")
}

func (m *Manager) verifyImageLabels(ctx context.Context, image string) error {
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "image", "inspect", "--format", "{{json .Config.Labels}}", image)
	if err != nil {
		return fmt.Errorf("inspect image labels: %w", err)
	}
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(output), &labels); err != nil {
		return fmt.Errorf("decode image labels: %w", err)
	}
	for key, expected := range m.cfg.RequiredImageLabels {
		if labels[key] != expected {
			return fmt.Errorf("target image label %s mismatch: expected %q, got %q", key, expected, labels[key])
		}
	}
	return nil
}

func (m *Manager) startCandidate(ctx context.Context, jobID string, slot Slot, image string) error {
	args := []string{"compose", "--project-name", m.cfg.ComposeProject, "--project-directory", m.cfg.ComposeWorkDir}
	for _, file := range m.cfg.ComposeEnvFiles {
		args = append(args, "--env-file", file)
	}
	for _, file := range m.cfg.ComposeFiles {
		args = append(args, "-f", file)
	}
	portBinding := slot.Host + ":" + strconv.Itoa(slot.Port) + ":" + strconv.Itoa(m.cfg.ContainerPort)
	args = append(args,
		"run", "-d", "--no-deps", "--name", slot.Name,
		"-e", "DEPLOYMENT_STANDBY=true",
		"-e", "DEPLOYMENT_SLOT="+slot.Name,
		"-e", "DEPLOYMENT_STATE_FILE="+m.cfg.DeploymentStateFile,
		"-p", portBinding, m.cfg.ComposeService,
	)
	env := map[string]string{
		m.cfg.ImageEnvironment:        image,
		"SUB2API_DEPLOYER_SOCKET_GID": strconv.Itoa(m.cfg.SocketGID),
	}
	if _, err := m.runner.Run(ctx, env, m.cfg.DockerBinary, args...); err != nil {
		return fmt.Errorf("start candidate container: %w", err)
	}
	candidate, found, err := m.inspectContainerRef(ctx, containerRef{Name: slot.Name})
	if err != nil {
		return fmt.Errorf("verify candidate container ownership: %w", err)
	}
	if !found {
		return errors.New("candidate container is absent after compose run")
	}
	if err := m.updateJob(jobID, StageStartingCandidate, "Candidate container identity is durable", func(job *Job) {
		job.CandidateContainerID = candidate.ID
	}); err != nil {
		return fmt.Errorf("persist candidate container identity: %w", err)
	}
	verified, found, err := m.inspectContainerRef(ctx, candidate)
	if err != nil {
		return fmt.Errorf("revalidate candidate container before restart policy update: %w", err)
	}
	if !found {
		return errors.New("revalidate candidate container before restart policy update: candidate is absent")
	}
	if _, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "update", "--restart", "unless-stopped", verified.ID); err != nil {
		_ = m.stopContainer(ctx, candidate)
		return fmt.Errorf("set candidate restart policy: %w", err)
	}
	return nil
}

func (m *Manager) waitForCandidate(ctx context.Context, container containerRef, port int, targetVersion string, requireStandby bool, expectedRuntimeSlot string, allowEmptyRuntimeSlot bool) error {
	deadline := time.Now().Add(m.cfg.HealthTimeout.Duration)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.containerHealthy(ctx, container, port); err == nil {
			version, versionErr := m.inspectContainerVersion(ctx, container.ID)
			if versionErr != nil {
				lastErr = versionErr
			} else if targetVersion != "" && strings.TrimPrefix(version, "v") != strings.TrimPrefix(targetVersion, "v") {
				return fmt.Errorf("candidate version mismatch: expected %s, got %s", targetVersion, version)
			} else {
				health, runtimeErr := m.applicationHealth(ctx, port)
				if runtimeErr != nil {
					lastErr = runtimeErr
				} else if actualSlot := strings.TrimSpace(health.DeploymentRuntime.Slot); expectedRuntimeSlot != "" && actualSlot != expectedRuntimeSlot && (!allowEmptyRuntimeSlot || actualSlot != "") {
					return fmt.Errorf("candidate runtime slot mismatch: expected %q, got %q", expectedRuntimeSlot, health.DeploymentRuntime.Slot)
				} else if requireStandby && strings.TrimSpace(health.DeploymentRuntime.State) != "standby" {
					return fmt.Errorf("candidate runtime mismatch: expected standby, got %q", health.DeploymentRuntime.State)
				} else {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		retryDelay := 2 * time.Second
		if remaining := time.Until(deadline); remaining < retryDelay {
			retryDelay = remaining
		}
		if retryDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("candidate did not become healthy")
	}
	return fmt.Errorf("candidate health timeout: %w", lastErr)
}

func (m *Manager) stabilize(ctx context.Context, container containerRef, port int) error {
	deadline := time.Now().Add(m.cfg.StabilizeDuration.Duration)
	for {
		if err := m.containerHealthy(ctx, container, port); err != nil {
			return fmt.Errorf("candidate failed during stabilization: %w", err)
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Manager) containerHealthy(ctx context.Context, container containerRef, port int) error {
	verified, found, err := m.inspectContainerRef(ctx, container)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("container %s is absent", container.Name)
	}
	status, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", verified.ID)
	if err != nil {
		return err
	}
	if status != "healthy" && status != "running" {
		return fmt.Errorf("container status is %s", status)
	}
	_, err = m.applicationHealth(ctx, port)
	return err
}

type applicationHealth struct {
	Status            string `json:"status"`
	DeploymentRuntime struct {
		State string `json:"state"`
		Slot  string `json:"slot"`
		Error string `json:"error"`
	} `json:"deployment_runtime"`
	Drain struct {
		Supported           bool  `json:"supported"`
		ActiveRequests      int64 `json:"active_requests"`
		HijackedConnections int64 `json:"hijacked_connections"`
		Blockers            int64 `json:"blockers"`
	} `json:"drain"`
}

func validateHandoffBinding(job *Job) error {
	if !job.HandoffPrepared {
		if job.HandoffContainer != "" || job.HandoffContainerID != "" {
			return errors.New("previous container handoff journal is incomplete")
		}
		return errors.New("previous container handoff intent is not durable")
	}
	handoff := containerRef{Name: job.HandoffContainer, ID: job.HandoffContainerID}
	if err := validateContainerRef(handoff); err != nil {
		return fmt.Errorf("invalid previous container handoff identity: %w", err)
	}
	if handoff.ID == "" {
		return errors.New("previous container handoff is not bound to an exact container ID")
	}
	if handoff != job.oldContainerRef() {
		return fmt.Errorf(
			"previous container handoff identity %s/%s does not match job identity %s/%s",
			handoff.Name, handoff.ID, job.OldContainer, job.OldContainerID,
		)
	}
	return nil
}

func (m *Manager) prepareOldContainerHandoff(ctx context.Context, jobID string) error {
	m.mu.RLock()
	if m.state.Job == nil || m.state.Job.ID != jobID {
		m.mu.RUnlock()
		return ErrJobNotFound
	}
	job := *m.state.Job
	m.mu.RUnlock()
	if job.HandoffPrepared {
		return validateHandoffBinding(&job)
	}
	if job.HandoffContainer != "" || job.HandoffContainerID != "" {
		return errors.New("previous container handoff journal is incomplete")
	}
	old := job.oldContainerRef()
	if old.ID == "" {
		return errors.New("previous container handoff cannot be bound without a persisted container ID")
	}
	_, running, err := m.inspectContainerRunning(ctx, old)
	if err != nil {
		return fmt.Errorf("inspect previous container before persisting handoff: %w", err)
	}
	if !running {
		return errors.New("previous container is already stopped without a durable handoff intent")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Job == nil || m.state.Job.ID != jobID {
		return ErrJobNotFound
	}
	if m.state.Job.HandoffPrepared {
		return validateHandoffBinding(m.state.Job)
	}
	if m.state.Job.HandoffContainer != "" || m.state.Job.HandoffContainerID != "" {
		return errors.New("previous container handoff journal became incomplete")
	}
	if m.state.Job.oldContainerRef() != old {
		return errors.New("previous container identity changed while preparing handoff")
	}
	next := cloneState(m.state)
	next.Job.Stage = StageDraining
	next.Job.Message = "Previous container handoff intent is durable; waiting for drain"
	next.Job.HandoffPrepared = true
	next.Job.HandoffContainer = old.Name
	next.Job.HandoffContainerID = old.ID
	next.Job.UpdatedAt = m.now().UTC()
	next.UpdatedAt = next.Job.UpdatedAt
	if err := saveState(m.cfg.StatePath, next); err != nil {
		return err
	}
	m.state = next
	return nil
}

func (m *Manager) completeOldContainerHandoff(ctx context.Context, job *Job, candidatePort int, candidateSlot string, allowEmptyCandidateSlot, allowUnobservableDrain bool) error {
	if err := validateHandoffBinding(job); err != nil {
		return err
	}
	old := job.oldContainerRef()
	_, running, err := m.inspectContainerRunning(ctx, old)
	if err != nil {
		return fmt.Errorf("inspect journal-bound previous container: %w", err)
	}
	if running {
		if err := m.waitForOldDrain(ctx, m.activePortForJob(job), allowUnobservableDrain); err != nil {
			_, stillRunning, inspectErr := m.inspectContainerRunning(ctx, old)
			if inspectErr != nil {
				return fmt.Errorf("wait for previous container drain: %v; recheck journal-bound container: %w", err, inspectErr)
			}
			if stillRunning {
				return fmt.Errorf("wait for previous container drain: %w", err)
			}
			running = false
		}
	}
	if err := m.validateManagedRoute(ctx, candidatePort); err != nil {
		return fmt.Errorf("candidate route changed during previous container handoff: %w", err)
	}
	if err := m.verifyRoutedHealth(ctx, candidateSlot, allowEmptyCandidateSlot); err != nil {
		return fmt.Errorf("candidate routed health failed during previous container handoff: %w", err)
	}
	if !running {
		return nil
	}
	stopErr := m.stopContainer(ctx, old)
	_, stillRunning, inspectErr := m.inspectContainerRunning(ctx, old)
	if inspectErr != nil {
		if stopErr != nil {
			return fmt.Errorf("stop journal-bound previous container: %v; prove stopped state: %w", stopErr, inspectErr)
		}
		return fmt.Errorf("prove journal-bound previous container stopped: %w", inspectErr)
	}
	if stillRunning {
		if stopErr != nil {
			return fmt.Errorf("stop journal-bound previous container: %w", stopErr)
		}
		return errors.New("journal-bound previous container is still running after docker stop")
	}
	return nil
}

func (m *Manager) waitForOldDrain(ctx context.Context, port int, allowUnobservable bool) error {
	timeout := m.cfg.DrainTimeout.Duration
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	quietPeriod := m.cfg.DrainDuration.Duration
	if quietPeriod < 0 {
		quietPeriod = 0
	}
	var zeroSince time.Time
	var lastErr error
	for {
		health, err := m.applicationHealth(drainCtx, port)
		if err != nil {
			lastErr = err
			zeroSince = time.Time{}
		} else if !health.Drain.Supported {
			if allowUnobservable {
				return nil
			}
			return ErrDrainUnobservable
		} else if health.Drain.ActiveRequests < 0 || health.Drain.HijackedConnections < 0 || health.Drain.Blockers != health.Drain.ActiveRequests+health.Drain.HijackedConnections {
			lastErr = errors.New("previous application returned an invalid drain snapshot")
			zeroSince = time.Time{}
		} else if health.Drain.Blockers == 0 {
			now := time.Now()
			if zeroSince.IsZero() {
				zeroSince = now
			}
			if now.Sub(zeroSince) >= quietPeriod {
				return nil
			}
			lastErr = nil
		} else {
			lastErr = fmt.Errorf("%d drain blocker(s) remain", health.Drain.Blockers)
			zeroSince = time.Time{}
		}

		wait := 250 * time.Millisecond
		if !zeroSince.IsZero() {
			remaining := quietPeriod - time.Since(zeroSince)
			if remaining < wait {
				wait = remaining
			}
		}
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-drainCtx.Done():
			timer.Stop()
			if lastErr != nil {
				return fmt.Errorf("%w: %v", ErrDrainTimeout, lastErr)
			}
			return fmt.Errorf("%w: %v", ErrDrainTimeout, drainCtx.Err())
		case <-timer.C:
		}
	}
}

func (m *Manager) applicationHealth(ctx context.Context, port int) (applicationHealth, error) {
	url := "http://127.0.0.1:" + strconv.Itoa(port) + m.cfg.HealthPath
	return m.fetchApplicationHealth(ctx, url, "")
}

func (m *Manager) fetchApplicationHealth(ctx context.Context, url, host string) (applicationHealth, error) {
	return m.fetchApplicationHealthWithClient(ctx, m.httpClient, url, host)
}

func (m *Manager) fetchApplicationHealthWithClient(ctx context.Context, client *http.Client, url, host string) (applicationHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return applicationHealth{}, err
	}
	if host != "" {
		req.Host = host
	}
	resp, err := client.Do(req)
	if err != nil {
		return applicationHealth{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		return applicationHealth{}, readErr
	}
	if resp.StatusCode != http.StatusOK {
		return applicationHealth{}, fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
	}
	var health applicationHealth
	if err := json.Unmarshal(body, &health); err != nil {
		return applicationHealth{}, fmt.Errorf("decode health response: %w", err)
	}
	if health.Status != "ok" {
		return applicationHealth{}, fmt.Errorf("health endpoint status is %q", health.Status)
	}
	return health, nil
}

func (m *Manager) verifyRoutedHealth(ctx context.Context, expectedSlot string, allowEmptySlot bool) error {
	health, err := m.fetchApplicationHealth(ctx, m.cfg.NginxProbeURL, m.cfg.NginxProbeHost)
	if err != nil {
		return err
	}
	actualSlot := strings.TrimSpace(health.DeploymentRuntime.Slot)
	if expectedSlot != "" && actualSlot == expectedSlot {
		return nil
	}
	if allowEmptySlot && actualSlot == "" {
		return nil
	}
	return fmt.Errorf("routed deployment runtime slot is %q, expected %q", actualSlot, expectedSlot)
}

func (m *Manager) deploymentRuntimeState(ctx context.Context, port int) (string, error) {
	health, err := m.applicationHealth(ctx, port)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(health.DeploymentRuntime.State), nil

}

func (m *Manager) ensureBackgroundActive(ctx context.Context, container containerRef, port int, slot string) error {
	verified, found, err := m.inspectContainerRef(ctx, container)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("container %s is absent", container.Name)
	}
	state, err := m.deploymentRuntimeState(ctx, port)
	if err != nil {
		return err
	}
	if strings.TrimSpace(slot) == "" {
		return errors.New("deployment slot is empty")
	}
	if err := atomicWrite(m.cfg.DeploymentStatePath, []byte(slot+"\n"), 0644); err != nil {
		return fmt.Errorf("persist active deployment slot: %w", err)
	}
	if state == "" || state == "active" {
		return nil
	}
	if _, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "kill", "--signal=USR1", verified.ID); err != nil {
		return fmt.Errorf("signal background activation: %w", err)
	}
	deadline := time.Now().Add(m.cfg.HealthTimeout.Duration)
	var lastState string
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastState, err = m.deploymentRuntimeState(ctx, port)
		if err == nil && lastState == "active" {
			return nil
		}
		if err == nil && lastState == "failed" {
			health, healthErr := m.applicationHealth(ctx, port)
			if healthErr == nil && health.DeploymentRuntime.Error != "" {
				return fmt.Errorf("application reported failed background activation: %s", health.DeploymentRuntime.Error)
			}
			return errors.New("application reported failed background activation")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("background activation timeout; last runtime state %q", lastState)
}

func (m *Manager) inspectContainerVersion(ctx context.Context, container string) (string, error) {
	if !containerIDRE.MatchString(container) {
		return "", errors.New("container ID must be 64 lowercase hexadecimal characters")
	}
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "exec", container, "/app/sub2api", "--version")
	if err != nil {
		return "", err
	}
	match := versionOutputRE.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", fmt.Errorf("could not parse application version from %q", output)
	}
	return strings.TrimPrefix(match[1], "v"), nil
}

func (m *Manager) inspectContainerImage(ctx context.Context, container string) (string, error) {
	if !containerIDRE.MatchString(container) {
		return "", errors.New("container ID must be 64 lowercase hexadecimal characters")
	}
	return m.runner.Run(ctx, nil, m.cfg.DockerBinary, "inspect", "--format", "{{.Config.Image}}", container)
}

func (m *Manager) removeContainerIfPresent(ctx context.Context, container containerRef) error {
	verified, found, err := m.inspectContainerRef(ctx, container)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if container.ID == "" {
		return fmt.Errorf("container %s has no persisted ID", container.Name)
	}
	_, err = m.runner.Run(ctx, nil, m.cfg.DockerBinary, "rm", "-f", verified.ID)
	return err
}

type dockerContainerDetails struct {
	ID     string            `json:"ID"`
	Name   string            `json:"Name"`
	Labels map[string]string `json:"Labels"`
}

func (m *Manager) inspectContainerRef(ctx context.Context, expected containerRef) (containerRef, bool, error) {
	if err := validateContainerRef(expected); err != nil {
		return containerRef{}, false, err
	}
	target := expected.Name
	if expected.ID != "" {
		target = expected.ID
	}
	const inspectFormat = `{"ID":{{json .Id}},"Name":{{json .Name}},"Labels":{{json .Config.Labels}}}`
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "container", "inspect", "--format", inspectFormat, target)
	if err != nil {
		if expected.ID != "" {
			idPresent, queryErr := m.containerIDPresent(ctx, expected.ID)
			if queryErr != nil {
				return containerRef{}, false, fmt.Errorf("inspect container ID %s: %v; prove ID absence: %w", expected.ID, err, queryErr)
			}
			if idPresent {
				return containerRef{}, false, fmt.Errorf("inspect container ID %s for %s: %w", expected.ID, expected.Name, err)
			}
		}
		nameID, namePresent, queryErr := m.containerNameIdentity(ctx, expected.Name)
		if queryErr != nil {
			return containerRef{}, false, fmt.Errorf("inspect container %s: %v; prove name absence: %w", expected.Name, err, queryErr)
		}
		if !namePresent {
			return containerRef{}, false, nil
		}
		if expected.ID != "" {
			return containerRef{}, false, fmt.Errorf("container identity drift: name %s now resolves to ID %s, expected absent ID %s", expected.Name, nameID, expected.ID)
		}
		return containerRef{}, false, fmt.Errorf("inspect container %s: %w", expected.Name, err)
	}
	var details dockerContainerDetails
	if err := json.Unmarshal([]byte(output), &details); err != nil {
		return containerRef{}, false, fmt.Errorf("decode container %s inspection: %w", expected.Name, err)
	}
	actualID := strings.TrimSpace(details.ID)
	if !containerIDRE.MatchString(actualID) {
		return containerRef{}, false, fmt.Errorf("container %s inspection returned invalid ID %q", expected.Name, actualID)
	}
	if expected.ID != "" && actualID != expected.ID {
		return containerRef{}, false, fmt.Errorf("container %s now resolves to ID %s, expected %s", expected.Name, actualID, expected.ID)
	}
	actualName := strings.TrimPrefix(strings.TrimSpace(details.Name), "/")
	if actualName != expected.Name {
		return containerRef{}, false, fmt.Errorf("container ID %s now has name %q, expected %q", actualID, actualName, expected.Name)
	}
	labels := details.Labels
	if labels[composeProjectLabel] != m.cfg.ComposeProject || labels[composeServiceLabel] != m.cfg.ComposeService {
		return containerRef{}, false, fmt.Errorf(
			"container %s is not owned by compose project %q service %q",
			expected.Name, m.cfg.ComposeProject, m.cfg.ComposeService,
		)
	}
	return containerRef{Name: expected.Name, ID: actualID}, true, nil
}

func (m *Manager) containerNameIdentity(ctx context.Context, name string) (string, bool, error) {
	filter := "name=^/" + regexp.QuoteMeta(name) + "$"
	ids, err := m.containerIDs(ctx, filter, "exact container lookup for "+name)
	if err != nil {
		return "", false, err
	}
	if len(ids) == 0 {
		return "", false, nil
	}
	if len(ids) != 1 {
		return "", false, fmt.Errorf("exact container lookup for %s returned multiple results", name)
	}
	return ids[0], true, nil
}

func (m *Manager) containerIDPresent(ctx context.Context, id string) (bool, error) {
	ids, err := m.containerIDs(ctx, "id="+id, "exact container ID lookup for "+id)
	if err != nil {
		return false, err
	}
	for _, candidate := range ids {
		if candidate == id {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) containerIDs(ctx context.Context, filter, description string) ([]string, error) {
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "container", "ls", "--all", "--no-trunc", "--filter", filter, "--format", "{{json .ID}}")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var id string
		if err := json.Unmarshal([]byte(line), &id); err != nil || !containerIDRE.MatchString(strings.TrimSpace(id)) {
			return nil, fmt.Errorf("decode %s", description)
		}
		ids = append(ids, strings.TrimSpace(id))
	}
	return ids, nil
}

func validateContainerRef(ref containerRef) error {
	if !containerNamePattern.MatchString(ref.Name) {
		return fmt.Errorf("invalid container name %q", ref.Name)
	}
	if ref.ID != "" && !containerIDRE.MatchString(ref.ID) {
		return fmt.Errorf("container %s has invalid persisted ID %q", ref.Name, ref.ID)
	}
	return nil
}

func validateStateContainerIdentities(state State) error {
	if state.Job != nil && (state.Job.HandoffPrepared || state.Job.HandoffContainer != "" || state.Job.HandoffContainerID != "") {
		if err := validateHandoffBinding(state.Job); err != nil {
			return fmt.Errorf("invalid job handoff journal: %w", err)
		}
	}
	refs := []struct {
		role string
		ref  containerRef
	}{
		{role: "active", ref: containerRef{Name: state.ActiveContainer, ID: state.ActiveContainerID}},
		{role: "previous", ref: containerRef{Name: state.PreviousContainer, ID: state.PreviousContainerID}},
	}
	if state.Job != nil {
		refs = append(refs,
			struct {
				role string
				ref  containerRef
			}{role: "job old", ref: state.Job.oldContainerRef()},
			struct {
				role string
				ref  containerRef
			}{role: "job candidate", ref: state.Job.candidateContainerRef()},
		)
	}
	for _, item := range refs {
		if item.ref.Name == "" && item.ref.ID == "" {
			continue
		}
		if item.ref.Name == "" {
			return fmt.Errorf("%s container has an ID without a name", item.role)
		}
		if err := validateContainerRef(item.ref); err != nil {
			return fmt.Errorf("invalid %s container identity: %w", item.role, err)
		}
	}
	return nil
}

func (m *Manager) bindLegacyContainerIDs(ctx context.Context) error {
	next := cloneState(m.state)
	changed := false
	bindRequired := func(name, id, role string) (string, error) {
		if name == "" || id != "" {
			return id, nil
		}
		ref, found, err := m.inspectContainerRef(ctx, containerRef{Name: name})
		if err != nil {
			return "", fmt.Errorf("bind legacy %s container: %w", role, err)
		}
		if !found {
			return "", fmt.Errorf("bind legacy %s container: container %s is absent", role, name)
		}
		changed = true
		return ref.ID, nil
	}

	var err error
	if next.ActiveContainerID, err = bindRequired(next.ActiveContainer, next.ActiveContainerID, "active"); err != nil {
		return err
	}
	if next.Job != nil {
		if next.Job.OldContainerID, err = bindRequired(next.Job.OldContainer, next.Job.OldContainerID, "job old"); err != nil {
			return err
		}
		if next.Job.CandidateContainer != "" && next.Job.CandidateContainerID == "" {
			candidateMayExist := next.Job.Stage != StagePulling && next.Job.Stage != StagePreparing
			if candidateMayExist {
				ref, found, inspectErr := m.inspectContainerRef(ctx, containerRef{Name: next.Job.CandidateContainer})
				if inspectErr != nil {
					return fmt.Errorf("bind legacy job candidate container: %w", inspectErr)
				}
				if found {
					next.Job.CandidateContainerID = ref.ID
				} else {
					next.Job.CandidateContainer = ""
				}
			} else {
				next.Job.CandidateContainer = ""
			}
			changed = true
		}
	}
	if next.PreviousContainer != "" && next.PreviousContainerID == "" {
		candidateReplacedPrevious := next.Job != nil && next.Job.CandidateContainer != "" && next.Job.CandidateContainer == next.PreviousContainer
		if candidateReplacedPrevious {
			next.PreviousContainer = ""
			next.PreviousSlot = ""
			next.PreviousPort = 0
			next.PreviousVersion = ""
			next.PreviousImage = ""
			changed = true
		} else {
			ref, found, inspectErr := m.inspectContainerRef(ctx, containerRef{Name: next.PreviousContainer})
			if inspectErr != nil {
				return fmt.Errorf("bind legacy previous container: %w", inspectErr)
			}
			if found {
				next.PreviousContainerID = ref.ID
			} else {
				next.PreviousContainer = ""
				next.PreviousSlot = ""
				next.PreviousPort = 0
				next.PreviousVersion = ""
				next.PreviousImage = ""
			}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	next.UpdatedAt = m.now().UTC()
	if err := saveState(m.cfg.StatePath, next); err != nil {
		return fmt.Errorf("persist bound legacy container IDs: %w", err)
	}
	m.state = next
	return nil
}

func (m *Manager) prepareInactiveSlot(ctx context.Context, job *Job, candidate Slot) error {
	if err := m.verifyOldRouteBeforeCandidateCleanup(ctx, job); err != nil {
		return fmt.Errorf("candidate slot is not safe to replace: %w", err)
	}
	for _, container := range m.knownContainersForSlot(candidate.Name, job.OldContainer, candidate.Name) {
		if err := m.removeContainerIfPresent(ctx, container); err != nil {
			return fmt.Errorf("remove stale container %s: %w", container.Name, err)
		}
	}
	candidateRef := containerRef{Name: candidate.Name, ID: job.CandidateContainerID}
	if candidateRef.ID == "" {
		for _, known := range m.knownContainersForSlot(candidate.Name) {
			if known.Name == candidate.Name {
				candidateRef = known
				break
			}
		}
	}
	if err := m.removeContainerIfPresent(ctx, candidateRef); err != nil {
		return err
	}
	return nil
}

func (m *Manager) knownContainersForSlot(slotName string, excluded ...string) []containerRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	excludedSet := make(map[string]struct{}, len(excluded)+1)
	for _, container := range excluded {
		excludedSet[container] = struct{}{}
	}
	excludedSet[m.state.ActiveContainer] = struct{}{}
	seen := make(map[string]containerRef)
	add := func(slot, container, id string) {
		if slot != slotName || container == "" {
			return
		}
		if _, skip := excludedSet[container]; skip {
			return
		}
		seen[container] = containerRef{Name: container, ID: id}
	}
	add(m.state.ActiveSlot, m.state.ActiveContainer, m.state.ActiveContainerID)
	add(m.state.PreviousSlot, m.state.PreviousContainer, m.state.PreviousContainerID)
	if m.state.Job != nil {
		add(m.state.Job.OldSlot, m.state.Job.OldContainer, m.state.Job.OldContainerID)
		add(m.state.Job.CandidateSlot, m.state.Job.CandidateContainer, m.state.Job.CandidateContainerID)
	}
	containers := make([]containerRef, 0, len(seen))
	for _, container := range seen {
		containers = append(containers, container)
	}
	return containers
}

func (m *Manager) verifyOldRouteBeforeCandidateCleanup(ctx context.Context, job *Job) error {
	oldPort := m.activePortForJob(job)
	if oldPort == 0 || job.CandidatePort == oldPort || job.CandidateContainer == job.OldContainer {
		return errors.New("candidate slot overlaps the active deployment")
	}
	if err := m.validateManagedRoute(ctx, oldPort); err != nil {
		return err
	}
	expectedSlot, allowEmpty := m.expectedOldRuntimeSlot(job)
	return m.verifyRoutedHealth(ctx, expectedSlot, allowEmpty)
}

func (m *Manager) inspectContainerRunning(ctx context.Context, container containerRef) (containerRef, bool, error) {
	verified, found, err := m.inspectContainerRef(ctx, container)
	if err != nil {
		return containerRef{}, false, err
	}
	if !found {
		return containerRef{}, false, fmt.Errorf("container %s is absent", container.Name)
	}
	if container.ID == "" {
		return containerRef{}, false, fmt.Errorf("container %s has no persisted ID", container.Name)
	}
	running, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "inspect", "--format", "{{.State.Running}}", verified.ID)
	if err != nil {
		return containerRef{}, false, err
	}
	switch strings.TrimSpace(running) {
	case "true":
		return verified, true, nil
	case "false":
		return verified, false, nil
	default:
		return containerRef{}, false, fmt.Errorf("container %s returned invalid running state %q", container.Name, strings.TrimSpace(running))
	}
}

func (m *Manager) startContainer(ctx context.Context, container containerRef) error {
	verified, running, err := m.inspectContainerRunning(ctx, container)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	_, err = m.runner.Run(ctx, nil, m.cfg.DockerBinary, "start", verified.ID)
	return err
}

func (m *Manager) stopContainer(ctx context.Context, container containerRef) error {
	verified, found, err := m.inspectContainerRef(ctx, container)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if container.ID == "" {
		return fmt.Errorf("container %s has no persisted ID", container.Name)
	}
	seconds := int(m.cfg.StopTimeout.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	_, err = m.runner.Run(ctx, nil, m.cfg.DockerBinary, "stop", "--time", strconv.Itoa(seconds), verified.ID)
	return err
}

func (m *Manager) inactiveSlot(activeSlot string) (Slot, error) {
	for _, slot := range m.cfg.Slots {
		if slot.Name != activeSlot {
			return slot, nil
		}
	}
	return Slot{}, fmt.Errorf("active slot %q is not configured", activeSlot)
}

func (m *Manager) slotByPort(port int) (Slot, bool) {
	for _, slot := range m.cfg.Slots {
		if slot.Port == port {
			return slot, true
		}
	}
	return Slot{}, false
}

func (m *Manager) slotByName(name string) (Slot, bool) {
	for _, slot := range m.cfg.Slots {
		if slot.Name == name {
			return slot, true
		}
	}
	return Slot{}, false
}

func (m *Manager) containerForSlotLocked(slotName string) containerRef {
	if m.state.ActiveSlot == slotName {
		return containerRef{Name: m.state.ActiveContainer, ID: m.state.ActiveContainerID}
	}
	if m.state.PreviousSlot == slotName {
		return containerRef{Name: m.state.PreviousContainer, ID: m.state.PreviousContainerID}
	}
	if m.state.Job != nil {
		if m.state.Job.CandidateSlot == slotName {
			return m.state.Job.candidateContainerRef()
		}
		if m.state.Job.OldSlot == slotName {
			return m.state.Job.oldContainerRef()
		}
	}
	return containerRef{}
}

func (m *Manager) otherKnownContainers(selectedSlot string, selectedContainer containerRef) []containerRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[string]containerRef)
	add := func(slot, container, id string) {
		if slot == selectedSlot || container == "" || container == selectedContainer.Name {
			return
		}
		seen[container] = containerRef{Name: container, ID: id}
	}
	add(m.state.ActiveSlot, m.state.ActiveContainer, m.state.ActiveContainerID)
	add(m.state.PreviousSlot, m.state.PreviousContainer, m.state.PreviousContainerID)
	if m.state.Job != nil {
		add(m.state.Job.OldSlot, m.state.Job.OldContainer, m.state.Job.OldContainerID)
		add(m.state.Job.CandidateSlot, m.state.Job.CandidateContainer, m.state.Job.CandidateContainerID)
	}
	containers := make([]containerRef, 0, len(seen))
	for _, container := range seen {
		containers = append(containers, container)
	}
	return containers
}

func (m *Manager) activePortForJob(job *Job) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state.ActiveContainer == job.OldContainer && m.state.ActivePort != 0 {
		return m.state.ActivePort
	}
	for _, slot := range m.cfg.Slots {
		if slot.Name == job.OldSlot {
			return slot.Port
		}
	}
	return m.state.ActivePort
}

func readUpstreamPort(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	clean := stripNginxComments(string(data))
	nameRE := regexp.MustCompile(`(?m)(?:^|[;{}])\s*upstream\s+([^\s{]+)\s*\{`)
	match := nameRE.FindStringSubmatch(clean)
	if len(match) != 2 {
		return 0, errors.New("managed upstream is not defined")
	}
	return readManagedUpstreamPort(data, match[1])
}
