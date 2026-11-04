package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
)

var ErrMiniMaxMediaConflict = errors.New("media idempotency key was already used with a different request")
var ErrMiniMaxMediaFunds = errors.New("insufficient media balance or quota")
var ErrMiniMaxMediaNotFound = errors.New("media task not found")

// All retail units are USD. Cost units use CostCurrency; FX is CNY per USD.
// Pointer fields distinguish an intentional zero price from missing prices.
type MiniMaxMediaRates struct {
	Image                *float64 `json:"image,omitempty"`
	Voice                *float64 `json:"voice,omitempty"`
	CharactersPerMillion *float64 `json:"characters_per_million,omitempty"`
	OutputSecond         *float64 `json:"output_second,omitempty"`
	InputSecond          *float64 `json:"input_second,omitempty"`
	ExtraImage           *float64 `json:"extra_image,omitempty"`
}
type MiniMaxVoiceCharge struct {
	ID     string  `json:"voice_id"`
	Kind   string  `json:"kind"`
	Retail float64 `json:"retail_usd"`
	Cost   float64 `json:"cost_usd"`
}
type MiniMaxMediaPricing struct {
	Version      int                  `json:"version,omitempty"`
	FreeImages   int                  `json:"free_images,omitempty"`
	CustomVoices []MiniMaxVoiceCharge `json:"custom_voices,omitempty"`

	Resolution   string            `json:"resolution,omitempty"`
	Retail       MiniMaxMediaRates `json:"retail"`
	Cost         MiniMaxMediaRates `json:"cost"`
	CostCurrency string            `json:"cost_currency"`
	CNYPerUSD    float64           `json:"cny_per_usd"`
	Multiplier   *float64          `json:"multiplier"`
}
type MiniMaxMediaBill struct {
	CompletedAt            time.Time     `json:"completed_at,omitempty"`
	ReconciliationRequired bool          `json:"reconciliation_required,omitempty"`
	BilledVoices           []string      `json:"billed_voices,omitempty"`
	MeterSource            string        `json:"meter_source,omitempty"`
	Usage                  minimax.Usage `json:"usage"`
	BillableImages         int           `json:"billable_images"`
	RetailUSD              float64       `json:"retail_usd"`
	ChargedUSD             float64       `json:"charged_usd"`
	UpstreamCost           float64       `json:"upstream_cost"`
	UpstreamUSD            float64       `json:"upstream_usd"`
}

func (p MiniMaxMediaPricing) Validate(model string) error {
	valid := func(v *float64) bool {
		return v != nil && *v >= 0 && !math.IsNaN(*v) && !math.IsInf(*v, 0) && *v <= 1e6
	}
	if !valid(p.Multiplier) {
		return errors.New("explicit media multiplier is required")
	}
	if p.CostCurrency != "USD" && p.CostCurrency != "CNY" {
		return errors.New("media cost_currency must be USD or CNY")
	}
	if p.CostCurrency == "CNY" && (p.CNYPerUSD <= 0 || math.IsNaN(p.CNYPerUSD) || math.IsInf(p.CNYPerUSD, 0)) {
		return errors.New("explicit CNY per USD exchange rate is required")
	}
	for _, rates := range []MiniMaxMediaRates{p.Retail, p.Cost} {
		switch model {
		case "voice_clone":
			if !valid(rates.Voice) {
				return errors.New("voice clone price is required")
			}
		case minimax.SpeechHD, minimax.SpeechTurbo, "speech-2.6-hd", "speech-2.6-turbo", "speech-02-hd", "speech-02-turbo", "voice_design":
			if p.Resolution != "" || !valid(rates.CharactersPerMillion) {
				return errors.New("speech requires explicit per-million-character retail and cost prices")
			}
		case minimax.H3, minimax.H3Highspeed, minimax.H3Lite, minimax.H3Max:
			if !minimax.ValidResolution(model, p.Resolution) || !valid(rates.OutputSecond) || !valid(rates.InputSecond) || !valid(rates.ExtraImage) {
				return errors.New("H3 requires resolution 768P/2K and explicit output/input/image retail and cost prices")
			}
		case minimax.Image:
			if p.Resolution != "" || !valid(rates.Image) {
				return errors.New("image-01 requires per-image prices")
			}
		default:
			return errors.New("unsupported MiniMax media model")
		}
	}
	return nil
}
func (p MiniMaxMediaPricing) Bill(model string, u minimax.Usage) (*MiniMaxMediaBill, error) {
	if err := p.Validate(model); err != nil {
		return nil, err
	}
	if err := u.Validate(); err != nil {
		return nil, err
	}
	free := 5
	if p.Version >= 2 {
		free = p.FreeImages
	}
	b := &MiniMaxMediaBill{Usage: u, BillableImages: max(u.InputImages-free, 0)}
	if u.VoiceCount > 0 && (p.Version < 2 || u.VoiceCount != len(p.CustomVoices)) {
		return nil, errors.New("custom voice charge cannot be attributed; reconciliation required")
	}
	if minimax.IsVideo(model) {
		if u.Characters != 0 || u.VoiceCount != 0 || u.OutputImages != 0 {
			return nil, errors.New("invalid video meter")
		}
	} else if u.OutputSeconds != 0 || u.InputSeconds != 0 || u.InputImages != 0 || u.InputAudioSeconds != 0 {
		return nil, errors.New("invalid synchronous media meter")
	}
	if model == minimax.Image && (u.Characters != 0 || u.VoiceCount != 0) || model != minimax.Image && u.OutputImages != 0 {
		return nil, errors.New("invalid image meter")
	}

	amount := func(r MiniMaxMediaRates) float64 {
		if model == "voice_clone" {
			return 0
		}
		if model == minimax.Image {
			return float64(u.OutputImages) * *r.Image
		}
		if !minimax.IsVideo(model) {
			return float64(u.Characters) / 1e6 * *r.CharactersPerMillion
		}
		return u.OutputSeconds**r.OutputSecond + u.InputSeconds**r.InputSecond + float64(b.BillableImages)**r.ExtraImage
	}
	retail, cost := amount(p.Retail), amount(p.Cost)
	if u.VoiceCount > 0 {
		for _, v := range p.CustomVoices {
			if v.ID == "" || v.Retail < 0 || v.Cost < 0 || math.IsNaN(v.Retail) || math.IsNaN(v.Cost) || math.IsInf(v.Retail, 0) || math.IsInf(v.Cost, 0) {
				return nil, errors.New("invalid voice fee snapshot")
			}
			retail += v.Retail
			cost += v.Cost
			b.BilledVoices = append(b.BilledVoices, v.ID)
		}
	}
	b.RetailUSD = QuantizeUsageBillingAmount(retail)
	b.ChargedUSD = QuantizeUsageBillingAmount(b.RetailUSD * *p.Multiplier)
	b.UpstreamCost = QuantizeUsageBillingAmount(cost)
	b.UpstreamUSD = b.UpstreamCost
	if p.CostCurrency == "CNY" {
		b.UpstreamUSD = QuantizeUsageBillingAmount(b.UpstreamCost / p.CNYPerUSD)
	}
	for _, v := range []float64{b.RetailUSD, b.ChargedUSD, b.UpstreamCost, b.UpstreamUSD} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v > 1e10 {
			return nil, errors.New("media cost exceeds supported range")
		}
	}
	return b, nil
}

