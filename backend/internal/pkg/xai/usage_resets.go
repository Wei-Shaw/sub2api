package xai

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// These are the Web billing facade RPCs, not the Build OAuth billing endpoint.
const (
	UsageResetsListURL    = "https://grok.com/grok_api_v2.GrokBuildBilling/GetRemainingResets"
	UsageResetRedeemURL   = "https://grok.com/grok_api_v2.GrokBuildBilling/RedeemReset"
	usageResetMaxResponse = 1 << 20
)

type UsageResetCard struct {
	TokenID   string    `json:"token_id"`
	ValidFrom time.Time `json:"valid_from"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (c UsageResetCard) Available(now time.Time) bool {
	return c.TokenID != "" && !now.Before(c.ValidFrom) && now.Before(c.ExpiresAt)
}

// UsageResetRPCError deliberately excludes upstream text, which may contain
// session information. HTTP 200 alone is not proof of gRPC-Web success.
type UsageResetRPCError struct{ HTTPStatus, GRPCStatus int }

func (e *UsageResetRPCError) Error() string {
	return fmt.Sprintf("Grok usage reset RPC failed (HTTP %d, gRPC %d)", e.HTTPStatus, e.GRPCStatus)
}

// NewUsageResetRequest uses an ephemeral Web SSO, supplied by an administrator.
// The caller must disable redirects and must not retry a redemption request.
func NewUsageResetRequest(ctx context.Context, sso, cardID string, redeem bool) (*http.Request, error) {
	if len(sso) > ssoMaxTokenLength || strings.ContainsAny(sso, "\r\n") {
		return nil, errors.New("a valid Grok Web SSO session is required")
	}
	sso = NormalizeSSOToken(sso)
	if sso == "" || strings.ContainsAny(sso, "\r\n\t \"") {
		return nil, errors.New("a valid Grok Web SSO session is required")
	}
	endpoint := UsageResetsListURL
	var message []byte
	if redeem {
		if strings.TrimSpace(cardID) == "" || len(cardID) > 1024 {
			return nil, errors.New("a reset card ID is required")
		}
		endpoint = UsageResetRedeemURL
		message = protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), cardID)
	}
	frame := make([]byte, 5, 5+len(message))
	binary.BigEndian.PutUint32(frame[1:], uint32(len(message)))
	frame = append(frame, message...)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(frame))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/grpc-web+proto")
	req.Header.Set("Accept", "application/grpc-web+proto")
	req.Header.Set("X-Grpc-Web", "1")
	req.Header.Set("X-User-Agent", "connect-es/2.1.1")
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("User-Agent", ssoDefaultUA)
	req.AddCookie(&http.Cookie{Name: "sso", Value: sso})
	req.AddCookie(&http.Cookie{Name: "sso-rw", Value: sso})
	return req, nil
}

// ParseUsageResetResponse decodes both the list's tokens and a redemption's
// still_redeemable fields: both are repeated ResetToken messages at field 1.
func ParseUsageResetResponse(resp *http.Response) ([]UsageResetCard, error) {
	if resp == nil || resp.Body == nil {
		return nil, errors.New("empty Grok reset response")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UsageResetRPCError{HTTPStatus: resp.StatusCode, GRPCStatus: -1}
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "application/grpc-web+proto") {
		return nil, errors.New("unexpected Grok reset response format")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, usageResetMaxResponse+1))
	if err != nil || len(body) > usageResetMaxResponse {
		return nil, errors.New("could not read Grok reset response")
	}
	var message []byte
	seenMessage, seenTrailers := false, false
	status := -1
	for len(body) > 0 {
		if seenTrailers || len(body) < 5 {
			return nil, errors.New("invalid Grok reset response framing")
		}
		flag, size := body[0], uint64(binary.BigEndian.Uint32(body[1:5]))
		body = body[5:]
		if size > uint64(len(body)) {
			return nil, errors.New("truncated Grok reset response")
		}
		payload := body[:int(size)]
		body = body[int(size):]
		switch flag {
		case 0:
			if seenMessage {
				return nil, errors.New("unexpected multiple Grok reset messages")
			}
			message, seenMessage = payload, true
		case 128:
			seenTrailers = true
			foundStatus := false
			for _, line := range strings.Split(string(payload), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
				if !ok || !strings.EqualFold(key, "grpc-status") {
					continue
				}
				if foundStatus {
					return nil, errors.New("duplicate Grok reset status")
				}
				foundStatus = true
				status, err = strconv.Atoi(strings.TrimSpace(value))
				if err != nil || status < 0 {
					return nil, errors.New("invalid Grok reset status")
				}
			}
		default:
			return nil, errors.New("unsupported Grok reset frame")
		}
	}
	if !seenTrailers || status < 0 {
		return nil, errors.New("missing final status in Grok reset response")
	}
	if status != 0 {
		return nil, &UsageResetRPCError{HTTPStatus: resp.StatusCode, GRPCStatus: status}
	}
	if !seenMessage {
		return nil, errors.New("missing result in Grok reset response")
	}
	cards := make([]UsageResetCard, 0)
	err = walkResetFields(message, 1, func(number protowire.Number, value []byte) error {
		if number != 1 {
			return nil
		}
		card, err := parseUsageResetCard(value)
		if err != nil {
			return err
		}
		cards = append(cards, card)
		return nil
	})
	return cards, err
}

func walkResetFields(message []byte, knownFields protowire.Number, field func(protowire.Number, []byte) error) error {
	for len(message) > 0 {
		number, wireType, n := protowire.ConsumeTag(message)
		if n < 0 {
			return errors.New("invalid Grok reset protobuf tag")
		}
		message = message[n:]
		if number <= knownFields && wireType != protowire.BytesType {
			return errors.New("unexpected Grok reset protobuf field type")
		}
		if wireType == protowire.BytesType {
			value, consumed := protowire.ConsumeBytes(message)
			if consumed < 0 {
				return errors.New("invalid Grok reset protobuf field")
			}
			if err := field(number, value); err != nil {
				return err
			}
			message = message[consumed:]
		} else {
			consumed := protowire.ConsumeFieldValue(number, wireType, message)
			if consumed < 0 {
				return errors.New("invalid Grok reset protobuf value")
			}
			message = message[consumed:]
		}
	}
	return nil
}

func parseUsageResetCard(message []byte) (UsageResetCard, error) {
	var card UsageResetCard
	err := walkResetFields(message, 3, func(number protowire.Number, value []byte) error {
		switch number {
		case 1:
			card.TokenID = string(value)
		case 2, 3:
			var timestamp timestamppb.Timestamp
			if proto.Unmarshal(value, &timestamp) != nil || timestamp.CheckValid() != nil {
				return errors.New("invalid Grok reset validity period")
			}
			if number == 2 {
				card.ValidFrom = timestamp.AsTime()
			} else {
				card.ExpiresAt = timestamp.AsTime()
			}
		}
		return nil
	})
	if err != nil {
		return card, err
	}
	if card.TokenID == "" || len(card.TokenID) > 1024 || !utf8.ValidString(card.TokenID) || card.ExpiresAt.IsZero() || !card.ExpiresAt.After(card.ValidFrom) {
		return card, errors.New("incomplete Grok reset card")
	}
	return card, nil
}
