export interface ImagePriceExpressionRow {
  model: string
  expression: string
}

export type ImagePriceExpressionError = 'incompleteRow' | 'duplicateModel' | 'modelWhitespace'

export const grokImageExpressionModel = 'grok-imagine-image-2.0'

// USD for the whole request, before the existing image/group multiplier.
// Unknown resolutions use an explicit operator fallback, not an official quote.
export const grokImageExpressionPreset = `(param("resolution") == nil || param("resolution") == "1k")
  ? (param("quality") == "medium"
      ? tier("1k-medium", fixed(0.06))
      : tier("1k-low", fixed(0.04))) * image_count
  : param("resolution") == "1.5k"
    ? (param("quality") == "medium"
        ? tier("1.5k-medium", fixed(0.07))
        : tier("1.5k-low", fixed(0.05))) * image_count
    : param("resolution") == "2k"
      ? (param("quality") == "medium"
          ? tier("2k-medium", fixed(0.08))
          : tier("2k-low", fixed(0.06))) * image_count
      : tier("unrecognized-resolution", fixed(0.08)) * image_count`

export function supportsImagePriceExpressions(platform: string): boolean {
  return platform === 'grok' || platform === 'openai' || platform === 'composite'
}

export function imagePriceExpressionsToRows(
  expressions?: Record<string, string> | null
): ImagePriceExpressionRow[] {
  return Object.entries(expressions ?? {}).map(([model, expression]) => ({ model, expression }))
}

export function validateImagePriceExpressionRows(
  rows: ImagePriceExpressionRow[]
): ImagePriceExpressionError | null {
  const models = new Set<string>()
  for (const { model, expression } of rows) {
    if (!model.trim() && !expression.trim()) continue
    if (!model.trim() || !expression.trim()) return 'incompleteRow'
    if (model !== model.trim()) return 'modelWhitespace'
    if (models.has(model)) return 'duplicateModel'
    models.add(model)
  }
  return null
}

export function serializeImagePriceExpressions(
  rows: ImagePriceExpressionRow[]
): Record<string, string> {
  const error = validateImagePriceExpressionRows(rows)
  if (error) throw new Error(error)
  // Object.fromEntries also preserves arbitrary model IDs such as "__proto__".
  // An empty map deliberately clears previously saved overrides on update.
  return Object.fromEntries(
    rows.filter(({ model, expression }) => model.trim() && expression.trim())
      .map(({ model, expression }) => [model, expression])
  )
}

export function hasImagePriceExpressionRows(rows: ImagePriceExpressionRow[]): boolean {
  return rows.some(({ model, expression }) => model.trim() || expression.trim())
}
