/**
 * Channel form 校验 (T9 拆分).
 *
 * 校验逻辑全部用纯函数表达, 输入 form state, 输出"通过"或"具体错误"对象.
 * 错误以 i18n key + params 返回, 让调用方自由选择渲染时机和方式 (显示在
 * dialog 顶部 / toast / inline). 复用了 components/types.ts 的 findModel
 * Conflict / validateIntervals (单点公式, 不重写).
 */
import {
  findModelConflict,
  validateIntervals,
} from '../components/types'
import type { ChannelFormState, PlatformSection } from '../components/channels/types'

/** 校验失败错误描述. */
export interface ValidationError {
  /** i18n key, 调用方用 t(key, params) 渲染. */
  key: string
  /** i18n 插值参数 (必要时). */
  params?: Record<string, string | number>
  /** 出错的平台 (用于 dialog 自动切换 tab). 为 null 表示无关联平台. */
  platform: string | null
}

export type ValidationResult =
  | { ok: true }
  | { ok: false; error: ValidationError }

/** Pipeline-style 校验入口, 按顺序检查每条规则, 任一失败即返回. */
export function validateChannelForm(form: ChannelFormState): ValidationResult {
  if (!form.name.trim()) {
    return fail('admin.channels.nameRequired', null)
  }

  const enabled = form.platforms.filter((s) => s.enabled)
  for (const checker of CHECKERS) {
    for (const section of enabled) {
      const result = checker(section)
      if (!result.ok) return result
    }
  }
  return { ok: true }
}

/** 单条规则校验签名: 拿到 section, 返回结果. */
type SectionChecker = (section: PlatformSection) => ValidationResult

const CHECKERS: SectionChecker[] = [
  checkGroups,
  checkPricingHasModels,
  checkPricingConflict,
  checkMappingConflict,
  checkPerRequestPriceRequired,
  checkIntervalsValid,
]

function checkGroups(section: PlatformSection): ValidationResult {
  if (section.group_ids.length === 0) {
    return fail(
      'admin.channels.noGroupsSelected',
      section.platform,
      { platform: platformLabelKey(section.platform) },
    )
  }
  return { ok: true }
}

function checkPricingHasModels(section: PlatformSection): ValidationResult {
  for (const entry of section.model_pricing) {
    if (entry.models.length === 0) {
      return fail(
        'admin.channels.emptyModelsInPricing',
        section.platform,
        { platform: platformLabelKey(section.platform) },
      )
    }
  }
  return { ok: true }
}

function checkPricingConflict(section: PlatformSection): ValidationResult {
  const allModels: string[] = []
  for (const entry of section.model_pricing) {
    allModels.push(...entry.models)
  }
  const conflict = findModelConflict(allModels)
  if (conflict) {
    return fail('admin.channels.modelConflict', section.platform, {
      model1: conflict[0],
      model2: conflict[1],
    })
  }
  return { ok: true }
}

function checkMappingConflict(section: PlatformSection): ValidationResult {
  const keys = Object.keys(section.model_mapping)
  if (keys.length === 0) return { ok: true }
  const conflict = findModelConflict(keys)
  if (conflict) {
    return fail('admin.channels.mappingConflict', section.platform, {
      model1: conflict[0],
      model2: conflict[1],
    })
  }
  return { ok: true }
}

function checkPerRequestPriceRequired(
  section: PlatformSection,
): ValidationResult {
  for (const entry of section.model_pricing) {
    if (entry.models.length === 0) continue
    if (entry.billing_mode !== 'per_request' && entry.billing_mode !== 'image') {
      continue
    }
    const noPrice =
      entry.per_request_price == null || entry.per_request_price === ''
    const noTiers = !entry.intervals || entry.intervals.length === 0
    if (noPrice && noTiers) {
      return fail(
        'admin.channels.form.perRequestPriceRequired',
        section.platform,
      )
    }
  }
  return { ok: true }
}

function checkIntervalsValid(section: PlatformSection): ValidationResult {
  for (const entry of section.model_pricing) {
    if (!entry.intervals || entry.intervals.length === 0) continue
    const intervalErr = validateIntervals(entry.intervals)
    if (!intervalErr) continue
    return fail('admin.channels.intervalError', section.platform, {
      platform: platformLabelKey(section.platform),
      model: entry.models.join(', '),
      error: intervalErr,
    })
  }
  return { ok: true }
}

function fail(
  key: string,
  platform: string | null,
  params?: Record<string, string | number>,
): { ok: false; error: ValidationError } {
  return { ok: false, error: { key, platform, params } }
}

/** 平台 i18n label key (host 提供 admin.groups.platforms.<id> 翻译). */
function platformLabelKey(platform: string): string {
  return platform // 给到 view 层时再 t('admin.groups.platforms.' + platform).
}
