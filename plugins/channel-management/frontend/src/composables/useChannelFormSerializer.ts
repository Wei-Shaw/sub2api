/** Channel form ↔ API 序列化 (T9 拆分).
 *  Form↔API pricing entry 单条映射在 utils/channelPricingMapper. */
import type {
  AccountStatsPricingRule,
  Channel,
  ChannelModelPricing,
  CreateChannelRequest,
  UpdateChannelRequest,
} from '../api/channels'
import {
  PLATFORM_ORDER,
  type AdminGroup,
  type ChannelFormState,
  type FormPricingRule,
  type GroupPlatform,
  type PlatformSection,
} from '../components/channels/types'
import { pricingAPIToForm, pricingFormToAPI } from '../utils/channelPricingMapper'

interface ChannelPayload {
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>>
  account_stats_pricing_rules: AccountStatsPricingRule[]
}

// ── form → API ──

export function formToAPI(form: ChannelFormState): ChannelPayload {
  const group_ids: number[] = []
  const model_pricing: ChannelModelPricing[] = []
  const model_mapping: Record<string, Record<string, string>> = {}
  for (const section of form.platforms) {
    if (!section.enabled) continue
    group_ids.push(...section.group_ids)
    if (Object.keys(section.model_mapping).length > 0) {
      model_mapping[section.platform] = { ...section.model_mapping }
    }
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      model_pricing.push(pricingFormToAPI(section.platform, entry))
    }
  }
  return {
    group_ids,
    model_pricing,
    model_mapping,
    account_stats_pricing_rules: accountStatsRulesToAPI(form),
  }
}

export function accountStatsRulesToAPI(
  form: ChannelFormState,
): AccountStatsPricingRule[] {
  const rules: AccountStatsPricingRule[] = []
  for (const section of form.platforms) {
    if (!section.enabled) continue
    for (const rule of section.account_stats_pricing_rules) {
      rules.push({
        name: rule.name,
        group_ids: rule.group_ids,
        account_ids: rule.account_ids,
        pricing: rule.pricing
          .filter((p) => p.models && p.models.length > 0)
          .map((p) => pricingFormToAPI(section.platform, p)),
      })
    }
  }
  return rules
}

// ── API → form ──

export function apiToForm(channel: Channel, groups: AdminGroup[]): PlatformSection[] {
  const map = buildGroupPlatformMap(groups)
  const active = collectActivePlatforms(channel, map)
  return PLATFORM_ORDER.filter((p) => active.has(p)).map((platform) => ({
    platform,
    enabled: true,
    collapsed: false,
    group_ids: (channel.group_ids || []).filter((gid) => map.get(gid) === platform),
    model_mapping: { ...((channel.model_mapping || {})[platform] || {}) },
    model_pricing: (channel.model_pricing || [])
      .filter((p) => (p.platform || 'anthropic') === platform)
      .map(pricingAPIToForm),
    account_stats_pricing_rules: [],
  }))
}

function buildGroupPlatformMap(groups: AdminGroup[]): Map<number, GroupPlatform> {
  const m = new Map<number, GroupPlatform>()
  for (const g of groups) m.set(g.id, g.platform)
  return m
}

function collectActivePlatforms(
  channel: Channel,
  map: Map<number, GroupPlatform>,
): Set<GroupPlatform> {
  const platforms = new Set<GroupPlatform>()
  for (const gid of channel.group_ids || []) {
    const p = map.get(gid)
    if (p) platforms.add(p)
  }
  for (const p of channel.model_pricing || []) {
    if (p.platform) platforms.add(p.platform as GroupPlatform)
  }
  for (const p of Object.keys(channel.model_mapping || {})) {
    if (PLATFORM_ORDER.includes(p as GroupPlatform)) platforms.add(p as GroupPlatform)
  }
  return platforms
}

/** 把扁平 rules 按 group_ids → platform 推断分发到对应 section. */
export function distributeRulesToPlatforms(
  apiRules: AccountStatsPricingRule[],
  sections: PlatformSection[],
  groups: AdminGroup[],
): void {
  const map = buildGroupPlatformMap(groups)
  for (const apiRule of apiRules) {
    const target = inferRulePlatform(apiRule, map)
    const section = target ? sections.find((s) => s.platform === target) : null
    if (!section) continue
    section.account_stats_pricing_rules.push({
      name: apiRule.name || '',
      group_ids: [...(apiRule.group_ids || [])],
      account_ids: [...(apiRule.account_ids || [])],
      pricing: (apiRule.pricing || []).map(pricingAPIToForm),
    } as FormPricingRule)
  }
}

function inferRulePlatform(
  apiRule: AccountStatsPricingRule,
  map: Map<number, GroupPlatform>,
): GroupPlatform | null {
  for (const gid of apiRule.group_ids || []) {
    const p = map.get(gid)
    if (p) return p
  }
  const fallback = apiRule.pricing?.[0]?.platform as GroupPlatform | undefined
  return fallback || null
}

// ── 高层 helper ──

export function createEmptyForm(): ChannelFormState {
  return {
    name: '',
    description: '',
    status: 'active',
    restrict_models: false,
    billing_model_source: 'channel_mapped',
    platforms: [],
    apply_pricing_to_account_stats: false,
  }
}

export function buildEditForm(channel: Channel, groups: AdminGroup[]): ChannelFormState {
  const sections = apiToForm(channel, groups)
  distributeRulesToPlatforms(channel.account_stats_pricing_rules || [], sections, groups)
  return {
    name: channel.name,
    description: channel.description || '',
    status: channel.status,
    restrict_models: channel.restrict_models || false,
    billing_model_source: channel.billing_model_source || 'channel_mapped',
    platforms: sections,
    apply_pricing_to_account_stats: channel.apply_pricing_to_account_stats || false,
  }
}

function basePayload(form: ChannelFormState) {
  const { group_ids, model_pricing, model_mapping, account_stats_pricing_rules } = formToAPI(form)
  return {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    group_ids,
    model_pricing,
    model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {},
    billing_model_source: form.billing_model_source,
    restrict_models: form.restrict_models,
    apply_pricing_to_account_stats: form.apply_pricing_to_account_stats,
    account_stats_pricing_rules,
  }
}

export function buildCreateRequest(form: ChannelFormState): CreateChannelRequest {
  return basePayload(form)
}

export function buildUpdateRequest(form: ChannelFormState): UpdateChannelRequest {
  return { ...basePayload(form), status: form.status }
}
