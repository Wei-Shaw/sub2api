package deployer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	composeProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	composeServicePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	containerNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]+$`)
)

type Config struct {
	LoadedFrom                 string            `json:"-"`
	SocketPath                 string            `json:"socket_path"`
	SocketMode                 uint32            `json:"socket_mode"`
	SocketGID                  int               `json:"socket_gid"`
	StatePath                  string            `json:"state_path"`
	ImageStatePath             string            `json:"image_state_path"`
	ImageRepository            string            `json:"image_repository"`
	RequiredImageLabels        map[string]string `json:"required_image_labels"`
	DockerBinary               string            `json:"docker_binary"`
	ComposeWorkDir             string            `json:"compose_work_dir"`
	ComposeProject             string            `json:"compose_project"`
	ComposeEnvFiles            []string          `json:"compose_env_files"`
	ComposeFiles               []string          `json:"compose_files"`
	ComposeService             string            `json:"compose_service"`
	ImageEnvironment           string            `json:"image_environment"`
	ContainerPort              int               `json:"container_port"`
	DeploymentStatePath        string            `json:"deployment_state_path"`
	DeploymentStateFile        string            `json:"deployment_state_file"`
	Slots                      []Slot            `json:"slots"`
	InitialContainer           string            `json:"initial_container"`
	InitialVersion             string            `json:"initial_version"`
	NginxUpstreamPath          string            `json:"nginx_upstream_path"`
	NginxSitePath              string            `json:"nginx_site_path"`
	NginxUpstreamName          string            `json:"nginx_upstream_name"`
	NginxTestCommand           []string          `json:"nginx_test_command"`
	NginxDumpCommand           []string          `json:"nginx_dump_command"`
	NginxReloadCommand         []string          `json:"nginx_reload_command"`
	NginxProbeURL              string            `json:"nginx_probe_url"`
	NginxProbeHost             string            `json:"nginx_probe_host,omitempty"`
	RouteConfirmationTimeout   Duration          `json:"route_confirmation_timeout"`
	HealthPath                 string            `json:"health_path"`
	HealthTimeout              Duration          `json:"health_timeout"`
	StabilizeDuration          Duration          `json:"stabilize_duration"`
	DrainDuration              Duration          `json:"drain_duration"`
	DrainTimeout               Duration          `json:"drain_timeout"`
	StopTimeout                Duration          `json:"stop_timeout"`
	ControlPlaneUpgradePath    string            `json:"control_plane_upgrade_path,omitempty"`
	ControlPlaneUpgradeCommand []string          `json:"control_plane_upgrade_command,omitempty"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.New("duration must be a string such as 90s or 5m")
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	cfg.LoadedFrom = path
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.SocketMode == 0 {
		c.SocketMode = 0660
	}
	if c.DockerBinary == "" {
		c.DockerBinary = "docker"
	}
	if c.ImageEnvironment == "" {
		c.ImageEnvironment = "SUB2API_IMAGE"
	}
	if c.ContainerPort == 0 {
		c.ContainerPort = 8080
	}
	if c.DeploymentStateFile == "" {
		c.DeploymentStateFile = "/run/sub2api-deployment/active-slot"
	}
	if c.DeploymentStatePath == "" && c.StatePath != "" {
		c.DeploymentStatePath = filepath.Join(filepath.Dir(c.StatePath), "runtime", "active-slot")
	}
	if c.NginxUpstreamName == "" {
		c.NginxUpstreamName = "sub2api_managed"
	}
	if c.HealthPath == "" {
		c.HealthPath = "/health"
	}
	if c.RouteConfirmationTimeout.Duration == 0 {
		c.RouteConfirmationTimeout.Duration = 10 * time.Second
	}
	if c.HealthTimeout.Duration == 0 {
		c.HealthTimeout.Duration = 12 * time.Minute
	}
	if c.StabilizeDuration.Duration == 0 {
		c.StabilizeDuration.Duration = 15 * time.Second
	}
	if c.DrainDuration.Duration == 0 {
		c.DrainDuration.Duration = 10 * time.Second
	}
	if c.DrainTimeout.Duration == 0 {
		c.DrainTimeout.Duration = 30 * time.Minute
	}
	if c.StopTimeout.Duration == 0 {
		c.StopTimeout.Duration = 2 * time.Minute
	}
	for i := range c.ComposeEnvFiles {
		c.ComposeEnvFiles[i] = resolvePath(c.ComposeWorkDir, c.ComposeEnvFiles[i])
	}
	for i := range c.ComposeFiles {
		c.ComposeFiles[i] = resolvePath(c.ComposeWorkDir, c.ComposeFiles[i])
	}
}

func resolvePath(base, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(base, value)
}

