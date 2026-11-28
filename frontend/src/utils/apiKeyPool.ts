import type { APIKeyExtraDraft } from '@/components/account/AccountAPIKeyPoolEditor.vue'

export interface APIKeySlotView {
  id?: string
  label?: string
  weight?: number
  enabled?: boolean
}

export function readAPIKeyPool(credentials: Record<string, unknown> | undefined): {
  strategy: 'round_robin' | 'weighted'
  primaryWeight: number
  extras: APIKeyExtraDraft[]
} {
  const slots = Array.isArray(credentials?.api_key_slots) ? (credentials?.api_key_slots as APIKeySlotView[]) : []
  const strategy = credentials?.api_key_strategy === 'weighted' ? 'weighted' : 'round_robin'
  const primary = slots.find((slot) => slot?.id === 'primary')
  const extras = slots
    .filter((slot) => slot?.id && slot.id !== 'primary')
    .map((slot) => ({
      id: String(slot.id),
      label: typeof slot.label === 'string' ? slot.label : '',
      weight: typeof slot.weight === 'number' && slot.weight > 0 ? slot.weight : 1,
      enabled: slot.enabled !== false,
      key: ''
    }))
  return {
    strategy,
    primaryWeight: typeof primary?.weight === 'number' && primary.weight > 0 ? primary.weight : 1,
    extras
  }
}

export function applyAPIKeyPool(
  credentials: Record<string, unknown>,
  input: {
    strategy: 'round_robin' | 'weighted'
    primaryKey?: string
    primaryWeight: number
    extras: APIKeyExtraDraft[]
  }
) {
  const extras = input.extras.filter((item) => item.id)
  if (extras.length === 0) {
    delete credentials.api_key_strategy
    delete credentials.api_key_slots
    delete credentials.api_keys
    return
  }
  const weight = (value: number) => {
    if (!Number.isFinite(value) || value < 1) return 1
    return Math.min(10000, Math.floor(value))
  }
  credentials.api_key_strategy = input.strategy === 'weighted' ? 'weighted' : 'round_robin'
  credentials.api_key_slots = [
    { id: 'primary', label: '', weight: weight(input.primaryWeight), enabled: true },
    ...extras.map((item) => ({
      id: item.id,
      label: item.label.trim(),
      weight: weight(item.weight),
      enabled: item.enabled
    }))
  ]
  const keys: Array<{ id: string; key: string }> = []
  if (input.primaryKey?.trim()) {
    keys.push({ id: 'primary', key: input.primaryKey.trim() })
  }
  for (const item of extras) {
    if (item.key.trim()) keys.push({ id: item.id, key: item.key.trim() })
  }
  if (keys.length > 0) credentials.api_keys = keys
  else delete credentials.api_keys
}
