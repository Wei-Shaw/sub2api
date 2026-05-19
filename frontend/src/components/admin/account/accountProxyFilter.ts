import type { Proxy } from '@/types'

interface BaseSelectOption {
  [key: string]: unknown
  value: string | number | boolean | null
  label: string
  disabled?: boolean
}

export interface AccountProxyFilterSelectOption extends BaseSelectOption {}

export interface AccountProxyFilterLabels {
  allLabel: string
  configuredLabel: string
  unconfiguredLabel: string
  separatorLabel: string
}

export function buildAccountProxyFilterOptions(
  proxies: Proxy[],
  labels: AccountProxyFilterLabels
): AccountProxyFilterSelectOption[] {
  const options: AccountProxyFilterSelectOption[] = [
    { value: '', label: labels.allLabel },
    { value: 'configured', label: labels.configuredLabel },
    { value: 'unconfigured', label: labels.unconfiguredLabel }
  ]

  if (proxies.length > 0) {
    options.push({
      value: '__separator__proxy',
      label: labels.separatorLabel,
      disabled: true
    })
  }

  proxies.forEach((proxy) => {
    options.push({
      value: `proxy:${proxy.id}`,
      label: proxy.name
    })
  })

  return options
}
