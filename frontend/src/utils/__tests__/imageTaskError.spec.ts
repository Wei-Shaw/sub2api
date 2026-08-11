import { describe, expect, it } from 'vitest'
import { parseImageTaskError } from '@/utils/imageTaskError'

describe('parseImageTaskError', () => {
  it('extracts image policy details without losing the raw provider response', () => {
    const error = {
      type: 'image_generation_user_error',
      code: 'content_policy_violation',
      message: 'The prompt may contain sexual content or nudity.',
    }

    const result = parseImageTaskError(error, 'fallback')

    expect(result.type).toBe('image_generation_user_error')
    expect(result.code).toBe('content_policy_violation')
    expect(result.message).toContain('sexual content')
    expect(result.raw).toContain('content_policy_violation')
    expect(result.isContentPolicy).toBe(true)
  })

  it('supports nested gateway errors and safe fallbacks', () => {
    expect(parseImageTaskError({ error: { message: 'nested failure' } }, 'fallback').message)
      .toBe('nested failure')
    expect(parseImageTaskError(null, 'fallback').message).toBe('fallback')
  })
})
