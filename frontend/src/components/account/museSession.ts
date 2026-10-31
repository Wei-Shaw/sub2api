export function parseMuseSessionDocument(raw: string) {
  if (new TextEncoder().encode(raw).byteLength > 64 * 1024) throw new Error('Session document exceeds 64 KiB')
  let bundle: unknown
  try { bundle = JSON.parse(raw) } catch { throw new Error('Session document must be valid JSON') }
  if (!bundle || typeof bundle !== 'object' || Array.isArray(bundle) || Object.keys(bundle).length === 0) {
    throw new Error('Session document must be a nonempty JSON object')
  }
  return bundle
}

export function buildMuseSession(ownerId: number, raw: string, replacement = false) {
  if (!Number.isSafeInteger(ownerId) || ownerId <= 0) throw new Error('Choose a Sub2API user')
  const credentials: Record<string, unknown> = {}
  if (raw.trim()) {
    credentials.muse_session = parseMuseSessionDocument(raw)
  } else if (!replacement) throw new Error('Connect Muse or import session data to continue')
  return { credentials, extra: { muse_owner_user_id: ownerId } }
}


// The direct helper handoff is deliberately narrower than the advanced legacy
// importer: only the four Muse auth cookies and their expiries can cross it.
export function parseMuseConnectSession(raw: string) {
  const bundle = parseMuseSessionDocument(raw) as Record<string, unknown>
  const allowed = ['hatch_sess', 'hatch_gw', 'hatch_vml', 'hatch_native_auth_device']
  const cookies = bundle.cookies
  if (Object.keys(bundle).some(k => k !== 'cookies' && k !== 'expires') || !cookies || typeof cookies !== 'object' || Array.isArray(cookies)) throw new Error('Invalid Muse connection')
  const values = cookies as Record<string, unknown>
  if (Object.keys(values).some(k => !allowed.includes(k)) || allowed.some(k => k !== 'hatch_vml' && !values[k]) ||
      Object.values(values).some(v => typeof v !== 'string' || !v || v.length > 16 * 1024 || Array.from(v).some(c => c.charCodeAt(0) <= 32 || c.charCodeAt(0) === 127 || c === ';'))) throw new Error('Invalid Muse connection')
  if (bundle.expires !== undefined) {
    if (!bundle.expires || typeof bundle.expires !== 'object' || Array.isArray(bundle.expires)) throw new Error('Invalid Muse connection')
    for (const [key, value] of Object.entries(bundle.expires)) {
      if (!allowed.includes(key) || typeof value !== 'number' || !Number.isSafeInteger(value) || value * 1000 <= Date.now()) throw new Error('Expired Muse connection')
    }
  }
  return bundle
}
