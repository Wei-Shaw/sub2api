export const OPENAI_WS_MODE_OFF = 'off'
export const OPENAI_WS_MODE_CTX_POOL = 'ctx_pool'
export const OPENAI_WS_MODE_PASSTHROUGH = 'passthrough'

export type OpenAIWSMode =
  | typeof OPENAI_WS_MODE_OFF
  | typeof OPENAI_WS_MODE_CTX_POOL
  | typeof OPENAI_WS_MODE_PASSTHROUGH

const OPENAI_WS_MODES = new Set<OpenAIWSMode>([
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_PASSTHROUGH
])

export interface ResolveOpenAIWSModeOptions {
  modeKey: string
  enabledKey: string
  fallbackEnabledKeys?: string[]
  defaultMode?: OpenAIWSMode
}

export const normalizeOpenAIWSMode = (mode: unknown): OpenAIWSMode | null => {
  if (typeof mode !== 'string') return null
  const normalized = mode.trim().toLowerCase()
  if (normalized === 'shared' || normalized === 'dedicated') {
    return OPENAI_WS_MODE_CTX_POOL
  }
  if (OPENAI_WS_MODES.has(normalized as OpenAIWSMode)) {
    return normalized as OpenAIWSMode
  }
  return null
}

export const openAIWSModeFromEnabled = (enabled: unknown): OpenAIWSMode | null => {
  if (typeof enabled !== 'boolean') return null
  return enabled ? OPENAI_WS_MODE_CTX_POOL : OPENAI_WS_MODE_OFF
}

export const isOpenAIWSModeEnabled = (mode: OpenAIWSMode): boolean => {
  return mode !== OPENAI_WS_MODE_OFF
}

export const resolveOpenAIWSModeConcurrencyHintKey = (
  mode: OpenAIWSMode
): 'admin.accounts.openai.wsModeConcurrencyHint' | 'admin.accounts.openai.wsModePassthroughHint' => {
  if (mode === OPENAI_WS_MODE_PASSTHROUGH) {
    return 'admin.accounts.openai.wsModePassthroughHint'
  }
  return 'admin.accounts.openai.wsModeConcurrencyHint'
}

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
  }

  return fallback
}
