/**
 * Pinia Stores Export
 * Central export point for all application stores
 */

export { useAuthStore } from './auth'
export { useAppStore } from './app'
export { useAdminSettingsStore } from './adminSettings'
export { useAdminReferralBadgeStore } from './adminReferralBadges'
export { useSubscriptionStore } from './subscriptions'
export { useOnboardingStore } from './onboarding'
export { useAnnouncementStore } from './announcements'
export { usePaymentStore } from './payment'
export { useReferralStore } from './referral'

// Re-export types for convenience
export type { User, LoginRequest, RegisterRequest, AuthResponse } from '@/types'
export type { Toast, ToastType, AppState } from '@/types'
