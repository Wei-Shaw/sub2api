export interface ProtocolModel {
  id: string
  displayName: string
}

export interface ProtocolPresetMapping {
  label: string
  from: string
  to: string
  color: string
}

export interface ProtocolDefinition {
  id: string
  displayName: string
  iconSvg: string
  themeColor: string
  models: ProtocolModel[]
  presetMappings: ProtocolPresetMapping[]
}

export interface ResolvedProtocolGroup {
  protocol: ProtocolDefinition
  models: ProtocolModel[]
}

// ---------------------------------------------------------------------------
// Built-in protocol icons (extracted from PlatformIcon.vue SVG paths)
// ---------------------------------------------------------------------------

const ANTHROPIC_ICON = '<svg viewBox="0 0 16 16" fill="currentColor" class="w-3.5 h-3.5 flex-shrink-0"><path d="m3.127 10.604 3.135-1.76.053-.153-.053-.085H6.11l-.525-.032-1.791-.048-1.554-.065-1.505-.08-.38-.081L0 7.832l.036-.234.32-.214.455.04 1.009.069 1.513.105 1.097.064 1.626.17h.259l.036-.105-.089-.065-.068-.064-1.566-1.062-1.695-1.121-.887-.646-.48-.327-.243-.306-.104-.67.435-.48.585.04.15.04.593.456 1.267.981 1.654 1.218.242.202.097-.068.012-.049-.109-.181-.9-1.626-.96-1.655-.428-.686-.113-.411a2 2 0 0 1-.068-.484l.496-.674L4.446 0l.662.089.279.242.411.94.666 1.48 1.033 2.014.302.597.162.553.06.17h.105v-.097l.085-1.134.157-1.392.154-1.792.052-.504.25-.605.497-.327.387.186.319.456-.045.294-.19 1.23-.37 1.93-.243 1.29h.142l.161-.16.654-.868 1.097-1.372.484-.545.565-.601.363-.287h.686l.505.751-.226.775-.707.895-.585.759-.839 1.13-.524.904.048.072.125-.012 1.897-.403 1.024-.186 1.223-.21.553.258.06.263-.218.536-1.307.323-1.533.307-2.284.54-.028.02.032.04 1.029.098.44.024h1.077l2.005.15.525.346.315.424-.053.323-.807.411-3.631-.863-.872-.218h-.12v.073l.726.71 1.331 1.202 1.667 1.55.084.383-.214.302-.226-.032-1.464-1.101-.565-.497-1.28-1.077h-.084v.113l.295.432 1.557 2.34.08.718-.112.234-.404.141-.444-.08-.911-1.28-.94-1.44-.759-1.291-.093.053-.448 4.821-.21.246-.484.186-.403-.307-.214-.496.214-.98.258-1.28.21-1.016.19-1.263.112-.42-.008-.028-.092.012-.953 1.307-1.448 1.957-1.146 1.227-.274.109-.477-.247.045-.44.266-.39 1.586-2.018.956-1.25.617-.723-.004-.105h-.036l-4.212 2.736-.75.096-.324-.302.04-.496.154-.162 1.267-.871z"/></svg>'

const OPENAI_ICON = '<svg viewBox="0 0 24 24" fill="currentColor" class="w-3.5 h-3.5 flex-shrink-0"><path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.119 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135l-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5z"/></svg>'

const GEMINI_ICON = '<svg viewBox="0 0 24 24" fill="currentColor" class="w-3.5 h-3.5 flex-shrink-0"><path d="M12 2l1.89 7.2L21 12l-7.11 2.8L12 22l-1.89-7.2L3 12l7.11-2.8L12 2z"/></svg>'

// ---------------------------------------------------------------------------
// Built-in protocols
// ---------------------------------------------------------------------------

