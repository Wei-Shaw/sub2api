export function buildMuseSession(ownerId: number, raw: string, replacement = false) {
  if (!Number.isSafeInteger(ownerId) || ownerId <= 0) throw new Error('Assign a valid Sub2API user ID')
  const credentials: Record<string, unknown> = {}
  if (raw.trim()) {
    let bundle: unknown
    try { bundle = JSON.parse(raw) } catch { throw new Error('Session document must be valid JSON') }
    if (!bundle || typeof bundle !== 'object' || Array.isArray(bundle) || Object.keys(bundle).length === 0) {
      throw new Error('Session document must be a nonempty JSON object')
    }
    if (new TextEncoder().encode(raw).byteLength > 64 * 1024) throw new Error('Session document exceeds 64 KiB')
    credentials.muse_session = bundle
  } else if (!replacement) throw new Error('Enter the authorized Muse app session document')
  return { credentials, extra: { muse_owner_user_id: ownerId } }
}
