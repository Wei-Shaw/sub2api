package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidProfileArn(t *testing.T) {
	t.Parallel()

	require.True(t, IsValidProfileArn("arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456"))
	require.True(t, IsValidProfileArn("  arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456  "),
		"两端空白应该被忽略")

	for _, bad := range []string{
		"",
		"not-an-arn",
		"arn:aws:s3:us-east-1:123456789012:profile/abcdef123456",          // 错的 service
		"arn:aws:codewhisperer:us-east-1:12345:profile/abcdef123456",      // account id 不是 12 位
		"arn:aws:codewhisperer:us-east-1:123456789012:profile/abc def123", // profile id 有非法字符
		"arn:aws:codewhisperer:us-east-1:123456789012:profile/short",      // profile id 长度不对
	} {
		require.False(t, IsValidProfileArn(bad), "expected invalid: %s", bad)
	}
}

func TestListProfilesHostFor(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://codewhisperer.us-east-1.amazonaws.com", ListProfilesHostFor("us-east-1"))
	require.Equal(t, "https://codewhisperer.us-east-1.amazonaws.com", ListProfilesHostFor(""),
		"空区域退回默认 us-east-1 host")
	require.Equal(t, "https://q.eu-central-1.amazonaws.com", ListProfilesHostFor("eu-central-1"),
		"非 us-east-1 区域走区域化的 Amazon Q host，不是 codewhisperer host")
}

func TestBuildListProfilesURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://codewhisperer.us-east-1.amazonaws.com/ListAvailableProfiles", BuildListProfilesURL("us-east-1"))
	require.Equal(t, "https://q.eu-central-1.amazonaws.com/ListAvailableProfiles", BuildListProfilesURL("eu-central-1"))
}

func TestBuildListProfilesRequestBody(t *testing.T) {
	t.Parallel()

	// 真实账号测试证实带 maxResults 字段（不管数字还是字符串）会被 AWS
	// 拒成 400 REQUEST_BODY_INVALID——首页请求体必须是空对象，不能带这个
	// 字段，即便这与参考实现 Kiro-Go 的做法不同。
	body, err := BuildListProfilesRequestBody("")
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(body), "首页请求体不应该带 maxResults，真实账号测试证实带了会被拒")

	body, err = BuildListProfilesRequestBody("token-123")
	require.NoError(t, err)
	require.JSONEq(t, `{"nextToken":"token-123"}`, string(body))
}

func TestParseListProfilesResponse(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"profiles": [
			{"arn": "arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456", "profileName": "default"},
			{"arn": "not-a-real-arn", "profileName": "garbage-should-be-dropped"}
		],
		"nextToken": "page-2"
	}`)

	parsed, err := ParseListProfilesResponse(raw)
	require.NoError(t, err)
	require.Len(t, parsed.Profiles, 1, "格式不对的 profile 必须被丢弃，不能让一条脏数据污染结果")
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456", parsed.Profiles[0].ARN)
	require.Equal(t, "default", parsed.Profiles[0].Name)
	require.Equal(t, "page-2", parsed.NextToken)
}

func TestParseListProfilesResponseEmpty(t *testing.T) {
	t.Parallel()

	parsed, err := ParseListProfilesResponse([]byte(`{}`))
	require.NoError(t, err)
	require.Empty(t, parsed.Profiles)
	require.Empty(t, parsed.NextToken)
}

func TestParseListProfilesResponseRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := ParseListProfilesResponse([]byte(`not json`))
	require.Error(t, err)
}
