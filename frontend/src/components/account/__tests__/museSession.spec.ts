import { describe, expect, it } from 'vitest'
import { buildMuseSession } from '../museSession'

describe('Muse session account payload', () => {
  it('does not expose malformed credential contents in errors', () => {
    expect(() => buildMuseSession(12, 'synthetic-private-value')).toThrow('Session document must be valid JSON')
  })
  it('retains an opaque document and assigns one local owner', () => {
    expect(buildMuseSession(12, '{"opaque":"fixture"}')).toEqual({ credentials: { muse_session: { opaque: 'fixture' } }, extra: { muse_owner_user_id: 12 } })
  })
  it('never fills a redacted secret when editing without a replacement', () => {
    expect(buildMuseSession(12, '', true).credentials).toEqual({})
  })
  it('rejects invalid owners and malformed or empty session documents', () => {
    for (const owner of [0, -1, 1.2, Number.NaN]) expect(() => buildMuseSession(owner, '{"opaque":"fixture"}')).toThrow()
    for (const raw of ['', 'null', '[]', '{}', 'invalid']) expect(() => buildMuseSession(12, raw)).toThrow()
  })
})
