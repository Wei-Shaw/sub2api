/** Preserve exact decimal billing amounts instead of rounding through a JS Number. */
export function formatBillingAmount(value: string | undefined | null): string {
  if (!value || !/^-?\d+(\.\d+)?$/.test(value)) return value || '0.00'
  const [integer = '0', fraction = ''] = value.split('.')
  const decimals = fraction.replace(/0+$/, '').padEnd(2, '0')
  return `${integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${decimals}`
}

export function currentBillingMonth(): string {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

export function isBillingMonth(value: string): boolean {
  return /^20\d{2}-(0[1-9]|1[0-2])$/.test(value)
}

/** Billing adjustments may be negative or smaller than JS number precision. */
export function hasNonZeroBillingAmount(value?: string | null): boolean {
  return Boolean(value && /^-?\d+(\.\d+)?$/.test(value) && /[1-9]/.test(value))
}
