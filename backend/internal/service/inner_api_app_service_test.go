package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type fakeInnerAPIAppRepo struct {
	apps      map[string]*InnerAPIApp
	createErr error
}

func newFakeInnerAPIAppRepo() *fakeInnerAPIAppRepo {
	return &fakeInnerAPIAppRepo{apps: map[string]*InnerAPIApp{}}
}

func (f *fakeInnerAPIAppRepo) GetByAppID(_ context.Context, appID string) (*InnerAPIApp, error) {
	if a, ok := f.apps[appID]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, ErrInnerAPIAppNotFound
}

func (f *fakeInnerAPIAppRepo) Create(_ context.Context, app *InnerAPIApp) (*InnerAPIApp, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	app.ID = int64(len(f.apps) + 1)
	cp := *app
	f.apps[app.AppID] = &cp
	return app, nil
}

func (f *fakeInnerAPIAppRepo) SetEnabled(_ context.Context, appID string, enabled bool) error {
	a, ok := f.apps[appID]
	if !ok {
		return ErrInnerAPIAppNotFound
	}
	a.Enabled = enabled
	return nil
}

func (f *fakeInnerAPIAppRepo) SetPermissions(_ context.Context, appID string, permissions []string) error {
	a, ok := f.apps[appID]
	if !ok {
		return ErrInnerAPIAppNotFound
	}
	a.Permissions = append([]string(nil), permissions...)
	return nil
}

func (f *fakeInnerAPIAppRepo) BumpTokenVersion(_ context.Context, appID string) (int, error) {
	a, ok := f.apps[appID]
	if !ok {
		return 0, ErrInnerAPIAppNotFound
	}
	a.TokenVersion++
	return a.TokenVersion, nil
}

func (f *fakeInnerAPIAppRepo) Delete(_ context.Context, appID string) error {
	if _, ok := f.apps[appID]; !ok {
		return ErrInnerAPIAppNotFound
	}
	delete(f.apps, appID)
	return nil
}

func (f *fakeInnerAPIAppRepo) List(_ context.Context) ([]*InnerAPIApp, error) {
	out := make([]*InnerAPIApp, 0, len(f.apps))
	for _, a := range f.apps {
		out = append(out, a)
	}
	return out, nil
}

// testCodec 返回一个配置了有效 32 字节密钥的 codec。
func testCodec() *InnerAPITokenCodec {
	key := strings.Repeat("ab", 32) // 64 hex chars = 32 bytes
	return NewInnerAPITokenCodec(&config.Config{InnerAPIRPC: config.InnerAPIRPCConfig{EncryptionKey: key}})
}

func TestInnerAPIAppService_CreateApp_TokenDecrypts(t *testing.T) {
	codec := testCodec()
	svc := NewInnerAPIAppService(newFakeInnerAPIAppRepo(), codec)
	ctx := context.Background()

	app, token, err := svc.CreateApp(ctx, "my-app", []string{InnerAPIPermissionMaterialsRead})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if !strings.HasPrefix(app.AppID, InnerAPIAppIDPrefix) {
		t.Fatalf("app_id missing prefix: %q", app.AppID)
	}
	if token == "" {
		t.Fatal("expected non-empty one-time token")
	}
	// token 必须能解密回该 app_id。
	gotID, _, err := codec.Parse(token)
	if err != nil {
		t.Fatalf("Parse token: %v", err)
	}
	if gotID != app.AppID {
		t.Fatalf("token app_id=%q want %q", gotID, app.AppID)
	}
}

func TestInnerAPIAppService_CreateApp_InvalidName(t *testing.T) {
	svc := NewInnerAPIAppService(newFakeInnerAPIAppRepo(), testCodec())
	if _, _, err := svc.CreateApp(context.Background(), "   ", nil); !errors.Is(err, ErrInnerAPIAppInvalidName) {
		t.Fatalf("expected ErrInnerAPIAppInvalidName, got %v", err)
	}
}

