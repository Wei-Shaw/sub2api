import { reactive REDACTED from 'vue'

export type IpGeoStatus = 'idle' | 'loading' | 'success' | 'error' | 'private'

export interface IpGeoDetail {
  countryCode?: string
  region?: string
  city?: string
  organization?: string
  timezone?: string
  accuracy?: number
  latitude?: string
  longitude?: string
REDACTED

export interface IpGeoEntry {
  status: IpGeoStatus
  label?: string
  detail?: IpGeoDetail
  fetchedAt?: number
REDACTED

const IDLE_ENTRY: IpGeoEntry = { status: 'idle' REDACTED
const CACHE_STORAGE_KEY = 'sub2api:ip-geo-cache:v1'
const CACHE_TTL_MS = 24 * 60 * 60 * 1000
const BATCH_CHUNK_SIZE = 50
const GEO_SINGLE_URL = 'https://get.geojs.io/v1/ip/geo'
const GEO_BATCH_URL = 'https://get.geojs.io/v1/ip/geo.json'

interface StoredEntry {
  label: string
  detail?: IpGeoDetail
  fetchedAt: number
REDACTED

const cache = reactive(new Map<string, IpGeoEntry>())

function isFreshSuccess(entry: IpGeoEntry | undefined): entry is IpGeoEntry & { status: 'success'; fetchedAt: number REDACTED {
  return entry?.status === 'success' && typeof entry.fetchedAt === 'number' && Date.now() - entry.fetchedAt <= CACHE_TTL_MS
REDACTED

function getFreshEntry(ip: string): IpGeoEntry | undefined {
  const entry = cache.get(ip)
  if (entry?.status === 'success' && !isFreshSuccess(entry)) {
    cache.delete(ip)
    persistToStorage()
    return undefined
  REDACTED
  return entry
REDACTED

export function isPrivateIp(ip: string): boolean {
  const v4 = ip.match(/^(\d{1,3REDACTED)\.(\d{1,3REDACTED)\.(\d{1,3REDACTED)\.(\d{1,3REDACTED)$/)
  if (v4) {
    const a = Number(v4[1])
    const b = Number(v4[2])
    if (a === 10) return true
    if (a === 127) return true
    if (a === 169 && b === 254) return true
    if (a === 172 && b >= 16 && b <= 31) return true
    if (a === 192 && b === 168) return true
    return false
  REDACTED
  const lower = ip.toLowerCase()
  if (lower === '::1') return true
  const firstSegment = lower.split(':', 1)[0]
  if (/^fe[89ab][0-9a-f]$/.test(firstSegment)) return true
  if (/^f[cd][0-9a-f]{2REDACTED$/.test(firstSegment)) return true
  return false
REDACTED

function loadFromStorage(): void {
  try {
    const raw = localStorage.getItem(CACHE_STORAGE_KEY)
    if (!raw) return
    const parsed = JSON.parse(raw) as Record<string, StoredEntry>
    const now = Date.now()
    for (const [ip, stored] of Object.entries(parsed)) {
      if (!stored || typeof stored.fetchedAt !== 'number') continue
      if (now - stored.fetchedAt > CACHE_TTL_MS) continue
      cache.set(ip, { status: 'success', label: stored.label, detail: stored.detail, fetchedAt: stored.fetchedAt REDACTED)
    REDACTED
  REDACTED catch {
    // 忽略损坏的本地缓存
  REDACTED
REDACTED

function persistToStorage(): void {
  try {
    const toStore: Record<string, StoredEntry> = {REDACTED
    for (const [ip, entry] of cache.entries()) {
      if (entry.status === 'success' && entry.label && entry.fetchedAt) {
        toStore[ip] = { label: entry.label, detail: entry.detail, fetchedAt: entry.fetchedAt REDACTED
      REDACTED
    REDACTED
    localStorage.setItem(CACHE_STORAGE_KEY, JSON.stringify(toStore))
  REDACTED catch {
    // 存储写入失败（如隐私模式禁用 localStorage）不影响功能
  REDACTED
REDACTED

loadFromStorage()

export function getEntry(ip: string): IpGeoEntry {
  return getFreshEntry(ip) ?? IDLE_ENTRY
REDACTED

export function formatGeoLabel(detail: IpGeoDetail): string {
  const parts = [detail.countryCode, detail.region, detail.city].filter(
    (part): part is string => Boolean(part && part.trim())
  )
  return parts.join(' · ')
REDACTED

interface RawGeoResponse {
  ip: string
  country_code?: string
  region?: string
  city?: string
  organization?: string
  timezone?: string
  accuracy?: number
  latitude?: string
  longitude?: string
REDACTED

function toDetail(raw: RawGeoResponse): IpGeoDetail {
  return {
    countryCode: raw.country_code,
    region: raw.region,
    city: raw.city,
    organization: raw.organization,
    timezone: raw.timezone,
    accuracy: raw.accuracy,
    latitude: raw.latitude,
    longitude: raw.longitude,
  REDACTED
REDACTED

function applyResult(ip: string, raw: RawGeoResponse | undefined): void {
  if (!raw || !raw.country_code) {
    cache.set(ip, { status: 'error' REDACTED)
    return
  REDACTED
  const detail = toDetail(raw)
  cache.set(ip, {
    status: 'success',
    label: formatGeoLabel(detail),
    detail,
    fetchedAt: Date.now(),
  REDACTED)
REDACTED

export async function fetchOne(ip: string, force = false): Promise<void> {
  if (isPrivateIp(ip)) {
    cache.set(ip, { status: 'private' REDACTED)
    return
  REDACTED
  const existing = getFreshEntry(ip)
  if (!force && (isFreshSuccess(existing) || existing?.status === 'loading')) {
    return
  REDACTED
  cache.set(ip, { status: 'loading' REDACTED)
  try {
    const response = await fetch(`${GEO_SINGLE_URLREDACTED/${encodeURIComponent(ip)REDACTED.json`)
    if (!response.ok) {
      cache.set(ip, { status: 'error' REDACTED)
      return
    REDACTED
    const raw = (await response.json()) as RawGeoResponse
    applyResult(ip, raw)
    persistToStorage()
  REDACTED catch {
    cache.set(ip, { status: 'error' REDACTED)
  REDACTED
REDACTED

export async function fetchBatch(ips: string[]): Promise<boolean> {
  const unique = Array.from(new Set(ips))
  const targets: string[] = []
  for (const ip of unique) {
    if (isPrivateIp(ip)) {
      cache.set(ip, { status: 'private' REDACTED)
      continue
    REDACTED
    const existing = cache.get(ip)
    if (isFreshSuccess(existing) || existing?.status === 'loading') continue
    targets.push(ip)
  REDACTED
  if (targets.length === 0) return true

  targets.forEach((ip) => cache.set(ip, { status: 'loading' REDACTED))

  let allChunksOk = true
  for (let i = 0; i < targets.length; i += BATCH_CHUNK_SIZE) {
    const chunk = targets.slice(i, i + BATCH_CHUNK_SIZE)
    try {
      const response = await fetch(`${GEO_BATCH_URLREDACTED?ip=${chunk.map(encodeURIComponent).join(',')REDACTED`)
      if (!response.ok) {
        chunk.forEach((ip) => cache.set(ip, { status: 'error' REDACTED))
        allChunksOk = false
        continue
      REDACTED
      const results = (await response.json()) as RawGeoResponse[]
      const byIp = new Map(results.map((r) => [r.ip, r]))
      chunk.forEach((ip) => applyResult(ip, byIp.get(ip)))
      persistToStorage()
    REDACTED catch {
      chunk.forEach((ip) => cache.set(ip, { status: 'error' REDACTED))
      allChunksOk = false
    REDACTED
  REDACTED
  return allChunksOk
REDACTED
