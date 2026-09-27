import viCommon from '@/i18n/locales/vi/common'

// The vitest build of vue-i18n has no message compiler, so specs that assert
// real Vietnamese copy resolve keys straight from the locale object.
export function viT(key: string, params?: Record<string, unknown>): string {
  const value = key
    .split('.')
    .reduce<unknown>((node, part) => (node as Record<string, unknown> | undefined)?.[part], viCommon)
  if (typeof value !== 'string') return key
  return value.replace(/\{(\w+)\}/g, (match, name: string) =>
    params?.[name] === undefined ? match : String(params[name])
  )
}
