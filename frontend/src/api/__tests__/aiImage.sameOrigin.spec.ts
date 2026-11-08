import { describe, expect, it } from 'vitest'
import { fetchImageBlob, toSameOriginImageUrl } from '../aiImage'

/**
 * 2026-10-24 主人报「水印在下载时没有加上」。
 *
 * 根因：图片在 img.chenshuapi.com，和页面不同源且没配 CORS，
 * fetch 读不到字节 → canvas 也拿不到像素 → 水印根本画不上去。
 *
 * 修法：把图片地址改写成同源的 /img-proxy/...（Caddy 反代到图片域名），
 * 同源取字节就不受 CORS 限制。这几条测试守住改写逻辑。
 */
describe('toSameOriginImageUrl', () => {
  it('把图片域名的直链改写成 /img-proxy 同源路径', () => {
    expect(toSameOriginImageUrl('https://img.chenshuapi.com/images/a-0.png')).toBe('/img-proxy/images/a-0.png')
  })

  it('保留查询参数（有些图带签名/时间戳）', () => {
    expect(toSameOriginImageUrl('https://img.chenshuapi.com/images/a-0.png?t=123&s=x')).toBe(
      '/img-proxy/images/a-0.png?t=123&s=x',
    )
  })

  it('本来就同源的地址不动', () => {
    const sameOrigin = `${window.location.origin}/images/a-0.png`
    expect(toSameOriginImageUrl(sameOrigin)).toBe(sameOrigin)
  })

  it('data: 和 blob: 原样返回（本来就不跨域）', () => {
    expect(toSameOriginImageUrl('data:image/png;base64,AAAA')).toBe('data:image/png;base64,AAAA')
    expect(toSameOriginImageUrl('blob:https://chenshuapi.com/abc')).toBe('blob:https://chenshuapi.com/abc')
  })

  it('空字符串不炸', () => {
    expect(toSameOriginImageUrl('')).toBe('')
  })
})

describe('fetchImageBlob', () => {
  it('反代成功就用反代的字节，不再去打直连', async () => {
    const originalFetch = globalThis.fetch
    const calls: string[] = []
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      calls.push(String(input))
      return new Response(new Uint8Array([1, 2, 3]), { status: 200, headers: { 'content-type': 'image/png' } })
    }) as typeof fetch

    try {
      const blob = await fetchImageBlob('https://img.chenshuapi.com/images/a-0.png')
      expect(blob.size).toBe(3)
      expect(calls).toEqual(['/img-proxy/images/a-0.png'])
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it('反代失败要退到直连（万一以后图片域名配了 CORS）', async () => {
    const originalFetch = globalThis.fetch
    const calls: string[] = []
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = String(input)
      calls.push(url)
      if (url.startsWith('/img-proxy')) throw new TypeError('Failed to fetch')
      return new Response(new Uint8Array([9]), { status: 200, headers: { 'content-type': 'image/png' } })
    }) as typeof fetch

    try {
      const blob = await fetchImageBlob('https://img.chenshuapi.com/images/a-0.png')
      expect(blob.size).toBe(1)
      expect(calls).toEqual(['/img-proxy/images/a-0.png', 'https://img.chenshuapi.com/images/a-0.png'])
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it('两边都失败就抛错，让调用方去走「直接下载」兜底', async () => {
    const originalFetch = globalThis.fetch
    globalThis.fetch = (async () => {
      throw new TypeError('Failed to fetch')
    }) as typeof fetch

    try {
      await expect(fetchImageBlob('https://img.chenshuapi.com/images/a-0.png')).rejects.toThrow()
    } finally {
      globalThis.fetch = originalFetch
    }
  })
})
