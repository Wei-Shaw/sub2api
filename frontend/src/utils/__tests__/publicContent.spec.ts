import { describe, expect, it } from 'vitest'
import { renderPublicMarkdown, sanitizePublicHTML } from '@/utils/publicContent'

describe('public content utils', () => {
  it('sanitizes dangerous html while keeping safe formatting', () => {
    const html = sanitizePublicHTML(`
      <script>alert(1)</script>
      <p style="color:#0f766e" onclick="alert(1)">教程正文</p>
      <a href="https://example.com" target="_blank">查看完整中文接入说明</a>
      <img src="/api/v1/pages/tutorial/images/demo.png" onerror="alert(1)" alt="教程截图">
      <div style="position:fixed;top:0;left:0;width:100%;height:100%">overlay</div>
      <svg onload="alert(1)"><circle cx="10" cy="10" r="10"></circle></svg>
      <img src="data:image/svg+xml,<svg onload=alert(1)>">
    `)

    expect(html).not.toContain('<script')
    expect(html).not.toContain('onclick=')
    expect(html).not.toContain('onerror=')
    expect(html).not.toContain('<svg')
    expect(html).not.toContain('data:image/svg+xml')
    expect(html).not.toContain('position:fixed')
    expect(html).toContain('教程正文')
    expect(html).toContain('style="color: #0f766e"')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noopener noreferrer nofollow"')
    expect(html).toContain('alt="教程截图"')
  })

  it('renders markdown and rewrites safe inline formatting only', () => {
    const html = renderPublicMarkdown('# 教程文档\n\n**加粗** 与 [链接](https://example.com)')

    expect(html).toContain('<h1>教程文档</h1>')
    expect(html).toContain('<strong>加粗</strong>')
    expect(html).toContain('<a href="https://example.com" rel="noopener noreferrer nofollow">链接</a>')
  })
})
