package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestCustomDomainForUserFromServiceRedactsSharedDomainSecretsAndUsers(t *testing.T) {
	target := "gateway.example.com"
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	domain := &service.CustomDomain{
		ID:                   7,
		UserID:               42,
		AllUsers:             true,
		UserIDs:              []int64{42, 99},
		Domain:               "api.customer.example",
		Status:               service.CustomDomainStatusPendingDNS,
		VerificationTXTName:  "_sub2api-verify.api.customer.example",
		VerificationTXTValue: "sub2api-domain-verification=secret-token",
		CNAMETarget:          &target,
		CreatedAt:            now,
		UpdatedAt:            now,
		User: &service.User{
			ID:       42,
			Email:    "owner@example.com",
			Username: "owner",
			Role:     service.RoleAdmin,
		},
		Users: []service.User{
			{ID: 42, Email: "owner@example.com", Role: service.RoleAdmin},
			{ID: 99, Email: "shared@example.com", Role: service.RoleUser},
		},
		CanManage: false,
	}

	got := CustomDomainForUserFromService(domain)
	if got == nil {
		t.Fatal("expected converted custom domain")
	}
	if got.Domain != domain.Domain || got.Status != domain.Status || got.CNAMETarget == nil || *got.CNAMETarget != target {
		t.Fatalf("shared domain should keep public routing fields: %#v", got)
	}
	if got.UserID != 0 || got.AllUsers || len(got.UserIDs) != 0 || got.User != nil || len(got.Users) != 0 {
		t.Fatalf("shared domain leaked owner or authorized-user metadata: %#v", got)
	}
	if got.VerificationTXTName != "" || got.VerificationTXTValue != "" {
		t.Fatalf("shared domain leaked verification fields: %#v", got)
	}
}

func TestCustomDomainForUserFromServiceKeepsOwnerManageFields(t *testing.T) {
	domain := &service.CustomDomain{
		ID:                   7,
		UserID:               42,
		AllUsers:             false,
		UserIDs:              []int64{42, 99},
		Domain:               "api.customer.example",
		Status:               service.CustomDomainStatusPendingDNS,
		VerificationTXTName:  "_sub2api-verify.api.customer.example",
		VerificationTXTValue: "sub2api-domain-verification=secret-token",
		User:                 &service.User{ID: 42, Email: "owner@example.com", Role: service.RoleAdmin},
		Users:                []service.User{{ID: 99, Email: "shared@example.com", Role: service.RoleUser}},
		CanManage:            true,
	}

	got := CustomDomainForUserFromService(domain)
	if got == nil {
		t.Fatal("expected converted custom domain")
	}
	if got.UserID != 42 || len(got.UserIDs) != 2 || got.User == nil || len(got.Users) != 1 {
		t.Fatalf("owner response should retain management metadata: %#v", got)
	}
	if got.VerificationTXTName == "" || got.VerificationTXTValue == "" {
		t.Fatalf("owner response should retain verification fields: %#v", got)
	}
}
