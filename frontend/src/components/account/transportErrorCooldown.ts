export const DEFAULT_TRANSPORT_ERROR_COOLDOWN_MINUTES = 10
export const MAX_TRANSPORT_ERROR_COOLDOWN_MINUTES = 1440

export function isValidTransportErrorCooldownMinutes(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) &&
    value >= 1 && value <= MAX_TRANSPORT_ERROR_COOLDOWN_MINUTES
}

export function readTransportErrorCooldown(extra?: Record<string, unknown>) {
  const minutes = extra?.transport_error_cooldown_minutes
  return {
    enabled: extra?.transport_error_cooldown_enabled !== false,
    minutes: isValidTransportErrorCooldownMinutes(minutes)
      ? minutes
      : DEFAULT_TRANSPORT_ERROR_COOLDOWN_MINUTES
  }
}
