/** Set an expiry from local calendar months, clamping month-end dates. */
export function getAccountExpiryTimestamp(months: number, now = new Date()): number {
  const expiry = new Date(now.getTime())
  const lastDay = new Date(expiry.getFullYear(), expiry.getMonth() + months + 1, 0).getDate()
  expiry.setMonth(expiry.getMonth() + months, Math.min(expiry.getDate(), lastDay))
  return Math.floor(expiry.getTime() / 1000)
}