export const PROTOCOL_ANTHROPIC: ProtocolDefinition = {
  id: 'anthropic',
  displayName: 'Anthropic',
  iconSvg: ANTHROPIC_ICON,
  themeColor: '#ea580c',
  models: [
    { id: 'claude-opus-4-7', displayName: 'Claude Opus 4.7' },
    { id: 'claude-opus-4-6', displayName: 'Claude Opus 4.6' },
    { id: 'claude-sonnet-4-6', displayName: 'Claude Sonnet 4.6' },
    { id: 'claude-opus-4-5-20251101', displayName: 'Claude Opus 4.5' },
    { id: 'claude-sonnet-4-5-20250929', displayName: 'Claude Sonnet 4.5' },
    { id: 'claude-haiku-4-5-20251001', displayName: 'Claude Haiku 4.5' },
    { id: 'claude-opus-4-20250514', displayName: 'Claude Opus 4' },
    { id: 'claude-sonnet-4-20250514', displayName: 'Claude Sonnet 4' },
    { id: 'claude-3-7-sonnet-20250219', displayName: 'Claude 3.7 Sonnet' },
    { id: 'claude-3-5-sonnet-20241022', displayName: 'Claude 3.5 Sonnet v2' },
    { id: 'claude-3-5-sonnet-20240620', displayName: 'Claude 3.5 Sonnet v1' },
    { id: 'claude-3-5-haiku-20241022', displayName: 'Claude 3.5 Haiku' },
  ],
  presetMappings: [
    { label: 'Opus 4.7', from: 'claude-opus-4-7', to: 'claude-opus-4-7', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
    { label: 'Opus 4.6', from: 'claude-opus-4-6', to: 'claude-opus-4-6', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
    { label: 'Sonnet 4.6', from: 'claude-sonnet-4-6', to: 'claude-sonnet-4-6', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
    { label: 'Sonnet 4.5', from: 'claude-sonnet-4-5-20250929', to: 'claude-sonnet-4-5-20250929', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
    { label: 'Opus 4.5', from: 'claude-opus-4-5-20251101', to: 'claude-opus-4-5-20251101', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
    { label: 'Haiku 4.5', from: 'claude-haiku-4-5-20251001', to: 'claude-haiku-4-5-20251001', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
    { label: 'Haiku 3.5', from: 'claude-3-5-haiku-20241022', to: 'claude-3-5-haiku-20241022', color: 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400' },
    { label: 'Opus→Sonnet', from: 'claude-opus-4-6', to: 'claude-sonnet-4-5-20250929', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' },
  ],
}

export const PROTOCOL_OPENAI: ProtocolDefinition = {
  id: 'openai',
  displayName: 'OpenAI',
  iconSvg: OPENAI_ICON,
  themeColor: '#10b981',
  models: [
    { id: 'gpt-5.5', displayName: 'GPT-5.5' },
    { id: 'gpt-5.4', displayName: 'GPT-5.4' },
    { id: 'gpt-5.4-mini', displayName: 'GPT-5.4 Mini' },
    { id: 'gpt-5.4-2026-03-05', displayName: 'GPT-5.4 (2026-03-05)' },
    { id: 'gpt-5.3-codex', displayName: 'GPT-5.3 Codex' },
    { id: 'gpt-5.3-codex-spark', displayName: 'GPT-5.3 Codex Spark' },
    { id: 'codex-auto-review', displayName: 'Codex Auto Review' },
    { id: 'gpt-5.2', displayName: 'GPT-5.2' },
    { id: 'gpt-5.2-2025-12-11', displayName: 'GPT-5.2 (2025-12-11)' },
    { id: 'gpt-5.2-chat-latest', displayName: 'GPT-5.2 Chat Latest' },
    { id: 'gpt-5.2-pro', displayName: 'GPT-5.2 Pro' },
    { id: 'gpt-5.2-pro-2025-12-11', displayName: 'GPT-5.2 Pro (2025-12-11)' },
    { id: 'gpt-4o-audio-preview', displayName: 'GPT-4o Audio Preview' },
    { id: 'gpt-4o-realtime-preview', displayName: 'GPT-4o Realtime Preview' },
    { id: 'gpt-image-1', displayName: 'GPT Image 1' },
    { id: 'gpt-image-1.5', displayName: 'GPT Image 1.5' },
    { id: 'gpt-image-2', displayName: 'GPT Image 2' },
  ],
  presetMappings: [
    { label: 'GPT-5.5', from: 'gpt-5.5', to: 'gpt-5.5', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' },
    { label: 'GPT-5.4', from: 'gpt-5.4', to: 'gpt-5.4', color: 'bg-rose-100 text-rose-700 hover:bg-rose-200 dark:bg-rose-900/30 dark:text-rose-400' },
    { label: 'GPT-5.2', from: 'gpt-5.2', to: 'gpt-5.2', color: 'bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900/30 dark:text-red-400' },
    { label: 'o3', from: 'o3', to: 'o3', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
    { label: 'o1', from: 'o1', to: 'o1', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
    { label: 'GPT-4o', from: 'gpt-4o', to: 'gpt-4o', color: 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400' },
    { label: 'GPT-4o Mini', from: 'gpt-4o-mini', to: 'gpt-4o-mini', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
    { label: 'GPT-4.1', from: 'gpt-4.1', to: 'gpt-4.1', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
    { label: 'Codex Spark', from: 'gpt-5.3-codex-spark', to: 'gpt-5.3-codex-spark', color: 'bg-teal-100 text-teal-700 hover:bg-teal-200 dark:bg-teal-900/30 dark:text-teal-400' },
    { label: 'Haiku→5.4', from: 'claude-haiku-4-5-20251001', to: 'gpt-5.4', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
    { label: 'Opus→5.4', from: 'claude-opus-4-6', to: 'gpt-5.4', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
    { label: 'Sonnet→5.4', from: 'claude-sonnet-4-6', to: 'gpt-5.4', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
  ],
}

export const PROTOCOL_GEMINI: ProtocolDefinition = {
  id: 'gemini',
  displayName: 'Gemini',
  iconSvg: GEMINI_ICON,
  themeColor: '#2563eb',
  models: [
    { id: 'gemini-3-pro-preview', displayName: 'Gemini 3 Pro Preview' },
    { id: 'gemini-3-flash-preview', displayName: 'Gemini 3 Flash Preview' },
    { id: 'gemini-3.1-flash-image', displayName: 'Gemini 3.1 Flash Image' },
    { id: 'gemini-2.5-pro', displayName: 'Gemini 2.5 Pro' },
    { id: 'gemini-2.5-flash', displayName: 'Gemini 2.5 Flash' },
    { id: 'gemini-2.5-flash-image', displayName: 'Gemini 2.5 Flash Image' },
    { id: 'gemini-2.0-flash', displayName: 'Gemini 2.0 Flash' },
  ],
  presetMappings: [
    { label: '2.5 Pro', from: 'gemini-2.5-pro', to: 'gemini-2.5-pro', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
    { label: '2.5 Flash', from: 'gemini-2.5-flash', to: 'gemini-2.5-flash', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
    { label: 'Flash 2.0', from: 'gemini-2.0-flash', to: 'gemini-2.0-flash', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
    { label: '2.5 Image', from: 'gemini-2.5-flash-image', to: 'gemini-2.5-flash-image', color: 'bg-sky-100 text-sky-700 hover:bg-sky-200 dark:bg-sky-900/30 dark:text-sky-400' },
    { label: '3.1 Image', from: 'gemini-3.1-flash-image', to: 'gemini-3.1-flash-image', color: 'bg-sky-100 text-sky-700 hover:bg-sky-200 dark:bg-sky-900/30 dark:text-sky-400' },
  ],
}

export const BUILTIN_PROTOCOLS: Record<string, ProtocolDefinition> = {
  anthropic: PROTOCOL_ANTHROPIC,
  openai: PROTOCOL_OPENAI,
  gemini: PROTOCOL_GEMINI,
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function getProtocol(id: string): ProtocolDefinition | undefined {
  return BUILTIN_PROTOCOLS[id]
}

export function resolveProtocolModels(protocolIds: string[]): ResolvedProtocolGroup[] {
  return protocolIds
    .map(id => BUILTIN_PROTOCOLS[id])
    .filter((p): p is ProtocolDefinition => !!p)
    .map(protocol => ({ protocol, models: protocol.models }))
}

export function resolveProtocolPresets(protocolIds: string[]): ProtocolPresetMapping[] {
  return protocolIds
    .map(id => BUILTIN_PROTOCOLS[id])
    .filter((p): p is ProtocolDefinition => !!p)
    .flatMap(p => p.presetMappings)
}

export function resolveAllProtocolModelIds(protocolIds: string[]): string[] {
  return protocolIds
    .map(id => BUILTIN_PROTOCOLS[id])
    .filter((p): p is ProtocolDefinition => !!p)
    .flatMap(p => p.models.map(m => m.id))
}

export function findProtocolForModel(modelId: string, protocolIds: string[]): ProtocolDefinition | undefined {
  for (const id of protocolIds) {
    const proto = BUILTIN_PROTOCOLS[id]
    if (proto?.models.some(m => m.id === modelId)) return proto
  }
  return undefined
}
