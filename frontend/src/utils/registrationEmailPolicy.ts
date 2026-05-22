const EMAIL_SUFFIX_TOKEN_SPLIT_RE = /[\s,，]+/
const EMAIL_SUFFIX_INVALID_CHAR_RE = /[^a-z0-9.-]/g
const EMAIL_SUFFIX_INVALID_CHAR_CHECK_RE = /[^a-z0-9.-]/
const EMAIL_SUFFIX_PREFIX_RE = /^@+/
const EMAIL_SUFFIX_WILDCARD_PREFIX = '*.'
const EMAIL_SUFFIX_MESSAGE_VISIBLE_LIMIT = 5
const EMAIL_SUFFIX_DOMAIN_PATTERN =
  /^[a-z0-9](?:[a-z0-9-]{0,61REDACTED[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61REDACTED[a-z0-9])?)+$/

// normalizeRegistrationEmailSuffixDomain converts raw input into a canonical domain token.
// Exact domains are returned without "@"; wildcard domains keep the "*." prefix.
export function normalizeRegistrationEmailSuffixDomain(raw: string): string {
  let value = String(raw || '').trim().toLowerCase()
  if (!value) {
    return ''
  REDACTED

  value = value.replace(EMAIL_SUFFIX_PREFIX_RE, '')
  return normalizeRegistrationEmailSuffixToken(value, false)
REDACTED

export function normalizeRegistrationEmailSuffixDomains(
  items: string[] | null | undefined
): string[] {
  if (!items || items.length === 0) {
    return []
  REDACTED

  const seen = new Set<string>()
  const normalized: string[] = []
  for (const item of items) {
    const domain = normalizeRegistrationEmailSuffixDomain(item)
    if (!isRegistrationEmailSuffixDomainValid(domain) || seen.has(domain)) {
      continue
    REDACTED
    seen.add(domain)
    normalized.push(domain)
  REDACTED
  return normalized
REDACTED

export function parseRegistrationEmailSuffixWhitelistInput(input: string): string[] {
  if (!input || !input.trim()) {
    return []
  REDACTED

  const seen = new Set<string>()
  const normalized: string[] = []

  for (const token of input.split(EMAIL_SUFFIX_TOKEN_SPLIT_RE)) {
    const domain = normalizeRegistrationEmailSuffixDomainStrict(token)
    if (!isRegistrationEmailSuffixDomainValid(domain) || seen.has(domain)) {
      continue
    REDACTED
    seen.add(domain)
    normalized.push(domain)
  REDACTED

  return normalized
REDACTED

export function normalizeRegistrationEmailSuffixWhitelist(
  items: string[] | null | undefined
): string[] {
  return normalizeRegistrationEmailSuffixDomains(items).map(toCanonicalRegistrationEmailSuffix)
REDACTED

function extractRegistrationEmailDomain(email: string): string {
  const raw = String(email || '').trim().toLowerCase()
  if (!raw) {
    return ''
  REDACTED
  const atIndex = raw.indexOf('@')
  if (atIndex <= 0 || atIndex >= raw.length - 1) {
    return ''
  REDACTED
  if (raw.indexOf('@', atIndex + 1) !== -1) {
    return ''
  REDACTED
  return raw.slice(atIndex + 1)
REDACTED

export function isRegistrationEmailSuffixAllowed(
  email: string,
  whitelist: string[] | null | undefined
): boolean {
  const normalizedWhitelist = normalizeRegistrationEmailSuffixWhitelist(whitelist)
  if (normalizedWhitelist.length === 0) {
    return true
  REDACTED
  const emailDomain = extractRegistrationEmailDomain(email)
  if (!emailDomain) {
    return false
  REDACTED
  const emailSuffix = `@${emailDomainREDACTED`
  return normalizedWhitelist.some((allowed) => {
    if (allowed.startsWith('@')) {
      return allowed === emailSuffix
    REDACTED
    if (allowed.startsWith(EMAIL_SUFFIX_WILDCARD_PREFIX)) {
      const base = allowed.slice(EMAIL_SUFFIX_WILDCARD_PREFIX.length)
      return emailDomain === base || emailDomain.endsWith(`.${baseREDACTED`)
    REDACTED
    return false
  REDACTED)
REDACTED

export function formatRegistrationEmailSuffixWhitelistForMessage(
  whitelist: string[] | null | undefined,
  options: {
    separator: string
    more: (count: number) => string
  REDACTED
): string {
  const normalizedWhitelist = normalizeRegistrationEmailSuffixWhitelist(whitelist)
  const visible = normalizedWhitelist.slice(0, EMAIL_SUFFIX_MESSAGE_VISIBLE_LIMIT)
  const hiddenCount = normalizedWhitelist.length - visible.length
  if (hiddenCount > 0) {
    visible.push(options.more(hiddenCount))
  REDACTED
  return visible.join(options.separator)
REDACTED

// Pasted domains should be strict: any invalid character drops the whole token.
function normalizeRegistrationEmailSuffixDomainStrict(raw: string): string {
  let value = String(raw || '').trim().toLowerCase()
  if (!value) {
    return ''
  REDACTED
  value = value.replace(EMAIL_SUFFIX_PREFIX_RE, '')
  return normalizeRegistrationEmailSuffixToken(value, true)
REDACTED

export function isRegistrationEmailSuffixDomainValid(domain: string): boolean {
  if (!domain) {
    return false
  REDACTED
  if (domain.startsWith(EMAIL_SUFFIX_WILDCARD_PREFIX)) {
    return EMAIL_SUFFIX_DOMAIN_PATTERN.test(domain.slice(EMAIL_SUFFIX_WILDCARD_PREFIX.length))
  REDACTED
  return !domain.includes('*') && EMAIL_SUFFIX_DOMAIN_PATTERN.test(domain)
REDACTED

function normalizeRegistrationEmailSuffixToken(value: string, strict: boolean): string {
  if (value.startsWith(EMAIL_SUFFIX_WILDCARD_PREFIX)) {
    const domain = value.slice(EMAIL_SUFFIX_WILDCARD_PREFIX.length)
    if (strict && (!domain || EMAIL_SUFFIX_INVALID_CHAR_CHECK_RE.test(domain))) {
      return ''
    REDACTED
    return `${EMAIL_SUFFIX_WILDCARD_PREFIXREDACTED${domain.replace(EMAIL_SUFFIX_INVALID_CHAR_RE, '')REDACTED`
  REDACTED

  if (value === '*') {
    return strict ? '' : value
  REDACTED

  if (strict && EMAIL_SUFFIX_INVALID_CHAR_CHECK_RE.test(value)) {
    return ''
  REDACTED
  return value.replace(/[*]/g, '').replace(EMAIL_SUFFIX_INVALID_CHAR_RE, '')
REDACTED

function toCanonicalRegistrationEmailSuffix(domain: string): string {
  return domain.startsWith(EMAIL_SUFFIX_WILDCARD_PREFIX) ? domain : `@${domainREDACTED`
REDACTED
