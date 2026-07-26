import { getModelsListCandidates } from '@/api/admin/groups'

export interface ModelCandidateOption {
  value: string
  label: string
}

const platformAliases: Record<string, string> = {
  claude: 'anthropic',
  codex: 'openai',
  google: 'gemini',
  vertex_ai: 'gemini',
  xai: 'grok'
}

function normalizeCandidatePlatform(platform: string): string {
  const normalized = platform.trim().toLowerCase()
  return platformAliases[normalized] ?? normalized
}

export async function fetchCatalogModelCandidates(platforms: string[]): Promise<string[]> {
  const normalizedPlatforms = Array.from(
    new Set(platforms.map(normalizeCandidatePlatform).filter(Boolean))
  )
  if (normalizedPlatforms.length === 0) return []

  const candidateLists = await Promise.all(
    normalizedPlatforms.map(platform => getModelsListCandidates(0, platform))
  )
  const candidates: string[] = []
  const seen = new Set<string>()
  for (const list of candidateLists) {
    for (const value of list) {
      const model = value.trim()
      if (!model || seen.has(model)) continue
      seen.add(model)
      candidates.push(model)
    }
  }
  return candidates
}

export function mergeModelCandidateOptions(catalogModels: string[] | null): ModelCandidateOption[] {
  if (catalogModels === null) return []

  return catalogModels.map(value => ({ value, label: value }))
}

export function filterCatalogModelSelection(
  selectedModels: string[],
  catalogModels: string[]
): string[] {
  const allowed = new Set(catalogModels)
  const seen = new Set<string>()
  const selection: string[] = []
  for (const value of selectedModels) {
    const model = value.trim()
    if (!model || !allowed.has(model) || seen.has(model)) continue
    seen.add(model)
    selection.push(model)
  }
  return selection
}
