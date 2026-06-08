/**
 * Channel form / dialog 共享类型 (T9 拆分).
 *
 * 这些类型原先内联在 ChannelsView.vue 中。拆分后被 ChannelDialog
 * / ChannelPlatformTab / AccountStatsRulesEditor / useChannelFormSerializer
 * / useChannelValidation 共同引用，作为单点真相 (复用原则).
 *
 * 命名约定:
 *   - GroupPlatform — host 业务字符串 (anthropic / openai / gemini / antigravity / 自定义)
 *   - PlatformSection — dialog 内一个平台 tab 的 form state
 *   - FormPricingRule — account stats 规则的 form 形态 (per-platform)
 *   - ChannelFormState — 整个对话框的 form state
 */
import type { PricingFormEntry } from '../types'

/** 平台标识 (业务字符串, 不限于 SDK Platform 枚举). */
export type GroupPlatform = string

/** Host 端 admin group 形态 (本 plugin 直接通过 axios 拉取 /admin/groups/all). */
export interface AdminGroup {
  id: number
  name: string
  platform: GroupPlatform
  rate_multiplier?: number
  account_count?: number
  [key: string]: unknown
}

/** 单个 account stats 规则的 form state (per-platform 内). */
export interface FormPricingRule {
  name: string
  group_ids: number[]
  account_ids: number[]
  pricing: PricingFormEntry[]
}

/** 一个平台 tab 内的 form state. */
export interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
  account_stats_pricing_rules: FormPricingRule[]
}

/** 对话框最外层 form state (基础设置 + 各平台 section). */
export interface ChannelFormState {
  name: string
  description: string
  status: string
  restrict_models: boolean
  billing_model_source: string
  platforms: PlatformSection[]
  apply_pricing_to_account_stats: boolean
}

/** 平台展示顺序 (作为 platform tab / picker 排序). */
export const PLATFORM_ORDER: GroupPlatform[] = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
]
