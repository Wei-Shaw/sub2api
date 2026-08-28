package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("instance not found")
	ErrPortInUse     = errors.New("listen port already allocated")
	ErrNoFreePort    = errors.New("no free port in range")
	ErrAlreadyExists = errors.New("instance already exists")
	ErrNameExists    = errors.New("instance name already exists")
)

type Status string

const (
	StatusRegistered Status = "registered"
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusStopping   Status = "stopping"
	StatusStopped    Status = "stopped"
	StatusUnhealthy  Status = "unhealthy"
	StatusError      Status = "error"
)

type DesiredState string

const (
	DesiredRunning DesiredState = "running"
	DesiredStopped DesiredState = "stopped"
)

// Profile is a WARP/WireGuard profile (or mock placeholder).
type Profile struct {
	PrivateKey string       `json:"private_key,omitempty"`
	Address    []string     `json:"address,omitempty"`
	DNS        []string     `json:"dns,omitempty"`
	MTU        int          `json:"mtu,omitempty"`
	Peers      []PeerConfig `json:"peers,omitempty"`
	// LicenseKey is optional metadata for registration flows.
	LicenseKey string `json:"license_key,omitempty"`
	// Cloudflare free-device registration (for DELETE /reg/{id}).
	DeviceID    string `json:"device_id,omitempty"`
	AccessToken string `json:"access_token,omitempty"` // encrypted at rest with PrivateKey
	AccountID   string `json:"account_id,omitempty"`
	// MockExitIP forces mock runtime probe result (local tests).
	MockExitIP string `json:"mock_exit_ip,omitempty"`
}

type PeerConfig struct {
	PublicKey  string   `json:"public_key"`
	Endpoint   string   `json:"endpoint"`
	AllowedIPs []string `json:"allowed_ips"`
}

// Instance is a managed WARP SOCKS endpoint.
type Instance struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	ListenHost    string       `json:"listen_host"`
	ListenPort    int          `json:"listen_port"`
	Runtime       string       `json:"runtime"`
	Status        Status       `json:"status"`
	DesiredState  DesiredState `json:"desired_state"`
	Profile       Profile      `json:"profile"`
	ExitIP        string       `json:"exit_ip,omitempty"`
	ExitColo      string       `json:"exit_colo,omitempty"`
	LatencyMs     *int64       `json:"latency_ms,omitempty"`
	LastHealthAt  *time.Time   `json:"last_health_at,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	FailCount     int          `json:"fail_count"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	SocksAuthUser string       `json:"socks_auth_user,omitempty"`
	SocksAuthPass string       `json:"socks_auth_pass,omitempty"`
}

func (i Instance) SocksURL() string {
	return fmt.Sprintf("socks5h://%s:%d", i.ListenHost, i.ListenPort)
}

// ProfileKeyCipher encrypts/decrypts profile private keys for at-rest storage.
type ProfileKeyCipher interface {
	EncryptString(plain string) (string, error)
	DecryptString(value string) (string, error)
}

// Store is a file-backed instance registry.
type Store struct {
	mu      sync.RWMutex
	dir     string
	path    string
	byID    map[string]*Instance
	ports   map[int]string // port -> id
	portMin int
	portMax int
	cipher  ProfileKeyCipher
}

func New(dir string, portMin, portMax int) (*Store, error) {
	return NewWithCipher(dir, portMin, portMax, nil)
}

func NewWithCipher(dir string, portMin, portMax int, cipher ProfileKeyCipher) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		dir:     dir,
		path:    filepath.Join(dir, "instances.json"),
		byID:    make(map[string]*Instance),
		ports:   make(map[int]string),
		portMin: portMin,
		portMax: portMax,
		cipher:  cipher,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []Instance
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	for i := range list {
		inst := list[i]
		if s.cipher != nil {
			if inst.Profile.PrivateKey != "" {
				plain, err := s.cipher.DecryptString(inst.Profile.PrivateKey)
				if err != nil {
					return fmt.Errorf("decrypt profile for %s: %w", inst.ID, err)
				}
				inst.Profile.PrivateKey = plain
			}
			if inst.Profile.AccessToken != "" {
				plain, err := s.cipher.DecryptString(inst.Profile.AccessToken)
				if err != nil {
					return fmt.Errorf("decrypt access_token for %s: %w", inst.ID, err)
				}
				inst.Profile.AccessToken = plain
			}
		}
		cp := inst
		s.byID[inst.ID] = &cp
		s.ports[inst.ListenPort] = inst.ID
	}
	return nil
}

