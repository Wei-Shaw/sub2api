/**
 * Pure helpers for inspecting Select option objects.
 * No Vue reactivity -- safe to import in any context.
 */

/** Read the "value" field from an option, using the configured key. */
export const getOptionValue = (
  option: Record<string, unknown> | unknown,
  valueKey: string,
): unknown => {
  if (typeof option === 'object' && option !== null) {
    return (option as Record<string, unknown>)[valueKey]
  }
  return option
}

/** Read the "label" field from an option, coerced to string. */
export const getOptionLabel = (
  option: Record<string, unknown> | unknown,
  labelKey: string,
): string => {
  if (typeof option === 'object' && option !== null) {
    return String((option as Record<string, unknown>)[labelKey] ?? '')
  }
  return String(option ?? '')
}

/** Check whether an option is disabled. */
export const isOptionDisabled = (option: Record<string, unknown> | unknown): boolean => {
  if (typeof option === 'object' && option !== null) {
    return !!(option as Record<string, unknown>).disabled
  }
  return false
}

/** Check whether an option is a group header (kind === 'group'). */
export const isGroupHeaderOption = (option: Record<string, unknown> | unknown): boolean => {
  if (typeof option === 'object' && option !== null) {
    return (option as Record<string, unknown>).kind === 'group'
  }
  return false
}
