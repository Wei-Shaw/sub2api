package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ipBanRepositoryStub struct {
	createdAddress string
	created        *IPBan
	deleteID       int64
}

func (s *ipBanRepositoryStub) Create(_ context.Context, ipAddress string) (*IPBan, error) {
	s.createdAddress = ipAddress
	return s.created, nil
}

func (s *ipBanRepositoryStub) List(_ context.Context, _ pagination.PaginationParams) ([]IPBan, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *ipBanRepositoryStub) Delete(_ context.Context, id int64) error {
	s.deleteID = id
	return nil
}

func (s *ipBanRepositoryStub) IsBanned(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestIPBanServiceCreateNormalizesIPAddress(t *testing.T) {
	repo := &ipBanRepositoryStub{created: &IPBan{ID: 1, IPAddress: "192.0.2.10"}}
	service := NewIPBanService(repo)

	if _, err := service.Create(context.Background(), " 192.0.2.10 "); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.createdAddress != "192.0.2.10" {
		t.Fatalf("created address = %q, want normalized address", repo.createdAddress)
	}
}

func TestIPBanServiceCreateRejectsInvalidIPAddress(t *testing.T) {
	repo := &ipBanRepositoryStub{}
	service := NewIPBanService(repo)

	if _, err := service.Create(context.Background(), "not-an-ip"); err != ErrIPBanInvalidAddress {
		t.Fatalf("Create() error = %v, want ErrIPBanInvalidAddress", err)
	}
	if repo.createdAddress != "" {
		t.Fatalf("repository received invalid address %q", repo.createdAddress)
	}
}

func TestIPBanServiceDeleteRejectsInvalidID(t *testing.T) {
	repo := &ipBanRepositoryStub{}
	service := NewIPBanService(repo)

	if err := service.Delete(context.Background(), 0); err != ErrIPBanNotFound {
		t.Fatalf("Delete() error = %v, want ErrIPBanNotFound", err)
	}
	if repo.deleteID != 0 {
		t.Fatalf("repository received invalid id %d", repo.deleteID)
	}
}
