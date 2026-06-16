package plugin

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginSchedulingHintsProvider implements service.PluginSchedulingHintsProvider
// by delegating to the plugin's GetSchedulingHints gRPC RPC via the
// PlatformRegistry.
type pluginSchedulingHintsProvider struct {
	registry *PlatformRegistry
}

// NewPluginSchedulingHintsProvider creates a PluginSchedulingHintsProvider
// backed by the given PlatformRegistry.
func NewPluginSchedulingHintsProvider(registry *PlatformRegistry) service.PluginSchedulingHintsProvider {
	return &pluginSchedulingHintsProvider{registry: registry}
}

// GetHints delegates to the plugin's GetSchedulingHints RPC. Returns an
// error if the platform is not registered or the plugin returns
// Unimplemented.
func (p *pluginSchedulingHintsProvider) GetHints(
	ctx context.Context,
	platform string,
	accounts []*service.Account,
	gatewayProtocol string,
	requestedModel string,
) (map[int64]service.SchedulingHintResult, error) {
	client, err := ClientForPlatform(p.registry, platform)
	if err != nil {
		return nil, err
	}
	pbAccounts := make([]*pb.SchedulingHintAccount, len(accounts))
	for i, acc := range accounts {
		creds, _ := json.Marshal(acc.Credentials)
		extra, _ := json.Marshal(acc.Extra)
		pbAccounts[i] = &pb.SchedulingHintAccount{
			AccountId:       acc.ID,
			CredentialsJson: creds,
			ExtraJson:       extra,
		}
	}
	resp, err := client.GetSchedulingHints(ctx, pbAccounts, gatewayProtocol, requestedModel)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unimplemented {
			return nil, err
		}
		return nil, err
	}
	return convertHintsFromProto(resp.GetHints()), nil
}

func convertHintsFromProto(pbHints map[int64]*pb.SchedulingHint) map[int64]service.SchedulingHintResult {
	if len(pbHints) == 0 {
		return nil
	}
	out := make(map[int64]service.SchedulingHintResult, len(pbHints))
	for id, h := range pbHints {
		out[id] = service.SchedulingHintResult{
			PriorityModifier:       h.GetPriorityModifier(),
			TemporarilyUnavailable: h.GetTemporarilyUnavailable(),
			Reason:                 h.GetReason(),
		}
	}
	return out
}
