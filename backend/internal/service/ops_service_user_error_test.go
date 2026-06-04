package service

import (
	"context"
	"database/sql"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type stubOpsRepoForUserErr struct {
	OpsRepository // 嵌入接口，未实现的方法 panic，仅覆盖 ListErrorLogs
	gotFilter     *OpsErrorLogFilter

	// GetErrorLogByID 控制字段
	detailToReturn    *OpsErrorLogDetail
	detailErrToReturn error
}

func (s *stubOpsRepoForUserErr) ListErrorLogs(ctx context.Context, f *OpsErrorLogFilter) (*OpsErrorLogList, error) {
	s.gotFilter = f
	return &OpsErrorLogList{
		Errors: []*OpsErrorLog{{
			Phase: "request", Type: "rate_limit_error",
			Model: "m", RequestedModel: "rm", StatusCode: 429,
			Message: "secret", UserEmail: "a@b.c",
		}},
		Total: 1, Page: 1, PageSize: 20,
	}, nil
}

func (s *stubOpsRepoForUserErr) GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
	if s.detailErrToReturn != nil {
		return nil, s.detailErrToReturn
	}
	return s.detailToReturn, nil
}

func TestListUserErrorRequests_ForcesScopeAndRedacts(t *testing.T) {
	stub := &stubOpsRepoForUserErr{}
	svc := &OpsService{opsRepo: stub}
	uid := int64(42)
	kid := int64(7)
	in := &OpsErrorLogFilter{UserID: nil, View: "errors", Phase: "upstream", APIKeyID: &kid}
	out, err := svc.ListUserErrorRequests(context.Background(), uid, in)
	if err != nil {
		t.Fatal(err)
	}
	// 强制按用户
	if stub.gotFilter.UserID == nil || *stub.gotFilter.UserID != uid {
		t.Fatalf("UserID not forced: %+v", stub.gotFilter.UserID)
	}
	// 强制 View=all（含业务限流/余额）
	if stub.gotFilter.View != "all" {
		t.Fatalf("View not forced to all: %q", stub.gotFilter.View)
	}
	// 强制排除 count_tokens
	if !stub.gotFilter.ExcludeCountTokens {
		t.Fatal("ExcludeCountTokens not forced")
	}
	// 强制清空 Phase（防止 "upstream" 绕过 status>=400 子句 + 与 ErrorPhasesAny 双重约束）
	if stub.gotFilter.Phase != "" {
		t.Fatalf("Phase not cleared: %q", stub.gotFilter.Phase)
	}
	// APIKeyID 透传保留（用户可按自己 key 过滤；越权由 user_id AND api_key_id 双重防护）
	if stub.gotFilter.APIKeyID == nil || *stub.gotFilter.APIKeyID != kid {
		t.Fatalf("APIKeyID should be preserved, got %v", stub.gotFilter.APIKeyID)
	}
	// 调用方传入的 filter 不应被原地篡改（验证 shallow copy 隔离生效）
	if in.View != "errors" || in.UserID != nil || in.Phase != "upstream" {
		t.Fatalf("caller filter was mutated: View=%q UserID=%v Phase=%q", in.View, in.UserID, in.Phase)
	}
	// 脱敏：返回条目含 message 字段
	if len(out.Items) != 1 || out.Items[0].Category != "rate_limit" || out.Items[0].Model != "rm" {
		t.Fatalf("bad item: %+v", out.Items)
	}
}

func TestGetUserErrorRequestDetail_OwnershipEnforced(t *testing.T) {
	ownerUID := int64(999)
	callerUID := int64(1)
	upstreamStatus := 503

	detail := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			ID:              42,
			Phase:           "upstream",
			Type:            "api_error",
			Model:           "gpt-4",
			RequestedModel:  "gpt-4-turbo",
			InboundEndpoint: "/v1/chat/completions",
			StatusCode:      502,
			Platform:        "openai",
			Message:         "upstream failed",
			UserID:          &ownerUID,
		},
		ErrorBody:          `{"error":"upstream"}`,
		UpstreamStatusCode: &upstreamStatus,
	}

	stub := &stubOpsRepoForUserErr{detailToReturn: detail}
	svc := &OpsService{opsRepo: stub}

	// 越权调用（callerUID=1,但记录属于 ownerUID=999）→ 应返回 NotFound,detail 为 nil
	got, err := svc.GetUserErrorRequestDetail(context.Background(), callerUID, 42)
	if err == nil {
		t.Fatal("expected error for unauthorized access, got nil")
	}
	if got != nil {
		t.Fatalf("expected nil detail for unauthorized access, got %+v", got)
	}
	// 验证错误为 NotFound(不暴露存在性)
	if !infraerrors.IsNotFound(err) {
		t.Fatalf("expected NotFound error, got: %v", err)
	}

	// 合法调用（callerUID=999 = ownerUID）→ 应返回 non-nil detail
	got2, err2 := svc.GetUserErrorRequestDetail(context.Background(), ownerUID, 42)
	if err2 != nil {
		t.Fatalf("expected no error for legitimate access, got %v", err2)
	}
	if got2 == nil {
		t.Fatal("expected non-nil detail for legitimate access")
	}
	if got2.ID != 42 {
		t.Errorf("want ID=42, got %d", got2.ID)
	}
	if got2.ErrorBody != `{"error":"upstream"}` {
		t.Errorf("want ErrorBody=%q, got %q", `{"error":"upstream"}`, got2.ErrorBody)
	}
	if got2.UpstreamStatusCode == nil || *got2.UpstreamStatusCode != 503 {
		t.Errorf("want UpstreamStatusCode=503, got %v", got2.UpstreamStatusCode)
	}
	if got2.Message != "upstream failed" {
		t.Errorf("want Message=%q, got %q", "upstream failed", got2.Message)
	}
}

