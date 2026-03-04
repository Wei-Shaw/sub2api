export interface ParsedSoraTokens {
  sessionTokens: string[]
  accessTokens: string[]
REDACTED

const sessionKeyNames = new Set(['sessiontoken', 'session_token', 'st'])
const accessKeyNames = new Set(['accesstoken', 'access_token', 'at'])

const sessionRegexes = [
  /\bsessionToken\b\s*:\s*["']([^"']+)["']/gi,
  /\bsession_token\b\s*:\s*["']([^"']+)["']/gi
]

const accessRegexes = [
  /\baccessToken\b\s*:\s*["']([^"']+)["']/gi,
  /\baccess_token\b\s*:\s*["']([^"']+)["']/gi
]

const sessionCookieRegex =
  /(?:^|[\n\r;])\s*(?:(?:set-cookie|cookie)\s*:\s*)?__Secure-(?:next-auth|authjs)\.session-token(?:\.(\d+))?=([^;\r\n]+)/gi

interface SessionCookieChunk {
  index: number
  value: string
REDACTED

const ignoredPlainLines = new Set([
  'set-cookie',
  'cookie',
  'strict-transport-security',
  'vary',
  'x-content-type-options',
  'x-openai-proxy-wasm'
])

function sanitizeToken(raw: string): string {
  return raw.trim().replace(/^["'`]+|["'`,;]+$/g, '')
REDACTED

function addUnique(list: string[], seen: Set<string>, rawValue: string): void {
  const token = sanitizeToken(rawValue)
  if (!token || seen.has(token)) {
    return
  REDACTED
  seen.add(token)
  list.push(token)
REDACTED

function isLikelyJWT(token: string): boolean {
  if (!token.startsWith('eyJ')) {
    return false
  REDACTED
  return token.split('.').length === 3
REDACTED

function collectFromObject(
  value: unknown,
  sessionTokens: string[],
  sessionSeen: Set<string>,
  accessTokens: string[],
  accessSeen: Set<string>
): void {
  if (Array.isArray(value)) {
    for (const item of value) {
      collectFromObject(item, sessionTokens, sessionSeen, accessTokens, accessSeen)
    REDACTED
    return
  REDACTED
  if (!value || typeof value !== 'object') {
    return
  REDACTED

  for (const [key, fieldValue] of Object.entries(value as Record<string, unknown>)) {
    if (typeof fieldValue === 'string') {
      const normalizedKey = key.toLowerCase()
      if (sessionKeyNames.has(normalizedKey)) {
        addUnique(sessionTokens, sessionSeen, fieldValue)
      REDACTED
      if (accessKeyNames.has(normalizedKey)) {
        addUnique(accessTokens, accessSeen, fieldValue)
      REDACTED
      continue
    REDACTED
    collectFromObject(fieldValue, sessionTokens, sessionSeen, accessTokens, accessSeen)
  REDACTED
REDACTED

function collectFromJSONString(
  raw: string,
  sessionTokens: string[],
  sessionSeen: Set<string>,
  accessTokens: string[],
  accessSeen: Set<string>
): void {
  const trimmed = raw.trim()
  if (!trimmed) {
    return
  REDACTED

  const candidates = [trimmed]
  const firstBrace = trimmed.indexOf('{')
  const lastBrace = trimmed.lastIndexOf('REDACTED')
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    candidates.push(trimmed.slice(firstBrace, lastBrace + 1))
  REDACTED

  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate)
      collectFromObject(parsed, sessionTokens, sessionSeen, accessTokens, accessSeen)
      return
    REDACTED catch {
      // ignore and keep trying other candidates
    REDACTED
  REDACTED
REDACTED

function collectByRegex(
  raw: string,
  regexes: RegExp[],
  tokens: string[],
  seen: Set<string>
): void {
  for (const regex of regexes) {
    regex.lastIndex = 0
    let match: RegExpExecArray | null
    match = regex.exec(raw)
    while (match) {
      if (match[1]) {
        addUnique(tokens, seen, match[1])
      REDACTED
      match = regex.exec(raw)
    REDACTED
  REDACTED
REDACTED

function collectFromSessionCookies(
  raw: string,
  sessionTokens: string[],
  sessionSeen: Set<string>
): void {
  const chunkMatches: SessionCookieChunk[] = []
  const singleValues: string[] = []

  sessionCookieRegex.lastIndex = 0
  let match: RegExpExecArray | null
  match = sessionCookieRegex.exec(raw)
  while (match) {
    const chunkIndex = match[1]
    const rawValue = match[2]
    const value = sanitizeToken(rawValue || '')
    if (value) {
      if (chunkIndex !== undefined && chunkIndex !== '') {
        const idx = Number.parseInt(chunkIndex, 10)
        if (Number.isInteger(idx) && idx >= 0) {
          chunkMatches.push({ index: idx, value REDACTED)
        REDACTED
      REDACTED else {
        singleValues.push(value)
      REDACTED
    REDACTED
    match = sessionCookieRegex.exec(raw)
  REDACTED

  const mergedChunkToken = mergeLatestChunkedSessionToken(chunkMatches)
  if (mergedChunkToken) {
    addUnique(sessionTokens, sessionSeen, mergedChunkToken)
  REDACTED

  for (const value of singleValues) {
    addUnique(sessionTokens, sessionSeen, value)
  REDACTED
REDACTED

function mergeChunkSegment(
  chunks: SessionCookieChunk[],
  requiredMaxIndex: number,
  requireComplete: boolean
): string {
  if (chunks.length === 0) {
    return ''
  REDACTED

  const byIndex = new Map<number, string>()
  for (const chunk of chunks) {
    byIndex.set(chunk.index, chunk.value)
  REDACTED

  if (!byIndex.has(0)) {
    return ''
  REDACTED
  if (requireComplete) {
    for (let i = 0; i <= requiredMaxIndex; i++) {
      if (!byIndex.has(i)) {
        return ''
      REDACTED
    REDACTED
  REDACTED

  const orderedIndexes = Array.from(byIndex.keys()).sort((a, b) => a - b)
  return orderedIndexes.map((idx) => byIndex.get(idx) || '').join('')
REDACTED

function mergeLatestChunkedSessionToken(chunks: SessionCookieChunk[]): string {
  if (chunks.length === 0) {
    return ''
  REDACTED

  const requiredMaxIndex = chunks.reduce((max, chunk) => Math.max(max, chunk.index), 0)

  const groupStarts: number[] = []
  chunks.forEach((chunk, idx) => {
    if (chunk.index === 0) {
      groupStarts.push(idx)
    REDACTED
  REDACTED)

  if (groupStarts.length === 0) {
    return mergeChunkSegment(chunks, requiredMaxIndex, false)
  REDACTED

  for (let i = groupStarts.length - 1; i >= 0; i--) {
    const start = groupStarts[i]
    const end = i + 1 < groupStarts.length ? groupStarts[i + 1] : chunks.length
    const merged = mergeChunkSegment(chunks.slice(start, end), requiredMaxIndex, true)
    if (merged) {
      return merged
    REDACTED
  REDACTED

  return mergeChunkSegment(chunks, requiredMaxIndex, false)
REDACTED

function collectPlainLines(
  raw: string,
  sessionTokens: string[],
  sessionSeen: Set<string>,
  accessTokens: string[],
  accessSeen: Set<string>
): void {
  const lines = raw
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)

  for (const line of lines) {
    const normalized = line.toLowerCase()
    if (ignoredPlainLines.has(normalized)) {
      continue
    REDACTED
    if (/^__secure-(next-auth|authjs)\.session-token(\.\d+)?=/i.test(line)) {
      continue
    REDACTED
    if (line.includes(';')) {
      continue
    REDACTED

    if (/^[a-zA-Z_][a-zA-Z0-9_]*=/.test(line)) {
      const parts = line.split('=', 2)
      const key = parts[0]?.trim().toLowerCase()
      const value = parts[1]?.trim() || ''
      if (key && sessionKeyNames.has(key)) {
        addUnique(sessionTokens, sessionSeen, value)
        continue
      REDACTED
      if (key && accessKeyNames.has(key)) {
        addUnique(accessTokens, accessSeen, value)
        continue
      REDACTED
    REDACTED

    if (line.includes('{') || line.includes('REDACTED') || line.includes(':') || /\s/.test(line)) {
      continue
    REDACTED

    if (isLikelyJWT(line)) {
      addUnique(accessTokens, accessSeen, line)
      continue
    REDACTED
    addUnique(sessionTokens, sessionSeen, line)
  REDACTED
REDACTED

export function parseSoraRawTokens(rawInput: string): ParsedSoraTokens {
  const raw = rawInput.trim()
  if (!raw) {
    return {
      sessionTokens: [],
      accessTokens: []
    REDACTED
  REDACTED

  const sessionTokens: string[] = []
  const accessTokens: string[] = []
  const sessionSeen = new Set<string>()
  const accessSeen = new Set<string>()

  collectFromJSONString(raw, sessionTokens, sessionSeen, accessTokens, accessSeen)
  collectByRegex(raw, sessionRegexes, sessionTokens, sessionSeen)
  collectByRegex(raw, accessRegexes, accessTokens, accessSeen)
  collectFromSessionCookies(raw, sessionTokens, sessionSeen)
  collectPlainLines(raw, sessionTokens, sessionSeen, accessTokens, accessSeen)

  return {
    sessionTokens,
    accessTokens
  REDACTED
REDACTED
