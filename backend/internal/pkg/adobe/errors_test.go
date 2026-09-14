//go:build unit

package adobe

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsRotatable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "上游 429 临时故障可换号",
			err:  NewUpstreamTemporaryError(`submit failed: 429 {"error":"rate limited"}`, 429, ErrorTypeStatus),
			want: true,
		},
		{
			name: "账号配额耗尽可换号",
			err:  NewQuotaExhaustedError("Adobe quota exhausted", 403),
			want: true,
		},
		{
			name: "token 失效可换号",
			err:  NewAuthError("Token invalid or expired", 401),
			want: true,
		},
		{
			name: "内容安全拒绝可跳到中转号",
			err:  NewContentRejectedError("submit failed: 451 image_unsafe", 451, ""),
			want: true,
		},
		{
			name: "权益不足可换更高套餐号",
			err:  NewNotEntitledError("model_not_entitled", 403, ""),
			want: true,
		},
		{
			// 请求本身的 4xx：换号也救不了。
			name: "终态请求错误不换号",
			err:  NewRequestError("submit failed: 400 bad"),
			want: false,
		},
		{
			name: "非本包错误不换号",
			err:  errors.New("network down"),
			want: false,
		},
		{
			name: "nil 不换号",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsRotatable(tt.err))
		})
	}
}

// 包装过的错误仍应被正确识别——service 层常会用 %w 加上下文再上抛。
func TestIsRotatableThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("generate image: %w", NewQuotaExhaustedError("exhausted", 403))
	require.True(t, IsRotatable(wrapped))

	wrappedTerminal := fmt.Errorf("generate image: %w", NewRequestError("bad request"))
	require.False(t, IsRotatable(wrappedTerminal))
}

// 各子类型都能被 errors.As 当作 *RequestError 取出，便于统一读 StatusCode。
func TestErrorTypesUnwrapToRequestError(t *testing.T) {
	for _, err := range []error{
		NewAuthError("auth", 401),
		NewQuotaExhaustedError("quota", 403),
		NewUpstreamTemporaryError("temp", 503, ErrorTypeStatus),
		NewContentRejectedError("unsafe", 451, ""),
		NewNotEntitledError("not entitled", 403, ""),
	} {
		var base *RequestError
		require.True(t, errors.As(err, &base), "%T 应能取出 *RequestError", err)
		require.NotZero(t, base.StatusCode)
	}
}

func TestErrorTypesAreDistinguishable(t *testing.T) {
	var auth *AuthError
	var quota *QuotaExhaustedError
	var entitled *NotEntitledError

	// 配额耗尽虽与鉴权失败共用 401/403，但必须能与 AuthError 区分开：
	// 前者应冷却账号，后者应触发凭据刷新。
	require.True(t, errors.As(error(NewQuotaExhaustedError("q", 403)), &quota))
	require.False(t, errors.As(error(NewQuotaExhaustedError("q", 403)), &auth))
	require.True(t, errors.As(error(NewAuthError("a", 401)), &auth))

	require.True(t, errors.As(error(NewNotEntitledError("e", 403, "")), &entitled))
	require.False(t, errors.As(error(NewNotEntitledError("e", 403, "")), &auth),
		"权益不足不能伪装成 token 失效")
	require.False(t, errors.As(error(NewAuthError("a", 403)), &entitled))
}

func TestRequestErrorUserMessage(t *testing.T) {
	err := &RequestError{Message: "submit failed: 400 raw upstream body"}
	require.Equal(t, "submit failed: 400 raw upstream body", err.User())

	err.UserMessage = "生成失败，请调整提示词后重试"
	require.Equal(t, "生成失败，请调整提示词后重试", err.User())
}

func TestIsRetryableStatus(t *testing.T) {
	// 408 是 Adobe 的降载信号（timeout_error / "system under load"），不是客户端超时。
	for _, status := range []int{408, 429, 451, 500, 502, 503, 504} {
		require.True(t, IsRetryableStatus(status), "status %d 应可重试", status)
	}
	for _, status := range []int{200, 400, 401, 403, 404, 422} {
		require.False(t, IsRetryableStatus(status), "status %d 不应可重试", status)
	}
}

