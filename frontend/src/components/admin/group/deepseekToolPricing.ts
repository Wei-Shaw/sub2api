import type { ChannelModelPricing } from '@/api/admin/channels'

export const deepseekTools = [
  { id: 'search', units: 1000, usd: 0.75, cny: 5 },
  { id: 'fetch', units: 1000, usd: 0.75, cny: 5 },
  { id: 'ocr', units: 1000, usd: 0.225, cny: 1.5 },
  { id: 'tts', units: 10000, usd: 0.45, cny: 3 },
  { id: 'asr', units: 3600, usd: 0.116, cny: 0.78 },
] as const

type Tool = typeof deepseekTools[number]
export type DeepseekToolPrices = Partial<Record<Tool['id'], ChannelModelPricing>>

export function matchesToolModel(pattern: string, tool: Tool): boolean {
  const normalized = pattern.trim().toLowerCase()
  const model = 'deepseek-tools-' + tool.id
  return normalized.endsWith('*') ? model.startsWith(normalized.slice(0, -1)) : model === normalized
}

// Keep tiered and wildcard rules in the general editor. A shared flat price
// can be split by model so editing one tool leaves the other models intact.
export function splitDeepseekToolPrices(platform: string, entries: ChannelModelPricing[] = []) {
  const tools: DeepseekToolPrices = {}
  const generic: ChannelModelPricing[] = []
  for (const entry of entries) {
    if (platform !== 'deepseek' || entry.billing_mode !== 'per_request' || entry.intervals?.length || !entry.models?.length) {
      generic.push(entry)
      continue
    }
    const otherModels = entry.models.filter(model => {
      const tool = deepseekTools.find(tool => model.trim().toLowerCase() === 'deepseek-tools-' + tool.id)
      if (!tool) return true
      tools[tool.id] = { ...entry, models: [model] }
      return false
    })
    if (otherModels.length) generic.push({ ...entry, models: otherModels })
  }
  return { tools, generic }
}

export function setDeepseekToolPrice(prices: DeepseekToolPrices, tool: Tool, amount: number | null): DeepseekToolPrices {
  const next = { ...prices }
  if (amount === null) {
    delete next[tool.id]
  } else {
    const entry = prices[tool.id] ?? {
      platform: 'deepseek', models: ['deepseek-tools-' + tool.id], billing_mode: 'per_request',
      input_price: null, output_price: null, cache_write_price: null, cache_read_price: null,
      image_input_price: null, image_output_price: null, intervals: [], time_pricing: null,
    }
    next[tool.id] = { ...entry, per_request_price: amount / tool.units }
  }
  return next
}
