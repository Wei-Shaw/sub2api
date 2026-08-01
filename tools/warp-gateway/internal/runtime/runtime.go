package runtime

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

// Handle is a running instance process/listener.
type Handle interface {
	Stop(ctx context.Context) error
	// LocalAddr returns host:port of SOCKS listener.
	LocalAddr() string
}

// Manager starts/stops instance runtimes.
type Manager interface {
	Start(ctx context.Context, inst *store.Instance) (Handle, error)
	Name() string
}

func New(runtimeName, singBoxPath, dataDir string) (Manager, error) {
	switch runtimeName {
	case "mock":
		return NewMockManager(), nil
	case "sing-box":
		return NewSingBoxManager(singBoxPath, dataDir), nil
	default:
		return nil, fmt.Errorf("unknown runtime %q", runtimeName)
	}
}
