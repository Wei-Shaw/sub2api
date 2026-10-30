package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
)

type MiniMaxMediaAsset struct {
	Category  string
	Kind      string
	ID        string
	AccountID int64
	UserID    int64
	APIKeyID  int64
	Activated bool
}

func (s *MiniMaxMediaService) OwnedVoices(ctx context.Context, key *APIKey, system []byte, kind string) ([]byte, error) {
	var result map[string]any
	if err := json.Unmarshal(system, &result); err != nil {
		return nil, err
	}
	if kind == "system" {
		return system, nil
	}
	repo, ok := s.repo.(interface {
		ListVoices(context.Context, int64, int64) ([]MiniMaxMediaAsset, error)
	})
	if !ok {
		return nil, errors.New("private voice registry unavailable")
	}
	voices, err := repo.ListVoices(ctx, key.UserID, key.ID)
	if err != nil {
		return nil, err
	}
	clones, designs := []map[string]string{}, []map[string]string{}
	for _, v := range voices {
		item := map[string]string{"voice_id": v.ID}
		switch v.Category {
		case "voice_clone":
			clones = append(clones, item)
		case "voice_design":
			designs = append(designs, item)
		}
	}
	if kind != "all" {
		delete(result, "system_voice")
	}
	if kind == "all" || kind == "voice_cloning" {
		result["voice_cloning"] = clones
	}
	if kind == "all" || kind == "voice_generation" {
		result["voice_generation"] = designs
	}
	return json.Marshal(result)
}

type MiniMaxMediaAssetRepository interface {
	FindAsset(context.Context, string, string, int64, int64) (*MiniMaxMediaAsset, error)
	SaveAsset(context.Context, *MiniMaxMediaAsset) error
}

func (s *MiniMaxMediaService) SaveAsset(ctx context.Context, a *MiniMaxMediaAsset) error {
	repo, ok := s.repo.(MiniMaxMediaAssetRepository)
	if !ok {
		return errors.New("media asset registry unavailable")
	}
	return repo.SaveAsset(ctx, a)
}
func (s *MiniMaxMediaService) findAsset(ctx context.Context, kind, id string, key *APIKey) (*MiniMaxMediaAsset, error) {
	repo, ok := s.repo.(MiniMaxMediaAssetRepository)
	if !ok {
		return nil, ErrMiniMaxMediaNotFound
	}
	return repo.FindAsset(ctx, kind, id, key.UserID, key.ID)
}

// Assets cannot move to a different provider account on retry or scheduling.
func (s *MiniMaxMediaService) SelectRequest(ctx context.Context, key *APIKey, r *minimax.Request, kind string) (*Account, func(), error) {
	var bound int64
	bind := func(a *MiniMaxMediaAsset, e error) error {
		if e != nil {
			return errors.New("private media asset is not registered to this key")
		}
		if bound != 0 && bound != a.AccountID {
			return errors.New("media assets belong to different upstream accounts")
		}
		bound = a.AccountID
		return nil
	}
	if kind == "voice_clone" {
		a, e := s.findAsset(ctx, "voice_clone_file", string(r.FileID), key)
		if e = bind(a, e); e != nil {
			return nil, nil, e
		}
		if r.ClonePrompt != nil {
			a, e = s.findAsset(ctx, "prompt_audio_file", string(r.ClonePrompt.PromptAudio), key)
			if e = bind(a, e); e != nil {
				return nil, nil, e
			}
		}
	}
	for _, id := range r.Voices() {
		a, e := s.findAsset(ctx, "voice", id, key)
		if errors.Is(e, ErrMiniMaxMediaNotFound) {
			continue
		}
		if e = bind(a, e); e != nil {
			return nil, nil, e
		}
	}
	return s.selectAccount(ctx, *key.GroupID, bound)
}
func (s *MiniMaxMediaService) prepareVoicePricing(ctx context.Context, key *APIKey, a *Account, r *minimax.Request, p *MiniMaxMediaPricing) error {
	if len(r.Voices()) == 0 {
		return nil
	}
	_, system, err := s.SystemVoices(ctx, a)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range r.Voices() {
		if seen[id] {
			continue
		}
		seen[id] = true
		if system[id] {
			continue
		}
		asset, e := s.findAsset(ctx, "voice", id, key)
		if e != nil || asset.AccountID != a.ID {
			return errors.New("voice must be a verified system voice or a private voice owned by this key")
		}
		if !asset.Activated {
			feeKind := asset.Category
			if feeKind != "voice_clone" && feeKind != "voice_design" {
				return errors.New("private voice fee category is unavailable")
			}
			retail := MiniMaxDefaultPrices()[feeKind]["voice"]
			if v, ok := key.Group.MiniMaxMediaPrices[feeKind]["voice"]; ok {
				retail = v
			}
			p.CustomVoices = append(p.CustomVoices, MiniMaxVoiceCharge{ID: id, Kind: feeKind, Retail: retail, Cost: MiniMaxDefaultPrices()[feeKind]["voice"]})
		}
	}
	// Voice fee costs are USD. A legacy CNY speech snapshot must convert the
	// fee to its own currency so upstream statistics still use one currency.
	if p.CostCurrency == "CNY" {
		for i := range p.CustomVoices {
			p.CustomVoices[i].Cost *= p.CNYPerUSD
		}
	}
	return nil
}