func (c Config) validate() error {
	required := map[string]string{
		"socket_path":         c.SocketPath,
		"state_path":          c.StatePath,
		"image_state_path":    c.ImageStatePath,
		"image_repository":    c.ImageRepository,
		"compose_work_dir":    c.ComposeWorkDir,
		"compose_project":     c.ComposeProject,
		"compose_service":     c.ComposeService,
		"initial_container":   c.InitialContainer,
		"nginx_upstream_path": c.NginxUpstreamPath,
		"nginx_site_path":     c.NginxSitePath,
		"nginx_probe_url":     c.NginxProbeURL,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(c.ComposeFiles) == 0 {
		return errors.New("compose_files must not be empty")
	}
	if !composeProjectPattern.MatchString(c.ComposeProject) {
		return errors.New("compose_project must match ^[a-z0-9][a-z0-9_-]*$")
	}
	if !composeServicePattern.MatchString(c.ComposeService) {
		return errors.New("compose_service must match ^[A-Za-z0-9][A-Za-z0-9_.-]*$")
	}
	if !containerNamePattern.MatchString(c.InitialContainer) {
		return errors.New("initial_container must match ^[A-Za-z0-9][A-Za-z0-9_.-]+$")
	}
	if len(c.Slots) != 2 {
		return errors.New("exactly two deployment slots are required")
	}
	if !filepath.IsAbs(c.DeploymentStateFile) {
		return errors.New("deployment_state_file must be an absolute container path")
	}
	if !filepath.IsAbs(c.DeploymentStatePath) {
		return errors.New("deployment_state_path must be an absolute host path")
	}
	if c.Slots[0].Name == c.Slots[1].Name || c.Slots[0].Port == c.Slots[1].Port {
		return errors.New("deployment slots must use distinct names and ports")
	}
	for _, slot := range c.Slots {
		if strings.TrimSpace(slot.Name) == "" || strings.TrimSpace(slot.Host) == "" || slot.Port < 1 || slot.Port > 65535 {
			return fmt.Errorf("invalid slot: %+v", slot)
		}
		if !containerNamePattern.MatchString(slot.Name) {
			return fmt.Errorf("deployment slot name %q must match ^[A-Za-z0-9][A-Za-z0-9_.-]+$", slot.Name)
		}
		if slot.Host != "127.0.0.1" {
			return fmt.Errorf("deployment slot %s must bind to 127.0.0.1", slot.Name)
		}
	}
	if c.SocketMode&0007 != 0 {
		return errors.New("socket_mode must not grant access to other users")
	}
	if c.SocketGID < 0 {
		return errors.New("socket_gid must not be negative")
	}
	if len(c.NginxTestCommand) == 0 || len(c.NginxDumpCommand) == 0 || len(c.NginxReloadCommand) == 0 {
		return errors.New("nginx test, dump, and reload commands are required")
	}
	probeURL, err := url.Parse(c.NginxProbeURL)
	if err != nil || probeURL.Scheme != "http" || probeURL.Host == "" || probeURL.User != nil || probeURL.RawQuery != "" || probeURL.Fragment != "" {
		return errors.New("nginx_probe_url must be an absolute http URL without user information, query, or fragment")
	}
	probeIP := net.ParseIP(probeURL.Hostname())
	if probeIP == nil || !probeIP.IsLoopback() {
		return errors.New("nginx_probe_url must use a literal loopback IP address")
	}
	if probeURL.Path != c.HealthPath {
		return fmt.Errorf("nginx_probe_url path must equal health_path %q", c.HealthPath)
	}
	if len(c.RequiredImageLabels) == 0 {
		return errors.New("required_image_labels must not be empty")
	}
	if c.RouteConfirmationTimeout.Duration < time.Second || c.HealthTimeout.Duration < time.Second || c.DrainTimeout.Duration < time.Second || c.StopTimeout.Duration < time.Second {
		return errors.New("route_confirmation_timeout, health_timeout, drain_timeout, and stop_timeout must be at least one second")
	}
	if c.HealthTimeout.Duration < 10*time.Minute {
		return errors.New("health_timeout must cover the 10 minute application migration budget")
	}
	if c.StabilizeDuration.Duration < 0 || c.DrainDuration.Duration < 0 {
		return errors.New("stabilize_duration and drain_duration must not be negative")
	}
	if c.DrainDuration.Duration >= c.DrainTimeout.Duration {
		return errors.New("drain_duration must be shorter than drain_timeout")
	}
	if (c.ControlPlaneUpgradePath == "") != (len(c.ControlPlaneUpgradeCommand) == 0) {
		return errors.New("control_plane_upgrade_path and control_plane_upgrade_command must be configured together")
	}
	if c.ControlPlaneUpgradePath != "" {
		if !filepath.IsAbs(c.ControlPlaneUpgradePath) {
			return errors.New("control_plane_upgrade_path must be an absolute path")
		}
		if !filepath.IsAbs(c.ControlPlaneUpgradeCommand[0]) {
			return errors.New("control_plane_upgrade_command executable must be an absolute path")
		}
	}
	return nil
}
