//go:build unit

package adobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func tokenWithClaims(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString([]byte("{}")) + "." +
		base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}

func TestNormalizeCookieString(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"裸字符串", "aux_sid=abc; other=1", "aux_sid=abc; other=1"},
		{"带 Cookie: 前缀", "Cookie: aux_sid=abc", "aux_sid=abc"},
		{"带前缀且大小写混杂", "COOKIE:  aux_sid=abc  ", "aux_sid=abc"},
		{"字符串数组", []string{"a=1", " b=2 ", ""}, "a=1; b=2"},
		{
			name:  "对象数组",
			input: []any{map[string]any{"name": "a", "value": "1"}, map[string]any{"name": "b", "value": "2"}},
			want:  "a=1; b=2",
		},
		{
			name:  "对象数组里缺 name 的条目被跳过",
			input: []any{map[string]any{"value": "1"}, map[string]any{"name": "b", "value": "2"}},
			want:  "b=2",
		},
		{"带 cookies 键的对象", map[string]any{"cookies": []any{"a=1"}}, "a=1"},
		{"带 cookie 键的对象", map[string]any{"cookie": "a=1"}, "a=1"},
		{"无关对象", map[string]any{"foo": "bar"}, ""},
		{"nil", nil, ""},
		{"不支持的类型", 42, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeCookieString(tt.input))
		})
	}
}

func TestRefreshAccessTokenFromCookie(t *testing.T) {
	t.Run("用 Firefly 身份换 token", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{
				"access_token": "access-token",
				"expires_in":   3600,
			}, nil), nil
		}}

		result, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "aux_sid=abc", RefreshOptions{SkipAccountFetch: true})
		require.NoError(t, err)
		require.Equal(t, "access-token", result.AccessToken)
		require.Equal(t, int64(3600), result.ExpiresIn)
		require.Nil(t, result.Account)

		require.Len(t, api.calls, 1, "SkipAccountFetch 应省掉查账号那次往返")
		req := api.calls[0]
		require.Equal(t, DefaultIdentity.Origin, req.Headers["origin"])
		require.Equal(t, DefaultIdentity.Referer, req.Headers["referer"])
		require.Equal(t, "aux_sid=abc", req.Headers["cookie"])
		require.Equal(t, "same-site", req.Headers["sec-fetch-site"])
		require.Equal(t, "cors", req.Headers["sec-fetch-mode"])
		require.Equal(t, "empty", req.Headers["sec-fetch-dest"])
		require.Contains(t, req.URL, "jslVersion=v2-v0.54.0-3-g58cfcb7")

		form, err := url.ParseQuery(string(req.Body))
		require.NoError(t, err)
		require.Equal(t, "clio-playground-web", form.Get("client_id"))
		require.Equal(t, DefaultIdentity.IMSScope, form.Get("scope"))
		require.Empty(t, form.Get("guest_allowed"), "Firefly IMS 不带 guest_allowed，否则会发访客 token")
	})

	t.Run("顺带查账号信息", func(t *testing.T) {
		api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
			if index == 0 {
				return jsonResponse(t, 200, map[string]any{"access_token": "tok"}, nil), nil
			}
			return jsonResponse(t, 200, map[string]any{
				"displayName": "Jane", "email": "jane@example.com", "userId": "u-1",
			}, nil), nil
		}}
		result, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "a=1", RefreshOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.Account)
		require.Equal(t, "Jane", result.Account.DisplayName)
		require.Equal(t, "jane@example.com", result.Account.Email)
	})

	// 账号信息只是附带产物，拿不到不应让刷新整体失败。
	t.Run("账号信息查询失败不影响刷新", func(t *testing.T) {
		api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
			if index == 0 {
				return jsonResponse(t, 200, map[string]any{"access_token": "tok"}, nil), nil
			}
			return jsonResponse(t, 500, map[string]any{}, nil), nil
		}}
		result, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "a=1", RefreshOptions{})
		require.NoError(t, err)
		require.Equal(t, "tok", result.AccessToken)
		require.Nil(t, result.Account)
	})

	t.Run("访客 token 视为会话未被接受", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{
				"access_token": tokenWithClaims(t, map[string]any{
					"user_id": "123_abc@GuestID",
				}),
			}, nil), nil
		}}
		_, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "aux_sid=abc", RefreshOptions{SkipAccountFetch: true})
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
		require.ErrorContains(t, err, "guest token")
	})

	t.Run("空 cookie 直接报错，不打上游", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			t.Fatal("不应发起请求")
			return nil, nil
		}}
		_, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "", RefreshOptions{})
		require.ErrorContains(t, err, "cookie is required")
	})
}