const (
	MiniMaxBillingStandard   = "standard"
	MiniMaxBillingSimple     = "simple"
	MiniMaxBillingKeyWindows = "key_windows"
)

type MiniMaxMediaTask struct {
	// Freeze admission policy so a restart in another mode cannot debit an
	// unbilled task or discard an existing standard-mode reservation.
	BillingPolicy  string                  `json:"billing_policy,omitempty"`
	RequestedModel string                  `json:"requested_model,omitempty"`
	RequestInfo    MiniMaxMediaRequestInfo `json:"request_info,omitempty"`
	ID             string                  `json:"task_id"`
	Kind           string                  `json:"kind"`
	UserID         int64                   `json:"-"`
	APIKeyID       int64                   `json:"-"`
	GroupID        int64                   `json:"-"`
	AccountID      int64                   `json:"-"`
	SubscriptionID *int64                  `json:"-"`
	AccountType    string                  `json:"-"`
	UpstreamID     string                  `json:"-"`
	Origin         string                  `json:"-"`
	IdempotencyKey string                  `json:"-"`
	RequestHash    string                  `json:"-"`
	Model          string                  `json:"model"`
	Resolution     string                  `json:"resolution,omitempty"`
	Duration       int                     `json:"duration,omitempty"`
	Stream         bool                    `json:"stream"`
	Pricing        MiniMaxMediaPricing     `json:"pricing"`
	ReservedUSD    float64                 `json:"reserved_usd"`
	Status         string                  `json:"status"`
	BillingState   string                  `json:"billing_state"`
	Bill           *MiniMaxMediaBill       `json:"billing,omitempty"`
	Result         json.RawMessage         `json:"result,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	// Reservation windows allow a refund to affect only the original window.
	SubscriptionWindows [3]*time.Time `json:"-"`
	KeyWindows          [3]*time.Time `json:"-"`
}

// Empty policy keeps historical standard-mode snapshots billable.
func (t *MiniMaxMediaTask) UsesStandardBilling() bool {
	return t.BillingPolicy == "" || t.BillingPolicy == MiniMaxBillingStandard
}
func (t *MiniMaxMediaTask) TracksKeyWindows() bool {
	return t.UsesStandardBilling() || t.BillingPolicy == MiniMaxBillingKeyWindows
}

type MiniMaxMediaRequestInfo struct {
	StartedAt time.Time `json:"started_at,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	IPAddress string    `json:"ip_address,omitempty"`
}
type MiniMaxMediaRepository interface {
	FindByIdempotencyKey(context.Context, int64, string) (*MiniMaxMediaTask, error)
	Reserve(context.Context, *MiniMaxMediaTask) (*MiniMaxMediaTask, bool, error)
	Get(context.Context, string, int64, int64) (*MiniMaxMediaTask, error)
	List(context.Context, int64, int64, int) ([]*MiniMaxMediaTask, error)
	Submitted(context.Context, string, string) error
	Record(context.Context, string, string, *MiniMaxMediaBill, json.RawMessage) error
	RefreshResult(context.Context, string, json.RawMessage) error
	Settle(context.Context, string) (bool, error)
	Claim(context.Context, int) ([]*MiniMaxMediaTask, error)
}
