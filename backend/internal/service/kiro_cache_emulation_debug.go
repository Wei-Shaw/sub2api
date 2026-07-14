package service

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 本文件为 Kiro 缓存模拟（cache emulation）匹配过程提供 debug 级排障日志。
//
// 目的：多机/线上环境经常出现「缓存没命中」或「命中率偏低」，但从计费结果反推原因很困难。
// 这里在缓存匹配链路上打点，输出到 request-scoped logger（logger.FromContext(ctx)，即 req_log，
// 已带 request_id / client_request_id），把「缓存 key、TTL、候选指纹、命中位置、命中/新建 token、
// 命中率」等关键信息落到一条 debug 日志里，便于对比两次请求为什么指纹不同、为什么没命中。
//
// 全部走 logger.L().Check(zap.DebugLevel)：debug 未开启时零成本，不影响热路径。

// kiroCacheDebugCandidate 记录一个被探测的候选断点（缓存 key 下的前缀指纹）。
type kiroCacheDebugCandidate struct {
	fingerprint      string        // 前缀指纹（sha256 hex）
	cumulativeTokens int           // 该断点累计输入 token 数
	ttl              time.Duration // 该断点声明的缓存 TTL
}

// kiroCacheDebug 汇总一次缓存匹配的中间状态，供 debug 日志输出。
// 由 compute 路径按需填充（仅在 debug 开启时创建，nil 表示不采集）。
type kiroCacheDebug struct {
	backend       string                    // 缓存后端："redis" | "memory"
	matched       bool                      // 是否命中任一候选
	matchedIndex  int                       // 命中的候选序号（1-based，newest-first）；0 表示未命中
	matchedFP     string                    // 命中的前缀指纹
	matchedTokens int                       // 命中断点对应的累计 token（即 cache_read 基数，未乘倍率）
	candidates    []kiroCacheDebugCandidate // 本次探测的候选列表（newest-first，受 lookback 上限约束）
	probeErr      error                     // Redis 探测错误（若有），命中会被当作 miss
}

// recordCandidates 记录本次探测的候选断点（newest-first）。dbg 为 nil 时安全 no-op。
func (d *kiroCacheDebug) recordCandidates(fingerprints []string, breakpoints []kiroResolvedBreakpoint) {
	if d == nil {
		return
	}
	d.candidates = make([]kiroCacheDebugCandidate, 0, len(fingerprints))
	for i := range fingerprints {
		cand := kiroCacheDebugCandidate{fingerprint: fingerprints[i]}
		if i < len(breakpoints) {
			cand.cumulativeTokens = breakpoints[i].cumulativeTokens
			cand.ttl = breakpoints[i].ttl
		}
		d.candidates = append(d.candidates, cand)
	}
}

// recordMatch 记录命中的候选（1-based index）。dbg 为 nil 时安全 no-op。
func (d *kiroCacheDebug) recordMatch(index int, fingerprint string, matchedTokens int) {
	if d == nil {
		return
	}
	d.matched = true
	d.matchedIndex = index
	d.matchedFP = fingerprint
	d.matchedTokens = matchedTokens
}

// kiroCacheDebugEnabled 报告当前是否需要采集缓存匹配 debug 信息。
// 与实际写日志共用同一个 debug 级别开关，保证「未开启即零采集、零日志」。
func kiroCacheDebugEnabled() bool {
	return logger.L().Check(zap.DebugLevel, "kiro.cache_emulation") != nil
}

