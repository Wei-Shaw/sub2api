/**
 * 分组上游订阅档位显示工具。
 * 优先使用调用方传入的配置（系统设置），否则回退内置种子 label。
 */

import type { GroupPlatform } from '@/types'
import type { GroupUpstreamPlanOption } from '@/api/admin/settings'

/** 与后端 DefaultGroupUpstreamPlansSeed 对齐的内置显示名 */
const DEFAULT_UPSTREAM_PLAN_LABELS: Record<string, Record<string, string>> = {
  openai: {
    free: 'Free',
    plus: 'Plus',
    team: 'Team',
    pro: 'Pro'
  },
  grok: {
    free: 'Grok Free',
    basic: 'Basic',
    supergrok: 'SuperGrok',
    supergrokheavy: 'SuperGrok Heavy'
  },
  antigravity: {
    'free-tier': 'Free',
    'g1-pro-tier': 'Pro',
    'g1-ultra-tier': 'Ultra'
  }
}

export type GroupUpstreamPlansMap = Record<string, GroupUpstreamPlanOption[] | undefined>

/**
 * 将 upstream_plan code 解析为展示文案。
 * @param platform 分组平台
 * @param code 档位 code
 * @param plansMap 可选：系统设置中的档位配置（优先）
 */
export function resolveUpstreamPlanLabel(
  platform: string | undefined | null,
  code: string | undefined | null,
  plansMap?: GroupUpstreamPlansMap | null
): string {
  const normalized = (code ?? '').trim()
  if (!normalized) return ''

  const platformKey = (platform ?? '').trim().toLowerCase()
  const codeKey = normalized.toLowerCase()

  if (plansMap && platformKey) {
    const opts = plansMap[platformKey]
    const found = opts?.find((o) => o.code?.toLowerCase() === codeKey)
    if (found?.label?.trim()) return found.label.trim()
  }

  const fallback = platformKey ? DEFAULT_UPSTREAM_PLAN_LABELS[platformKey]?.[codeKey] : undefined
  return fallback || normalized
}

/**
 * 在仅有 code、可能有 platform 时格式化展示用档位文案。
 */
export function formatGroupUpstreamPlan(
  platform: GroupPlatform | string | undefined | null,
  code: string | undefined | null,
  plansMap?: GroupUpstreamPlansMap | null
): string {
  return resolveUpstreamPlanLabel(platform, code, plansMap)
}
