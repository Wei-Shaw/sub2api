package service

import (
	"sort"
	"strings"
	"sync"
)

const (
	ReliabilityMetricCounter = "counter"
	ReliabilityMetricGauge   = "gauge"
)

type ReliabilityMetricDefinition struct {
	Name   string
	Kind   string
	Labels []string
}

// ReliabilityMetricRegistry is the smallest integration seam needed by the
// reliability core. An application metrics backend may implement it later;
// local and test environments safely fall back to the no-op implementation.
type ReliabilityMetricRegistry interface {
	RegisterReliabilityMetric(definition ReliabilityMetricDefinition)
}

type ReliabilityMetricRecorder interface {
	Add(name string, delta float64, labels map[string]string)
	Set(name string, value float64, labels map[string]string)
}

// ReliabilityMetricSample is a read-only point-in-time projection. It never
// contains task payloads, URLs, or credentials.
type ReliabilityMetricSample struct {
	Name   string
	Value  float64
	Labels map[string]string
}

// ReliabilityMetrics is the default in-process reliability recorder. The
// project has no mandatory external metrics backend, but production paths must
// still record actual measurements rather than leaving a test-only interface.
type ReliabilityMetrics struct {
	mu          sync.RWMutex
	samples     map[string]ReliabilityMetricSample
	definitions map[string]ReliabilityMetricDefinition
}

func NewReliabilityMetrics() *ReliabilityMetrics {
	return &ReliabilityMetrics{
		samples:     make(map[string]ReliabilityMetricSample),
		definitions: make(map[string]ReliabilityMetricDefinition),
	}
}

func (m *ReliabilityMetrics) RegisterReliabilityMetric(definition ReliabilityMetricDefinition) {
	if m == nil || definition.Name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	definition.Labels = append([]string(nil), definition.Labels...)
	m.definitions[definition.Name] = definition
}

func (m *ReliabilityMetrics) Add(name string, delta float64, labels map[string]string) {
	if m == nil || name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := reliabilityMetricKey(name, labels)
	sample := m.samples[key]
	if sample.Name == "" {
		sample = ReliabilityMetricSample{Name: name, Labels: cloneReliabilityMetricLabels(labels)}
	}
	sample.Value += delta
	m.samples[key] = sample
}

func (m *ReliabilityMetrics) Set(name string, value float64, labels map[string]string) {
	if m == nil || name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples[reliabilityMetricKey(name, labels)] = ReliabilityMetricSample{
		Name: name, Value: value, Labels: cloneReliabilityMetricLabels(labels),
	}
}

func (m *ReliabilityMetrics) Snapshot() []ReliabilityMetricSample {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ReliabilityMetricSample, 0, len(m.samples))
	for _, sample := range m.samples {
		out = append(out, ReliabilityMetricSample{
			Name: sample.Name, Value: sample.Value, Labels: cloneReliabilityMetricLabels(sample.Labels),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return reliabilityMetricKey(out[i].Name, out[i].Labels) < reliabilityMetricKey(out[j].Name, out[j].Labels)
	})
	return out
}

var defaultReliabilityMetrics = struct {
	sync.RWMutex
	recorder ReliabilityMetricRecorder
}{recorder: NewReliabilityMetrics()}

// InstallReliabilityMetricsRecorder replaces the process-local recorder and
// returns an idempotent restore function. Production starts with the
// concurrency-safe in-process recorder above.
func InstallReliabilityMetricsRecorder(recorder ReliabilityMetricRecorder) func() {
	if recorder == nil {
		recorder = noopReliabilityMetrics{}
	}
	defaultReliabilityMetrics.Lock()
	previous := defaultReliabilityMetrics.recorder
	defaultReliabilityMetrics.recorder = recorder
	defaultReliabilityMetrics.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			defaultReliabilityMetrics.Lock()
			defaultReliabilityMetrics.recorder = previous
			defaultReliabilityMetrics.Unlock()
		})
	}
}

func RecordReliabilityMetricAdd(name string, delta float64, labels map[string]string) {
	defaultReliabilityMetrics.RLock()
	recorder := defaultReliabilityMetrics.recorder
	defaultReliabilityMetrics.RUnlock()
	if recorder != nil {
		recorder.Add(name, delta, labels)
	}
}

func RecordReliabilityMetricSet(name string, value float64, labels map[string]string) {
	defaultReliabilityMetrics.RLock()
	recorder := defaultReliabilityMetrics.recorder
	defaultReliabilityMetrics.RUnlock()
	if recorder != nil {
		recorder.Set(name, value, labels)
	}
}

func reliabilityMetricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, name)
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\x1f")
}

func cloneReliabilityMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

type noopReliabilityMetrics struct{}

func (noopReliabilityMetrics) Add(string, float64, map[string]string) {}
func (noopReliabilityMetrics) Set(string, float64, map[string]string) {}

var reliabilityMetricDefinitions = []ReliabilityMetricDefinition{
	{Name: "video_finalization_total", Kind: ReliabilityMetricCounter, Labels: []string{"status"}},
	{Name: "video_finalization_conflict_total", Kind: ReliabilityMetricCounter},
	{Name: "billing_reservation_active_total", Kind: ReliabilityMetricGauge},
	{Name: "billing_reservation_overrun_total", Kind: ReliabilityMetricCounter},
	{Name: "billing_settlement_retry_total", Kind: ReliabilityMetricCounter},
	{Name: "domain_outbox_pending_total", Kind: ReliabilityMetricGauge, Labels: []string{"event_type"}},
	{Name: "domain_outbox_dead_total", Kind: ReliabilityMetricGauge, Labels: []string{"event_type"}},
	{Name: "domain_outbox_oldest_age_seconds", Kind: ReliabilityMetricGauge},
	{Name: "video_dispatch_unknown_total", Kind: ReliabilityMetricCounter, Labels: []string{"provider"}},
}

func RegisterReliabilityMetrics(registry ReliabilityMetricRegistry) ReliabilityMetricRecorder {
	if registry != nil {
		for _, definition := range reliabilityMetricDefinitions {
			copyDefinition := definition
			copyDefinition.Labels = append([]string(nil), definition.Labels...)
			registry.RegisterReliabilityMetric(copyDefinition)
		}
	}
	if recorder, ok := registry.(ReliabilityMetricRecorder); ok && recorder != nil {
		return recorder
	}
	return noopReliabilityMetrics{}
}
