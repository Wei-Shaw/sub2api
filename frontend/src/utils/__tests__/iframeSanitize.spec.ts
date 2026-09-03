import { describe, expect, it } from 'vitest'
import DOMPurify from 'dompurify'
import {
  DEFAULT_ALLOWED_IFRAME_HOSTS,
  IFRAME_SANDBOX_TOKENS,
  isAllowedIframeSrc,
  resolveAllowedIframeHosts,
  sanitizeCustomPageHtml,
} from '@/utils/iframeSanitize'

describe('isAllowedIframeSrc', () => {
  it('allows exact hosts and dot-boundary subdomains from the allowlist', () => {
    expect(isAllowedIframeSrc('https://youtube.com/embed/abc')).toBe(true)
    expect(isAllowedIframeSrc('https://www.youtube.com/embed/abc')).toBe(true)
    expect(isAllowedIframeSrc('https://www.youtube-nocookie.com/embed/abc')).toBe(true)
    expect(isAllowedIframeSrc('https://player.vimeo.com/video/123')).toBe(true)
    expect(isAllowedIframeSrc('https://player.bilibili.com/player.html?bvid=1')).toBe(true)
  })

  it('rejects lookalike hosts', () => {
    expect(isAllowedIframeSrc('https://evil-youtube.com/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('https://youtube.com.evil.com/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('https://notyoutube.com/embed/abc')).toBe(false)
    // 用户名部分伪装成白名单主机
    expect(isAllowedIframeSrc('https://youtube.com@evil.com/embed/abc')).toBe(false)
  })

  it('rejects every non-https scheme', () => {
    expect(isAllowedIframeSrc('data:text/html,<h1>phish</h1>')).toBe(false)
    expect(
      isAllowedIframeSrc('data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg=='),
    ).toBe(false)
    expect(isAllowedIframeSrc('javascript:alert(1)')).toBe(false)
    expect(isAllowedIframeSrc('JavaScript:alert(1)')).toBe(false)
    expect(isAllowedIframeSrc('blob:https://youtube.com/1234')).toBe(false)
    expect(isAllowedIframeSrc('http://www.youtube.com/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('vbscript:msgbox(1)')).toBe(false)
  })

  it('rejects protocol-relative, relative and empty srcs', () => {
    expect(isAllowedIframeSrc('//www.youtube.com/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('')).toBe(false)
    expect(isAllowedIframeSrc('   ')).toBe(false)
    expect(isAllowedIframeSrc(null)).toBe(false)
    expect(isAllowedIframeSrc(undefined)).toBe(false)
  })

  it('rejects credentials and non-443 explicit ports', () => {
    expect(isAllowedIframeSrc('https://user:pass@www.youtube.com/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('https://www.youtube.com:8443/embed/abc')).toBe(false)
    expect(isAllowedIframeSrc('https://www.youtube.com:443/embed/abc')).toBe(true)
  })

  it('honours a caller-supplied allowlist instead of the defaults', () => {
    const hosts = ['embed.example.com']
    expect(isAllowedIframeSrc('https://embed.example.com/x', hosts)).toBe(true)
    expect(isAllowedIframeSrc('https://a.embed.example.com/x', hosts)).toBe(true)
    expect(isAllowedIframeSrc('https://xembed.example.com/x', hosts)).toBe(false)
    expect(isAllowedIframeSrc('https://www.youtube.com/embed/abc', hosts)).toBe(false)
    expect(isAllowedIframeSrc('https://www.youtube.com/embed/abc', [])).toBe(false)
  })
})

describe('resolveAllowedIframeHosts', () => {
  it('falls back to the defaults when nothing usable is configured', () => {
    expect(resolveAllowedIframeHosts(undefined)).toEqual([...DEFAULT_ALLOWED_IFRAME_HOSTS])
    expect(resolveAllowedIframeHosts(null)).toEqual([...DEFAULT_ALLOWED_IFRAME_HOSTS])
    expect(resolveAllowedIframeHosts('youtube.com')).toEqual([...DEFAULT_ALLOWED_IFRAME_HOSTS])
    expect(resolveAllowedIframeHosts([])).toEqual([...DEFAULT_ALLOWED_IFRAME_HOSTS])
    expect(resolveAllowedIframeHosts(['', 'https://x.com/a', 'localhost', 42])).toEqual([
      ...DEFAULT_ALLOWED_IFRAME_HOSTS,
    ])
  })

  it('normalizes and deduplicates configured hosts', () => {
    expect(
      resolveAllowedIframeHosts([' .Embed.Example.COM. ', 'embed.example.com', 'a.test']),
    ).toEqual(['embed.example.com', 'a.test'])
  })
})

describe('sanitizeCustomPageHtml', () => {
  it('keeps an allowlisted iframe and forces our sandbox / referrerpolicy', () => {
    const out = sanitizeCustomPageHtml(
      '<p>hi</p><iframe src="https://www.youtube.com/embed/abc" width="560" height="315" allowfullscreen></iframe>',
    )
    expect(out).toContain('<iframe')
    expect(out).toContain('src="https://www.youtube.com/embed/abc"')
    expect(out).toContain(`sandbox="${IFRAME_SANDBOX_TOKENS}"`)
    expect(out).toContain('referrerpolicy="no-referrer"')
    expect(out).toContain('width="560"')
    expect(out).toContain('<p>hi</p>')
    // allow-same-origin 与 allow-scripts 同时出现等于没有沙箱
    expect(IFRAME_SANDBOX_TOKENS).not.toContain('allow-same-origin')
  })

  it('overrides an author-supplied sandbox and drops weakening attributes', () => {
    const out = sanitizeCustomPageHtml(
      '<iframe src="https://player.vimeo.com/video/1" sandbox="allow-scripts allow-same-origin allow-top-navigation" allow="camera; microphone" name="_top" srcdoc="<h1>x</h1>"></iframe>',
    )
    expect(out).toContain(`sandbox="${IFRAME_SANDBOX_TOKENS}"`)
    expect(out).not.toContain('allow-same-origin')
    expect(out).not.toContain('allow-top-navigation')
    expect(out).not.toContain('allow="camera')
    expect(out).not.toContain('srcdoc')
    expect(out).not.toContain('name="_top"')
  })

  it('removes iframes whose src is not allowlisted', () => {
    expect(sanitizeCustomPageHtml('<iframe src="https://evil.com/phish"></iframe>')).not.toContain(
      '<iframe',
    )
    expect(
      sanitizeCustomPageHtml('<iframe src="https://youtube.com.evil.com/phish"></iframe>'),
    ).not.toContain('<iframe')
    expect(
      sanitizeCustomPageHtml('<iframe src="data:text/html,<h1>login</h1>"></iframe>'),
    ).not.toContain('<iframe')
    expect(sanitizeCustomPageHtml('<iframe src="javascript:alert(1)"></iframe>')).not.toContain(
      '<iframe',
    )
    expect(
      sanitizeCustomPageHtml('<iframe src="//www.youtube.com/embed/abc"></iframe>'),
    ).not.toContain('<iframe')
    expect(sanitizeCustomPageHtml('<iframe></iframe>')).not.toContain('<iframe')
  })

  it('keeps the surrounding markup when an iframe is dropped', () => {
    const out = sanitizeCustomPageHtml(
      '<p>before</p><iframe src="https://evil.com/x"></iframe><p>after</p>',
    )
    expect(out).toContain('<p>before</p>')
    expect(out).toContain('<p>after</p>')
    expect(out).not.toContain('evil.com')
  })

  it('respects a narrowed allowlist passed by the caller', () => {
    const out = sanitizeCustomPageHtml(
      '<iframe src="https://www.youtube.com/embed/abc"></iframe><iframe src="https://embed.example.com/x"></iframe>',
      { allowedIframeHosts: ['embed.example.com'] },
    )
    expect(out).toContain('https://embed.example.com/x')
    expect(out).not.toContain('youtube.com')
  })

  it('still sanitizes non-iframe payloads exactly like the default config', () => {
    const dirty =
      '<p onclick="alert(1)">t</p><script>alert(1)</script><img src=x onerror="alert(1)"><a href="javascript:alert(1)">l</a><svg><style>@import"x"</style></svg>'
    const out = sanitizeCustomPageHtml(dirty)
    expect(out).not.toContain('<script')
    expect(out).not.toContain('onerror')
    expect(out).not.toContain('onclick')
    expect(out).not.toContain('javascript:')
    expect(out).toContain('<p>t</p>')
    // 与默认实例（同样的 ADD_TAGS / ADD_ATTR）在非 iframe 内容上结果一致
    expect(out).toBe(
      DOMPurify.sanitize(dirty, {
        ADD_TAGS: ['iframe'],
        ADD_ATTR: ['allowfullscreen', 'frameborder', 'src'],
      }),
    )
  })

  it('returns an empty string for empty input', () => {
    expect(sanitizeCustomPageHtml('')).toBe('')
  })
})

describe('hook isolation', () => {
  it('does not register hooks on the shared default DOMPurify instance', () => {
    // 先跑一次我们的净化，确保私有实例已创建并挂上 hook
    sanitizeCustomPageHtml('<iframe src="https://www.youtube.com/embed/abc"></iframe>')

    // 默认实例（AnnouncementBell / LegalDocumentView 等使用）必须完全不受影响：
    // 用同样放宽的配置时，它仍按原样保留任意 iframe——证明 hook 没有泄漏到全局。
    const leaked = DOMPurify.sanitize('<iframe src="https://evil.com/phish"></iframe>', {
      ADD_TAGS: ['iframe'],
      ADD_ATTR: ['src'],
    })
    expect(leaked).toContain('<iframe')
    expect(leaked).toContain('https://evil.com/phish')
    expect(leaked).not.toContain('sandbox')

    // 默认配置下的普通净化行为同样不变
    expect(DOMPurify.sanitize('<b>x</b><script>alert(1)</script>')).toBe('<b>x</b>')
  })
})
