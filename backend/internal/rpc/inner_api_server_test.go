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

type innerAPIBatchDeleteMaterialRepoStub struct {
	service.UserMaterialRepository
	userID     int64
	ids        []string
	deletedIDs []string
}

func (r *innerAPIBatchDeleteMaterialRepoStub) SoftDeleteByPublicIDs(_ context.Context, userID int64, ids []string) ([]string, error) {
	r.userID = userID
	r.ids = append([]string(nil), ids...)
	return append([]string(nil), r.deletedIDs...), nil
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
		{"materials batch delete", &innerpb.BatchDeleteMaterialsRequest{}, service.InnerAPIPermissionMaterialsWrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiredPermission(tc.req); got != tc.want {
				t.Fatalf("requiredPermission()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestBatchDeleteMaterialsRequestSummary(t *testing.T) {
	ids := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
	}
	summary := innerAPIRequestSummary(&innerpb.BatchDeleteMaterialsRequest{
		AccountId: "acct_7",
		Ids:       ids,
	})
	if summary["account_id"] != "acct_7" || summary["id_count"] != 2 {
		t.Fatalf("batch delete request summary = %#v", summary)
	}
}

func TestInnerAPIBatchDeleteMaterials(t *testing.T) {
	first := "550e8400-e29b-41d4-a716-446655440000"
	second := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	materialRepo := &innerAPIBatchDeleteMaterialRepoStub{deletedIDs: []string{second}}
	materialService := service.NewUserMaterialService(materialRepo, nil, nil, nil)
	userRepo := &innerAPIUserAccountRepoStub{byAccount: &service.User{ID: 7, AccountID: "acct_7"}}
	server := newInnerAPIServer(nil, materialService, userRepo)

	response, err := server.BatchDeleteMaterials(context.Background(), &innerpb.BatchDeleteMaterialsRequest{
		AccountId: "acct_7",
		Ids:       []string{first, second},
	})
	if err != nil {
		t.Fatalf("BatchDeleteMaterials() error = %v", err)
	}
	if materialRepo.userID != 7 || len(materialRepo.ids) != 2 {
		t.Fatalf("repository call = user %d ids %#v", materialRepo.userID, materialRepo.ids)
	}
	if response.GetDeletedCount() != 1 || len(response.GetDeletedIds()) != 1 || response.GetDeletedIds()[0] != second {
		t.Fatalf("BatchDeleteMaterials() response = %#v", response)
	}
}
