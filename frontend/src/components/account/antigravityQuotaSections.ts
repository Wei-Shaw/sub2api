import type { AntigravityQuotaBucket, AntigravityUsageInfo } from '@/types/antigravityUsage'

type ModelQuota = NonNullable<AntigravityUsageInfo['antigravity_quota']>[string]
type Family = 'gemini' | 'argon' | 'thirdParty' | 'other'

export interface QuotaSection {
  key: string
  family: Family
  name: string
  labelKey?: string
  description?: string
  buckets: AntigravityQuotaBucket[]
  models: [string, ModelQuota][]
}

function bucketFamily(buckets: AntigravityQuotaBucket[]): Family {
  if (buckets.some(b => b.id.startsWith('gemini-argon-'))) return 'argon'
  if (buckets.some(b => b.id === 'gemini-5h' || b.id === 'gemini-weekly')) return 'gemini'
  if (buckets.some(b => b.id === '3p-5h' || b.id === '3p-weekly')) return 'thirdParty'
  return 'other'
}

const windowOrder = (window: string) => window === '5h' ? 0 : window === 'weekly' ? 1 : 2

/** Keep upstream pools intact. Model rows are details, never inferred windows. */
export function antigravityQuotaSections(usage: AntigravityUsageInfo | null): QuotaSection[] {
  const sections: QuotaSection[] = (usage?.antigravity_quota_groups ?? []).map((group, index) => {
    const family = bucketFamily(group.buckets)
    return {
      key: `${index}:${group.buckets.map(b => b.id).join(',')}`,
      family,
      name: group.name || group.buckets[0]?.id || '',
      labelKey: family === 'other' ? undefined : `antigravityQuota.${family}`,
      description: group.description,
      buckets: [...group.buckets].sort((a, b) => windowOrder(a.window) - windowOrder(b.window)),
      models: []
    }
  })
  const separateArgon = sections.some(section => section.family === 'argon')
  const models = Object.entries(usage?.antigravity_quota ?? {}).sort(([a], [b]) => a.localeCompare(b))
  for (const entry of models) {
    const id = entry[0].toLowerCase()
    const family: Family = id.startsWith('gemini-4-argon') && separateArgon ? 'argon'
      : id.startsWith('gemini-') ? 'gemini'
        : id.startsWith('claude-') || id.startsWith('gpt-') ? 'thirdParty' : 'other'
    // Unknown models stay separate from unknown upstream pools: membership is
    // not established merely because their quota/reset values happen to match.
    let section = sections.find(s => s.family === family && (family !== 'other' || s.buckets.length === 0))
    if (!section) {
      section = { key: `models:${family}`, family, name: '', labelKey: `antigravityQuota.${family}`, buckets: [], models: [] }
      sections.push(section)
    }
    section.models.push(entry)
  }
  return sections
}
