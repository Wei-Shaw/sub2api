import { shallowRef } from 'vue'
import builtinCatalog from './platformCatalog.builtin.json'

/**
 * 平台清单：后端的平台清单（domain/platforms.go）加上多协议 API Key 供应商 profile。
 *
 * 平台选项、展示名、多协议账号表单的默认端点与分流规则都从这里读取。清单文件
 * platformCatalog.builtin.json 由后端测试 TestFrontendBuiltinPlatformCatalogInSync
 * 生成并校验与后端一致；前端随后端一同构建发布，无需运行时拉取。
 */

export type PlatformGateway = 'anthropic' | 'openai' | 'gemini' | 'antigravity'
export type ProviderRouting = 'by_inbound' | 'by_model'
export type ProviderNativeProtocol = 'chat_completions' | 'anthropic' | 'responses'

export interface ProviderProtocolRule {
  pattern: string
  /** 首选协议；入站协议不在 protocols 中时使用。 */
  protocol: string
  /** 模型支持的协议集合（首项即 protocol）；入站协议在其中时同协议直通。仅一个协议时省略。 */
  protocols?: string[]
}

export interface ProviderModeInfo {
  mode: string
  /** 以上游协议为键的官方默认基址；缺少某协议即该模式不提供该原生端点。 */
  base_urls: Partial<Record<string, string>>
  protocol_rules?: ProviderProtocolRule[]
}

export interface ProviderProfileInfo {
  default_mode: string
  routing: ProviderRouting
  responses_path?: string
  /** 上游 /models 为每个模型给出 supported_endpoints，按模型分流时未命中规则的模型据此选协议。 */
  model_catalog?: boolean
  /** 默认模式在前。 */
  modes: ProviderModeInfo[]
}

export interface PlatformSpec {
  id: string
  display_name: string
  gateway: PlatformGateway
  cn_provider: boolean
  litellm_provider?: string
  multi_protocol?: ProviderProfileInfo
}

export interface PlatformCatalog {
  platforms: PlatformSpec[]
  /** 组合分组逐平台回退的顺序（先命中者生效）。 */
  composite_precedence: string[]
}

export const BUILTIN_PLATFORM_CATALOG: PlatformCatalog = builtinCatalog as PlatformCatalog

const catalogState = shallowRef<PlatformCatalog>(BUILTIN_PLATFORM_CATALOG)

/** 替换当前清单（测试用于模拟新登记的平台）。 */
export function setPlatformCatalog(catalog: PlatformCatalog): void {
  catalogState.value = catalog
}

/** 恢复内置清单（测试用）。 */
export function resetPlatformCatalog(): void {
  catalogState.value = BUILTIN_PLATFORM_CATALOG
}

/** 全部具体平台，按展示顺序。 */
export function listPlatforms(): PlatformSpec[] {
  return catalogState.value.platforms
}

export function listPlatformIds(): string[] {
  return listPlatforms().map(spec => spec.id)
}

export function getPlatformSpec(platform: string | null | undefined): PlatformSpec | undefined {
  if (!platform) return undefined
  return catalogState.value.platforms.find(spec => spec.id === platform)
}

export function isKnownPlatform(platform: string | null | undefined): boolean {
  return !!getPlatformSpec(platform)
}

/** 平台展示名；未登记的值原样返回。 */
export function platformDisplayName(platform: string | null | undefined): string {
  return getPlatformSpec(platform)?.display_name ?? platform ?? ''
}

export function compositePrecedencePlatformIds(): string[] {
  return catalogState.value.composite_precedence
}

export function platformUsesGateway(platform: string | null | undefined, gateway: PlatformGateway): boolean {
  return getPlatformSpec(platform)?.gateway === gateway
}

export function getProviderProfile(platform: string | null | undefined): ProviderProfileInfo | undefined {
  return getPlatformSpec(platform)?.multi_protocol ?? undefined
}

/** 指定接入模式的端点；未知模式回落默认模式（与后端 ProviderProfile.Endpoints 一致）。 */
export function getProviderMode(platform: string | null | undefined, mode: string | null | undefined): ProviderModeInfo | undefined {
  const profile = getProviderProfile(platform)
  if (!profile) return undefined
  const trimmed = (mode ?? '').trim()
  return (
    profile.modes.find(item => item.mode === trimmed) ??
    profile.modes.find(item => item.mode === profile.default_mode) ??
    profile.modes[0]
  )
}
