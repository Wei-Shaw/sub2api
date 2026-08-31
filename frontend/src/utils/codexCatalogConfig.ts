export interface CodexCatalogReasoningLevel {
  effort?: unknown
}

export interface CodexCatalogModel {
  slug: string
  default_reasoning_level?: unknown
  supported_reasoning_levels?: CodexCatalogReasoningLevel[]
}

function trimEffort(value: unknown): string {
  if (typeof value !== 'string') return ''
  return value.trim()
}

export function parseCodexCatalogModels(content: string | null | undefined): CodexCatalogModel[] {
  if (!content) return []
  try {
    const payload: unknown = JSON.parse(content)
    if (typeof payload !== 'object' || payload === null || !('models' in payload)) return []
    const models = (payload as { models?: unknown }).models
    if (!Array.isArray(models)) return []
    return models.flatMap((model) => {
      if (typeof model !== 'object' || model === null || !('slug' in model)) return []
      const slug = trimEffort((model as { slug?: unknown }).slug)
      if (!slug) return []
      return [{ ...(model as CodexCatalogModel), slug }]
    })
  } catch {
    return []
  }
}

export function findCodexCatalogModel(
  content: string | null | undefined,
  slug: string
): CodexCatalogModel | undefined {
  const wanted = slug.trim()
  if (!wanted) return undefined
  return parseCodexCatalogModels(content).find((model) => model.slug === wanted)
}

export function selectCodexConfigReasoningEffort(
  model: CodexCatalogModel | undefined
): string | null {
  if (!model) return null
  const efforts = (model.supported_reasoning_levels ?? []).flatMap((level) => {
    const effort = trimEffort(level?.effort)
    return effort ? [effort] : []
  })
  if (efforts.length === 0) return null

  const defaultLevel = trimEffort(model.default_reasoning_level)
  if (defaultLevel && efforts.includes(defaultLevel)) {
    return defaultLevel === 'none' ? null : defaultLevel
  }
  return efforts.find((effort) => effort !== 'none') ?? null
}

export function formatCodexReasoningEffortTomlLine(effort: string | null): string {
  if (!effort) return ''
  return `model_reasoning_effort = "${effort}"\n`
}