func TestRefreshAccessTokenErrors(t *testing.T) {
	refresh := func(t *testing.T, resp *Response) error {
		t.Helper()
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) { return resp, nil }}
		_, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "a=1", RefreshOptions{SkipAccountFetch: true})
		return err
	}

	// cookie 失效需要用户重新导出，不是可重试的临时故障——归错会让刷新器一直空转重试。
	t.Run("401 归为鉴权失效", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 401, map[string]any{}, nil))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
	})

	t.Run("403 带 IMS 错误体归为鉴权失效", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 403, map[string]any{"error": "invalid_credentials"}, nil))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
	})

	// WAF 拦截出口 IP 时返回 HTML 或空 body 的 403：不能当 cookie 失效，否则账号会被批量永久置 error。
	t.Run("403 HTML 归为可重试", func(t *testing.T) {
		err := refresh(t, &Response{StatusCode: 403, Headers: map[string]string{}, Body: []byte("<html>Access Denied</html>")})
		var auth *AuthError
		require.False(t, errors.As(err, &auth))
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
	})

	t.Run("403 空 body 归为可重试", func(t *testing.T) {
		err := refresh(t, &Response{StatusCode: 403, Headers: map[string]string{}})
		var auth *AuthError
		require.False(t, errors.As(err, &auth))
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
	})

	t.Run("403 JSON 但无错误字段归为可重试", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 403, map[string]any{"message": "forbidden"}, nil))
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
	})

	// 只有 invalid_credentials 是针对单个 cookie 的明确拒绝；其它错误码只让当次请求换号，
	// 不能让一次全局故障（client_id 失效、限流等）把整批账号永久置 error。
	for _, code := range []string{"invalid_client", "access_denied", "rate_limited"} {
		t.Run("403 JSON 错误码 "+code+" 归为可重试", func(t *testing.T) {
			err := refresh(t, jsonResponse(t, 403, map[string]any{"error": code}, nil))
			var auth *AuthError
			require.False(t, errors.As(err, &auth))
			var temporary *UpstreamTemporaryError
			require.True(t, errors.As(err, &temporary))
		})
	}

	for _, code := range []string{"invalid_client", "unauthorized_client", "invalid_scope"} {
		t.Run("401 全局配置错误码 "+code+" 归为可重试", func(t *testing.T) {
			err := refresh(t, jsonResponse(t, 401, map[string]any{"error_code": code}, nil))
			var auth *AuthError
			require.False(t, errors.As(err, &auth))
			var temporary *UpstreamTemporaryError
			require.True(t, errors.As(err, &temporary))
		})
	}

	t.Run("401 带 invalid_credentials 归为鉴权失效", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 401, map[string]any{"error": "invalid_credentials"}, nil))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
	})

	t.Run("5xx 归为可重试", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 503, map[string]any{}, nil))
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
	})

	t.Run("响应不是 JSON", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return &Response{StatusCode: 200, Headers: map[string]string{}, Body: []byte("<html>")}, nil
		}}
		_, err := testClient(api, nil).RefreshAccessTokenFromCookie(
			context.Background(), "a=1", RefreshOptions{SkipAccountFetch: true})
		require.ErrorContains(t, err, "not valid json")
	})

	t.Run("响应缺 access_token", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 200, map[string]any{
			"error":             "invalid_request",
			"error_description": "no session",
			"foo":               "bar",
		}, nil))
		require.ErrorContains(t, err, "missing access_token")
		require.ErrorContains(t, err, "keys=")
		require.ErrorContains(t, err, "invalid_request")
		require.ErrorContains(t, err, "no session")
		var auth *AuthError
		require.False(t, errors.As(err, &auth), "非 invalid_credentials 不应归为鉴权失效")
	})

	t.Run("IMS 说 session cookies empty 归为鉴权失效", func(t *testing.T) {
		err := refresh(t, jsonResponse(t, 200, map[string]any{
			"error":             "invalid_credentials",
			"error_description": "All session cookies are empty",
		}, nil))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
		require.ErrorContains(t, err, "ims_sid")
	})
}

