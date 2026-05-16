import type { ProxyIPVersion, ProxyProtocol } from '@/types'

export interface ParsedProxyAddress {
  protocol: ProxyProtocol
  host: string
  ip_version: ProxyIPVersion
  port: number
  username: string
  password: string
}

export function normalizeProxyHost(host: string): string {
  const trimmed = host.trim()
  if (!trimmed) return ''
  if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
    const inner = trimmed.slice(1, -1).trim()
    if (inner.includes(':')) return inner
  }
  return trimmed
}

export function detectProxyIPVersion(host: string): ProxyIPVersion {
  return normalizeProxyHost(host).includes(':') ? 'ipv6' : 'ipv4'
}

export function formatProxyHost(host: string): string {
  const normalized = normalizeProxyHost(host)
  if (!normalized) return ''
  return normalized.includes(':') ? `[${normalized}]` : normalized
}

export function formatProxyAddress(host: string, port: number): string {
  return `${formatProxyHost(host)}:${port}`
}

export function buildProxyUrl(row: {
  protocol: ProxyProtocol
  host: string
  port: number
  username?: string | null
  password?: string | null
}): string {
  const user = row.username ? encodeURIComponent(row.username) : ''
  const pass = row.password ? encodeURIComponent(row.password) : ''
  let auth = ''
  if (user && pass) auth = `${user}:${pass}@`
  else if (user) auth = `${user}@`
  else if (pass) auth = `:${pass}@`
  return `${row.protocol}://${auth}${formatProxyAddress(row.host, row.port)}`
}

export function parseProxyUrl(line: string): ParsedProxyAddress | null {
  const trimmed = line.trim()
  if (!trimmed) return null

  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return null
  }

  const protocol = parsed.protocol.replace(/:$/, '').toLowerCase()
  if (!['http', 'https', 'socks5', 'socks5h'].includes(protocol)) {
    return null
  }

  const port = Number(parsed.port)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    return null
  }

  const host = normalizeProxyHost(parsed.hostname)
  if (!host) return null

  return {
    protocol: protocol as ProxyProtocol,
    host,
    ip_version: detectProxyIPVersion(host),
    port,
    username: parsed.username ? decodeURIComponent(parsed.username) : '',
    password: parsed.password ? decodeURIComponent(parsed.password) : ''
  }
}