func (s *Store) persistLocked(byID map[string]*Instance) error {
	list := make([]Instance, 0, len(byID))
	for _, inst := range byID {
		cp := *inst
		if s.cipher != nil {
			if cp.Profile.PrivateKey != "" {
				enc, err := s.cipher.EncryptString(cp.Profile.PrivateKey)
				if err != nil {
					return fmt.Errorf("encrypt profile key for %s: %w", cp.ID, err)
				}
				cp.Profile.PrivateKey = enc
			}
			if cp.Profile.AccessToken != "" {
				enc, err := s.cipher.EncryptString(cp.Profile.AccessToken)
				if err != nil {
					return fmt.Errorf("encrypt access_token for %s: %w", cp.ID, err)
				}
				cp.Profile.AccessToken = enc
			}
		}
		list = append(list, cp)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// DecryptProfile returns a copy of profile with private key decrypted for runtime use.
func (s *Store) DecryptProfile(p Profile) (Profile, error) {
	if s == nil || s.cipher == nil || p.PrivateKey == "" {
		return p, nil
	}
	plain, err := s.cipher.DecryptString(p.PrivateKey)
	if err != nil {
		return p, err
	}
	p.PrivateKey = plain
	return p, nil
}

func (s *Store) AllocatePort(preferred int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if preferred > 0 {
		if _, used := s.ports[preferred]; used {
			return 0, ErrPortInUse
		}
		if preferred < s.portMin || preferred > s.portMax {
			return 0, fmt.Errorf("port %d outside range %d-%d", preferred, s.portMin, s.portMax)
		}
		return preferred, nil
	}
	for p := s.portMin; p <= s.portMax; p++ {
		if _, used := s.ports[p]; !used {
			return p, nil
		}
	}
	return 0, ErrNoFreePort
}

// NameSet returns a copy of all instance names currently registered.
func (s *Store) NameSet() map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]struct{}, len(s.byID))
	for _, inst := range s.byID {
		if inst == nil || inst.Name == "" {
			continue
		}
		out[inst.Name] = struct{}{}
	}
	return out
}

func (s *Store) Create(inst *Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[inst.ID]; ok {
		return ErrAlreadyExists
	}
	if owner, used := s.ports[inst.ListenPort]; used {
		return fmt.Errorf("%w: %d owned by %s", ErrPortInUse, inst.ListenPort, owner)
	}
	// Reject duplicate display names so multi-batch pool creation stays unique.
	for _, existing := range s.byID {
		if existing != nil && existing.Name == inst.Name {
			return fmt.Errorf("%w: %q", ErrNameExists, inst.Name)
		}
	}
	cp := cloneInstance(inst)
	now := time.Now().UTC()
	cp.CreatedAt = now
	cp.UpdatedAt = now
	if cp.Status == "" {
		cp.Status = StatusRegistered
	}
	if cp.DesiredState == "" {
		cp.DesiredState = DesiredRunning
	}
	candidate := cloneInstances(s.byID)
	candidate[inst.ID] = &cp
	if err := s.persistLocked(candidate); err != nil {
		return err
	}
	s.byID = candidate
	s.ports[inst.ListenPort] = inst.ID
	*inst = cloneInstance(&cp)
	return nil
}

func (s *Store) Update(id string, mut func(*Instance)) (*Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := cloneInstance(inst)
	mut(&cp)
	cp.UpdatedAt = time.Now().UTC()
	candidate := cloneInstances(s.byID)
	candidate[id] = &cp
	if err := s.persistLocked(candidate); err != nil {
		return nil, err
	}
	s.byID = candidate
	out := cloneInstance(&cp)
	return &out, nil
}

func (s *Store) Get(id string) (*Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inst, ok := s.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := cloneInstance(inst)
	return &out, nil
}

func (s *Store) List() []Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Instance, 0, len(s.byID))
	for _, inst := range s.byID {
		out = append(out, cloneInstance(inst))
	}
	return out
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	candidate := cloneInstances(s.byID)
	delete(candidate, id)
	if err := s.persistLocked(candidate); err != nil {
		return err
	}
	s.byID = candidate
	delete(s.ports, inst.ListenPort)
	return nil
}

func cloneInstances(src map[string]*Instance) map[string]*Instance {
	dst := make(map[string]*Instance, len(src))
	for id, inst := range src {
		if inst == nil {
			dst[id] = nil
			continue
		}
		cp := cloneInstance(inst)
		dst[id] = &cp
	}
	return dst
}

func cloneInstance(inst *Instance) Instance {
	cp := *inst
	cp.Profile.Address = append([]string(nil), inst.Profile.Address...)
	cp.Profile.DNS = append([]string(nil), inst.Profile.DNS...)
	cp.Profile.Peers = make([]PeerConfig, len(inst.Profile.Peers))
	for i := range inst.Profile.Peers {
		cp.Profile.Peers[i] = inst.Profile.Peers[i]
		cp.Profile.Peers[i].AllowedIPs = append([]string(nil), inst.Profile.Peers[i].AllowedIPs...)
	}
	if inst.LatencyMs != nil {
		value := *inst.LatencyMs
		cp.LatencyMs = &value
	}
	if inst.LastHealthAt != nil {
		value := *inst.LastHealthAt
		cp.LastHealthAt = &value
	}
	return cp
}