// 单个 profile 端点会对部分 region 的账号 404，必须依次试完两个。
func TestFetchAccountInfoFallsBackToSecondEndpoint(t *testing.T) {
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 404, map[string]any{}, nil), nil
		}
		return jsonResponse(t, 200, map[string]any{"email": "a@b.c"}, nil), nil
	}}
	info, err := testClient(api, nil).FetchAccountInfo(context.Background(), "tok")
	require.NoError(t, err)
	require.Equal(t, "a@b.c", info.Email)
	require.Len(t, api.calls, 2)
	require.Equal(t, profileURLs[0], api.calls[0].URL)
	require.Equal(t, profileURLs[1], api.calls[1].URL)
}

func TestFetchAccountInfoAllEndpointsFail(t *testing.T) {
	api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return jsonResponse(t, 404, map[string]any{}, nil), nil
	}}
	_, err := testClient(api, nil).FetchAccountInfo(context.Background(), "tok")
	require.ErrorContains(t, err, "account info unavailable")

	_, err = testClient(api, nil).FetchAccountInfo(context.Background(), "  ")
	require.ErrorContains(t, err, "empty access token")
}

func TestFetchCreditsBalance(t *testing.T) {
	t.Run("用 Firefly origin 与专属 api key", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{
				"total": map[string]any{
					"quota":          map[string]any{"total": 100, "used": 25, "available": 75},
					"availableUntil": "2026-01-01",
				},
			}, nil), nil
		}}
		balance, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{"user_id": "user-1"}))
		require.NoError(t, err)
		require.Equal(t, int64(100), *balance.Total)
		require.Equal(t, int64(25), *balance.Used)
		require.Equal(t, int64(75), *balance.Available)
		require.Equal(t, "2026-01-01", balance.AvailableUntil)

		headers := api.calls[0].Headers
		// credits 端点用的 api key 与出图端点不同，混用会 403。
		require.Equal(t, "SunbreakWebUI1", headers["x-api-key"])
		require.Equal(t, DefaultIdentity.Origin, headers["origin"])
		require.Equal(t, DefaultIdentity.Referer, headers["referer"])
		require.Equal(t, DefaultIdentity.UserAgent, headers["user-agent"])
		require.Equal(t, "cross-site", headers["sec-fetch-site"])
		require.Equal(t, "user-1", headers["x-account-id"])
	})

	t.Run("24 位 hex user_id 补成 x-account-id @AdobeID", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{
				"total": map[string]any{"quota": map[string]any{"total": 1, "used": 0, "available": 1}},
			}, nil), nil
		}}
		_, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{"user_id": "8bb5048b5fa5386f0a495f88"}))
		require.NoError(t, err)
		require.Equal(t, "8BB5048B5FA5386F0A495F88@AdobeID", api.calls[0].Headers["x-account-id"])
	})

	// 缺字段与「余额为 0」必须能区分开。
	t.Run("上游没返回额度时为 nil", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{"total": map[string]any{}}, nil), nil
		}}
		balance, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
		require.NoError(t, err)
		require.Nil(t, balance.Available)
	})

	t.Run("token 里没有账号 id", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			t.Fatal("不应发起请求")
			return nil, nil
		}}
		_, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{}))
		require.ErrorContains(t, err, "missing account id")
	})

	t.Run("401 错误带上游 body 预览", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 401, map[string]any{
				"error": map[string]any{"code": "401013", "message": "ErrInvalidOauthToken"},
			}, nil), nil
		}}
		_, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
		require.ErrorContains(t, err, "ErrInvalidOauthToken")
	})

	t.Run("配额耗尽", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 403, map[string]any{},
				map[string]string{"x-access-error": "taste_exhausted"}), nil
		}}
		_, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
		var quota *QuotaExhaustedError
		require.True(t, errors.As(err, &quota))
	})

	t.Run("403 model_not_entitled 不是鉴权失效", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 403, map[string]any{
				"error_code": "model_not_entitled",
			}, nil), nil
		}}
		_, err := testClient(api, nil).FetchCreditsBalance(
			context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
		var entitled *NotEntitledError
		require.True(t, errors.As(err, &entitled))
		var auth *AuthError
		require.False(t, errors.As(err, &auth))
	})
}

