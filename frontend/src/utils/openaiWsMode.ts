export const OPENAI_WS_MODE_OFF = 'off'
export const OPENAI_WS_MODE_SHARED = 'shared'
export const OPENAI_WS_MODE_DEDICATED = 'dedicated'

export type OpenAIWSMode =
  | typeof OPENAI_WS_MODE_OFF
  | typeof OPENAI_WS_MODE_SHARED
  | typeof OPENAI_WS_MODE_DEDICATED

const OPENAI_WS_MODES = new Set<OpenAIWSMode>([
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_SHARED,
  OPENAI_WS_MODE_DEDICATED
])

export interface ResolveOpenAIWSModeOptions {
  modeKey: string
  enabledKey: string
  fallbackEnabledKeys?: string[]
  defaultMode?: OpenAIWSMode
REDACTED

export const normalizeOpenAIWSMode = (mode: unknown): OpenAIWSMode | null => {
  if (typeof mode !== 'string') return null
  const normalized = mode.trim().toLowerCase()
  if (OPENAI_WS_MODES.has(normalized as OpenAIWSMode)) {
    return normalized as OpenAIWSMode
  REDACTED
  return null
REDACTED

export const openAIWSModeFromEnabled = (enabled: unknown): OpenAIWSMode | null => {
  if (typeof enabled !== 'boolean') return null
  return enabled ? OPENAI_WS_MODE_SHARED : OPENAI_WS_MODE_OFF
REDACTED

export const isOpenAIWSModeEnabled = (mode: OpenAIWSMode): boolean => {
  return mode !== OPENAI_WS_MODE_OFF
REDACTED

export const resolveOpenAIWSModeFromExtra = (
  extra: Record<string, unknown> | null | undefined,
  options: ResolveOpenAIWSModeOptions
): OpenAIWSMode => {
  const fallback = options.defaultMode ?? OPENAI_WS_MODE_OFF
  if (!extra) return fallback

  const mode = normalizeOpenAIWSMode(extra[options.modeKey])
  if (mode) return mode

  const enabledMode = openAIWSModeFromEnabled(extra[options.enabledKey])
  if (enabledMode) return enabledMode

  const fallbackKeys = options.fallbackEnabledKeys ?? []
  for (const key of fallbackKeys) {
    const modeFromFallbackKey = openAIWSModeFromEnabled(extra[key])
    if (modeFromFallbackKey) return modeFromFallbackKey
  REDACTED

  return fallback
REDACTED
