/**
 * API Client for Sub2API Backend
 * Central export point for all API modules
 */

// Re-export the HTTP client
export { apiClient } from './client'

// Auth API
export { authAPI, isTotp2FARequired, type LoginResponse } from './auth'

// User APIs
export { keysAPI } from './keys'
export { usageAPI } from './usage'
export { userAPI } from './user'
export { redeemAPI, type RedeemHistoryItem } from './redeem'
export { userGroupsAPI } from './groups'
export { totpAPI } from './totp'
export { default as announcementsAPI } from './announcements'

// Admin APIs
export { adminAPI } from './admin'

// Default export
export { default } from './client'
