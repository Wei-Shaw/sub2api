import { describe, expect, it } from 'vitest'
import {
  AIImageError,
  isImagePermissionError,
  isPersistableImageUrl,
  toDisplayUrl,
} from '../aiImage'

describe('toDisplayUrl', () => {
  it('prefers base64 when the gateway returned b64_json', () => {
    expect(toDisplayUrl({ b64_json: 'AAAA' })).toBe('data:image/png;base64,AAAA')
  })

  it('falls back to the url field', () => {
    expect(toDisplayUrl({ url: 'https://cdn.example/a.png' })).toBe('https://cdn.example/a.png')
  })

  it('returns an empty string when neither field is present', () => {
    expect(toDisplayUrl({})).toBe('')
  })
})

describe('isPersistableImageUrl', () => {
  it('keeps remote urls', () => {
    expect(isPersistableImageUrl('https://cdn.example/a.png')).toBe(true)
  })

  it('rejects base64 data urls so localStorage cannot overflow', () => {
    expect(isPersistableImageUrl('data:image/png;base64,AAAA')).toBe(false)
  })

  it('rejects empty values', () => {
    expect(isPersistableImageUrl('')).toBe(false)
  })
})

describe('isImagePermissionError', () => {
  it('treats a 403 as "this key is not an image model"', () => {
    expect(isImagePermissionError(new AIImageError('forbidden', 'permission_error', 403))).toBe(true)
  })

  it('recognises the gateway image-generation permission message', () => {
    expect(
      isImagePermissionError(
        new AIImageError('Image generation is not allowed for this group', 'invalid_request_error', 400),
      ),
    ).toBe(true)
  })

  it('does not swallow unrelated failures', () => {
    expect(isImagePermissionError(new AIImageError('upstream exploded', 'api_error', 502))).toBe(false)
    expect(isImagePermissionError(new Error('plain error'))).toBe(false)
  })
})
