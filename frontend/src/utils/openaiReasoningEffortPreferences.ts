export const OPENAI_REASONING_EFFORT_PREFERENCES = [
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max'
] as const

export type OpenAIReasoningEffortPreference = (typeof OPENAI_REASONING_EFFORT_PREFERENCES)[number]

const aliases: Record<string, OpenAIReasoningEffortPreference> = {
  minimal: 'minimal',
  low: 'low',
  medium: 'medium',
  high: 'high',
  xhigh: 'xhigh',
  extrahigh: 'xhigh',
  max: 'max'
}

function normalizeEffort(value: string): OpenAIReasoningEffortPreference | undefined {
  return aliases[value.trim().toLowerCase().replace(/[-_\s]/g, '')]
}

export function normalizeOpenAIReasoningEffortPreferences(
  value: unknown
): OpenAIReasoningEffortPreference[] {
  const values = Array.isArray(value)
    ? value
    : typeof value === 'string'
      ? value.split(',')
      : []

  const normalized = new Set<OpenAIReasoningEffortPreference>()
  for (const item of values) {
    if (typeof item !== 'string') continue
    const effort = normalizeEffort(item)
    if (effort) normalized.add(effort)
  }
  return OPENAI_REASONING_EFFORT_PREFERENCES.filter((effort) => normalized.has(effort))
}
