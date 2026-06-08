/**
 * Model mapping utilities for Antigravity forms.
 *
 * Provides wildcard validation and mapping object construction,
 * inlined from host useModelWhitelist to keep plugin self-contained.
 */
import type { ModelMapping } from '@sub2api/plugin-sdk'
import { getDefaultModelMapping } from '../api/antigravity'

/** Validate wildcard pattern: * only allowed at end, and only one. */
export function isValidWildcardPattern(pattern: string): boolean {
  const starIndex = pattern.indexOf('*')
  if (starIndex === -1) return true
  return starIndex === pattern.length - 1 && pattern.lastIndexOf('*') === starIndex
}

/** Build model_mapping object from mapping entries. */
export function buildModelMappingObject(
  mappings: ModelMapping[]
): Record<string, string> | null {
  const result: Record<string, string> = {}
  for (const m of mappings) {
    const from = m.from.trim()
    const to = m.to.trim()
    if (!from || !to) continue
    if (!isValidWildcardPattern(from)) continue
    if (to.includes('*')) continue
    result[from] = to
  }
  return Object.keys(result).length > 0 ? result : null
}

// Cache for default mappings from backend
let defaultMappingsCache: ModelMapping[] | null = null

/** Fetch default model mappings from backend API (cached). */
export async function fetchDefaultMappings(): Promise<ModelMapping[]> {
  if (defaultMappingsCache !== null) return defaultMappingsCache
  try {
    const mapping = await getDefaultModelMapping()
    defaultMappingsCache = Object.entries(mapping).map(([from, to]) => ({ from, to }))
  } catch (e: unknown) {
    console.warn('[plugin-gateway-antigravity] default mappings API failed, using empty fallback', e)
    defaultMappingsCache = []
  }
  return defaultMappingsCache
}
