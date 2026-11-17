import { watch } from 'vue'
import type { AccountPlatform, AccountType, Group, Proxy } from '@/types'

function latest<T extends { id: number; created_at: string }>(rows: T[]) {
  return rows.reduce<T | undefined>((selected, row) => {
    const date = Date.parse(row.created_at) || 0
    const previous = selected ? Date.parse(selected.created_at) || 0 : -1
    return !selected || date > previous || (date === previous && row.id > selected.id) ? row : selected
  }, undefined)
}

export function latestUsableProxy(proxies: Proxy[], now = Date.now()) {
  return latest(proxies.filter(proxy => proxy.status === 'active'
    && (!proxy.expires_at || Date.parse(proxy.expires_at) > now)
    && proxy.latency_status !== 'failed' && proxy.quality_status !== 'failed'))?.id ?? null
}

export function latestUsableGroup(groups: Group[], platform: AccountPlatform, type: AccountType, mixed: boolean, simple: boolean) {
  const platforms = platform === 'antigravity' && mixed ? ['antigravity', 'anthropic', 'gemini'] : [platform]
  return latest(groups.filter(group => group.status === 'active'
    && (platforms.includes(group.platform as AccountPlatform) || (!simple && group.platform === 'composite'))
    && (!group.require_oauth_only || type === 'oauth')))?.id
}

// Fork-only creation defaults. Async option loading may fill untouched fields;
// explicit selections (including direct connection / no groups) stay in place.
export function useAccountCreationDefaults(options: {
  show: () => boolean
  proxies: () => Proxy[]
  groups: () => Group[]
  mixed: () => boolean
  simple: () => boolean
  form: { platform: AccountPlatform; type: AccountType; proxy_id: number | null; group_ids: number[] }
}) {
  let proxyTouched = false
  let groupsTouched = false
  watch([options.show, () => options.form.platform, () => options.form.type, options.proxies, options.groups, options.mixed, options.simple],
    ([show, platform], old) => {
      if (!show) return
      if (!old?.[0]) { proxyTouched = false; groupsTouched = false }
      else if (old[1] !== platform) groupsTouched = false
      if (!proxyTouched) options.form.proxy_id = latestUsableProxy(options.proxies())
      if (!groupsTouched) {
        const group = latestUsableGroup(options.groups(), options.form.platform, options.form.type, options.mixed(), options.simple())
        options.form.group_ids = group == null ? [] : [group]
      }
    }, { immediate: true, deep: true })
  return { proxyChanged: () => { proxyTouched = true }, groupsChanged: () => { groupsTouched = true } }
}
