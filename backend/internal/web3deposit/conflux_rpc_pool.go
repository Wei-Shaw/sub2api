package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
)

const (
	DefaultConfluxRPCRequestTimeout  = 10 * time.Second
	DefaultConfluxRPCFailureCooldown = 30 * time.Second
)

var ErrConfluxRPCUnavailable = errors.New("conflux rpc endpoints are unavailable")

type ConfluxRPCPoolOptions struct {
	RequestTimeout  time.Duration
	FailureCooldown time.Duration
	HTTPClient      *http.Client
	Now             func() time.Time
	dial            confluxRPCDialer
}

type ConfluxRPCEndpointState struct {
	ID             string
	Healthy        bool
	UnhealthyUntil time.Time
}

type confluxRPCEndpoint struct {
	id             string
	client         confluxRPCCaller
	unhealthyUntil time.Time
}

type confluxRPCCaller interface {
	CallContext(context.Context, any, string, ...any) error
	Close()
}

type confluxRPCDialer func(context.Context, string, *http.Client) (confluxRPCCaller, error)

type ConfluxRPCPool struct {
	mu              sync.Mutex
	endpoints       []*confluxRPCEndpoint
	next            int
	requestTimeout  time.Duration
	failureCooldown time.Duration
	now             func() time.Time
}

func NewConfluxRPCPool(ctx context.Context, rawURLs []string, options ConfluxRPCPoolOptions) (*ConfluxRPCPool, error) {
	if len(rawURLs) == 0 {
		return nil, fmt.Errorf("create conflux rpc pool: %w", ErrConfluxRPCUnavailable)
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = DefaultConfluxRPCRequestTimeout
	}
	if options.FailureCooldown <= 0 {
		options.FailureCooldown = DefaultConfluxRPCFailureCooldown
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.dial == nil {
		options.dial = func(ctx context.Context, rawURL string, httpClient *http.Client) (confluxRPCCaller, error) {
			return rpc.DialOptions(ctx, rawURL, rpc.WithHTTPClient(httpClient))
		}
	}

	pool := &ConfluxRPCPool{
		endpoints:       make([]*confluxRPCEndpoint, 0, len(rawURLs)),
		requestTimeout:  options.RequestTimeout,
		failureCooldown: options.FailureCooldown,
		now:             options.Now,
	}
	connectedEndpoints := 0
	for index, rawURL := range rawURLs {
		endpoint := &confluxRPCEndpoint{id: fmt.Sprintf("endpoint_%d", index+1)}
		client, err := options.dial(ctx, rawURL, options.HTTPClient)
		if err != nil {
			pool.endpoints = append(pool.endpoints, endpoint)
			continue
		}
		endpoint.client = client
		connectedEndpoints++
		pool.endpoints = append(pool.endpoints, endpoint)
	}
	if connectedEndpoints == 0 {
		return nil, fmt.Errorf("create conflux rpc pool: %w", ErrConfluxRPCUnavailable)
	}
	return pool, nil
}

func (p *ConfluxRPCPool) CallContext(ctx context.Context, result any, method string, args ...any) error {
	endpoints := p.candidates()
	failedEndpointIDs := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if err := ctx.Err(); err != nil {
			return err
		}

		attemptCtx, cancel := context.WithTimeout(ctx, p.requestTimeout)
		err := endpoint.client.CallContext(attemptCtx, result, method, args...)
		cancel()
		if err == nil {
			p.markHealthy(endpoint)
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		p.markUnhealthy(endpoint)
		failedEndpointIDs = append(failedEndpointIDs, endpoint.id)
	}
	return &ConfluxRPCPoolError{Method: method, EndpointIDs: failedEndpointIDs}
}

func (p *ConfluxRPCPool) EndpointStates() []ConfluxRPCEndpointState {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	states := make([]ConfluxRPCEndpointState, 0, len(p.endpoints))
	for _, endpoint := range p.endpoints {
		states = append(states, ConfluxRPCEndpointState{
			ID:             endpoint.id,
			Healthy:        endpoint.client != nil && !endpoint.unhealthyUntil.After(now),
			UnhealthyUntil: endpoint.unhealthyUntil,
		})
	}
	return states
}

func (p *ConfluxRPCPool) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, endpoint := range p.endpoints {
		if endpoint.client != nil {
			endpoint.client.Close()
		}
	}
}

func (p *ConfluxRPCPool) candidates() []*confluxRPCEndpoint {
	p.mu.Lock()
	defer p.mu.Unlock()

	endpointCount := len(p.endpoints)
	start := p.next % endpointCount
	p.next = (p.next + 1) % endpointCount
	now := p.now()
	healthy := make([]*confluxRPCEndpoint, 0, endpointCount)
	unhealthy := make([]*confluxRPCEndpoint, 0, endpointCount)
	for offset := 0; offset < endpointCount; offset++ {
		endpoint := p.endpoints[(start+offset)%endpointCount]
		if endpoint.client == nil {
			continue
		}
		if endpoint.unhealthyUntil.After(now) {
			unhealthy = append(unhealthy, endpoint)
			continue
		}
		healthy = append(healthy, endpoint)
	}
	if len(healthy) > 0 {
		return healthy
	}
	return unhealthy
}

func (p *ConfluxRPCPool) markHealthy(endpoint *confluxRPCEndpoint) {
	p.mu.Lock()
	endpoint.unhealthyUntil = time.Time{}
	p.mu.Unlock()
}

func (p *ConfluxRPCPool) markUnhealthy(endpoint *confluxRPCEndpoint) {
	p.mu.Lock()
	endpoint.unhealthyUntil = p.now().Add(p.failureCooldown)
	p.mu.Unlock()
}

type ConfluxRPCPoolError struct {
	Method      string
	EndpointIDs []string
}

func (e *ConfluxRPCPoolError) Error() string {
	return fmt.Sprintf("conflux rpc method %q failed on endpoints %v", e.Method, e.EndpointIDs)
}

func (e *ConfluxRPCPoolError) Unwrap() error {
	return ErrConfluxRPCUnavailable
}
