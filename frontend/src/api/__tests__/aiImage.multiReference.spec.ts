import { describe, expect, it, vi, afterEach } from 'vitest'
import { collectReferenceImages } from '../aiImage'

/**
 * 2026-10-26 主人定「方案 A」：爆款复刻要把「竞品图 + 本产品图」一起送给模型。
 *
 * 后端本来就支持多图（`openai_images.go` 对 `image` / `image[i]` 都 append，
 * 唯一校验只是「至少一张」），所以这里只需要前端把每张都 append 进去。
 */
function file(name: string): File {
  return new File([new Uint8Array([1, 2, 3])], name, { type: 'image/png' })
}

describe('collectReferenceImages', () => {
  it('单图写法照旧工作', () => {
    const one = file('product.png')
    expect(collectReferenceImages({ referenceImage: one })).toEqual([one])
  })

  it('多图写法按数组顺序保留', () => {
    const a = file('ref-1.png')
    const b = file('product.png')
    expect(collectReferenceImages({ referenceImages: [a, b] })).toEqual([a, b])
  })

  it('两种写法同时给时，单图排在前面（老调用兼容）', () => {
    const single = file('single.png')
    const a = file('ref-1.png')
    expect(collectReferenceImages({ referenceImage: single, referenceImages: [a] })).toEqual([single, a])
  })

  it('null / undefined 不会变成文件塞进表单', () => {
    expect(collectReferenceImages({ referenceImage: null })).toEqual([])
    expect(collectReferenceImages({})).toEqual([])
    // 数组里混进空值也要剔掉：真的把 undefined 塞进 FormData 会直接抛错
    const a = file('a.png')
    expect(collectReferenceImages({ referenceImages: [a, undefined as unknown as File] })).toEqual([a])
  })
})

describe('多图请求真的会 append 多张', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('两张参考图 → 两个 image 字段', async () => {
    const appended: Array<[string, unknown]> = []
    const realFormData = globalThis.FormData
    class RecordingFormData {
      append(key: string, value: unknown) {
        appended.push([key, value])
      }
    }
    vi.stubGlobal('FormData', RecordingFormData as unknown as typeof FormData)

    const fetchMock = vi.fn(async () => {
      // 让异步提交直接判定为「不支持」，从而走同步 edits 那条路
      return new Response(JSON.stringify({ error: { message: 'not supported' } }), { status: 404 })
    })
    vi.stubGlobal('fetch', fetchMock)

    try {
      const { generateAIImage } = await import('../aiImage')
      const a = file('ref-1.png')
      const b = file('product.png')
      await generateAIImage({
        apiKey: 'sk-test-123456',
        prompt: 'test',
        referenceImages: [a, b],
      }).catch(() => undefined)

      const imageFields = appended.filter(([key]) => key === 'image')
      // 两条路径（async + sync 兜底）都可能 append，取「至少有一次是两个」
      const counts = imageFields.length
      expect(counts).toBeGreaterThanOrEqual(2)
      expect(appended.filter(([key]) => key === 'image').map(([, value]) => value)).toContain(a)
      expect(appended.filter(([key]) => key === 'image').map(([, value]) => value)).toContain(b)
    } finally {
      vi.stubGlobal('FormData', realFormData)
    }
  })
})
