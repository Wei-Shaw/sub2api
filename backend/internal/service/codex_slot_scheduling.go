package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

func codexDeviceSlotLeaseKey(slot *CodexResolvedDeviceSlot) string {
	if slot.SlotID > 0 {
		return strconv.FormatInt(slot.SlotID, 10)
	}
	profile := CodexResolvedProfile{OSClass: slot.OSClass, Surface: slot.CanonicalSurface, Architecture: slot.Architecture}
	return strings.Join([]string{strconv.FormatInt(slot.AccountID, 10), profile.Key(), strconv.Itoa(slot.SlotIndex), strconv.FormatInt(slot.Epoch, 10)}, ":")
}

// acquireCodexConversationSlot only schedules LOCAL capacity. An upstream 429
// never authorizes relocating a conversation or changing a device identity.
func (s *OpenAIGatewayService) acquireCodexConversationSlot(ctx context.Context, account *Account, request codexProfileRequest, preferred *CodexResolvedDeviceSlot, body []byte) (*CodexResolvedDeviceSlot, *CodexDeviceConversationLease, error) {
	if s.concurrencyService == nil {
		return nil, nil, fmt.Errorf("device_shared requires a distributed conversation lease")
	}
	acquire := func(slot *CodexResolvedDeviceSlot) (*CodexDeviceConversationLease, bool, error) {
		return s.concurrencyService.AcquireCodexDeviceConversationLease(ctx, codexDeviceSlotLeaseKey(slot), account.CodexIdentityPolicy.SessionPolicy.MaxActiveConversationsPerSlot)
	}
	repo, supported := s.accountRepo.(CodexConversationBindingRepository)
	hash := ""
	if request.ConversationHash != "" && preferred.SlotID > 0 && supported {
		digest := sha256.Sum256([]byte(request.ConversationHash))
		hash = hex.EncodeToString(digest[:])
		pinned, err := repo.FindCodexConversationBinding(ctx, account.ID, preferred.APIKeyID, preferred.OSClass, preferred.CanonicalSurface, hash)
		if err != nil {
			return nil, nil, fmt.Errorf("find Codex conversation slot: %w", err)
		}
		if pinned != nil {
			lease, acquired, err := acquire(pinned)
			if err != nil {
				return nil, nil, err
			}
			if !acquired {
				return nil, nil, ErrCodexDeviceSessionBusy
			}
			lease.keepAffinityAlive(func(ctx context.Context) error { return repo.RefreshCodexConversationBinding(ctx, pinned.BindingID) })
			return pinned, lease, nil
		}
	}

	// Missing identity or a continuation without a known pin cannot safely move.
	canSelect := hash != "" && !gjson.GetBytes(body, "previous_response_id").Exists() && !gjson.GetBytes(body, "response.previous_response_id").Exists()
	try := func(candidate *CodexResolvedDeviceSlot) (*CodexResolvedDeviceSlot, *CodexDeviceConversationLease, bool, error) {
		lease, acquired, err := acquire(candidate)
		if err != nil || !acquired {
			return nil, nil, false, err
		}
		if hash == "" {
			if supported && candidate.BindingID > 0 {
				lease.keepAffinityAlive(func(ctx context.Context) error { return repo.RefreshCodexConversationBinding(ctx, candidate.BindingID) })
			}
			return candidate, lease, true, nil
		}
		winner, err := repo.BindCodexConversationSlot(ctx, account.ID, preferred.APIKeyID, preferred.OSClass, preferred.CanonicalSurface, hash, candidate.SlotID)
		if err != nil {
			lease.Release()
			return nil, nil, false, fmt.Errorf("pin Codex conversation slot: %w", err)
		}
		if winner == nil {
			lease.Release()
			return nil, nil, false, fmt.Errorf("codex conversation slot resolver returned nil")
		}
		if winner.SlotID != candidate.SlotID {
			lease.Release()
			lease, acquired, err = acquire(winner)
			if err != nil {
				return nil, nil, false, err
			}
			// A racing request pinned this conversation. Do not try another slot.
			if !acquired {
				return nil, nil, false, ErrCodexDeviceSessionBusy
			}
		}
		lease.keepAffinityAlive(func(ctx context.Context) error { return repo.RefreshCodexConversationBinding(ctx, winner.BindingID) })
		return winner, lease, true, nil
	}
	selected, lease, acquired, err := try(preferred)
	if err != nil || acquired {
		return selected, lease, err
	}
	if canSelect {
		if slots, ok := s.accountRepo.(CodexDeviceBindingRepository); ok {
			available, err := slots.ListCodexDeviceSlots(ctx, account.ID, preferred.OSClass, preferred.CanonicalSurface, false)
			if err != nil {
				return nil, nil, fmt.Errorf("list Codex conversation slots: %w", err)
			}
			for i := range available {
				candidate := &available[i]
				if candidate.SlotID == preferred.SlotID || candidate.AccountID != account.ID ||
					candidate.ProfileID != preferred.ProfileID || candidate.Epoch != preferred.Epoch ||
					candidate.OSClass != preferred.OSClass || candidate.CanonicalSurface != preferred.CanonicalSurface ||
					candidate.Architecture != preferred.Architecture || candidate.State != "active" {
					continue
				}
				selected, lease, acquired, err = try(candidate)
				if err != nil || acquired {
					return selected, lease, err
				}
			}
		}
	}
	return nil, nil, ErrCodexDeviceSessionBusy
}
