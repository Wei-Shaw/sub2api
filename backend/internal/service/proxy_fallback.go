package service

import "time"

// ResolveProxyFallbackTarget 计算一个过期代理 start 应把账号改投到哪里。
// 返回 (targetID, change)：
//   - change=false：不改动账号（mode=none，或链路成环/无解的兜底）
//   - change=true, targetID=nil：改投为直连
//   - change=true, targetID!=nil：改投到该备用代理 id
//
// byID 是「全部代理」的快照（id -> Proxy），now 为判定基准时间。
func ResolveProxyFallbackTarget(start Proxy, byID map[int64]Proxy, now time.Time) (*int64, bool) {
	switch start.FallbackMode {
	case FallbackModeDirect:
		return nil, true
	case FallbackModeProxy:
		visited := map[int64]struct{REDACTED{start.ID: {REDACTEDREDACTED
		curID := start.BackupProxyID
		for {
			if curID == nil {
				return nil, false
		REDACTED
			if _, seen := visited[*curID]; seen {
				return nil, false
		REDACTED
			p, ok := byID[*curID]
			if !ok {
				return nil, false
		REDACTED
			if !(&p).IsExpired(now) && p.Status != StatusExpired {
				id := p.ID
				return &id, true
		REDACTED
			visited[*curID] = struct{REDACTED{REDACTED
			switch p.FallbackMode {
			case FallbackModeDirect:
				return nil, true
			case FallbackModeProxy:
				curID = p.BackupProxyID
			default:
				return nil, false
		REDACTED
	REDACTED
	default:
		return nil, false
REDACTED
REDACTED
