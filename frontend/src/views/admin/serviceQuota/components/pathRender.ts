/**
 * pathRender.ts — service-quota path 显示的共享 helpers。
 *
 * 由 PathChevron.vue（监控页 RuntimeTable）与 ConfigView.vue（规则配置页）共同使用，
 * 确保两处对"全部 nil 视为通配 / 平台 chip / channel-group-account-model 链"的渲染规则
 * 完全一致（复用原则）。
 *
 * 故意只 export 纯函数 + 类型，不暴露 Vue 渲染逻辑——具体 chip 样式仍由组件控制。
 */

export interface PathSummary {
  platform?: string | null
  channel_id?: number | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
}

/**
 * isPathAllStar 判断 path 5 个字段是否全部未限定。
 *
 * 与 service_quota_paths 的唯一索引语义一致：NULL/0/'' 折叠成"无限制"。
 * summary == null/undefined 也视为全部 nil。
 */
export function isPathAllStar(summary: PathSummary | null | undefined): boolean {
  if (!summary) return true
  return (
    !summary.platform &&
    (summary.channel_id == null || summary.channel_id === 0) &&
    (summary.group_id == null || summary.group_id === 0) &&
    (summary.account_id == null || summary.account_id === 0) &&
    !summary.model_pattern
  )
}

export interface PathSegment {
  kind: 'channel' | 'group' | 'account' | 'model'
  /** channel/group/account 段：实体 id；为 null 表示该维度未限定（渲染灰色 *）。
   *  model 段恒为 null，字面值在 literal 字段。 */
  entityId: number | null
  /** model 段的字面值；其他段恒为空字符串，由 PathChevron 通过 useEntityName 解析。 */
  literal: string
}

/**
 * pathTailSegments 返回 path 平台之后的 4 段（channel / group / account / model_pattern）。
 * 不含 platform：平台段单独走 chip 渲染逻辑。
 *
 * 故意不在这里把 channel_id 拼成 "ch#42" —— PathChevron 会用 useEntityName 异步
 * 解析成真实名称；本函数只透传 id 让上层决定怎么渲染。
 */
export function pathTailSegments(summary: PathSummary | null | undefined): PathSegment[] {
  return [
    { kind: 'channel', entityId: summary?.channel_id ?? null, literal: '' },
    { kind: 'group', entityId: summary?.group_id ?? null, literal: '' },
    { kind: 'account', entityId: summary?.account_id ?? null, literal: '' },
    { kind: 'model', entityId: null, literal: summary?.model_pattern || '' },
  ]
}

/**
 * formatPlatformLabel 通过全局 i18n 渲染平台标签。
 *
 * 用全局 i18n 而非 useI18n() 是为了让函数在 setup script 与单元测试里都能直接
 * 调用，而不需要每个调用方都自行注入 t。下游 admin.groups.platforms.<p> 已包含
 * anthropic / openai / gemini / antigravity 四个平台值。
 */
import { i18n } from '@/i18n'

export function formatPlatformLabel(platform: string): string {
  if (!platform) return ''
  const t = i18n.global.t as (key: string, fallback?: string) => string
  return t(`admin.groups.platforms.${platform}`, platform)
}
