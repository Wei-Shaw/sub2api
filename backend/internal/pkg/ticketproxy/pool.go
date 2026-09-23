// Package ticketproxy owns the persistent pool used only for Codex ticket probes.
package ticketproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const FailureLimit = 1
const MaxProxies = 2000
const MaxPoolProxies = 20000

var Default = NewStore("/app/data/ticket-proxy-pool.json")

type Proxy struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (p Proxy) URL() string {
	u := url.URL{Scheme: p.Protocol, Host: net.JoinHostPort(p.Host, strconv.Itoa(p.Port))}
	if p.Username != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}

// Parse accepts URL, host:port and host:port:user:password formats. Errors never echo credentials.
func Parse(lines []string) ([]Proxy, int, error) {
	if len(lines) > MaxProxies {
		return nil, 0, fmt.Errorf("最多一次导入 %d 条代理", MaxProxies)
	}
	pool := []Proxy{}
	seen := map[Proxy]bool{}
	duplicates := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		bad := func() ([]Proxy, int, error) {
			return nil, 0, fmt.Errorf("第 %d 行格式不正确，请使用 IP:端口、IP:端口:用户名:密码或完整代理 URL", i+1)
		}
		if !strings.Contains(line, "://") {
			if !strings.HasPrefix(line, "[") && strings.Count(line, ":") >= 3 {
				parts := strings.SplitN(line, ":", 4)
				line = "http://" + url.UserPassword(parts[2], parts[3]).String() + "@" + parts[0] + ":" + parts[1]
			} else {
				line = "http://" + line
			}
		}
		u, err := url.Parse(line)
		if err != nil || u.Opaque != "" || u.Hostname() == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(u.Hostname(), " \t\r\n") {
			return bad()
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return bad()
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return bad()
		}
		if strings.Contains(u.Hostname(), ":") && net.ParseIP(u.Hostname()) == nil {
			return bad()
		}
		p := Proxy{Protocol: u.Scheme, Host: strings.ToLower(u.Hostname()), Port: port}
		if u.User != nil {
			p.Username = u.User.Username()
			p.Password, _ = u.User.Password()
			if p.Username == "" || p.Password == "" {
				return bad()
			}
		}
		if seen[p] {
			duplicates++
			continue
		}
		seen[p] = true
		pool = append(pool, p)
	}
	if len(pool) == 0 {
		return nil, 0, errors.New("请至少填写一条代理")
	}
	return pool, duplicates, nil
}

type entry struct {
	Proxy
	ID       string `json:"id"`
	Failures int    `json:"failures"`
}
type state struct {
	Version int     `json:"version"`
	Proxies []entry `json:"proxies"`
	Removed int     `json:"removed_count"`
}
type Status struct {
	Managed      bool `json:"managed"`
	Count        int  `json:"count"`
	Removed      int  `json:"removed_count"`
	FailureLimit int  `json:"failure_limit"`
}
type ImportResult struct {
	Status
	Added      int `json:"added"`
	Duplicates int `json:"duplicates"`
}
type Lease struct{ ID, URL string }
type Store struct {
	mu     sync.Mutex
	path   string
	cursor int
}

func NewStore(path string) *Store { return &Store{path: path} }

// Read under the mutex so persisted updates and imported legacy pools are observed.
func (s *Store) read() (state, bool, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return state{Version: 1}, false, nil
	}
	if err != nil {
		return state{}, true, err
	}
	data = bytes.TrimSpace(data)
	var st state
	if len(data) > 0 && data[0] == '[' {
		if err = json.Unmarshal(data, &st.Proxies); err != nil {
			return state{}, true, err
		}
		st.Version = 1
		for i := range st.Proxies {
			st.Proxies[i].ID = uuid.NewString()
		}
		if err = s.write(st); err != nil {
			return state{}, true, err
		}
	} else if err = json.Unmarshal(data, &st); err != nil {
		return state{}, true, err
	}
	if st.Version != 1 {
		return state{}, true, errors.New("unsupported ticket proxy pool version")
	}
	return st, true, nil
}

func (s *Store) write(st state) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".ticket-pool-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
func summary(st state, managed bool) Status {
	return Status{Managed: managed, Count: len(st.Proxies), Removed: st.Removed, FailureLimit: FailureLimit}
}
func (s *Store) Status() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, managed, err := s.read()
	return summary(st, managed), err
}
func (s *Store) Import(proxies []Proxy, duplicates int) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _, err := s.read()
	if err != nil {
		return ImportResult{}, err
	}
	seen := map[Proxy]bool{}
	for _, p := range st.Proxies {
		seen[p.Proxy] = true
	}
	added := 0
	for _, p := range proxies {
		if seen[p] {
			duplicates++
			continue
		}
		seen[p] = true
		added++
		st.Proxies = append(st.Proxies, entry{Proxy: p, ID: uuid.NewString()})
	}
	if len(st.Proxies) > MaxPoolProxies {
		return ImportResult{}, fmt.Errorf("代理池最多保留 %d 条代理", MaxPoolProxies)
	}
	if err = s.write(st); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Status: summary(st, true), Added: added, Duplicates: duplicates}, nil
}

// A managed empty pool must never fall back to the old rotator or direct access.
func (s *Store) Acquire() (Lease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, managed, err := s.read()
	if err != nil || len(st.Proxies) == 0 {
		return Lease{}, managed, err
	}
	index := s.cursor % len(st.Proxies)
	s.cursor = (index + 1) % len(st.Proxies)
	p := st.Proxies[index]
	return Lease{ID: p.ID, URL: p.URL()}, true, nil
}

// IDs prevent old in-flight results from deleting a proxy re-imported later.
func (s *Store) Report(id string, success bool) (bool, error) {
	if id == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _, err := s.read()
	if err != nil {
		return false, err
	}
	index := slices.IndexFunc(st.Proxies, func(p entry) bool { return p.ID == id })
	if index < 0 {
		return false, nil
	}
	p := &st.Proxies[index]
	if success {
		if p.Failures == 0 {
			return false, nil
		}
		p.Failures = 0
	} else {
		p.Failures++
	}
	removed := p.Failures >= FailureLimit
	if removed {
		st.Proxies = slices.Delete(st.Proxies, index, index+1)
		st.Removed++
	}
	if err = s.write(st); err != nil {
		return false, err
	}
	return removed, nil
}
