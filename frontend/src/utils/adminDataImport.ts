import type { AdminDataAccount, AdminDataPayload, AdminDataProxy, ProxyProtocol } from '@/types'

const DEFAULT_DATA_TYPE = 'sub2api-data'
const DEFAULT_DATA_VERSION = 1

type UnknownRecord = Record<string, unknown>

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined
}

function readNullableString(value: unknown): string | null | undefined {
  if (value == null) return null
  return typeof value === 'string' ? value : undefined
}

function readNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function readBoolean(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}

function readObject(value: unknown): Record<string, unknown> | undefined {
  return isRecord(value) ? value : undefined
}

function normalizeStatus(value: unknown): 'active' | 'inactive' {
  return String(value ?? 'active').trim().toLowerCase() === 'inactive' ? 'inactive' : 'active'
}

function buildProxyKey(
  protocol: string,
  host: string,
  port: number,
  username?: string | null,
  password?: string | null
): string {
  return [
    protocol.trim(),
    host.trim(),
    String(port),
    (username ?? '').trim(),
    (password ?? '').trim()
  ].join('|')
}

function normalizeProxy(raw: unknown): AdminDataProxy {
  const item = readObject(raw) ?? {}
  const protocol = readString(item.protocol) ?? ''
  const host = readString(item.host) ?? ''
  const port = readNumber(item.port) ?? 0
  const username = readNullableString(item.username)
  const password = readNullableString(item.password)
  const proxyKey =
    readString(item.proxy_key) ??
    readString(item.proxyKey) ??
    buildProxyKey(protocol, host, port, username, password)

  return {
    proxy_key: proxyKey,
    name: readString(item.name) ?? '',
    protocol: protocol as ProxyProtocol,
    host,
    port,
    username,
    password,
    status: normalizeStatus(item.status)
  }
}

function normalizeAccount(
  raw: unknown
): { account: AdminDataAccount; inlineProxy?: AdminDataProxy } {
  const item = readObject(raw) ?? {}
  const inlineProxy = readObject(item.proxy) ? normalizeProxy(item.proxy) : undefined
  const proxyKey =
    readString(item.proxy_key) ??
    readString(item.proxyKey) ??
    inlineProxy?.proxy_key ??
    null

  return {
    account: {
      name: readString(item.name) ?? '',
      notes: readNullableString(item.notes),
      platform: (readString(item.platform) ?? '') as AdminDataAccount['platform'],
      type: (readString(item.type) ?? '') as AdminDataAccount['type'],
      credentials: readObject(item.credentials) ?? {},
      extra: readObject(item.extra),
      proxy_key: proxyKey,
      concurrency: readNumber(item.concurrency) ?? 0,
      priority: readNumber(item.priority) ?? 0,
      rate_multiplier: readNumber(item.rate_multiplier ?? item.rateMultiplier) ?? null,
      expires_at: readNumber(item.expires_at ?? item.expiresAt) ?? null,
      auto_pause_on_expired: readBoolean(item.auto_pause_on_expired ?? item.autoPauseOnExpired)
    },
    inlineProxy
  }
}

function dedupeProxies(items: AdminDataProxy[]): AdminDataProxy[] {
  const proxyByKey = new Map<string, AdminDataProxy>()
  for (const item of items) {
    proxyByKey.set(item.proxy_key, item)
  }
  return [...proxyByKey.values()]
}

function normalizePayloadObject(
  source: UnknownRecord,
  exportedAt: string
): AdminDataPayload {
  const proxies = Array.isArray(source.proxies) ? source.proxies.map(normalizeProxy) : []
  const normalizedAccounts = Array.isArray(source.accounts)
    ? source.accounts.map(normalizeAccount)
    : []
  const inlineProxies = normalizedAccounts
    .map((item) => item.inlineProxy)
    .filter((item): item is AdminDataProxy => Boolean(item))

  return {
    type: readString(source.type) ?? DEFAULT_DATA_TYPE,
    version: readNumber(source.version) ?? DEFAULT_DATA_VERSION,
    exported_at: readString(source.exported_at) ?? readString(source.exportedAt) ?? exportedAt,
    proxies: dedupeProxies([...proxies, ...inlineProxies]),
    accounts: normalizedAccounts.map((item) => item.account)
  }
}

export function normalizeAccountImportPayload(
  input: unknown,
  exportedAt: string = new Date().toISOString()
): AdminDataPayload {
  if (Array.isArray(input)) {
    return normalizePayloadObject({ accounts: input }, exportedAt)
  }

  if (!isRecord(input)) {
    throw new Error('Unsupported import payload')
  }

  if (isRecord(input.data)) {
    return normalizePayloadObject(input.data, exportedAt)
  }

  if (Array.isArray(input.accounts) || Array.isArray(input.proxies)) {
    return normalizePayloadObject(input, exportedAt)
  }

  if ('name' in input && 'platform' in input && 'type' in input) {
    return normalizePayloadObject({ accounts: [input] }, exportedAt)
  }

  throw new Error('Unsupported import payload')
}

export function mergeAccountImportPayloads(
  payloads: AdminDataPayload[],
  exportedAt: string = new Date().toISOString()
): AdminDataPayload {
  return {
    type: DEFAULT_DATA_TYPE,
    version: DEFAULT_DATA_VERSION,
    exported_at: exportedAt,
    proxies: dedupeProxies(payloads.flatMap((payload) => payload.proxies)),
    accounts: payloads.flatMap((payload) => payload.accounts)
  }
}
