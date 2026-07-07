import type { AdminGroup } from '@/types'

export interface ApiKeyGroupFilterOption {
  value: number | null
  label: string
  kind?: 'group'
  disabled?: boolean
}

export interface ApiKeyGroupFilterLabels {
  all: string
  exclusive: string
  public: string
  subscription: string
  disabled: string
}

// Sentinel values for section-header rows (negative so they never collide with real group ids).
// Select.vue generates :key from `${typeof value}:${String(value ?? '')}` — using distinct
// numbers avoids the duplicate "object:" keys that null-valued headers would produce.
const HEADER_EXCLUSIVE = -1
const HEADER_PUBLIC = -2
const HEADER_SUBSCRIPTION = -3
const HEADER_DISABLED = -4

/**
 * Build options for the "API Key group" filter Select.
 *
 * Active groups are partitioned into exclusive / public / subscription sections,
 * each preceded by a disabled section-header row. Disabled groups are collected
 * into a final "disabled" section so admins can filter users whose keys are still
 * bound to a now-disabled group. Empty sections render no header. The leading
 * "all" option (value null) clears the filter.
 *
 * Section-header rows use negative sentinel values (-1 … -4) instead of null so
 * that Vue's v-for :key expression produces distinct strings and avoids duplicate-
 * key warnings (fixes F2).
 */
export function buildApiKeyGroupFilterOptions(
  groups: AdminGroup[],
  labels: ApiKeyGroupFilterLabels
): ApiKeyGroupFilterOption[] {
  const exclusive: ApiKeyGroupFilterOption[] = []
  const publicGroups: ApiKeyGroupFilterOption[] = []
  const subscription: ApiKeyGroupFilterOption[] = []
  const disabledGroups: ApiKeyGroupFilterOption[] = []

  for (const grp of groups) {
    const item: ApiKeyGroupFilterOption = { value: grp.id, label: grp.name }
    if (grp.status !== 'active') {
      disabledGroups.push(item)
    } else if (grp.subscription_type === 'subscription') {
      subscription.push(item)
    } else if (grp.is_exclusive) {
      exclusive.push(item)
    } else {
      publicGroups.push(item)
    }
  }

  const options: ApiKeyGroupFilterOption[] = [{ value: null, label: labels.all }]

  const sections: Array<[string, number, ApiKeyGroupFilterOption[]]> = [
    [labels.exclusive,    HEADER_EXCLUSIVE,    exclusive],
    [labels.public,       HEADER_PUBLIC,       publicGroups],
    [labels.subscription, HEADER_SUBSCRIPTION, subscription],
    [labels.disabled,     HEADER_DISABLED,     disabledGroups],
  ]
  for (const [label, headerValue, items] of sections) {
    if (items.length === 0) continue
    options.push({ value: headerValue, label, kind: 'group', disabled: true })
    options.push(...items)
  }
  return options
}
