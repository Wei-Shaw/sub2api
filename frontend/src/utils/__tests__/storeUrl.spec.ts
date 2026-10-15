import { describe, expect, it } from 'vitest'
import { buildStoreUrl } from '../storeUrl'

describe('buildStoreUrl', () => {
  const baseUrl = 'https://www.gptplusch.store/products?category=other&filter=codex-token&embed=1'

  it('将当前用户邮箱写入 URL Fragment', () => {
    expect(buildStoreUrl(baseUrl, ' buyer@example.com ')).toBe(
      `${baseUrl}#email=buyer%40example.com`
    )
  })

  it('不传邮箱时保持商城地址不变', () => {
    expect(buildStoreUrl(baseUrl)).toBe(baseUrl)
  })

  it('保留已有 Fragment 参数', () => {
    expect(buildStoreUrl(`${baseUrl}#source=sub2api`, 'buyer@example.com')).toBe(
      `${baseUrl}#source=sub2api&email=buyer%40example.com`
    )
  })
})
