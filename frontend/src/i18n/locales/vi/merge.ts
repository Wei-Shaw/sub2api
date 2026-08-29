type LocaleObject = Record<string, unknown>

function isLocaleObject(value: unknown): value is LocaleObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function mergeLocaleMessages(base: LocaleObject, ...additionsList: LocaleObject[]): LocaleObject {
  const merged: LocaleObject = { ...base }

  for (const additions of additionsList) {
    for (const [key, value] of Object.entries(additions)) {
      merged[key] = isLocaleObject(merged[key]) && isLocaleObject(value)
        ? mergeLocaleMessages(merged[key] as LocaleObject, value)
        : value
    }
  }

  return merged
}
