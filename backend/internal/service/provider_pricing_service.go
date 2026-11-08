package service

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// providerPricingDefaultRaw 是编译进二进制的默认价格数据。
//
// 为什么是 embed 而不是读文件：本仓库的正式镜像是多阶段构建，运行层只拷贝
// 编译产物与 resources 目录，源码树里的任何数据文件都不会进镜像。如果默认走文件，
// 容器里就会永远读不到价格文件，第三方价格接口会直接 503。
// 编进二进制后，容器 / 裸机二进制 / 源码运行三种部署方式行为一致。
//
//go:embed provider_pricing_default.json
var providerPricingDefaultRaw []byte

// 公开价格接口的两种失败语义，调用方据此区分 503（数据不可用）与 500（数据非法）。
var (
	// ErrProviderPricingUnavailable 价格数据不可用，对外映射为 HTTP 503。
	ErrProviderPricingUnavailable = errors.New("pricing unavailable")
	// ErrProviderPricingInvalid 价格数据存在但 JSON 解析失败，对外映射为 HTTP 500。
	ErrProviderPricingInvalid = errors.New("invalid pricing configuration")
)

// ProviderPricingDocument 是公开价格文件（Hvoy schema 1.0）的顶层结构。
type ProviderPricingDocument struct {
	// SchemaVersion 价格 schema 版本，当前为 1.0
	SchemaVersion string `json:"schema_version"`
	// Currency 计价币种（如 CNY）
	Currency string `json:"currency"`
	// PriceUnit 计价单位（如 per_1m_tokens）
	PriceUnit string `json:"price_unit"`
	// SiteName 站点名称
	SiteName string `json:"site_name"`
	// SiteDomain 站点域名
	SiteDomain string `json:"site_domain"`
	// Models 模型价格条目列表
	Models []ProviderPricingModel `json:"models"`
}

// ProviderPricingModel 是单个模型的价格条目。
//
// CacheCreatePrice 与 CacheCreatePrice1H 刻意不写 omitempty：第三方契约要求
// 这两个字段永远出现在响应里，缺值时以 null 呈现。其余字段沿用旧实现的
// omitempty 语义，避免改变字节形态。
type ProviderPricingModel struct {
	// ModelName 模型名
	ModelName string `json:"model_name"`
	// GroupName 所属分组名
	GroupName string `json:"group_name"`
	// InputPrice 输入价格
	InputPrice *float64 `json:"input_price,omitempty"`
	// OutputPrice 输出价格
	OutputPrice *float64 `json:"output_price,omitempty"`
	// CacheInputPrice 缓存读取价格
	CacheInputPrice *float64 `json:"cache_input_price,omitempty"`
	// CacheCreatePrice 缓存写入价格（允许为 null）
	CacheCreatePrice *float64 `json:"cache_create_price"`
	// CacheCreatePrice1H 1 小时缓存写入价格（允许为 null）
	CacheCreatePrice1H *float64 `json:"cache_create_price_1h"`
	// Enabled 是否对外启用
	Enabled bool `json:"enabled"`
	// Note 备注
	Note string `json:"note,omitempty"`
}

// ProviderPricingService 提供对外公开的价格数据。
//
// 默认使用编译进二进制的数据；若配置了外部文件路径，则每请求重新读盘并优先采用它，
// 这样运维改价无需重启（这是第三方已经依赖的既有行为）。
// 外部文件读不到或格式非法时回落到内置数据，而不是把 503 抛给第三方调用方——
// 一处配置失误不应该让外部集成整体失效，日志里看得见就够了。
type ProviderPricingService struct {
	// filePath 可选的价格文件覆盖路径，为空表示只用内置数据
	filePath string
}

// NewProviderPricingService 构造公开价格服务；filePath 为空表示只用内置数据。
func NewProviderPricingService(filePath string) *ProviderPricingService {
	return &ProviderPricingService{filePath: filePath}
}

// ProvideProviderPricingService 由依赖注入构造公开价格服务。
//
// 需要一个适配函数：wire 无法注入裸 string，路径必须从 Config 里取。
func ProvideProviderPricingService(cfg *config.Config) *ProviderPricingService {
	filePath := ""
	if cfg != nil {
		filePath = cfg.Pricing.ProviderPricingFile
	}
	return NewProviderPricingService(filePath)
}

// Load 返回价格文档快照。
//
// 内置数据在编译期已确定，因此正常情况下不会失败；
// 返回错误只可能来自内置数据本身损坏，属于不该发生的构建事故。
func (s *ProviderPricingService) Load() (*ProviderPricingDocument, error) {
	raw := providerPricingDefaultRaw

	if s.filePath != "" {
		fileRaw, err := os.ReadFile(s.filePath)
		switch {
		case err != nil:
			logger.LegacyPrintf("service.provider_pricing",
				"read_pricing_file_failed: path=%s err=%v action=use_embedded", s.filePath, err)
		case !json.Valid(fileRaw):
			logger.LegacyPrintf("service.provider_pricing",
				"parse_pricing_file_failed: path=%s action=use_embedded", s.filePath)
		default:
			raw = fileRaw
		}
	}

	var doc ProviderPricingDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		logger.LegacyPrintf("service.provider_pricing", "parse_embedded_pricing_failed: err=%v", err)
		return nil, ErrProviderPricingInvalid
	}
	return &doc, nil
}