// FREE 账号的完整响应，含 planCap 和一个已知的 credit pool。
// 这是本轮解析扩展的锚点：抓包看到的真实形状。
func TestFetchCreditsBalanceParsesPlanCapAndPools(t *testing.T) {
	api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return jsonResponse(t, 200, map[string]any{
			"total": map[string]any{
				"quota":          map[string]any{"total": 10, "used": 0, "available": 10},
				"planCap":        "FREE",
				"availableUntil": "2026-09-10T23:59:59.999Z",
			},
			"credits": map[string]any{
				"firefly_free_credit": map[string]any{
					"quota": map[string]any{"total": 10, "used": 0, "available": 10},
				},
			},
		}, nil), nil
	}}
	balance, err := testClient(api, nil).FetchCreditsBalance(
		context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
	require.NoError(t, err)
	require.Equal(t, "FREE", balance.PlanCap)
	require.Len(t, balance.CreditPools, 1)
	require.Equal(t, "firefly_free_credit", balance.CreditPools[0].Name)
	require.Equal(t, int64(10), *balance.CreditPools[0].Total)
	require.Equal(t, int64(0), *balance.CreditPools[0].Used)
	require.Equal(t, int64(10), *balance.CreditPools[0].Available)
	require.Equal(t, "2026-09-10T23:59:59.999Z", balance.AvailableUntil)
}

// 上游没返回新字段时不能 panic，也不能凭空造 planCap。
func TestFetchCreditsBalanceHandlesMissingPlanCapAndPools(t *testing.T) {
	api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return jsonResponse(t, 200, map[string]any{
			"total": map[string]any{
				"quota": map[string]any{"total": 100, "used": 25, "available": 75},
			},
		}, nil), nil
	}}
	balance, err := testClient(api, nil).FetchCreditsBalance(
		context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
	require.NoError(t, err)
	require.Equal(t, "", balance.PlanCap)
	require.Nil(t, balance.CreditPools)
}

// 池名不是硬编码：任何未知名字的子池都应被读进来，并按字典序稳定输出。
// 这条守的是「付费档池名换成 firefly_paid_credit / firefly_pro_credit 也能自然工作」。
func TestFetchCreditsBalanceReadsUnknownPoolNames(t *testing.T) {
	api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return jsonResponse(t, 200, map[string]any{
			"total": map[string]any{
				"quota": map[string]any{"total": 5, "used": 1, "available": 4},
			},
			"credits": map[string]any{
				"zzz_last_credit":    map[string]any{"quota": map[string]any{"total": 2, "available": 2}},
				"aaa_first_credit":   map[string]any{"quota": map[string]any{"total": 3, "available": 3}},
				"middle_paid_credit": map[string]any{"quota": map[string]any{"total": 0, "available": 0}},
			},
		}, nil), nil
	}}
	balance, err := testClient(api, nil).FetchCreditsBalance(
		context.Background(), tokenWithClaims(t, map[string]any{"user_id": "u"}))
	require.NoError(t, err)
	require.Len(t, balance.CreditPools, 3)
	require.Equal(t, "aaa_first_credit", balance.CreditPools[0].Name)
	require.Equal(t, "middle_paid_credit", balance.CreditPools[1].Name)
	require.Equal(t, "zzz_last_credit", balance.CreditPools[2].Name)
}
