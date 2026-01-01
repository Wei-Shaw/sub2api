package geminicli

import "testing"

func TestDriveStorageInfo(t *testing.T) {
	// 测试 DriveStorageInfo 结构体
	info := &DriveStorageInfo{
		Limit: 100 * 1024 * 1024 * 1024, // 100GB
		Usage: 50 * 1024 * 1024 * 1024,  // 50GB
REDACTED

	if info.Limit != 100*1024*1024*1024 {
		t.Errorf("Expected limit 100GB, got %d", info.Limit)
REDACTED
	if info.Usage != 50*1024*1024*1024 {
		t.Errorf("Expected usage 50GB, got %d", info.Usage)
REDACTED
REDACTED
