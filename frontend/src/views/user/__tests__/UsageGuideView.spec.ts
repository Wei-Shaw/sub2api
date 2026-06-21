import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const viewPath = resolve(currentDir, '../UsageGuideView.vue')
const routerPath = resolve(currentDir, '../../../router/index.ts')

describe('UsageGuideView', () => {
  it('声明使用方法控制台和两个教程栏目', () => {
    expect(existsSync(viewPath)).toBe(true)
    if (!existsSync(viewPath)) return

    const source = readFileSync(viewPath, 'utf8')

    expect(source).toContain('const guideTopics')
    expect(source).toContain("id: 'codex'")
    expect(source).toContain("title: 'Codex 接入'")
    expect(source).toContain("id: 'image-generation'")
    expect(source).toContain("title: '生图方法'")
    expect(source).toContain('activeTopicId')
    expect(source).toContain('data-test="usage-guide-topic-nav-desktop"')
    expect(source).toContain('data-test="usage-guide-topic-tabs-mobile"')
  })

  it('声明 8 个使用步骤和 10 张截图，保持指定图片顺序', () => {
    expect(existsSync(viewPath)).toBe(true)
    if (!existsSync(viewPath)) return

    const source = readFileSync(viewPath, 'utf8')
    const expectedTokens = [
      "title: '访问 aaccx.pw/shop 页面，点击图中的进入按钮'",
      "alt: '步骤 1 截图 1'",
      "title: '新用户注册，老用户登录'",
      "alt: '步骤 2 截图 1'",
      "title: '选择订阅的页面，选择合适的套餐'",
      "alt: '步骤 3 截图 1'",
      "title: '完成支付后，悠一会给你一个兑换码'",
      "alt: '步骤 4 截图 1'",
      "alt: '步骤 4 截图 2'",
      "title: '兑换成功后，去 API Key 页面生成密钥'",
      "alt: '步骤 5 截图 1'",
      "title: '选择分组，并且可以设置高级功能'",
      "alt: '步骤 6 截图 1'",
      "title: '启动 cc-switch，粘贴 API Key 和请求端口'",
      "alt: '步骤 7 截图 1'",
      "alt: '步骤 7 截图 2'",
      "imagePosition: 'beforeTitle'",
      "alt: '步骤 8 截图 1'",
      "title: '保存配置后，重启 Codex，即可使用！'",
    ]

    let previousIndex = -1
    for (const token of expectedTokens) {
      const index = source.indexOf(token)
      expect(index, `缺少或顺序错误：${token}`).toBeGreaterThan(previousIndex)
      previousIndex = index
    }

    expect(source.match(/data-test="usage-guide-step"/g)?.length).toBe(1)
    expect(source.match(/step: /g)).toHaveLength(8)
    expect(source.match(/alt: '/g)).toHaveLength(10)
  })

  it('生图方法教程只展示用户需要知道的接入和扣费信息', () => {
    expect(existsSync(viewPath)).toBe(true)
    if (!existsSync(viewPath)) return

    const source = readFileSync(viewPath, 'utf8')
    const expectedTokens = [
      "title: '生图方法'",
      '29/39/59 元套餐已支持生图',
      'https://api.aaccx.pw/v1',
      'POST /v1/images/generations',
      '$0.10 / 张',
      '$0.20 / 张',
      '$0.40 / 张',
      'Authorization: Bearer sk-xxxx',
      'gpt-image-2',
    ]

    for (const token of expectedTokens) {
      expect(source, `缺少生图教程信息：${token}`).toContain(token)
    }

    expect(source).not.toContain('groups.id')
    expect(source).not.toContain('127.0.0.1:18080')
  })

  it('注册为登录后用户页面路由', () => {
    const routerSource = readFileSync(routerPath, 'utf8')

    expect(routerSource).toContain("path: '/usage-guide'")
    expect(routerSource).toContain("name: 'UsageGuide'")
    expect(routerSource).toContain("component: () => import('@/views/user/UsageGuideView.vue')")
    expect(routerSource).toContain("titleKey: 'usageGuide.title'")
    expect(routerSource).toContain("descriptionKey: 'usageGuide.description'")
    expect(routerSource).toContain('requiresAuth: true')
    expect(routerSource).toContain('requiresAdmin: false')
  })
})
