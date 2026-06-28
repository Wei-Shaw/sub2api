package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type fakeBillingAppRepo struct {
	apps      map[string]*BillingApp
	createErr error
}

func newFakeBillingAppRepo() *fakeBillingAppRepo {
	return &fakeBillingAppRepo{apps: map[string]*BillingApp{}}
}

func (f *fakeBillingAppRepo) GetByAppID(_ context.Context, appID string) (*BillingApp, error) {
	if a, ok := f.apps[appID]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, ErrBillingAppNotFound
}

func (f *fakeBillingAppRepo) Create(_ context.Context, app *BillingApp) (*BillingApp, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	app.ID = int64(len(f.apps) + 1)
	cp := *app
	f.apps[app.AppID] = &cp
	return app, nil
}

func (f *fakeBillingAppRepo) SetEnabled(_ context.Context, appID string, enabled bool) error {
	a, ok := f.apps[appID]
	if !ok {
		return ErrBillingAppNotFound
	}
	a.Enabled = enabled
	return nil
}

func (f *fakeBillingAppRepo) BumpTokenVersion(_ context.Context, appID string) (int, error) {
	a, ok := f.apps[appID]
	if !ok {
		return 0, ErrBillingAppNotFound
	}
	a.TokenVersion++
	return a.TokenVersion, nil
}

func (f *fakeBillingAppRepo) Delete(_ context.Context, appID string) error {
	if _, ok := f.apps[appID]; !ok {
		return ErrBillingAppNotFound
	}
	delete(f.apps, appID)
	return nil
}

func (f *fakeBillingAppRepo) List(_ context.Context) ([]*BillingApp, error) {
	out := make([]*BillingApp, 0, len(f.apps))
	for _, a := range f.apps {
		out = append(out, a)
	}
	return out, nil
}

// testCodec 返回一个配置了有效 32 字节密钥的 codec。
func testCodec() *BillingAppTokenCodec {
	key := strings.Repeat("ab", 32) // 64 hex chars = 32 bytes
	return NewBillingAppTokenCodec(&config.Config{BalanceRPC: config.BalanceRPCConfig{EncryptionKey: key}})
}

func TestBillingAppService_CreateApp_TokenDecrypts(t *testing.T) {
	codec := testCodec()
	svc := NewBillingAppService(newFakeBillingAppRepo(), codec)
	ctx := context.Background()

	app, token, err := svc.CreateApp(ctx, "my-app")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if !strings.HasPrefix(app.AppID, BillingAppIDPrefix) {
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

func TestBillingAppService_CreateApp_InvalidName(t *testing.T) {
	svc := NewBillingAppService(newFakeBillingAppRepo(), testCodec())
	if _, _, err := svc.CreateApp(context.Background(), "   "); !errors.Is(err, ErrBillingAppInvalidName) {
		t.Fatalf("expected ErrBillingAppInvalidName, got %v", err)
	}
}

func TestBillingAppService_Authenticate(t *testing.T) {
	codec := testCodec()
	svc := NewBillingAppService(newFakeBillingAppRepo(), codec)
	ctx := context.Background()

	app, token, err := svc.CreateApp(ctx, "auth-app")
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
	if _, err := svc.Authenticate(ctx, "not-a-valid-token"); !errors.Is(err, ErrBillingAppUnauthenticated) {
		t.Fatalf("garbage token: expected unauthenticated, got %v", err)
	}

	// token 合法但 app 不在库（已删）→ 统一未认证。
	orphanToken, _ := codec.Mint("bapp_orphan", 1)
	if _, err := svc.Authenticate(ctx, orphanToken); !errors.Is(err, ErrBillingAppUnauthenticated) {
		t.Fatalf("orphan token: expected unauthenticated, got %v", err)
	}

	// 停用后立即失效（缓存被主动删除，重新加载读到 disabled）。
	if err := svc.SetEnabled(ctx, app.AppID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrBillingAppUnauthenticated) {
		t.Fatalf("disabled app: expected unauthenticated, got %v", err)
	}
}

func TestBillingAppService_TokenNotConfigured(t *testing.T) {
	// 空密钥：CreateApp / Authenticate 都应报「未配置」。
	codec := NewBillingAppTokenCodec(&config.Config{})
	svc := NewBillingAppService(newFakeBillingAppRepo(), codec)
	ctx := context.Background()

	if _, _, err := svc.CreateApp(ctx, "x"); !errors.Is(err, ErrBillingAppTokenNotConfigured) {
		t.Fatalf("CreateApp without key: expected ErrBillingAppTokenNotConfigured, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, "whatever"); !errors.Is(err, ErrBillingAppTokenNotConfigured) {
		t.Fatalf("Authenticate without key: expected ErrBillingAppTokenNotConfigured, got %v", err)
	}
}

func TestBillingAppService_RefreshToken_InvalidatesOld(t *testing.T) {
	svc := NewBillingAppService(newFakeBillingAppRepo(), testCodec())
	ctx := context.Background()

	app, oldToken, err := svc.CreateApp(ctx, "refresh-app")
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
	if _, err := svc.Authenticate(ctx, oldToken); !errors.Is(err, ErrBillingAppUnauthenticated) {
		t.Fatalf("old token after refresh: expected unauthenticated, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, newToken); err != nil {
		t.Fatalf("new token after refresh: %v", err)
	}
}

func TestBillingAppService_DeleteApp(t *testing.T) {
	svc := NewBillingAppService(newFakeBillingAppRepo(), testCodec())
	ctx := context.Background()

	app, token, err := svc.CreateApp(ctx, "del-app")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if err := svc.DeleteApp(ctx, app.AppID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	// 删除后 token 失效（app 查不到）。
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrBillingAppUnauthenticated) {
		t.Fatalf("token after delete: expected unauthenticated, got %v", err)
	}
}
