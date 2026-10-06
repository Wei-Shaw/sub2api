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
  } else if (!replacement) throw new Error('Import a Muse session file to continue')
  return { credentials, extra: { muse_owner_user_id: ownerId } }
}
