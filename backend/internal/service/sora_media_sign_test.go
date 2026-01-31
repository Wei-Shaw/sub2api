package service

import "testing"

func TestSoraMediaSignVerify(t *testing.T) {
	key := "test-key"
	path := "/tmp/abc.png"
	query := "a=1&b=2"
	expires := int64(1700000000)

	signature := SignSoraMediaURL(path, query, expires, key)
	if signature == "" {
		t.Fatal("签名为空")
REDACTED
	if !VerifySoraMediaURL(path, query, expires, signature, key) {
		t.Fatal("签名校验失败")
REDACTED
	if VerifySoraMediaURL(path, "a=1", expires, signature, key) {
		t.Fatal("签名参数不同仍然通过")
REDACTED
	if VerifySoraMediaURL(path, query, expires+1, signature, key) {
		t.Fatal("签名过期校验未失败")
REDACTED
REDACTED

func TestSoraMediaSignWithEmptyKey(t *testing.T) {
	signature := SignSoraMediaURL("/tmp/a.png", "a=1", 1, "")
	if signature != "" {
		t.Fatalf("空密钥不应生成签名")
REDACTED
	if VerifySoraMediaURL("/tmp/a.png", "a=1", 1, "sig", "") {
		t.Fatalf("空密钥不应通过校验")
REDACTED
REDACTED
