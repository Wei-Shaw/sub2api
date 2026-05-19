import type { SelectOption } from '@/types'

export interface AccountModelFilterOption extends SelectOption {
  kind?: 'group'
  platform?: string
}

export interface AccountModelFilterEntry {
  value: string
  label: string
}

export interface AccountModelFilterGroup {
  platform: string
  label: string
  models: AccountModelFilterEntry[]
}

export const ACCOUNT_MODEL_FILTER_LIMITED = '__limited__'
export const ACCOUNT_MODEL_FILTER_UNLIMITED = '__unlimited__'

interface AccountModelFilterLabels {
  allLabel?: string
  restrictedLabel?: string
  unrestrictedLabel?: string
}

export function buildAccountModelFilterOptions(
  groups: AccountModelFilterGroup[],
  platform: string,
  labels: AccountModelFilterLabels = {}
): AccountModelFilterOption[] {
  const normalizedPlatform = String(platform || '').trim()
  const allLabel = labels.allLabel || '全部模型'
  const restrictedLabel = labels.restrictedLabel || '有限制模型'
  const unrestrictedLabel = labels.unrestrictedLabel || '无限制模型'
  const baseOptions: AccountModelFilterOption[] = [
    { value: '', label: allLabel },
    { value: ACCOUNT_MODEL_FILTER_LIMITED, label: restrictedLabel },
    { value: ACCOUNT_MODEL_FILTER_UNLIMITED, label: unrestrictedLabel }
  ]

  if (!normalizedPlatform) {
    const options: AccountModelFilterOption[] = [...baseOptions]
    for (const group of groups) {
      if (!group.models.length) continue
      options.push({
        value: `__group__${group.platform}`,
        label: group.label,
        kind: 'group',
        disabled: true,
        platform: group.platform
      })
      for (const model of group.models) {
        options.push({
          value: model.value,
          label: model.label,
          platform: group.platform
        })
      }
    }
    return options
  }

  const matchedGroup = groups.find(group => group.platform === normalizedPlatform)
  if (!matchedGroup) {
    return [...baseOptions]
  }

  return [
    ...baseOptions,
    ...matchedGroup.models.map(model => ({
      value: model.value,
      label: model.label,
      platform: matchedGroup.platform
    }))
  ]
}

export function normalizeAccountModelFilterValue(
  groups: AccountModelFilterGroup[],
  platform: string,
  model: string
): string {
  const normalizedModel = String(model || '').trim()
  if (!normalizedModel) {
    return ''
  }

  const normalizedPlatform = String(platform || '').trim()
  if (!normalizedPlatform) {
    return normalizedModel
  }

  if (normalizedModel === ACCOUNT_MODEL_FILTER_LIMITED || normalizedModel === ACCOUNT_MODEL_FILTER_UNLIMITED) {
    return normalizedModel
  }

  const matchedGroup = groups.find(group => group.platform === normalizedPlatform)
  if (!matchedGroup) {
    return ''
  }

  return matchedGroup.models.some(item => item.value === normalizedModel) ? normalizedModel : ''
}
