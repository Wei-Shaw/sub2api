import type { AccountPlatform, GroupPlatform REDACTED from '@/types'

export interface PlatformOption<T extends string = string> {
  value: T
  label: string
REDACTED

/**
 * Concrete upstream platforms supported by accounts and request routing.
 * Keep platform selectors derived from this catalog so newly added providers
 * do not silently disappear from list filters.
 */
export const CONCRETE_PLATFORM_OPTIONS = [
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED,
  { value: 'grok', label: 'Grok' REDACTED,
  { value: 'kimi', label: 'Kimi' REDACTED,
  { value: 'zhipu', label: 'Zhipu GLM' REDACTED,
  { value: 'deepseek', label: 'DeepSeek' REDACTED
] as const satisfies readonly PlatformOption<AccountPlatform>[]

/** Platforms that can own a group. */
export const GROUP_PLATFORM_OPTIONS = [
  ...CONCRETE_PLATFORM_OPTIONS,
  { value: 'composite', label: 'Composite' REDACTED
] as const satisfies readonly PlatformOption<GroupPlatform>[]
