package service

import (
	"context"
	"net"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrIPBanInvalidAddress = infraerrors.BadRequest("IP_BAN_INVALID_ADDRESS", "invalid IP address")
	ErrIPBanExists         = infraerrors.Conflict("IP_BAN_EXISTS", "IP address is already banned")
	ErrIPBanNotFound       = infraerrors.NotFound("IP_BAN_NOT_FOUND", "IP ban not found")
)

type IPBan struct {
	ID        int64     `json:"id"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

type IPBanRepository interface {
	Create(ctx context.Context, ipAddress string) (*IPBan, error)
	List(ctx context.Context, params pagination.PaginationParams) ([]IPBan, *pagination.PaginationResult, error)
	Delete(ctx context.Context, id int64) error
	IsBanned(ctx context.Context, ipAddress string) (bool, error)
}

type IPBanService struct {
	repo IPBanRepository
}

func NewIPBanService(repo IPBanRepository) *IPBanService {
	return &IPBanService{repo: repo}
}

func normalizeIPBanAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed := net.ParseIP(value)
	if parsed == nil {
		return "", ErrIPBanInvalidAddress
	}
	return parsed.String(), nil
}

func (s *IPBanService) Create(ctx context.Context, ipAddress string) (*IPBan, error) {
	normalized, err := normalizeIPBanAddress(ipAddress)
	if err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, normalized)
}

func (s *IPBanService) List(ctx context.Context, params pagination.PaginationParams) ([]IPBan, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params)
}

func (s *IPBanService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrIPBanNotFound
	}
	return s.repo.Delete(ctx, id)
}

func (s *IPBanService) IsBanned(ctx context.Context, ipAddress string) (bool, error) {
	normalized, err := normalizeIPBanAddress(ipAddress)
	if err != nil {
		return false, nil
	}
	return s.repo.IsBanned(ctx, normalized)
}
