/** Keep rounded upstream refill hints relative instead of inventing a date. */
export function formatOllamaResetText(
  text: string | undefined,
  locale: string,
  t: (key: string, params: Record<string, string>) => string
): string | undefined {
  if (!text) return undefined
  const match = /^Refills in (\d+(?:\.\d+)?) (seconds?|minutes?|hours?|days?|weeks?|months?|years?)\.?$/i.exec(text.trim())
  if (!match) return text
  const amount = Number(match[1])
  if (!Number.isFinite(amount)) return text
  const unit = match[2].toLowerCase().replace(/s$/, '') as Intl.RelativeTimeFormatUnit
  const relative = new Intl.RelativeTimeFormat(locale, { numeric: 'always' }).format(amount, unit)
  return t('admin.accounts.ollamaCloud.approximateReset', { time: relative })
}
