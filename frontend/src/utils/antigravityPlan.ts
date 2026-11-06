// Use the upstream tier ID: the display name "Google AI Ultra" is shared by
// both plans. A legacy "Ultra" value alone cannot establish its multiplier.
export function antigravityPlan(raw: unknown): { label: string; className: string } | null {
  if (typeof raw !== 'string' || !raw.trim()) return null
  const key = raw.trim().toLowerCase()
  switch (key) {
    case 'g1-ultra-lite-tier':
    case 'ultra 5x':
      return { label: 'Ultra 5x', className: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300' }
    case 'g1-ultra-tier':
    case 'ultra 20x':
      return { label: 'Ultra 20x', className: 'bg-amber-100 text-amber-800 ring-1 ring-inset ring-amber-300/60 dark:bg-amber-900/40 dark:text-amber-300 dark:ring-amber-600/50' }
    case 'g1-pro-tier':
    case 'pro':
      return { label: 'Pro', className: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300' }
    case 'free-tier':
    case 'free':
      return { label: 'Free', className: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300' }
    case 'ultra':
      return { label: 'Ultra', className: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300' }
    default:
      return null
  }
}
