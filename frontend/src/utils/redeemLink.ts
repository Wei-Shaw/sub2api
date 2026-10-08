export interface RedeemLinkPayload {
  code: string
  auto: boolean
  cleanUrl: string
}

export function parseRedeemLink(href: string): RedeemLinkPayload {
  const url = new URL(href)
  const code = new URLSearchParams(url.hash.replace(/^#/, '')).get('code')?.trim() || ''
  const auto = url.searchParams.get('auto') === '1' && Boolean(code)

  url.searchParams.delete('auto')
  url.hash = ''

  return {
    code,
    auto,
    cleanUrl: `${url.pathname}${url.search}`,
  }
}
