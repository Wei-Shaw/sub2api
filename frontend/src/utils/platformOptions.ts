import type { GroupPlatform } from '@/types'

export const ADMIN_PLATFORM_VALUES = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
  'composite',
  'kiro',
  'deepseek',
  'glm',
  'zai',
  'minimax',
  'opencode'
] as const satisfies readonly GroupPlatform[]

export type AdminPlatformValue = (typeof ADMIN_PLATFORM_VALUES)[number]
export type PlatformLabelNamespace = 'admin.groups.platforms' | 'admin.accounts.platforms'
export type TranslatePlatformLabel = (key: string, fallback?: string) => string

export function platformLabelKey(namespace: PlatformLabelNamespace, platform: string): string {
  return `${namespace}.${platform}`
}

export function platformDisplayName(
  t: TranslatePlatformLabel,
  platform: string,
  namespace: PlatformLabelNamespace = 'admin.groups.platforms'
): string {
  return t(platformLabelKey(namespace, platform), platform)
}

export function makePlatformOptions<T extends string = AdminPlatformValue>(
  t: TranslatePlatformLabel,
  namespace: PlatformLabelNamespace = 'admin.groups.platforms',
  values: readonly T[] = ADMIN_PLATFORM_VALUES as readonly unknown[] as readonly T[]
): Array<{ value: T; label: string }> {
  return values.map((value) => ({
    value,
    label: platformDisplayName(t, value, namespace)
  }))
}

export function platformOrderIndex(platform: string, values: readonly string[] = ADMIN_PLATFORM_VALUES): number {
  const index = values.indexOf(platform)
  return index === -1 ? Number.MAX_SAFE_INTEGER : index
}
