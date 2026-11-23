// Live model catalog of an Antigravity account. The derivation mirrors the backend
// (backend/internal/service/antigravity_live_model_mapping.go) so the edit modal can
// show the model names the gateway accepts without another request.

export const ANTIGRAVITY_MODEL_MAPPING_MODE_KEY = 'model_mapping_mode'
export const ANTIGRAVITY_MODEL_MAPPING_MODE_LIVE = 'live'
export const ANTIGRAVITY_MODEL_CATALOG_EXTRA_KEY = 'antigravity_model_catalog'

export interface AntigravityModelCatalog {
  models: string[]
  syncedAt: string
}

const TIER_ORDER = ['low', 'medium', 'high', 'xhigh']

export function isAntigravityLiveModelMapping(credentials: Record<string, unknown> | null | undefined): boolean {
  const mode = credentials?.[ANTIGRAVITY_MODEL_MAPPING_MODE_KEY]
  return typeof mode === 'string' && mode.trim().toLowerCase() === ANTIGRAVITY_MODEL_MAPPING_MODE_LIVE
}

/** Sets or removes the live mode flag on an Antigravity credentials object. */
export function applyAntigravityModelMappingMode(credentials: Record<string, unknown>, live: boolean): void {
  if (live) {
    credentials[ANTIGRAVITY_MODEL_MAPPING_MODE_KEY] = ANTIGRAVITY_MODEL_MAPPING_MODE_LIVE
  } else {
    delete credentials[ANTIGRAVITY_MODEL_MAPPING_MODE_KEY]
  }
}

/** Trims, drops empty IDs and "-disabled" placeholders, dedupes and sorts. */
export function normalizeAntigravityCatalogIds(ids: readonly unknown[]): string[] {
  const out = new Set<string>()
  for (const raw of ids) {
    if (typeof raw !== 'string') continue
    const id = raw.trim()
    if (id && !id.endsWith('-disabled')) out.add(id)
  }
  return [...out].sort()
}

export function readAntigravityModelCatalog(extra: Record<string, unknown> | null | undefined): AntigravityModelCatalog | null {
  const raw = extra?.[ANTIGRAVITY_MODEL_CATALOG_EXTRA_KEY]
  if (!raw || typeof raw !== 'object') return null
  const { models, synced_at: syncedAt } = raw as { models?: unknown; synced_at?: unknown }
  if (!Array.isArray(models)) return null
  const ids = normalizeAntigravityCatalogIds(models)
  if (ids.length === 0) return null
  return { models: ids, syncedAt: typeof syncedAt === 'string' ? syncedAt : '' }
}

/**
 * Model names a live-mode account accepts: an ID family <base>-<tier> with at least
 * two tiers including high is listed once as <base>; every other ID as itself.
 */
export function deriveAntigravityLiveModelNames(catalog: readonly string[]): string[] {
  const ids = normalizeAntigravityCatalogIds(catalog)
  const tiers = new Map<string, Set<string>>()
  for (const id of ids) {
    const tier = TIER_ORDER.find((candidate) => id.endsWith(`-${candidate}`))
    if (!tier) continue
    const base = id.slice(0, -(tier.length + 1))
    if (!base) continue
    if (!tiers.has(base)) tiers.set(base, new Set())
    tiers.get(base)!.add(tier)
  }
  const names = new Set<string>()
  const hidden = new Set<string>()
  for (const [base, present] of tiers) {
    if (present.size < 2 || !present.has('high')) continue
    names.add(base)
    for (const tier of present) hidden.add(`${base}-${tier}`)
  }
  for (const id of ids) {
    if (!hidden.has(id)) names.add(id)
  }
  return [...names].sort()
}
