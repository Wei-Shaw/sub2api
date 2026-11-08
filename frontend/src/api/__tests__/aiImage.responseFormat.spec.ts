import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AIImageError, generateAIImage } from '../aiImage'

/**
 * 这一组盯的是「图片怎么拿到手」，2026-10-16 改过一次方向：
 *
 * **以前**：只走同步接口，显式要 `response_format=url`。
 * 问题是这条通道只给得出 base64，网关只能现造一个 `data:...;base64,...` 的假网址，
 * 前端为了不撑爆 localStorage 必须丢掉它 → **刷新后图片就没了**。
 *
 * **现在**：优先走**异步**接口。平台会把每张图转存进对象存储，
 * 返回 `https://img.chenshuapi.com/...` 这种真网址，刷新后还在；
 * 任务记录的 TTL 是 24 小时（「存一天」就是这么来的）。
 *
 * 兜底：服务端没开对象存储时异步接口回 404，这时自动退回同步通道，
 * 功能不会整个不可用 —— 这条兜底必须一直有，否则换个部署环境就废了。
 */

const fetchMock = vi.fn()

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: String(status),
    json: async () => body,
  } as unknown as Response
}

function sentBody(callIndex = 0): Record<string, unknown> {
  const init = fetchMock.mock.calls[callIndex][1] as RequestInit
  return JSON.parse(String(init.body)) as Record<string, unknown>
}

function sentUrl(callIndex = 0): string {
  return String(fetchMock.mock.calls[callIndex][0])
}

/** 服务端没开对象存储时的固定响应。 */
function asyncDisabledResponse() {
  return jsonResponse(
    { error: { type: 'not_found_error', code: 'not_found_error', message: 'async image tasks are not enabled' } },
    404,
  )
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const baseRequest = { apiKey: 'sk-test', prompt: '白色背景主图' }

describe('generateAIImage 走异步通道（图片存平台对象存储）', () => {
  it('先提交异步任务，轮询到完成后返回对象存储的网址', async () => {
    vi.useFakeTimers()
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({ task_id: 't1', status: 'processing', poll_url: '/v1/images/tasks/t1' }, 202),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          task_id: 't1',
          status: 'completed',
          result: { data: [{ url: 'https://img.chenshuapi.com/images/a.png' }] },
        }),
      )

    const pending = generateAIImage(baseRequest)
    await vi.advanceTimersByTimeAsync(3000)
    const result = await pending

    expect(sentUrl(0)).toContain('/v1/images/generations/async')
    // 异步通道下固定要 b64_json：平台会先把图转存进桶，再改写成桶的网址，
    // 少一次「网关现造 data: 假网址」的中间态。
    expect(sentBody(0).response_format).toBe('b64_json')
    expect(sentUrl(1)).toContain('/v1/images/tasks/t1')
    expect(result.data[0].url).toBe('https://img.chenshuapi.com/images/a.png')
  })

  it('任务还没完成就继续轮询，不会提前当成功', async () => {
    vi.useFakeTimers()
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ task_id: 't2', poll_url: '/v1/images/tasks/t2' }, 202))
      .mockResolvedValueOnce(jsonResponse({ task_id: 't2', status: 'processing' }))
      .mockResolvedValueOnce(jsonResponse({ task_id: 't2', status: 'processing' }))
      .mockResolvedValueOnce(
        jsonResponse({ task_id: 't2', status: 'completed', result: { data: [{ url: 'https://img.chenshuapi.com/b.png' }] } }),
      )

    const pending = generateAIImage(baseRequest)
    await vi.advanceTimersByTimeAsync(3000 * 4)
    const result = await pending

    expect(fetchMock).toHaveBeenCalledTimes(4)
    expect(result.data[0].url).toBe('https://img.chenshuapi.com/b.png')
  })

  it('任务失败时把上游的原因抛出来，而不是干等', async () => {
    vi.useFakeTimers()
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ task_id: 't3', poll_url: '/v1/images/tasks/t3' }, 202))
      .mockResolvedValueOnce(
        jsonResponse({
          task_id: 't3',
          status: 'failed',
          http_status: 502,
          error: { type: 'api_error', code: 'api_error', message: 'upstream returned 502' },
        }),
      )

    const pending = generateAIImage(baseRequest)
    const assertion = expect(pending).rejects.toMatchObject({ message: 'upstream returned 502' })
    await vi.advanceTimersByTimeAsync(3000)
    await assertion
  })

  it('带参考图时走 edits 的异步接口，并且是 multipart', async () => {
    vi.useFakeTimers()
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ task_id: 't4', poll_url: '/v1/images/tasks/t4' }, 202))
      .mockResolvedValueOnce(
        jsonResponse({ task_id: 't4', status: 'completed', result: { data: [{ url: 'https://img.chenshuapi.com/c.png' }] } }),
      )
    const file = new File([new Uint8Array([1])], 'p.png', { type: 'image/png' })

    const pending = generateAIImage({ ...baseRequest, referenceImage: file })
    await vi.advanceTimersByTimeAsync(3000)
    const result = await pending

    expect(sentUrl(0)).toContain('/v1/images/edits/async')
    const form = fetchMock.mock.calls[0][1]?.body as FormData
    expect(form.get('model')).toBe('gpt-image-2')
    expect(form.get('response_format')).toBe('b64_json')
    expect(result.data[0].url).toBe('https://img.chenshuapi.com/c.png')
  })

  it('结果里没有 data 时，至少用 image_url 兜一张，别让用户白等', async () => {
    vi.useFakeTimers()
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ task_id: 't5', poll_url: '/v1/images/tasks/t5' }, 202))
      .mockResolvedValueOnce(
        jsonResponse({ task_id: 't5', status: 'completed', image_url: 'https://img.chenshuapi.com/d.png' }),
      )

    const pending = generateAIImage(baseRequest)
    await vi.advanceTimersByTimeAsync(3000)
    const result = await pending

    expect(result.data[0].url).toBe('https://img.chenshuapi.com/d.png')
  })

  it('提交时没返回任务号就报错，不会拿一个空任务号去轮询', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ status: 'processing' }, 202))

    await expect(generateAIImage(baseRequest)).rejects.toMatchObject({ code: 'parse_error' })
  })
})

