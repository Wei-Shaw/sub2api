export const BILLING_MODE_TOKEN = 'token'
export const BILLING_MODE_PER_REQUEST = 'per_request'
export const BILLING_MODE_IMAGE = 'image'

export function getBillingModeLabel(mode: string | null | undefined, t: (key: string) => string): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST: return t('admin.usage.billingModePerRequest')
    case BILLING_MODE_IMAGE: return t('admin.usage.billingModeImage')
    default: return t('admin.usage.billingModeToken')
  }
}

export function getBillingModeBadgeClass(mode: string | null | undefined): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST: return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
    case BILLING_MODE_IMAGE: return 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300'
    default: return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
  }
}
