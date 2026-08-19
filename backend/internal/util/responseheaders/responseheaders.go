package responseheaders

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// defaultAllowed 定义允许透传的响应头白名单
// 注意：以下头部由 Go HTTP 包自动处理，不应手动设置：
//   - content-length: 由 ResponseWriter 根据实际写入数据自动设置
//   - transfer-encoding: 由 HTTP 库根据需要自动添加/移除
//   - connection: 由 HTTP 库管理连接复用
var defaultAllowed = map[string]struct{REDACTED{
	"content-type":                   {REDACTED,
	"content-encoding":               {REDACTED,
	"content-language":               {REDACTED,
	"cache-control":                  {REDACTED,
	"etag":                           {REDACTED,
	"last-modified":                  {REDACTED,
	"expires":                        {REDACTED,
	"vary":                           {REDACTED,
	"date":                           {REDACTED,
	"x-request-id":                   {REDACTED,
	"x-ratelimit-limit-requests":     {REDACTED,
	"x-ratelimit-limit-tokens":       {REDACTED,
	"x-ratelimit-remaining-requests": {REDACTED,
	"x-ratelimit-remaining-tokens":   {REDACTED,
	"x-ratelimit-reset-requests":     {REDACTED,
	"x-ratelimit-reset-tokens":       {REDACTED,
	"retry-after":                    {REDACTED,
	"location":                       {REDACTED,
	"www-authenticate":               {REDACTED,
	// Codex uses this response header to avoid estimating reasoning tokens a
	// second time when upstream usage already includes them.
	"x-reasoning-included": {REDACTED,
REDACTED

// hopByHopHeaders 是跳过的 hop-by-hop 头部，这些头部由 HTTP 库自动处理
var hopByHopHeaders = map[string]struct{REDACTED{
	"content-length":    {REDACTED,
	"transfer-encoding": {REDACTED,
	"connection":        {REDACTED,
REDACTED

type CompiledHeaderFilter struct {
	allowed     map[string]struct{REDACTED
	forceRemove map[string]struct{REDACTED
REDACTED

var defaultCompiledHeaderFilter = CompileHeaderFilter(config.ResponseHeaderConfig{REDACTED)

func CompileHeaderFilter(cfg config.ResponseHeaderConfig) *CompiledHeaderFilter {
	allowed := make(map[string]struct{REDACTED, len(defaultAllowed)+len(cfg.AdditionalAllowed))
	for key := range defaultAllowed {
		allowed[key] = struct{REDACTED{REDACTED
REDACTED
	// 关闭时只使用默认白名单，additional/force_remove 不生效
	if cfg.Enabled {
		for _, key := range cfg.AdditionalAllowed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
		REDACTED
			allowed[normalized] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	forceRemove := map[string]struct{REDACTED{REDACTED
	if cfg.Enabled {
		forceRemove = make(map[string]struct{REDACTED, len(cfg.ForceRemove))
		for _, key := range cfg.ForceRemove {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
		REDACTED
			forceRemove[normalized] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	return &CompiledHeaderFilter{
		allowed:     allowed,
		forceRemove: forceRemove,
REDACTED
REDACTED

func FilterHeaders(src http.Header, filter *CompiledHeaderFilter) http.Header {
	if filter == nil {
		filter = defaultCompiledHeaderFilter
REDACTED

	filtered := make(http.Header, len(src))
	for key, values := range src {
		lower := strings.ToLower(key)
		if _, blocked := filter.forceRemove[lower]; blocked {
			continue
	REDACTED
		if _, ok := filter.allowed[lower]; !ok {
			continue
	REDACTED
		// 跳过 hop-by-hop 头部，这些由 HTTP 库自动处理
		if _, isHopByHop := hopByHopHeaders[lower]; isHopByHop {
			continue
	REDACTED
		for _, value := range values {
			filtered.Add(key, value)
	REDACTED
REDACTED
	return filtered
REDACTED

func WriteFilteredHeaders(dst http.Header, src http.Header, filter *CompiledHeaderFilter) {
	filtered := FilterHeaders(src, filter)
	for key, values := range filtered {
		for _, value := range values {
			dst.Add(key, value)
	REDACTED
REDACTED
REDACTED
