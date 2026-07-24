package deployer

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenAndServeRefusesLiveUnixSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "s2a-http-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := Config{SocketPath: filepath.Join(dir, "deployer.sock")}
	listener, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewHTTPServer(cfg, &Manager{})
	err = server.ListenAndServe()
	if err == nil || !strings.Contains(err.Error(), "already accepting connections") {
		t.Fatalf("ListenAndServe error=%v, want live socket rejection", err)
	}
	connection, err := net.Dial("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("original live socket was removed: %v", err)
	}
	_ = connection.Close()
}
