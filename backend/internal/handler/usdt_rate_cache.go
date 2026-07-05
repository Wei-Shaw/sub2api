package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type usdtRateEntry struct {
	CNYPerUSDT float64   `json:"cny_per_usdt"`
	UpdatedAt  time.Time `json:"updated_at"`
	Source     string    `json:"source"`
}

type usdtRateCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	entry *usdtRateEntry
	sf    singleflight.Group
}

var defaultUSDTRateCache = &usdtRateCache{ttl: 60 * time.Second}

func (c *usdtRateCache) Get(ctx context.Context) (usdtRateEntry, error) {
	c.mu.RLock()
	cached := c.entry
	c.mu.RUnlock()
	if cached != nil && time.Since(cached.UpdatedAt) < c.ttl {
		return *cached, nil
	}

	v, err, _ := c.sf.Do("usdt_rate", func() (any, error) {
		rate, fetchErr := fetchUSDTRateFromCoinGecko(ctx)
		if fetchErr != nil {
			// Fall back to stale cache if available so the UI keeps working
			// during a transient upstream outage.
			c.mu.RLock()
			stale := c.entry
			c.mu.RUnlock()
			if stale != nil {
				return *stale, nil
			}
			return usdtRateEntry{}, fetchErr
		}
		entry := usdtRateEntry{
			CNYPerUSDT: rate,
			UpdatedAt:  time.Now().UTC(),
			Source:     "coingecko",
		}
		c.mu.Lock()
		c.entry = &entry
		c.mu.Unlock()
		return entry, nil
	})
	if err != nil {
		return usdtRateEntry{}, err
	}
	entry, ok := v.(usdtRateEntry)
	if !ok {
		return usdtRateEntry{}, fmt.Errorf("usdt rate cache: unexpected value type %T", v)
	}
	return entry, nil
}

func fetchUSDTRateFromCoinGecko(ctx context.Context) (float64, error) {
	const endpoint = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny"
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("coingecko request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("coingecko status=%d", resp.StatusCode)
	}
	var body struct {
		Tether struct {
			CNY float64 `json:"cny"`
		} `json:"tether"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode coingecko: %w", err)
	}
	if body.Tether.CNY <= 0 {
		return 0, fmt.Errorf("coingecko returned non-positive rate: %v", body.Tether.CNY)
	}
	return body.Tether.CNY, nil
}
