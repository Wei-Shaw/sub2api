export const chartGrayPalette = [
  '#111827',
  '#374151',
  '#4b5563',
  '#6b7280',
  '#9ca3af',
  '#d1d5db',
  '#525252',
  '#737373',
] as const

export function grayBadgeClass(): string {
  return 'border-gray-300 bg-gray-100 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
}

export function grayBadgeLightClass(): string {
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
}

export function grayBorderClass(): string {
  return 'border-gray-200 dark:border-dark-600'
}

export function grayAccentBarClass(): string {
  return 'bg-gray-900 dark:bg-gray-100'
}

export function grayTextClass(): string {
  return 'text-gray-900 dark:text-gray-100'
}

export function grayMutedTextClass(): string {
  return 'text-gray-500 dark:text-gray-400'
}

export function grayIconClass(): string {
  return 'text-gray-600 dark:text-gray-300'
}

export function grayButtonClass(): string {
  return 'bg-gray-900 text-white hover:bg-gray-800 active:bg-black dark:bg-gray-100 dark:text-dark-950 dark:hover:bg-white'
}

export function grayDiscountClass(): string {
  return 'bg-gray-200 text-gray-800 dark:bg-dark-600 dark:text-gray-100'
}

export function grayGradientClass(): string {
  return 'from-gray-900 to-gray-700 dark:from-gray-100 dark:to-gray-300'
}

export function grayGradientTextClass(): string {
  return 'text-gray-100 dark:text-dark-950'
}

export function grayGradientSubtextClass(): string {
  return 'text-gray-300 dark:text-gray-700'
}

export function modelChipClass(): string {
  return 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600'
}

export function grayProgressBarClass(percent: number): string {
  return percent >= 90 ? 'bg-red-500' : 'bg-gray-700 dark:bg-gray-300'
}
