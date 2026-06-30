const DEFAULT_API_BASE_URL = '/api/v1'
const API_BASE_URL = normalizeAPIBaseURL(import.meta.env.VITE_API_BASE_URL)

function normalizePath(path: string): string {
  return path.startsWith('/') ? path : `/${pathREDACTED`
REDACTED

function normalizeAPIBaseURL(value: unknown): string {
  const raw = String(value || DEFAULT_API_BASE_URL).trim() || DEFAULT_API_BASE_URL
  const withoutTrailingSlash = raw.replace(/\/+$/, '')
  if (/^[a-z][a-z\d+.-]*:\/\//i.test(withoutTrailingSlash) || withoutTrailingSlash.startsWith('//')) {
    return withoutTrailingSlash
  REDACTED
  return normalizePath(withoutTrailingSlash)
REDACTED

export function getAPIBaseURL(): string {
  return API_BASE_URL
REDACTED

export function buildApiUrl(path: string): string {
  const base = getAPIBaseURL().replace(/\/+$/, '')
  let suffix = normalizePath(path)
  if (suffix === DEFAULT_API_BASE_URL) {
    suffix = ''
  REDACTED else if (suffix.startsWith(`${DEFAULT_API_BASE_URLREDACTED/`)) {
    suffix = suffix.slice(DEFAULT_API_BASE_URL.length)
  REDACTED
  return `${baseREDACTED${suffixREDACTED`
REDACTED

export function buildGatewayUrl(path: string): string {
  const suffix = normalizePath(path)
  try {
    const origin =
      typeof window === 'undefined'
        ? new URL(getAPIBaseURL()).origin
        : new URL(getAPIBaseURL(), window.location.origin).origin
    return `${originREDACTED${suffixREDACTED`
  REDACTED catch {
    return suffix
  REDACTED
REDACTED
