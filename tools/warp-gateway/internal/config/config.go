package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds warp-gateway process configuration.
type Config struct {
	Listen           string        `json:"listen"`
	Token            string        `json:"token"`
	DataDir          string        `json:"data_dir"`
	DefaultHost      string        `json:"default_host"`
	PortRangeStart   int           `json:"port_range_start"`
	PortRangeEnd     int           `json:"port_range_end"`
	HealthInterval   time.Duration `json:"health_interval"`
	ProbeURL         string        `json:"probe_url"`
	Runtime          string        `json:"runtime"` // mock | sing-box
	SingBoxPath      string        `json:"sing_box_path"`
	UnhealthyAfter   int           `json:"unhealthy_after"`
	ReconcileOnStart bool          `json:"reconcile_on_start"`
	// ProfileKey encrypts private keys at rest (AES-256-GCM). Falls back to Token.
	ProfileKey string `json:"profile_key,omitempty"`
	// TLS for multi-host control plane (optional).
	TLSCertFile string `json:"tls_cert_file,omitempty"`
	TLSKeyFile  string `json:"tls_key_file,omitempty"`
	// ClientCAFile enables mTLS: require client certs signed by this CA.
	ClientCAFile string `json:"client_ca_file,omitempty"`
}

func Default() Config {
	return Config{
		Listen:           "127.0.0.1:19798",
		Token:            "",
		DataDir:          "./data/warp-gateway",
		DefaultHost:      "127.0.0.1",
		PortRangeStart:   41001,
		PortRangeEnd:     41100,
		HealthInterval:   30 * time.Second,
		// Use IP URL so health probes work even when local DNS is fake-ip hijacked.
		ProbeURL:         "https://1.1.1.1/cdn-cgi/trace",
		Runtime:          "mock",
		SingBoxPath:      "sing-box",
		UnhealthyAfter:   3,
		ReconcileOnStart: true,
	}
}

// LoadFromEnv overlays environment variables on defaults.
func LoadFromEnv() Config {
	cfg := Default()
	if v := os.Getenv("WARP_GATEWAY_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("WARP_GATEWAY_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("WARP_GATEWAY_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("WARP_GATEWAY_RUNTIME"); v != "" {
		cfg.Runtime = v
	}
	if v := os.Getenv("WARP_GATEWAY_SING_BOX"); v != "" {
		cfg.SingBoxPath = v
	}
	if v := os.Getenv("WARP_GATEWAY_PROBE_URL"); v != "" {
		cfg.ProbeURL = v
	}
	if v := os.Getenv("WARP_GATEWAY_DEFAULT_HOST"); v != "" {
		cfg.DefaultHost = v
	}
	if v := os.Getenv("WARP_GATEWAY_PROFILE_KEY"); v != "" {
		cfg.ProfileKey = v
	}
	if v := os.Getenv("WARP_GATEWAY_TLS_CERT"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("WARP_GATEWAY_TLS_KEY"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("WARP_GATEWAY_CLIENT_CA"); v != "" {
		cfg.ClientCAFile = v
	}
	return cfg
}

// ProfileSecret returns material used for at-rest profile encryption.
func (c Config) ProfileSecret() string {
	if c.ProfileKey != "" {
		return c.ProfileKey
	}
	return c.Token
}

func (c Config) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("listen is required")
	}
	if c.PortRangeStart <= 0 || c.PortRangeEnd < c.PortRangeStart {
		return fmt.Errorf("invalid port range %d-%d", c.PortRangeStart, c.PortRangeEnd)
	}
	switch c.Runtime {
	case "mock", "sing-box":
	default:
		return fmt.Errorf("unsupported runtime %q (mock|sing-box)", c.Runtime)
	}
	return nil
}

func (c Config) String() string {
	b, _ := json.Marshal(c)
	return string(b)
}
