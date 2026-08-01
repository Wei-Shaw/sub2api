import type { ChannelServiceTierConfig } from '@/api/admin/channels'

export type ServiceTierKey = 'standard' | 'priority' | 'flex'

export function defaultServiceTierConfig(): ChannelServiceTierConfig {
  return {
    standard: { enabled: true, multiplier: 1 },
    priority: { enabled: true, multiplier: 2 },
    flex: { enabled: true, multiplier: 0.5 },
    use_outbound_tier_for_billing: true,
  }
}

export function cloneServiceTierConfig(config?: ChannelServiceTierConfig | null): ChannelServiceTierConfig {
  if (!config) return defaultServiceTierConfig()
  return {
    standard: { ...config.standard },
    priority: { ...config.priority },
    flex: { ...config.flex },
    use_outbound_tier_for_billing: config.use_outbound_tier_for_billing !== false,
  }
}

export function validateServiceTierConfig(config: ChannelServiceTierConfig): 'no_enabled_tier' | 'invalid_multiplier' | null {
  const options = [config.standard, config.priority, config.flex]
  if (!options.some(option => option.enabled)) return 'no_enabled_tier'
  if (options.some(option => !Number.isFinite(option.multiplier) || option.multiplier < 0.01 || option.multiplier > 100)) {
    return 'invalid_multiplier'
  }
  return null
}
