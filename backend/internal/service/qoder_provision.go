package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qoder"
	"go.uber.org/zap"
)

// Account.Extra keys where the auto-provisioned Qoder resources are cached so a
// PAT-only account reuses the same agent/environment across requests.
const (
	qoderExtraAgentID = "qoder_agent_id"
	qoderExtraEnvID   = "qoder_env_id"
)

// qoderDefaultModel is the agent model used when the account does not pin one.
const qoderDefaultModel = "ultimate"

const (
	qoderProvisionLockTTL  = 30 * time.Second
	qoderProvisionLockWait = 15 * time.Second
)

// qoderProvisionResult carries the cached agent/environment identifiers.
type qoderProvisionResult struct {
	AgentID string
	EnvID   string
}

// resolveQoderAgentModel selects the model used to create the account's agent.
// A credentials.model override wins; otherwise the default "ultimate" is used.
// The client-supplied request model does not change the agent (one cached agent
// per account) but is still recorded for billing/display upstream.
func resolveQoderAgentModel(account *Account) string {
	if m := strings.TrimSpace(account.GetCredential("model")); m != "" {
		return m
	}
	if m := strings.TrimSpace(account.GetExtraString("qoder_model")); m != "" {
		return m
	}
	return qoderDefaultModel
}

// EnsureAgentAndEnvironment returns the account's Qoder agent and environment,
// creating and caching them on first use. Concurrent provisioning across
// instances is serialized with a best-effort Redis lock; peers wait for the
// winner to publish the ids into the account's extra rather than creating
// duplicates.
func (s *QoderGatewayService) EnsureAgentAndEnvironment(ctx context.Context, account *Account) (qoderProvisionResult, error) {
	if r, ok := qoderProvisionFromAccount(account); ok {
		return r, nil
	}

	release, acquired := s.acquireQoderProvisionLock(ctx, account.ID)
	if acquired {
		defer release()
	} else if r, ok := s.waitForQoderProvision(ctx, account); ok {
		return r, nil
	}

	// Re-check after acquiring the lock (or after a failed wait): another
	// instance may have provisioned while we blocked.
	if fresh, err := s.accountRepo.GetByID(ctx, account.ID); err == nil && fresh != nil {
		if r, ok := qoderProvisionFromAccount(fresh); ok {
			account.Extra = fresh.Extra
			return r, nil
		}
	}

	client := s.newQoderClient(account)

	envID, err := client.CreateEnvironment(ctx, qoderResourceName("env", account))
	if err != nil {
		return qoderProvisionResult{}, fmt.Errorf("create qoder environment: %w", err)
	}
	agentID, err := client.CreateAgent(ctx, qoderResourceName("agent", account), resolveQoderAgentModel(account))
	if err != nil {
		return qoderProvisionResult{}, fmt.Errorf("create qoder agent: %w", err)
	}

	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[qoderExtraAgentID] = agentID
	account.Extra[qoderExtraEnvID] = envID
	if err := s.accountRepo.Update(ctx, account); err != nil {
		// The resources exist upstream; log but still return them so the current
		// request succeeds. A later request will retry the persistence.
		logger.L().With(zap.String("component", "service.qoder_gateway")).
			Warn("qoder.provision_persist_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}

	return qoderProvisionResult{AgentID: agentID, EnvID: envID}, nil
}

func qoderProvisionFromAccount(account *Account) (qoderProvisionResult, bool) {
	agentID := strings.TrimSpace(account.GetExtraString(qoderExtraAgentID))
	envID := strings.TrimSpace(account.GetExtraString(qoderExtraEnvID))
	if agentID != "" && envID != "" {
		return qoderProvisionResult{AgentID: agentID, EnvID: envID}, true
	}
	return qoderProvisionResult{}, false
}

func qoderResourceName(kind string, account *Account) string {
	return fmt.Sprintf("sub2api-%s-%d", kind, account.ID)
}

func (s *QoderGatewayService) acquireQoderProvisionLock(ctx context.Context, accountID int64) (func(), bool) {
	if s.redis == nil {
		return func() {}, true
	}
	key := fmt.Sprintf("qoder:provision:%d", accountID)
	ok, err := s.redis.SetNX(ctx, key, "1", qoderProvisionLockTTL).Result()
	if err != nil {
		// Redis trouble should not block provisioning; proceed without the lock.
		return func() {}, true
	}
	if !ok {
		return func() {}, false
	}
	return func() { _ = s.redis.Del(context.Background(), key).Err() }, true
}

// waitForQoderProvision polls the account row until a peer publishes the
// provisioned ids or the wait budget elapses.
func (s *QoderGatewayService) waitForQoderProvision(ctx context.Context, account *Account) (qoderProvisionResult, bool) {
	deadline := time.Now().Add(qoderProvisionLockWait)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return qoderProvisionResult{}, false
		case <-ticker.C:
			if fresh, err := s.accountRepo.GetByID(ctx, account.ID); err == nil && fresh != nil {
				if r, ok := qoderProvisionFromAccount(fresh); ok {
					account.Extra = fresh.Extra
					return r, true
				}
			}
			if time.Now().After(deadline) {
				return qoderProvisionResult{}, false
			}
		}
	}
}

// newQoderClient builds a qoder.Client bound to the account's PAT, base URL and
// upstream transport (proxy + concurrency limiting via httpUpstream).
func (s *QoderGatewayService) newQoderClient(account *Account) *qoder.Client {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	doer := func(req *http.Request) (*http.Response, error) {
		return s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}
	return qoder.NewClient(account.GetQoderBaseURL(), account.GetQoderApiKey(), doer)
}
