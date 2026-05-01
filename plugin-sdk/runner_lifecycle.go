package pluginsdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetMigration returns the SQL body for a migration the plugin declared in
// Manifest.Migrations. The host calls this immediately after Init for every
// MigrationDecl entry, re-verifies the SHA-256 against the manifest pin,
// and applies the body under MigrationRunner's advisory lock.
//
// The dispatch is to the plugin's MigrationProvider implementation. Plugins
// that declare migrations but forget to implement the provider receive
// codes.FailedPrecondition so the bug surfaces at startup instead of being
// masked by the proto-generated codes.Unimplemented default. The SDK also
// computes the SHA-256 here so the host can cross-check the SDK-reported
// checksum against the manifest pin (a third tripwire on top of the
// host-side recomputation in fetchAndRunPluginMigrations).
func (r *runner) GetMigration(_ context.Context, req *pb.GetMigrationRequest) (*pb.GetMigrationResponse, error) {
	provider, ok := r.plugin.(MigrationProvider)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition,
			"pluginsdk: plugin does not implement MigrationProvider — declare Manifest.Migrations together with OpenMigration",
		)
	}
	filename := req.GetFilename()
	if filename == "" {
		return nil, status.Error(codes.InvalidArgument, "pluginsdk: GetMigration filename is empty")
	}

	body, err := provider.OpenMigration(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, status.Errorf(codes.NotFound, "pluginsdk: migration %q not found", filename)
		}
		return nil, status.Errorf(codes.Internal, "pluginsdk: open migration %q: %v", filename, err)
	}

	sum := sha256.Sum256(body)
	return &pb.GetMigrationResponse{
		Filename:         filename,
		Sql:              body,
		ChecksumSha256:   hex.EncodeToString(sum[:]),
		NonTransactional: r.lookupMigrationNonTx(filename),
	}, nil
}

// lookupMigrationNonTx returns whether the plugin's manifest declared the
// given migration filename as non-transactional. The SDK echoes this back to
// the host so plugins do not have to keep the flag in two places. Returns
// false when the manifest has no matching declaration — the host has
// already pinned what to fetch by filename and applies whichever subset of
// declarations it cares about, so a missing entry is harmless.
func (r *runner) lookupMigrationNonTx(filename string) bool {
	m := r.plugin.Manifest()
	if m == nil {
		return false
	}
	for _, decl := range m.Migrations {
		if decl.Filename == filename {
			return decl.NonTransactional
		}
	}
	return false
}

// GetFrontendBundle streams a single file from the plugin's frontend assets.
// The file path semantics are defined by the plugin's
// FrontendBundleProvider implementation.
func (r *runner) GetFrontendBundle(req *pb.FrontendBundleRequest, stream grpc.ServerStreamingServer[pb.FileChunk]) error {
	provider, ok := r.plugin.(FrontendBundleProvider)
	if !ok {
		return errors.New("pluginsdk: plugin does not provide a frontend bundle")
	}
	data, err := provider.OpenFrontendFile(req.GetPath())
	if err != nil {
		return err
	}
	for offset := 0; offset < len(data); offset += frontendChunkSize {
		end := offset + frontendChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := &pb.FileChunk{
			Data:     data[offset:end],
			Filename: req.GetPath(),
			Eof:      end == len(data),
		}
		if err := stream.Send(chunk); err != nil {
			return err
		}
	}
	if len(data) == 0 {
		return stream.Send(&pb.FileChunk{Filename: req.GetPath(), Eof: true})
	}
	return nil
}
