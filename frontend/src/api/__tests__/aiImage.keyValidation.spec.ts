import { beforeEach, describe, expect, it, vi } from 'vitest'
import { validateAIImageKey, validateVisionKey } from '../aiImage'
import type { ApiKey } from '@/types'

const { list } = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../keys', () => ({ keysAPI: { list } }))

function makeKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 1,
    user_id: 7,
    key: 'sk-image-key',
    name: 'image key',
    group_id: 3,
    status: 'active',
    group: {
      id: 3,
      name: '生图分组',
      allow_image_generation: true,
    },
    ...overrides,
  } as ApiKey
}

function page(items: ApiKey[]) {
  return { items, total: items.length, page: 1, page_size: 100, pages: 1 }
}

describe('validateAIImageKey', () => {
  beforeEach(() => {
    list.mockReset()
  })

  it('accepts a key whose group allows image generation', async () => {
    list.mockResolvedValue(page([makeKey()]))

    const result = await validateAIImageKey('sk-image-key')

    expect(result).toEqual({ ok: true, groupName: '生图分组' })
  })

  it('rejects a key whose group has image generation disabled', async () => {
    list.mockResolvedValue(
      page([
        makeKey({
          group: { id: 4, name: '纯文本分组', allow_image_generation: false } as ApiKey['group'],
        }),
      ]),
    )

    const result = await validateAIImageKey('sk-image-key')

    expect(result.ok).toBe(false)
    expect(result.reason).toBe('not-image-model')
    expect(result.groupName).toBe('纯文本分组')
  })

  it('reports not-owned when the key is not in the current account key list', async () => {
    list.mockResolvedValue(page([makeKey({ key: 'sk-other-key' })]))

    const result = await validateAIImageKey('sk-unknown-key')

    expect(result).toEqual({ ok: false, reason: 'not-owned' })
  })

  it('reports invalid when the matching key is not active', async () => {
    list.mockResolvedValue(page([makeKey({ status: 'inactive' })]))

    const result = await validateAIImageKey('sk-image-key')

    expect(result.ok).toBe(false)
    expect(result.reason).toBe('invalid')
  })

  it('reports invalid for an empty key without calling the API', async () => {
    const result = await validateAIImageKey('   ')

    expect(result).toEqual({ ok: false, reason: 'invalid' })
    expect(list).not.toHaveBeenCalled()
  })

  it('reports load-failed when the key list cannot be read', async () => {
    list.mockRejectedValue(new Error('network down'))

    const result = await validateAIImageKey('sk-image-key')

    expect(result).toEqual({ ok: false, reason: 'load-failed' })
  })

  it('walks every page of the key list', async () => {
    list
      .mockResolvedValueOnce({ items: [makeKey({ id: 1, key: 'sk-a' })], pages: 2, page: 1 })
      .mockResolvedValueOnce({ items: [makeKey({ id: 2, key: 'sk-b' })], pages: 2, page: 2 })

    const result = await validateAIImageKey('sk-b')

    expect(list).toHaveBeenCalledTimes(2)
    expect(result).toEqual({ ok: true, groupName: '生图分组' })
  })
})

/**
 * 视觉密钥的校验规则和生图密钥**故意不同**：
 * 视觉模型挂在对话渠道上，那个分组本来就不会开 allow_image_generation。
 * 如果照搬生图密钥的规则，用户填一把完全正确的 Gemini 密钥也会被判「不是生图模型」。
 */
describe('validateVisionKey', () => {
  beforeEach(() => {
    list.mockReset()
  })

  it('接受一把所在分组没开生图能力的密钥（这正是视觉模型密钥的常态）', async () => {
    list.mockResolvedValue(
      page([
        makeKey({
          key: 'sk-vision-key',
          group: { id: 9, name: 'Gemini 分组', allow_image_generation: false } as ApiKey['group'],
        }),
      ]),
    )

    const result = await validateVisionKey('sk-vision-key')

    expect(result).toEqual({ ok: true, groupName: 'Gemini 分组' })
  })

  it('仍然要求密钥属于当前账号', async () => {
    list.mockResolvedValue(page([makeKey({ key: 'sk-other-key' })]))

    const result = await validateVisionKey('sk-unknown-key')

    expect(result).toEqual({ ok: false, reason: 'not-owned' })
  })

  it('仍然要求密钥处于启用状态', async () => {
    list.mockResolvedValue(page([makeKey({ key: 'sk-vision-key', status: 'inactive' })]))

    const result = await validateVisionKey('sk-vision-key')

    expect(result.ok).toBe(false)
    expect(result.reason).toBe('invalid')
  })

  it('空密钥直接判无效，不打接口', async () => {
    const result = await validateVisionKey('   ')

    expect(result).toEqual({ ok: false, reason: 'invalid' })
    expect(list).not.toHaveBeenCalled()
  })

  it('密钥列表读不出来时报 load-failed', async () => {
    list.mockRejectedValue(new Error('network down'))

    const result = await validateVisionKey('sk-vision-key')

    expect(result).toEqual({ ok: false, reason: 'load-failed' })
  })
})
