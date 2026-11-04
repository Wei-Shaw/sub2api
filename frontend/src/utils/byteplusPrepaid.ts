export function isBytePlusEndpoint(raw: unknown): boolean {
  if (typeof raw !== 'string') return false
  try {
    const url = new URL(raw.trim())
    return url.protocol === 'https:' && !url.username && !url.password && !url.search && !url.hash &&
      ['', '443'].includes(url.port) && url.pathname.replace(/\/+$/, '') === '/api/v3' &&
      ['ark.ap-southeast.bytepluses.com', 'ark.eu-west.bytepluses.com'].includes(url.hostname)
  } catch { return false }
}

/** Shift a decimal string, never a floating-point provider balance. */
export function packTokens(raw: string, scale: 0 | 3 | 6): string {
  const value = raw.trim()
  if (!/^(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?$/.test(value)) throw new Error('invalid token amount')
  const [whole, fraction = ''] = value.replace(/,/g, '').split('.')
  const digits = whole + fraction.padEnd(scale, '0')
  const point = whole.length + scale
  const integer = digits.slice(0, point).replace(/^0+(?=\d)/, '') || '0'
  const rest = digits.slice(point).replace(/0+$/, '')
  if (rest.length > 6) throw new Error('token balance precision exceeds 1/64 token')
  return rest ? `${integer}.${rest}` : integer
}
