import { BILLING_MODE_TOKEN, type BillingMode } from '@/constants/channel'

export interface OfficialModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
}

const MTOK = 1_000_000

const OFFICIAL_PRICING: Record<string, OfficialModelPricing> = {
  'openai:gpt-4.1': tokenPricing(2, 8),
  'openai:gpt-4.1-2025-04-14': tokenPricing(2, 8),
  'anthropic:claude-sonnet-4': tokenPricing(3, 15, 3.75, 0.3),
  'anthropic:claude-sonnet-4-20250514': tokenPricing(3, 15, 3.75, 0.3),
}

function tokenPricing(input: number, output: number, cacheWrite: number | null = null, cacheRead: number | null = null): OfficialModelPricing {
  return {
    billing_mode: BILLING_MODE_TOKEN,
    input_price: input / MTOK,
    output_price: output / MTOK,
    cache_write_price: cacheWrite == null ? null : cacheWrite / MTOK,
    cache_read_price: cacheRead == null ? null : cacheRead / MTOK,
    image_output_price: null,
    per_request_price: null,
  }
}

function normalizeKeyPart(value: string): string {
  return value.trim().toLowerCase()
}

export function getOfficialModelPricing(platform: string, modelName: string): OfficialModelPricing | null {
  const key = `${normalizeKeyPart(platform)}:${normalizeKeyPart(modelName)}`
  return OFFICIAL_PRICING[key] ?? null
}
