type LocaleRecord = Record<string, unknown>

function isLocaleRecord(value: unknown): value is LocaleRecord {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

export function mergeLocaleMessages(base: LocaleRecord, overlay: LocaleRecord): LocaleRecord {
  const merged: LocaleRecord = { ...base }
  for (const [key, value] of Object.entries(overlay)) {
    const current = merged[key]
    merged[key] = isLocaleRecord(current) && isLocaleRecord(value)
      ? mergeLocaleMessages(current, value)
      : value
  }
  return merged
}