func TestInnerAPIAppService_Authenticate(t *testing.T) {
	codec := testCodec()
	svc := NewInnerAPIAppService(newFakeInnerAPIAppRepo(), codec)
	ctx := context.Background()

	app, token, err := svc.CreateApp(ctx, "auth-app", []string{InnerAPIPermissionBalanceRead})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	// 合法 token 通过。
	got, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate valid: %v", err)
	}
	if got.AppID != app.AppID {
		t.Fatalf("authenticated app_id=%q want %q", got.AppID, app.AppID)
	}

	// 篡改/垃圾 token → 统一未认证。
	if _, err := svc.Authenticate(ctx, "not-a-valid-token"); !errors.Is(err, ErrInnerAPIAppUnauthenticated) {
		t.Fatalf("garbage token: expected unauthenticated, got %v", err)
	}

	// token 合法但 app 不在库（已删）→ 统一未认证。
	orphanToken, _ := codec.Mint("iapp_orphan", 1)
	if _, err := svc.Authenticate(ctx, orphanToken); !errors.Is(err, ErrInnerAPIAppUnauthenticated) {
		t.Fatalf("orphan token: expected unauthenticated, got %v", err)
	}

	// 停用后立即失效（缓存被主动删除，重新加载读到 disabled）。
	if err := svc.SetEnabled(ctx, app.AppID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrInnerAPIAppUnauthenticated) {
		t.Fatalf("disabled app: expected unauthenticated, got %v", err)
	}
}

func TestInnerAPIAppService_TokenNotConfigured(t *testing.T) {
	// 空密钥：CreateApp / Authenticate 都应报「未配置」。
	codec := NewInnerAPITokenCodec(&config.Config{})
	svc := NewInnerAPIAppService(newFakeInnerAPIAppRepo(), codec)
	ctx := context.Background()

	if _, _, err := svc.CreateApp(ctx, "x", nil); !errors.Is(err, ErrInnerAPIAppTokenNotConfigured) {
		t.Fatalf("CreateApp without key: expected ErrInnerAPIAppTokenNotConfigured, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, "whatever"); !errors.Is(err, ErrInnerAPIAppTokenNotConfigured) {
		t.Fatalf("Authenticate without key: expected ErrInnerAPIAppTokenNotConfigured, got %v", err)
	}
}

func TestInnerAPIAppService_RefreshToken_InvalidatesOld(t *testing.T) {
	svc := NewInnerAPIAppService(newFakeInnerAPIAppRepo(), testCodec())
	ctx := context.Background()

	app, oldToken, err := svc.CreateApp(ctx, "refresh-app", []string{InnerAPIPermissionBalanceWrite})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	// 旧 token 当前有效。
	if _, err := svc.Authenticate(ctx, oldToken); err != nil {
		t.Fatalf("old token before refresh: %v", err)
	}

	newToken, err := svc.RefreshToken(ctx, app.AppID)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	// 刷新后：旧 token 立即失效，新 token 有效。
	if _, err := svc.Authenticate(ctx, oldToken); !errors.Is(err, ErrInnerAPIAppUnauthenticated) {
		t.Fatalf("old token after refresh: expected unauthenticated, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, newToken); err != nil {
		t.Fatalf("new token after refresh: %v", err)
	}
}

func TestInnerAPIAppService_DeleteApp(t *testing.T) {
	svc := NewInnerAPIAppService(newFakeInnerAPIAppRepo(), testCodec())
	ctx := context.Background()

	app, token, err := svc.CreateApp(ctx, "del-app", []string{InnerAPIPermissionBalanceRead})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if err := svc.DeleteApp(ctx, app.AppID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	// 删除后 token 失效（app 查不到）。
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrInnerAPIAppUnauthenticated) {
		t.Fatalf("token after delete: expected unauthenticated, got %v", err)
	}
}

func TestValidateInnerAPIPermissions(t *testing.T) {
	got, err := ValidateInnerAPIPermissions([]string{
		InnerAPIPermissionMaterialsWrite,
		InnerAPIPermissionBalanceRead,
		InnerAPIPermissionMaterialsWrite,
	})
	if err != nil {
		t.Fatalf("ValidateInnerAPIPermissions: %v", err)
	}
	want := []string{InnerAPIPermissionBalanceRead, InnerAPIPermissionMaterialsWrite}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("permissions=%v want %v", got, want)
	}
	if _, err := ValidateInnerAPIPermissions([]string{"admin:all"}); !errors.Is(err, ErrInnerAPIAppInvalidPermission) {
		t.Fatalf("unknown permission: expected invalid permission, got %v", err)
	}
}

func TestInnerAPIAppHasPermission(t *testing.T) {
	app := &InnerAPIApp{Permissions: []string{InnerAPIPermissionMaterialsRead}}
	if !app.HasPermission(InnerAPIPermissionMaterialsRead) {
		t.Fatal("expected materials:read to be granted")
	}
	if app.HasPermission(InnerAPIPermissionMaterialsWrite) {
		t.Fatal("did not expect materials:write to be granted")
	}
}
