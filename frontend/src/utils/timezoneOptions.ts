// Include UTC, common aliases and the saved value even when the browser's
// timezone catalog uses a different canonical IANA spelling.
export function getTimezoneOptions(currentValue = ''): { value: string; label: string }[] {
  const intl = Intl as typeof Intl & { supportedValuesOf?: (key: 'timeZone') => string[] }
  const fallback = [
    'UTC', 'Asia/Taipei', 'Asia/Shanghai', 'Asia/Tokyo', 'Asia/Singapore',
    'Europe/London', 'Europe/Paris', 'America/New_York', 'America/Los_Angeles',
    'Australia/Sydney',
  ]
  let available: string[] = []
  try {
    available = intl.supportedValuesOf?.('timeZone') ?? []
  } catch {
    // Older browsers can use the common list or enter a custom IANA timezone.
  }
  const values = new Set([...fallback, ...available])
  if (currentValue && currentValue !== 'off') values.add(currentValue)
  return [...values].sort().map(value => ({ value, label: value }))
}