func TestGetUserErrorRequestDetail_NotFound(t *testing.T) {
	stub := &stubOpsRepoForUserErr{detailErrToReturn: sql.ErrNoRows}
	svc := &OpsService{opsRepo: stub}

	got, err := svc.GetUserErrorRequestDetail(context.Background(), 1, 999)
	if err == nil {
		t.Fatal("expected error for not found, got nil")
	}
	if got != nil {
		t.Fatalf("expected nil detail, got %+v", got)
	}
}

func TestGetUserErrorRequestDetail_InvalidID(t *testing.T) {
	stub := &stubOpsRepoForUserErr{}
	svc := &OpsService{opsRepo: stub}

	_, err := svc.GetUserErrorRequestDetail(context.Background(), 1, 0)
	if err == nil {
		t.Fatal("expected error for id=0")
	}
	_, err = svc.GetUserErrorRequestDetail(context.Background(), 1, -5)
	if err == nil {
		t.Fatal("expected error for id=-5")
	}
}

func TestListUserErrorRequests_EnablesMatchDeletedKeyOwner(t *testing.T) {
	stub := &stubOpsRepoForUserErr{}
	svc := &OpsService{opsRepo: stub}
	uid := int64(42)

	if _, err := svc.ListUserErrorRequests(context.Background(), uid, &OpsErrorLogFilter{}); err != nil {
		t.Fatal(err)
	}
	if stub.gotFilter == nil || !stub.gotFilter.MatchDeletedKeyOwner {
		t.Fatal("ListUserErrorRequests should enable MatchDeletedKeyOwner for the user scope")
	}
}

func TestGetUserErrorRequestDetail_DeletedKeyOwnerAccess(t *testing.T) {
	ownerUID := int64(777)
	otherUID := int64(2)

	// 情况2:user_id=NULL,靠 deleted_key_owner_user_id 归因到 ownerUID
	mk := func() *OpsErrorLogDetail {
		return &OpsErrorLogDetail{
			OpsErrorLog: OpsErrorLog{
				ID:            55,
				Phase:         "auth",
				Type:          "api_error",
				StatusCode:    401,
				Message:       "Invalid API key",
				UserID:        nil,
				APIKeyName:    "my-old-key",
				APIKeyDeleted: true,
			},
			DeletedKeyOwnerUserID: &ownerUID,
		}
	}

	// 原所有者(经 deleted_key 归因)→ 放行
	svcOwner := &OpsService{opsRepo: &stubOpsRepoForUserErr{detailToReturn: mk()}}
	got, err := svcOwner.GetUserErrorRequestDetail(context.Background(), ownerUID, 55)
	if err != nil {
		t.Fatalf("owner via deleted_key should be allowed, got err: %v", err)
	}
	if got == nil || got.ID != 55 {
		t.Fatalf("expected detail ID=55, got %+v", got)
	}
	if !got.KeyDeleted || got.KeyName != "my-old-key" {
		t.Fatalf("expected KeyDeleted=true KeyName=my-old-key, got %+v", got)
	}

	// 他人 → NotFound,不泄露存在性
	svcOther := &OpsService{opsRepo: &stubOpsRepoForUserErr{detailToReturn: mk()}}
	got2, err2 := svcOther.GetUserErrorRequestDetail(context.Background(), otherUID, 55)
	if err2 == nil || got2 != nil {
		t.Fatalf("non-owner should get (nil, NotFound), got detail=%+v err=%v", got2, err2)
	}
	if !infraerrors.IsNotFound(err2) {
		t.Fatalf("expected NotFound, got %v", err2)
	}
}
