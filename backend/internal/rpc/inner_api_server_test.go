package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/rpc/innerpb"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type innerAPIUserAccountRepoStub struct {
	byAccount *service.User
	byID      *service.User
}

func (r *innerAPIUserAccountRepoStub) GetByAccountID(context.Context, string) (*service.User, error) {
	return r.byAccount, nil
}

func (r *innerAPIUserAccountRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*service.User, error) {
	return r.byID, nil
}

func TestInnerAPIResolvesPublicAccountIDOnly(t *testing.T) {
	repo := &innerAPIUserAccountRepoStub{
		byAccount: &service.User{ID: 7, AccountID: "acct_public_7"},
		byID:      &service.User{ID: 9, AccountID: "acct_payer_9"},
	}
	s := newInnerAPIServer(nil, nil, repo)

	user, err := s.userByAccountID(context.Background(), "acct_public_7")
	if err != nil || user.ID != 7 {
		t.Fatalf("account lookup = (%+v, %v)", user, err)
	}
	if got := s.accountIDByUserID(context.Background(), 9); got != "acct_payer_9" {
		t.Fatalf("payer account id = %q", got)
	}
}

func TestMaterialResponse(t *testing.T) {
	createdAt := time.Date(2026, 8, 21, 3, 20, 36, 0, time.UTC)
	got := materialResponse(&service.UserMaterial{
		ID:          42,
		PublicID:    "550e8400-e29b-41d4-a716-446655440000",
		UserID:      7,
		FileName:    "reference.png",
		CosURL:      "https://cdn.example.test/reference.png",
		ContentType: "image/png",
		SizeBytes:   1234,
		Kind:        "image",
		Source:      "upload",
		CreatedAt:   createdAt,
	}, "acct_7")

	if got.GetId() != "550e8400-e29b-41d4-a716-446655440000" || got.GetAccountId() != "acct_7" {
		t.Fatalf("material identity = (%q, %q), want opaque id and %q", got.GetId(), got.GetAccountId(), "acct_7")
	}
	if got.GetFileName() != "reference.png" || got.GetUrl() != "https://cdn.example.test/reference.png" {
		t.Fatalf("material file = (%q, %q)", got.GetFileName(), got.GetUrl())
	}
	if got.GetContentType() != "image/png" || got.GetSizeBytes() != 1234 || got.GetKind() != "image" || got.GetSource() != "upload" {
		t.Fatalf("material metadata = (%q, %d, %q, %q)", got.GetContentType(), got.GetSizeBytes(), got.GetKind(), got.GetSource())
	}
	if got.GetCreatedAt() != "2026-08-21T03:20:36Z" {
		t.Fatalf("created_at = %q", got.GetCreatedAt())
	}
}

func TestParseAmount(t *testing.T) {
	ok := []struct {
		in   string
		want float64
	}{
		{"1", 1},
		{"0.5", 0.5},
		{"12.34567890", 12.3456789},
	}
	for _, c := range ok {
		got, err := parseAmount(c.in)
		if err != nil {
			t.Fatalf("parseAmount(%q) unexpected err: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("parseAmount(%q)=%v want %v", c.in, got, c.want)
		}
	}

	bad := []string{"", "abc", "0", "-1", "NaN", "Inf", "1e400"}
	for _, c := range bad {
		if _, err := parseAmount(c); err == nil {
			t.Fatalf("parseAmount(%q) expected error, got nil", c)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	cases := map[float64]string{
		1:        "1",
		0.5:      "0.5",
		12.34:    "12.34",
		100.0001: "100.0001",
	}
	for in, want := range cases {
		if got := formatAmount(in); got != want {
			t.Fatalf("formatAmount(%v)=%q want %q", in, got, want)
		}
	}
}

func TestRequiredPermission(t *testing.T) {
	cases := []struct {
		name string
		req  any
		want string
	}{
		{"deduct", &innerpb.DeductRequest{}, service.InnerAPIPermissionBalanceWrite},
		{"refund", &innerpb.RefundRequest{}, service.InnerAPIPermissionBalanceWrite},
		{"balance", &innerpb.GetBalanceRequest{}, service.InnerAPIPermissionBalanceRead},
		{"materials list", &innerpb.ListMaterialsRequest{}, service.InnerAPIPermissionMaterialsRead},
		{"materials get", &innerpb.GetMaterialRequest{}, service.InnerAPIPermissionMaterialsRead},
		{"materials upload", &innerpb.UploadMaterialRequest{}, service.InnerAPIPermissionMaterialsWrite},
		{"materials add by url", &innerpb.AddMaterialByUrlRequest{}, service.InnerAPIPermissionMaterialsWrite},
		{"materials delete", &innerpb.DeleteMaterialRequest{}, service.InnerAPIPermissionMaterialsWrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiredPermission(tc.req); got != tc.want {
				t.Fatalf("requiredPermission()=%q want %q", got, tc.want)
			}
		})
	}
}
