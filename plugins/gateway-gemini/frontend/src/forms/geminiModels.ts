/**
 * Gemini-specific model lists and preset mappings.
 * Extracted from host useModelWhitelist.ts — only the Gemini subset.
 */

export const geminiModels: string[] = [
  'gemini-3.1-flash-image',
  'gemini-2.5-flash-image',
  'gemini-2.0-flash',
  'gemini-2.5-flash',
  'gemini-2.5-pro',
  'gemini-3-flash-preview',
  'gemini-3-pro-preview'
]

export const geminiPresetMappings = [
  { label: 'Flash 2.0', from: 'gemini-2.0-flash', to: 'gemini-2.0-flash', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
  { label: '2.5 Flash', from: 'gemini-2.5-flash', to: 'gemini-2.5-flash', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
  { label: '2.5 Image', from: 'gemini-2.5-flash-image', to: 'gemini-2.5-flash-image', color: 'bg-sky-100 text-sky-700 hover:bg-sky-200 dark:bg-sky-900/30 dark:text-sky-400' },
  { label: '2.5 Pro', from: 'gemini-2.5-pro', to: 'gemini-2.5-pro', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
  { label: '3.1 Image', from: 'gemini-3.1-flash-image', to: 'gemini-3.1-flash-image', color: 'bg-sky-100 text-sky-700 hover:bg-sky-200 dark:bg-sky-900/30 dark:text-sky-400' }
]

/**
 * Build a model_mapping object from whitelist or mapping mode.
 * Inlined from host useModelWhitelist.buildModelMappingObject.
 */
export function buildModelMappingObject(
  mode: 'whitelist' | 'mapping',
  allowedModels: string[],
  modelMappings: { from: string; to: string }[]
): Record<string, string> | null {
  const mapping: Record<string, string> = {}
  if (mode === 'whitelist') {
    for (const model of allowedModels) {
      if (!model.includes('*')) {
        mapping[model] = model
      }
    }
  } else {
    for (const m of modelMappings) {
      const from = m.from.trim()
      const to = m.to.trim()
      if (!from || !to) continue
      const starIdx = from.indexOf('*')
      if (starIdx !== -1 && (starIdx !== from.length - 1 || from.lastIndexOf('*') !== starIdx)) continue
      if (to.includes('*')) continue
      mapping[from] = to
    }
  }
  return Object.keys(mapping).length > 0 ? mapping : null
}
