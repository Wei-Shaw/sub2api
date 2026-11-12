package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

type MetaTokenProvider struct {
	accountRepo  AccountRepository
	httpUpstream HTTPUpstream
	locks        sync.Map
}

func NewMetaTokenProvider(accountRepo AccountRepository, httpUpstream HTTPUpstream) *MetaTokenProvider {
	return &MetaTokenProvider{accountRepo: accountRepo, httpUpstream: httpUpstream}
}

func (p *MetaTokenProvider) GetAPIKey(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("meta: account is required")
	}
	if apiKey := account.GetMetaAPIKey(); apiKey != "" {
		return apiKey, nil
	}
	if account.GetMetaDCAToken() == "" {
		return "", errors.New("meta: api_key or dca_token is required")
	}
	return p.mintAndStore(ctx, account)
}

func (p *MetaTokenProvider) InvalidateAPIKey(ctx context.Context, account *Account) error {
	if p == nil || account == nil || account.ID == 0 || !account.IsMeta() {
		return nil
	}
	if account.GetMetaDCAToken() == "" {
		return nil
	}
	lock := p.accountLock(account.ID)
	lock.Lock()
	defer lock.Unlock()
	latest, err := p.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		return err
	}
	if latest == nil || latest.GetMetaDCAToken() == "" || latest.GetMetaAPIKey() == "" {
		return nil
	}
	credentials := cloneCredentials(latest.Credentials)
	delete(credentials, "api_key")
	latest.Credentials = credentials
	if err := p.accountRepo.Update(ctx, latest); err != nil {
		return err
	}
	account.Credentials = credentials
	return nil
}

func (p *MetaTokenProvider) mintAndStore(ctx context.Context, account *Account) (string, error) {
	if p == nil {
		return "", errors.New("meta: token provider is not configured")
	}
	lock := p.accountLock(account.ID)
	lock.Lock()
	defer lock.Unlock()

	stored := account
	if p.accountRepo != nil && account.ID != 0 {
		if latest, err := p.accountRepo.GetByID(ctx, account.ID); err == nil && latest != nil {
			stored = latest
			if apiKey := latest.GetMetaAPIKey(); apiKey != "" {
				account.Credentials = latest.Credentials
				return apiKey, nil
			}
		}
	}
	dcaToken := stored.GetMetaDCAToken()
	if dcaToken == "" {
		return "", errors.New("meta: dca_token is required to mint an api key")
	}
	minted, err := meta.NewClient(p.transportFor(stored)).MintAPIKey(ctx, dcaToken)
	if err != nil {
		return "", err
	}
	credentials := cloneCredentials(stored.Credentials)
	credentials["api_key"] = minted.APIKey
	if baseURL := strings.TrimSpace(minted.BaseURL); baseURL != "" {
		credentials["base_url"] = strings.TrimRight(baseURL, "/")
	}
	if minted.UserEmail != "" {
		credentials["email"] = minted.UserEmail
	}
	if minted.UserFullName != "" {
		credentials["name"] = minted.UserFullName
	}
	if minted.SubsTierName != "" {
		credentials["subs_tier_name"] = minted.SubsTierName
	}
	if minted.SubsTierID != "" {
		credentials["subs_tier_id"] = minted.SubsTierID
	}
	credentials["is_subs_active"] = minted.IsSubsActive
	credentials["has_payment_method"] = minted.HasPaymentMethod
	credentials["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	stored.Credentials = credentials
	if p.accountRepo != nil && stored.ID != 0 {
		if err := p.accountRepo.Update(ctx, stored); err != nil {
			return "", err
		}
	}
	account.Credentials = credentials
	return minted.APIKey, nil
}

func (p *MetaTokenProvider) transportFor(account *Account) meta.Doer {
	if p == nil || p.httpUpstream == nil || account == nil {
		return nil
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	return metaUpstreamDoer{upstream: p.httpUpstream, proxyURL: proxyURL, accountID: account.ID, concurrency: account.Concurrency}
}

func (p *MetaTokenProvider) accountLock(accountID int64) *sync.Mutex {
	value, _ := p.locks.LoadOrStore(accountID, &sync.Mutex{})
	lock, ok := value.(*sync.Mutex)
	if !ok {
		return &sync.Mutex{}
	}
	return lock
}

type metaUpstreamDoer struct {
	upstream    HTTPUpstream
	proxyURL    string
	accountID   int64
	concurrency int
}

func (d metaUpstreamDoer) Do(req *http.Request) (*http.Response, error) {
	return d.upstream.Do(req, d.proxyURL, d.accountID, d.concurrency)
}

func cloneCredentials(credentials map[string]any) map[string]any {
	out := make(map[string]any, len(credentials)+4)
	for key, value := range credentials {
		out[key] = value
	}
	return out
}
