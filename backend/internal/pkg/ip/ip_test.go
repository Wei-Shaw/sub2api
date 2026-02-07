//go:build unit

package ip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
REDACTED{
		// 私有 IPv4
		{"10.x 私有地址", "10.0.0.1", trueREDACTED,
		{"10.x 私有地址段末", "10.255.255.255", trueREDACTED,
		{"172.16.x 私有地址", "172.16.0.1", trueREDACTED,
		{"172.31.x 私有地址", "172.31.255.255", trueREDACTED,
		{"192.168.x 私有地址", "192.168.1.1", trueREDACTED,
		{"127.0.0.1 本地回环", "127.0.0.1", trueREDACTED,
		{"127.x 回环段", "127.255.255.255", trueREDACTED,

		// 公网 IPv4
		{"8.8.8.8 公网 DNS", "8.8.8.8", falseREDACTED,
		{"1.1.1.1 公网", "1.1.1.1", falseREDACTED,
		{"172.15.255.255 非私有", "172.15.255.255", falseREDACTED,
		{"172.32.0.0 非私有", "172.32.0.0", falseREDACTED,
		{"11.0.0.1 公网", "11.0.0.1", falseREDACTED,

		// IPv6
		{"::1 IPv6 回环", "::1", trueREDACTED,
		{"fc00:: IPv6 私有", "fc00::1", trueREDACTED,
		{"fd00:: IPv6 私有", "fd00::1", trueREDACTED,
		{"2001:db8::1 IPv6 公网", "2001:db8::1", falseREDACTED,

		// 无效输入
		{"空字符串", "", falseREDACTED,
		{"非法字符串", "not-an-ip", falseREDACTED,
		{"不完整 IP", "192.168", falseREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isPrivateIP(tc.ip)
			require.Equal(t, tc.expected, got, "isPrivateIP(%q)", tc.ip)
	REDACTED)
REDACTED
REDACTED
