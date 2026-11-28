/**
 * Select the best Codex catalog model for config.toml templates.
 * Priority: latest flagship (gpt-6+, claude-5+, etc.) > preferred > first catalog entry.
 */

type FlagshipPattern = {
  regex: RegExp
  weight: (match: string[]) => number
}

const FLAGSHIP_PATTERNS: FlagshipPattern[] = [
  {
    regex: /^gpt-(\d+)(?:\.(\d+))?/,
    weight: (m) => parseInt(m[1]) * 100 + parseInt(m[2] || '0')
  },
  {
    regex: /^claude-(sonnet|opus)-(\d+)(?:\.(\d+))?/,
    weight: (m) => parseInt(m[2]) * 100 + parseInt(m[3] || '0')
  },
  {
    regex: /^gemini-(\d+)(?:\.(\d+))?/,
    weight: (m) => parseInt(m[1]) * 100 + parseInt(m[2] || '0')
  },
  {
    regex: /^grok-(\d+)(?:\.(\d+))?/,
    weight: (m) => parseInt(m[1]) * 100 + parseInt(m[2] || '0')
  }
]

/** Minimum weight treated as flagship (major version >= 5). */
const FLAGSHIP_MIN_WEIGHT = 500

/**
 * From a model slug list, pick the highest-version flagship model.
 * Returns null when no slug matches a flagship pattern with weight >= 500.
 */
export function findLatestFlagshipModel(models: string[]): string | null {
  let bestModel: string | null = null
  let bestWeight = -1

  for (const model of models) {
    for (const pattern of FLAGSHIP_PATTERNS) {
      const match = model.match(pattern.regex)
      if (match) {
        const weight = pattern.weight(match)
        if (weight >= FLAGSHIP_MIN_WEIGHT && weight > bestWeight) {
          bestWeight = weight
          bestModel = model
        }
      }
    }
  }

  return bestModel
}

/**
 * Choose a Codex config model from catalog slugs.
 * 1) Latest flagship in catalog
 * 2) preferredModel if present
 * 3) First catalog entry
 * 4) preferredModel when catalog is empty
 */
export function selectCodexCatalogModel(models: string[], preferredModel: string): string {
  if (models.length === 0) return preferredModel

  const flagshipModel = findLatestFlagshipModel(models)
  if (flagshipModel) return flagshipModel

  if (models.includes(preferredModel)) return preferredModel
  return models[0] || preferredModel
}
