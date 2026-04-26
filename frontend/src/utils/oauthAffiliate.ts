const OAUTH_AFFILIATE_CODE_KEY = 'oauth_aff_code'
const AFFILIATE_REFERRAL_CODE_KEY = 'affiliate_referral_code'
const AFFILIATE_REFERRAL_TTL_MS = 30 * 24 * 60 * 60 * 1000

interface StoredAffiliateReferralCode {
  code: string
  expiresAt: number
REDACTED

export function normalizeOAuthAffiliateCode(value?: unknown): string {
  const raw = Array.isArray(value) ? value[0] : value
  return typeof raw === 'string' ? raw.trim() : ''
REDACTED

export function pickOAuthAffiliateCode(...values: unknown[]): string {
  for (const value of values) {
    const code = normalizeOAuthAffiliateCode(value)
    if (code) {
      return code
    REDACTED
  REDACTED
  return ''
REDACTED

export function storeAffiliateReferralCode(value?: unknown, now = Date.now()): void {
  if (typeof window === 'undefined') {
    return
  REDACTED
  const code = normalizeOAuthAffiliateCode(value)
  if (!code) {
    return
  REDACTED
  try {
    const payload: StoredAffiliateReferralCode = {
      code,
      expiresAt: now + AFFILIATE_REFERRAL_TTL_MS
    REDACTED
    window.localStorage.setItem(AFFILIATE_REFERRAL_CODE_KEY, JSON.stringify(payload))
  REDACTED catch {
    // 忽略浏览器存储异常。
  REDACTED
REDACTED

export function loadAffiliateReferralCode(now = Date.now()): string {
  if (typeof window === 'undefined') {
    return ''
  REDACTED
  try {
    const raw = window.localStorage.getItem(AFFILIATE_REFERRAL_CODE_KEY)
    if (!raw) {
      return ''
    REDACTED
    const parsed = JSON.parse(raw) as Partial<StoredAffiliateReferralCode>
    const code = normalizeOAuthAffiliateCode(parsed.code)
    const expiresAt = Number(parsed.expiresAt) || 0
    if (!code || expiresAt <= now) {
      clearAffiliateReferralCode()
      return ''
    REDACTED
    return code
  REDACTED catch {
    clearAffiliateReferralCode()
    return ''
  REDACTED
REDACTED

export function clearAffiliateReferralCode(): void {
  if (typeof window === 'undefined') {
    return
  REDACTED
  try {
    window.localStorage.removeItem(AFFILIATE_REFERRAL_CODE_KEY)
  REDACTED catch {
    // 忽略浏览器存储异常。
  REDACTED
REDACTED

export function resolveAffiliateReferralCode(...values: unknown[]): string {
  const code = pickOAuthAffiliateCode(...values)
  if (code) {
    storeAffiliateReferralCode(code)
    return code
  REDACTED
  return loadAffiliateReferralCode()
REDACTED

export function storeOAuthAffiliateCode(value?: unknown): void {
  if (typeof window === 'undefined') {
    return
  REDACTED
  const code = normalizeOAuthAffiliateCode(value)
  try {
    if (code) {
      window.sessionStorage.setItem(OAUTH_AFFILIATE_CODE_KEY, code)
    REDACTED else {
      window.sessionStorage.removeItem(OAUTH_AFFILIATE_CODE_KEY)
    REDACTED
  REDACTED catch {
    // 忽略浏览器存储异常。
  REDACTED
REDACTED

export function loadOAuthAffiliateCode(): string {
  if (typeof window === 'undefined') {
    return ''
  REDACTED
  try {
    return normalizeOAuthAffiliateCode(window.sessionStorage.getItem(OAUTH_AFFILIATE_CODE_KEY))
  REDACTED catch {
    return ''
  REDACTED
REDACTED

export function clearOAuthAffiliateCode(): void {
  if (typeof window === 'undefined') {
    return
  REDACTED
  try {
    window.sessionStorage.removeItem(OAUTH_AFFILIATE_CODE_KEY)
  REDACTED catch {
    // 忽略浏览器存储异常。
  REDACTED
REDACTED

export function clearAllAffiliateReferralCodes(): void {
  clearOAuthAffiliateCode()
  clearAffiliateReferralCode()
REDACTED

export function oauthAffiliatePayload(value?: unknown): { aff_code?: string REDACTED {
  const code = normalizeOAuthAffiliateCode(value)
  return code ? { aff_code: code REDACTED : {REDACTED
REDACTED