// logKiroCacheDecision 把一次缓存匹配的完整结果写入 request-scoped debug 日志。
//
// reason 用于快速分类：hit / miss_no_live_fingerprint / disabled / profile_unbuildable /
// no_credential_key / no_cacheable_breakpoint 等。result 为「未乘计费倍率」的原始 token 拆分，
// 便于与实际断点 token 对齐分析；scaled 为乘倍率后的最终计费值。
func logKiroCacheDecision(
	ctx context.Context,
	reason string,
	account *Account,
	model string,
	cacheKey uint64,
	profile *kiroCacheProfile,
	dbg *kiroCacheDebug,
	rawResult *kiroCacheEmulationUsage,
	scaled *kiroCacheEmulationUsage,
	ratio float64,
) {
	reqLog := logger.FromContext(ctx)
	checked := reqLog.Check(zap.DebugLevel, "kiro.cache_emulation")
	if checked == nil {
		return
	}

	fields := make([]zap.Field, 0, 24)
	fields = append(fields,
		zap.String("component", "kiro.cache_emulation"),
		zap.String("reason", reason),
		// 缓存 key 用十六进制便于跨请求肉眼比对（同凭证应稳定一致）。
		zap.String("cache_key", strconv.FormatUint(cacheKey, 16)),
		zap.String("model", model),
	)
	if account != nil {
		fields = append(fields, zap.Int64("account_id", account.ID))
	}

	if profile != nil {
		breakpoints := profile.cacheableBreakpoints()
		fields = append(fields,
			zap.Int("total_input_tokens", profile.totalInputTokens),
			zap.Int("min_cacheable_tokens", profile.minCacheable),
			zap.Int("block_count", len(profile.blocks)),
			zap.Int("breakpoint_count", len(profile.breakpoints)),
			zap.Int("cacheable_breakpoint_count", len(breakpoints)),
		)
		if last := profile.lastCacheableBreakpoint(); last != nil {
			fields = append(fields,
				zap.Int("last_breakpoint_tokens", min(last.cumulativeTokens, profile.totalInputTokens)),
				zap.String("last_breakpoint_ttl", last.ttl.String()),
			)
		}
	}

	if dbg != nil {
		fields = append(fields,
			zap.String("backend", dbg.backend),
			zap.Bool("matched", dbg.matched),
			zap.Int("matched_index", dbg.matchedIndex),
			zap.Int("candidate_count", len(dbg.candidates)),
		)
		if dbg.matched {
			fields = append(fields,
				zap.String("matched_fingerprint", dbg.matchedFP),
				zap.Int("matched_tokens", dbg.matchedTokens),
			)
		}
		if dbg.probeErr != nil {
			fields = append(fields, zap.NamedError("probe_error", dbg.probeErr))
		}
		if len(dbg.candidates) > 0 {
			fields = append(fields, zap.Array("candidates", kiroCacheDebugCandidates(dbg.candidates)))
		}
	}

	if rawResult != nil {
		hitRatio := 0.0
		if profile != nil && profile.totalInputTokens > 0 {
			hitRatio = float64(rawResult.CacheReadInputTokens) / float64(profile.totalInputTokens)
		}
		fields = append(fields,
			zap.Int("raw_cache_read_tokens", rawResult.CacheReadInputTokens),
			zap.Int("raw_cache_creation_tokens", rawResult.CacheCreationInputTokens),
			zap.Int("raw_cache_creation_5m_tokens", rawResult.CacheCreation5mInputTokens),
			zap.Int("raw_cache_creation_1h_tokens", rawResult.CacheCreation1hInputTokens),
			zap.Float64("hit_ratio", hitRatio),
		)
	}

	fields = append(fields, zap.Float64("billing_ratio", ratio))
	if scaled != nil {
		fields = append(fields,
			zap.Int("cache_read_tokens", scaled.CacheReadInputTokens),
			zap.Int("cache_creation_tokens", scaled.CacheCreationInputTokens),
			zap.Int("cache_creation_5m_tokens", scaled.CacheCreation5mInputTokens),
			zap.Int("cache_creation_1h_tokens", scaled.CacheCreation1hInputTokens),
			zap.Int("input_tokens", scaled.InputTokens),
		)
	}

	checked.Write(fields...)
}

// kiroCacheDebugCandidates 让候选列表以结构化数组写入 zap 日志。
type kiroCacheDebugCandidates []kiroCacheDebugCandidate

func (c kiroCacheDebugCandidates) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for i := range c {
		cand := c[i]
		if err := enc.AppendObject(zapObjectFunc(func(oe zapcore.ObjectEncoder) error {
			oe.AddString("fingerprint", cand.fingerprint)
			oe.AddInt("cumulative_tokens", cand.cumulativeTokens)
			oe.AddString("ttl", cand.ttl.String())
			return nil
		})); err != nil {
			return err
		}
	}
	return nil
}

// zapObjectFunc 适配一个闭包到 zapcore.ObjectMarshaler，避免为候选项单独定义类型。
type zapObjectFunc func(zapcore.ObjectEncoder) error

func (f zapObjectFunc) MarshalLogObject(enc zapcore.ObjectEncoder) error { return f(enc) }
