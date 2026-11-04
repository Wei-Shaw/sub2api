package chatgptnetwork

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuntimeToggleSelectsSeparatePools(t *testing.T) {
	old := runtimeValue.Load()
	t.Cleanup(func() { runtimeValue.Store(old) })
	var networks []string
	base := &http.Transport{DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
		networks = append(networks, network)
		return nil, errors.New("observed")
	}}
	rt, ok := Wrap(base, false).(*transport)
	if !ok {
		t.Fatal("unexpected transport type")
	}
	defer rt.CloseIdleConnections()
	for _, enabled := range []bool{false, true, false, true} {
		SetEnabled(enabled)
		req, _ := http.NewRequest("GET", "http://chatgpt.com/test", nil)
		_, _ = rt.RoundTrip(req)
	}
	want := []string{"tcp", "tcp6", "tcp", "tcp6"}
	for i, n := range want {
		if networks[i] != n {
			t.Fatalf("networks %v", networks)
		}
	}
	SetEnabled(true)
	req, _ := http.NewRequest("GET", "http://api.anthropic.com/test", nil)
	_, _ = rt.RoundTrip(req)
	if networks[4] != "tcp" {
		t.Fatal("other platform affected")
	}
}

func TestRuntimeToggleDoesNotReuseIPv4Connection(t *testing.T) {
	old := runtimeValue.Load()
	t.Cleanup(func() { runtimeValue.Store(old) })
	v4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ipv4") }))
	defer v4.Close()
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip(err)
	}
	v6 := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ipv6") }))
	_ = v6.Listener.Close()
	v6.Listener = ln
	v6.Start()
	defer v6.Close()
	base := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		addr := v4.Listener.Addr().String()
		if network == "tcp6" {
			addr = v6.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	client := &http.Client{Transport: Wrap(base, false)}
	defer client.CloseIdleConnections()
	for _, enabled := range []bool{false, false, true, true, false, true} {
		SetEnabled(enabled)
		resp, err := client.Get("http://chatgpt.com/test")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		want := "ipv4"
		if enabled {
			want = "ipv6"
		}
		if string(body) != want {
			t.Fatalf("got %s want %s", body, want)
		}
	}
}

func TestPersistedPolicySynchronizesIndependentInstances(t *testing.T) {
	var persisted atomic.Bool
	var unavailable atomic.Bool
	load := func(context.Context) (bool, error) {
		if unavailable.Load() {
			return false, errors.New("database unavailable")
		}
		return persisted.Load(), nil
	}
	first := &policyCache{load: load}
	second := &policyCache{load: load}
	for _, enabled := range []bool{false, true, false} {
		persisted.Store(enabled)
		for _, instance := range []*policyCache{first, second} {
			refreshPolicyForTest(t, instance)
			if got := instance.get(); got != enabled {
				t.Fatalf("got %v want %v", got, enabled)
			}
		}
	}
	persisted.Store(true)
	refreshPolicyForTest(t, first)
	if !first.get() {
		t.Fatal("did not refresh")
	}
	unavailable.Store(true)
	refreshPolicyForTest(t, first)
	if !first.get() {
		t.Fatal("storage failure reset the policy")
	}
}

func TestPersistedPolicyCachesReadsAndAppliesLocalSave(t *testing.T) {
	old := persistedPolicy.Load()
	oldValue := runtimeValue.Load()
	t.Cleanup(func() { persistedPolicy.Store(old); runtimeValue.Store(oldValue) })
	calls := 0
	ConfigureSource(false, func(context.Context) (bool, error) { calls++; return false, nil })
	for i := 0; i < 10; i++ {
		if Enabled() {
			t.Fatal("default changed")
		}
	}
	if calls != 0 {
		t.Fatal("cache not used")
	}
	SetEnabled(true)
	if !Enabled() {
		t.Fatal("local save not applied")
	}
	p := persistedPolicy.Load()
	refreshPolicyForTest(t, p)
	if Enabled() {
		t.Fatal("remote save not applied")
	}
	if calls != 1 {
		t.Fatalf("unexpected reads: %d", calls)
	}
}

func refreshPolicyForTest(t *testing.T, p *policyCache) {
	t.Helper()
	p.mu.Lock()
	p.next = time.Time{}
	p.mu.Unlock()
	p.get()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		done := !p.refreshing
		p.mu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("policy refresh did not finish")
}
func TestPersistedPolicySlowStorageDoesNotBlockRequestsOrOverwriteSave(t *testing.T) {
	old := persistedPolicy.Load()
	oldValue := runtimeValue.Load()
	t.Cleanup(func() { persistedPolicy.Store(old); runtimeValue.Store(oldValue) })
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	p := &policyCache{load: func(context.Context) (bool, error) { calls.Add(1); close(started); <-release; return false, nil }}
	persistedPolicy.Store(p)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			Enabled()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("requests blocked on storage")
	}
	<-started
	if calls.Load() != 1 {
		t.Fatal("concurrent duplicate refresh")
	}
	SetEnabled(true)
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		finished := !p.refreshing
		p.mu.Unlock()
		if finished {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !Enabled() {
		t.Fatal("stale read overwrote local save")
	}
}

func TestPersistedPolicyIdleInstancePollsWithoutRequests(t *testing.T) {
	old := persistedPolicy.Load()
	t.Cleanup(func() { persistedPolicy.Store(old) })
	read := make(chan struct{})
	p := &policyCache{load: func(context.Context) (bool, error) { close(read); return true, nil }}
	persistedPolicy.Store(p)
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() { p.watch(ticks); close(done) }()
	ticks <- time.Now()
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("idle instance did not refresh")
	}
	close(ticks)
	<-done
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		value, refreshing := p.value, p.refreshing
		p.mu.Unlock()
		if !refreshing {
			if !value {
				t.Fatal("remote policy not applied")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("refresh did not finish")
}