describe('generateAIImage 退回同步通道（服务端没开对象存储时）', () => {
  it('异步接口报「未启用」时，自动退回同步，功能不会整个不可用', async () => {
    fetchMock
      .mockResolvedValueOnce(asyncDisabledResponse())
      .mockResolvedValueOnce(jsonResponse({ data: [{ url: 'https://cdn.example/sync.png' }] }))

    const result = await generateAIImage(baseRequest)

    expect(sentUrl(0)).toContain('/v1/images/generations/async')
    expect(sentUrl(1)).toContain('/v1/images/generations')
    expect(sentUrl(1)).not.toContain('/async')
    // 同步通道才要 url：这条路上「网关不回填 base64」才拿得到可下载地址。
    expect(sentBody(1).response_format).toBe('url')
    expect(result.data[0].url).toBe('https://cdn.example/sync.png')
  })

  it('同步通道里账号不具备 url 能力时，再退一步要 base64', async () => {
    fetchMock
      .mockResolvedValueOnce(asyncDisabledResponse())
      .mockResolvedValueOnce(
        jsonResponse({ error: { message: 'no available accounts for this capability', type: 'api_error' } }, 503),
      )
      .mockResolvedValueOnce(jsonResponse({ data: [{ b64_json: 'AAAA' }] }))

    const result = await generateAIImage(baseRequest)

    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(sentBody(1).response_format).toBe('url')
    expect(sentBody(2).response_format).toBe('b64_json')
    expect(result.data[0].b64_json).toBe('AAAA')
  })

  it('同步通道降级后仍然失败时，把真正的错误抛出去', async () => {
    fetchMock
      .mockResolvedValueOnce(asyncDisabledResponse())
      .mockResolvedValueOnce(jsonResponse({ error: { message: 'no available accounts' } }, 503))
      .mockResolvedValueOnce(jsonResponse({ error: { message: 'still broken' } }, 502))

    await expect(generateAIImage(baseRequest)).rejects.toMatchObject({ status: 502 })
  })
})

describe('generateAIImage 不该乱重试', () => {
  it('异步接口报无关的 404 时不退回同步（不是「没启用」就别乱猜）', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ error: { message: 'model not found' } }, 404))

    await expect(generateAIImage(baseRequest)).rejects.toBeInstanceOf(AIImageError)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('内容违规不重试，只请求一次', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ error: { message: 'prompt violates content policy', type: 'invalid_request_error' } }, 400),
    )

    await expect(generateAIImage(baseRequest)).rejects.toBeInstanceOf(AIImageError)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
