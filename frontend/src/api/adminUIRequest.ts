export const ADMIN_UI_REQUEST_HEADER = 'X-Admin-UI-Request'
export const USER_UI_REQUEST_HEADER = 'X-User-UI-Request'

function isAdminPath(path: string): boolean {
  return (
    path === '/admin' ||
    path.startsWith('/admin/') ||
    path === '/api/v1/admin' ||
    path.startsWith('/api/v1/admin/')
  )
REDACTED

function requestPath(rawURL: string): string {
  const value = rawURL.trim()
  if (!value) return ''
  try {
    const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost'
    return new URL(value, origin).pathname
  REDACTED catch {
    return value.split(/[?#]/, 1)[0]
  REDACTED
REDACTED

/** Normalize Axios relative paths and absolute API paths to a comparable form. */
function normalizeAPIPath(path: string): string {
  const raw = requestPath(path)
  if (!raw) return ''
  if (raw === '/api/v1' || raw.startsWith('/api/v1/')) {
    return raw.slice('/api/v1'.length) || '/'
  REDACTED
  if (raw.startsWith('/')) {
    return raw
  REDACTED
  return `/${rawREDACTED`
REDACTED

/**
 * User-facing web APIs that may emit Server-Timing when ENABLE_SERVER_TIMING is on.
 * Mirrors backend isUserTimingPath allowlist (excluding public payment surfaces).
 */
export function isUserTimingAPIPath(requestURL: string): boolean {
  const path = normalizeAPIPath(requestURL)
  if (!path) return false

  if (
    path === '/auth/me' ||
    path === '/auth/revoke-all-sessions' ||
    path === '/auth/oauth/bind-token'
  ) {
    return true
  REDACTED
  if (path === '/user' || path.startsWith('/user/')) return true
  if (path === '/keys' || path.startsWith('/keys/')) return true
  if (path === '/groups/available' || path === '/groups/rates') return true
  if (path === '/channels/available') return true
  if (path === '/usage' || path.startsWith('/usage/')) return true
  if (path === '/announcements' || path.startsWith('/announcements/')) return true
  if (path === '/redeem' || path.startsWith('/redeem/')) return true
  if (path === '/subscriptions' || path.startsWith('/subscriptions/')) return true
  if (path === '/channel-monitors' || path.startsWith('/channel-monitors/')) return true
  if (path.startsWith('/payment/')) {
    if (path.startsWith('/payment/public') || path.startsWith('/payment/webhook')) {
      return false
    REDACTED
    return true
  REDACTED
  return false
REDACTED

export function shouldMarkAdminUIRequest(requestURL: string, pagePath?: string): boolean {
  const currentPath =
    pagePath ?? (typeof window !== 'undefined' ? window.location.pathname : '')
  return isAdminPath(requestPath(requestURL)) || isAdminPath(currentPath)
REDACTED

export function shouldMarkUserUIRequest(requestURL: string): boolean {
  return isUserTimingAPIPath(requestURL)
REDACTED
