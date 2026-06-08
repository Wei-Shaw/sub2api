package pluginsdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// dummyMigrationPlugin is a Plugin + MigrationProvider stub the round-trip
// tests serve via the SDK runner. It deliberately keeps zero side effects so
// each test case stays self-contained.
type dummyMigrationPlugin struct {
	manifest *Manifest
	files    map[string][]byte
	openErr  error
}

func (d *dummyMigrationPlugin) Manifest() *Manifest      { return d.manifest }
func (d *dummyMigrationPlugin) Init(PluginContext) error { return nil }
func (d *dummyMigrationPlugin) Shutdown() error          { return nil }

// OpenMigration matches the MigrationProvider contract: unknown filenames
// must surface fs.ErrNotExist so callers can distinguish them from
// transient I/O failures.
func (d *dummyMigrationPlugin) OpenMigration(filename string) ([]byte, error) {
	if d.openErr != nil {
		return nil, d.openErr
	}
	body, ok := d.files[filename]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return body, nil
}

// startMigrationServer mounts a runner over bufconn so tests can drive the
// PluginLifecycle.GetMigration RPC without spawning a real plugin process.
func startMigrationServer(t *testing.T, plugin Plugin) (pb.PluginLifecycleClient, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	r := &runner{
		plugin:   plugin,
		logger:   slog.New(slog.NewTextHandler(testWriter{t: t}, nil)),
		shutdown: make(chan struct{}),
	}
	pb.RegisterPluginLifecycleServer(srv, r)
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		_ = lis.Close()
		t.Fatalf("dial bufconn: %v", err)
	}
	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return pb.NewPluginLifecycleClient(conn), cleanup
}

// testWriter pipes runner logs into the test's t.Log so a failing test still
// shows the SDK side of the conversation.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func TestGetMigration_RoundTrip(t *testing.T) {
	const filename = "001_dummy.sql"
	const sqlBody = "SELECT 1;\n"
	expectedSum := sha256.Sum256([]byte(sqlBody))
	expectedHex := hex.EncodeToString(expectedSum[:])

	plugin := &dummyMigrationPlugin{
		manifest: &Manifest{
			Name: "dummy",
			Migrations: []MigrationDecl{
				{Filename: filename, ChecksumSha256: expectedHex, NonTransactional: true},
			},
		},
		files: map[string][]byte{filename: []byte(sqlBody)},
	}

	client, cleanup := startMigrationServer(t, plugin)
	defer cleanup()

	resp, err := client.GetMigration(context.Background(), &pb.GetMigrationRequest{Filename: filename})
	if err != nil {
		t.Fatalf("GetMigration: %v", err)
	}
	if got := resp.GetFilename(); got != filename {
		t.Errorf("filename: got %q, want %q", got, filename)
	}
	if got := string(resp.GetSql()); got != sqlBody {
		t.Errorf("sql body: got %q, want %q", got, sqlBody)
	}
	if got := resp.GetChecksumSha256(); got != expectedHex {
		t.Errorf("checksum: got %q, want %q", got, expectedHex)
	}
	if !resp.GetNonTransactional() {
		t.Errorf("non_transactional: got false, want true (decl flag should round-trip)")
	}
}

func TestGetMigration_UnknownFilename(t *testing.T) {
	plugin := &dummyMigrationPlugin{
		manifest: &Manifest{Name: "dummy"},
		files:    map[string][]byte{}, // empty: every lookup → fs.ErrNotExist
	}
	client, cleanup := startMigrationServer(t, plugin)
	defer cleanup()

	_, err := client.GetMigration(context.Background(), &pb.GetMigrationRequest{Filename: "missing.sql"})
	if err == nil {
		t.Fatal("GetMigration on missing filename: want error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code: got %v, want NotFound", st.Code())
	}
}

func TestGetMigration_EmptyFilename(t *testing.T) {
	plugin := &dummyMigrationPlugin{manifest: &Manifest{Name: "dummy"}}
	client, cleanup := startMigrationServer(t, plugin)
	defer cleanup()

	_, err := client.GetMigration(context.Background(), &pb.GetMigrationRequest{Filename: ""})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code: got %v, want InvalidArgument", st.Code())
	}
}

// nonProviderPlugin omits MigrationProvider so we can verify the runner
// surfaces the wiring bug as FailedPrecondition instead of letting the
// generated Unimplemented default leak through.
type nonProviderPlugin struct{}

func (nonProviderPlugin) Manifest() *Manifest      { return &Manifest{Name: "no-provider"} }
func (nonProviderPlugin) Init(PluginContext) error { return nil }
func (nonProviderPlugin) Shutdown() error          { return nil }

func TestGetMigration_PluginWithoutProvider(t *testing.T) {
	client, cleanup := startMigrationServer(t, nonProviderPlugin{})
	defer cleanup()

	_, err := client.GetMigration(context.Background(), &pb.GetMigrationRequest{Filename: "any.sql"})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("code: got %v, want FailedPrecondition", st.Code())
	}
}

// providerOpenError verifies that a generic OpenMigration error (not
// fs.ErrNotExist) surfaces as Internal.
func TestGetMigration_OpenError(t *testing.T) {
	plugin := &dummyMigrationPlugin{
		manifest: &Manifest{Name: "dummy"},
		openErr:  errors.New("synthetic disk failure"),
	}
	client, cleanup := startMigrationServer(t, plugin)
	defer cleanup()

	_, err := client.GetMigration(context.Background(), &pb.GetMigrationRequest{Filename: "anything.sql"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("code: got %v, want Internal", st.Code())
	}
}
