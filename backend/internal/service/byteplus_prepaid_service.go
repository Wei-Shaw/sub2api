package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/byteplusbilling"
	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
)

var ErrBytePlusProviderStatus = errors.New("provider task status could not be verified; its funding hold was retained")

type BytePlusPrepaidInput struct {
	PoolID          string    `json:"pool_id"`
	Name            string    `json:"name"`
	PackID          string    `json:"pack_id"`
	Model           string    `json:"model"`
	RemainingTokens string    `json:"remaining_tokens"`
	ExpiresAt       time.Time `json:"expires_at"`
	Confirmed       bool      `json:"confirmed"`
	AllowPayAsYouGo bool      `json:"allow_pay_as_you_go"`
}

type BytePlusPrepaidController interface {
	Status(context.Context, int64) (*packguard.Status, error)
	Pools(context.Context) ([]packguard.Pool, error)
	Update(context.Context, int64, string, BytePlusPrepaidInput) (*packguard.Status, error)
	Reconcile(context.Context, int64, string) (*packguard.Status, error)
}

type BytePlusPrepaidService struct {
	accounts AccountRepository
	billing  *byteplusbilling.Client
	store    *packguard.Store
	upstream HTTPUpstream
}

func NewBytePlusPrepaidService(accounts AccountRepository, db *sql.DB, upstream HTTPUpstream) (*BytePlusPrepaidService, error) {
	if accounts == nil || db == nil || upstream == nil {
		return nil, fmt.Errorf("prepaid funding requires account, SQL and guarded transport dependencies")
	}
	return &BytePlusPrepaidService{accounts: accounts, store: packguard.NewStore(db), upstream: upstream, billing: byteplusbilling.New()}, nil
}

func (s *BytePlusPrepaidService) account(ctx context.Context, id int64) (*Account, string, string, error) {
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, "", "", err
	}
	if a == nil || a.Type != AccountTypeAPIKey {
		return nil, "", "", packguard.ErrInvalid
	}
	base := a.GetCredential("base_url")
	// This also works before the independent native-provider PR is merged.
	if base == "" && a.Platform == "byteplus" {
		base = "https://ark.ap-southeast.bytepluses.com/api/v3"
	}
	base, err = packguard.NormalizeBase(base)
	if err != nil {
		return nil, "", "", err
	}
	key := strings.TrimSpace(a.GetCredential("api_key"))
	if key == "" {
		return nil, "", "", packguard.ErrInvalid
	}
	headers := http.Header{"Authorization": []string{"Bearer " + key}}
	a.ApplyHeaderOverrides(headers)
	parts := strings.Fields(headers.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] != key {
		return nil, "", "", packguard.ErrInvalid
	}
	return a, base, key, nil
}

func (s *BytePlusPrepaidService) Status(ctx context.Context, id int64) (*packguard.Status, error) {
	_, base, key, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.store.Status(ctx, id, packguard.HashCredential(key), base)
}
func (s *BytePlusPrepaidService) Pools(ctx context.Context) ([]packguard.Pool, error) {
	return s.store.Pools(ctx)
}

func (s *BytePlusPrepaidService) Update(ctx context.Context, id int64, action string, in BytePlusPrepaidInput) (*packguard.Status, error) {
	a, base, key, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	if action == "enable" || action == "link" {
		if !in.Confirmed {
			return nil, packguard.ErrInvalid
		}
		if in.Name == "" {
			in.Name = a.Name
		}
		if err = s.store.Enroll(ctx, id, packguard.HashCredential(key), base, in.Name, in.PoolID); err != nil {
			return nil, err
		}
		return s.Status(ctx, id)
	}
	st, err := s.store.Status(ctx, id, packguard.HashCredential(key), base)
	if err != nil {
		return nil, err
	}
	if st.Pool == nil {
		return nil, packguard.ErrInvalid
	}
	if base != packguard.QualifiedAPIBase && action != "freeze" && action != "disable" {
		return nil, packguard.ErrRegion
	}
	// Pausing or opting out remains available after an accidental key edit;
	// resuming and changing funding require the verified credential and region.
	if !st.CredentialMatches && action != "freeze" && action != "disable" {
		return nil, packguard.ErrScope
	}
	switch action {
	case "grant":
		if !in.Confirmed {
			return nil, packguard.ErrInvalid
		}
		err = s.store.AddGrant(ctx, st.Pool.ID, in.PackID, in.Model, in.RemainingTokens, in.ExpiresAt)
	case "verify-grant":
		if !in.Confirmed {
			return nil, packguard.ErrInvalid
		}
		err = s.store.VerifyGrant(ctx, st.Pool.ID, in.PackID, in.RemainingTokens, in.ExpiresAt)
	case "freeze", "resume":
		err = s.store.SetState(ctx, st.Pool.ID, action, in.Confirmed)
	case "disable":
		err = s.store.SetState(ctx, st.Pool.ID, action, in.AllowPayAsYouGo)
	default:
		err = packguard.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	return s.Status(ctx, id)
}

func (s *BytePlusPrepaidService) Reconcile(ctx context.Context, id int64, reservationID string) (*packguard.Status, error) {
	a, base, key, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	st, err := s.store.Status(ctx, id, packguard.HashCredential(key), base)
	if err != nil {
		return nil, err
	}
	if st.Pool == nil || !st.CredentialMatches {
		return nil, packguard.ErrScope
	}
	task, err := s.store.Task(ctx, st.Pool.ID, reservationID)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, base+"/contents/generations/tasks/"+url.PathEscape(task), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	a.ApplyHeaderOverrides(req.Header)
	proxy := ""
	if a.ProxyID != nil && a.Proxy != nil {
		proxy = a.Proxy.URL()
	}
	resp, err := s.upstream.Do(req, proxy, id, a.Concurrency)
	if err != nil {
		return nil, ErrBytePlusProviderStatus
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrBytePlusProviderStatus
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, packguard.MaxResponseBytes+1))
	if err != nil || len(data) > packguard.MaxResponseBytes {
		return nil, ErrBytePlusProviderStatus
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Usage  struct {
			Completion int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &result) != nil || result.ID != task {
		return nil, ErrBytePlusProviderStatus
	}
	switch result.Status {
	case "succeeded":
		if result.Usage.Completion <= 0 {
			return nil, ErrBytePlusProviderStatus
		}
		err = s.store.FinishTask(ctx, st.Pool.ID, task, result.Usage.Completion, true)
	case "failed", "expired", "cancelled", "canceled":
		err = s.store.FinishTask(ctx, st.Pool.ID, task, 0, false)
	case "queued", "running": // A poll is not permission to refund a queued job.
	default:
		return nil, ErrBytePlusProviderStatus
	}
	if err != nil {
		return nil, err
	}
	return s.Status(ctx, id)
}

// ProviderBilling is separate from ledger mutations and task reconciliation.
func (s *BytePlusPrepaidService) ProviderBilling(ctx context.Context, id int64, query bool) (*byteplusbilling.Status, error) {
	st, err := s.Status(ctx, id)
	if err != nil {
		return nil, err
	}
	if st.Pool == nil {
		result := s.billing.Read("")
		result.Configured = false
		result.Setup = "pool_required"
		return &result, nil
	}
	if !st.CredentialMatches {
		return nil, packguard.ErrScope
	}
	result := s.billing.Read(st.Pool.ID)
	if query {
		result = s.billing.Query(ctx, st.Pool.ID)
	}
	return &result, nil
}