// 真正的收益不是谓词本身，而是 408 能让网关换账号：runAdobeImagesFailover 只在
// IsRotatable 为 true 时才试下一个账号。这条把「分类 → 换号」这一步钉死。
func TestSubmit408IsRotatable(t *testing.T) {
	resp := &Response{
		StatusCode: http.StatusRequestTimeout,
		Body:       []byte(`{"error_code":"timeout_error","message":"system under load"}`),
	}
	c := &Client{}
	err := c.errorForSubmit(resp, "submit", "")
	require.Error(t, err)

	var temporary *UpstreamTemporaryError
	require.ErrorAs(t, err, &temporary, "408 应归为上游临时故障")
	require.True(t, IsRotatable(err), "408 必须可换号，否则一次瞬时限流就打死整个请求")
	require.Contains(t, err.Error(), "system under load")
}

func TestSubmit451ImageUnsafeIsContentRejected(t *testing.T) {
	resp := &Response{
		StatusCode: http.StatusUnavailableForLegalReasons,
		Body:       []byte(`{"error_code":"image_unsafe","message":"nsfw"}`),
	}
	c := &Client{}
	err := c.errorForSubmit(resp, "submit", "")
	require.Error(t, err)

	var rejected *ContentRejectedError
	require.ErrorAs(t, err, &rejected, "image_unsafe 必须从可重试 451 里拆出来")
	var temporary *UpstreamTemporaryError
	require.False(t, errors.As(err, &temporary), "image_unsafe 不能再伪装成临时故障")
	require.True(t, IsRotatable(err), "内容拒绝仍可跳到组内中转号")
}

func TestPoll451ImageUnsafeIsContentRejected(t *testing.T) {
	resp := &Response{
		StatusCode: http.StatusUnavailableForLegalReasons,
		Body:       []byte(`{"error_code":"image_unsafe"}`),
	}
	c := &Client{}
	err := c.errorForStatus(resp, "image poll")
	var rejected *ContentRejectedError
	require.ErrorAs(t, err, &rejected)
}

func TestSubmit451WithoutUnsafeStaysTemporary(t *testing.T) {
	resp := &Response{
		StatusCode: http.StatusUnavailableForLegalReasons,
		Body:       []byte(`{"error_code":"unavailable","message":"legal hold"}`),
	}
	c := &Client{}
	err := c.errorForSubmit(resp, "submit", "")
	var temporary *UpstreamTemporaryError
	require.ErrorAs(t, err, &temporary)
	var rejected *ContentRejectedError
	require.False(t, errors.As(err, &rejected))
}

func TestErrorCodeAndContentRejectedHelpers(t *testing.T) {
	require.Equal(t, ErrorCodeImageUnsafe, ErrorCode(`{"error_code":"image_unsafe"}`))
	require.True(t, IsContentRejectedBody(`{"error_code":"image_unsafe"}`))
	require.False(t, IsContentRejectedBody(`{"error_code":"timeout_error"}`))
	require.False(t, IsContentRejectedBody("not json"))
	require.False(t, IsContentRejectedCode(""))
}

func TestNotEntitledHelpers(t *testing.T) {
	require.True(t, IsNotEntitledCode(ErrorCodeModelNotEntitled))
	require.True(t, IsNotEntitledCode("USER_NOT_ENTITLED"))
	require.True(t, IsNotEntitledBody(`{"error_code":"model_not_entitled"}`))
	require.True(t, IsNotEntitledBody(`{"error_code":"user_not_entitled"}`))
	require.False(t, IsNotEntitledBody(`{"error_code":"image_unsafe"}`))
	require.False(t, IsNotEntitledCode(""))
	require.False(t, IsNotEntitledCode("taste_exhausted"))
}
