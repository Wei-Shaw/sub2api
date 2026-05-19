package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDingTalkClient_ExchangeCodeForUserToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/v1.0/oauth2/userAccessToken", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"USER_TOKEN_X","expireIn":7200,"refreshToken":"R","corpId":"dingABC"REDACTED`))
REDACTED))
	defer server.Close()

	cli := &DingTalkClient{
		cfg: dingTalkClientConfig{
			ClientID: "k", ClientSecret: "s",
			TokenURL: server.URL + "/v1.0/oauth2/userAccessToken",
	REDACTED,
		httpClient: server.Client(),
REDACTED
	resp, err := cli.ExchangeCodeForUserToken(context.Background(), "AUTH_CODE")
REDACTED
	require.Equal(t, "USER_TOKEN_X", resp.AccessToken)
	require.Equal(t, "dingABC", resp.CorpID)
REDACTED

func TestDingTalkClient_GetUnionIdByUserToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "USER_TOKEN_X", r.Header.Get("x-acs-dingtalk-access-token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nick":"张三","unionId":"UID_AAA","openId":"OPEN","avatarUrl":"http://x"REDACTED`))
REDACTED))
	defer server.Close()

	cli := &DingTalkClient{
		cfg:        dingTalkClientConfig{UserInfoURL: server.URL + "/v1.0/contact/users/me"REDACTED,
		httpClient: server.Client(),
REDACTED
	unionID, nick, err := cli.GetUnionIdByUserToken(context.Background(), "USER_TOKEN_X")
REDACTED
	require.Equal(t, "UID_AAA", unionID)
	require.Equal(t, "张三", nick)
REDACTED

func TestDingTalkClient_GetAppToken_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_, _ = w.Write([]byte(`{"accessToken":"APP_TKN","expireIn":7200REDACTED`))
REDACTED))
	defer server.Close()

	cli := &DingTalkClient{
		cfg:        dingTalkClientConfig{ClientID: "k", ClientSecret: "s", TokenURL: server.URL + "/gettoken"REDACTED,
		httpClient: server.Client(),
REDACTED
	t1, err := cli.GetAppToken(context.Background())
REDACTED
	t2, err := cli.GetAppToken(context.Background())
REDACTED
	require.Equal(t, t1, t2)
	require.Equal(t, 1, callCount, "second call should hit cache")
REDACTED

func TestDingTalkClient_GetUserIdByUnionId_60011(t *testing.T) {
	appTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"accessToken":"APP_TKN","expireIn":7200REDACTED`))
REDACTED))
	defer appTokenServer.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":60011,"errmsg":"not in directory"REDACTED`))
REDACTED))
	defer server.Close()

	cli := &DingTalkClient{
		cfg:        dingTalkClientConfig{TokenURL: appTokenServer.URL + "/gettoken"REDACTED,
		httpClient: server.Client(),
REDACTED
	cli.appToken = "APP_TKN"
	cli.appTokenExp = time.Now().Add(time.Hour)
	cli.cfg.UserInfoURL = server.URL + "/v1.0/contact/users/byUnionId"

	_, err := cli.GetUserIdByUnionId(context.Background(), "UID_AAA")
REDACTED
	apiErr, ok := err.(*DingTalkAPIError)
	require.True(t, ok)
	require.Equal(t, "60011", apiErr.Code)
REDACTED

// TestDingTalkClient_GetDeptInfo_Success 验证 GetDeptInfo 正常情况返回部门信息。
func TestDingTalkClient_GetDeptInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"dept_id":42,"name":"AI数据","parent_id":1REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	cli := &DingTalkClient{
		cfg: dingTalkClientConfig{
			UserInfoURL: server.URL + "/stub", // 不含 /contact/users/me，走 test stub 路径
	REDACTED,
		httpClient: server.Client(),
REDACTED
	cli.appToken = "APP_TKN"
	cli.appTokenExp = time.Now().Add(time.Hour)

	info, err := cli.GetDeptInfo(context.Background(), 42)
REDACTED
	require.Equal(t, int64(42), info.DeptID)
	require.Equal(t, "AI数据", info.Name)
	require.Equal(t, int64(1), info.ParentID)
REDACTED

// TestDingTalkClient_GetDeptInfo_ErrCode60003 验证 errcode=60003（部门不存在）时返回错误。
func TestDingTalkClient_GetDeptInfo_ErrCode60003(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":60003,"errmsg":"dept not found"REDACTED`))
REDACTED))
	defer server.Close()

	cli := &DingTalkClient{
		cfg:        dingTalkClientConfig{UserInfoURL: server.URL + "/stub"REDACTED,
		httpClient: server.Client(),
REDACTED
	cli.appToken = "APP_TKN"
	cli.appTokenExp = time.Now().Add(time.Hour)

	_, err := cli.GetDeptInfo(context.Background(), 999)
REDACTED
	apiErr, ok := err.(*DingTalkAPIError)
	require.True(t, ok)
	require.Equal(t, "60003", apiErr.Code)
REDACTED
