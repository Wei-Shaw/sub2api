import { describe, expect, it } from 'vitest'
import { renderAgentMarkdown } from '../agentMarkdown'

describe('Agent Markdown renderer', () => {
  it('renders compact GFM structures used in Agent responses', () => {
    const html = renderAgentMarkdown([
      '## Result',
      '',
      '- **Status:** ready',
      '- `count`: 2',
      '',
      '| Field | Value |',
      '| --- | --- |',
      '| status | active |',
      '',
      '```json',
      '{"ok":true}',
      '```'
    ].join('\n'))

    expect(html).toContain('<h2>Result</h2>')
    expect(html).toContain('<ul>')
    expect(html).toContain('<strong>Status:</strong>')
    expect(html).toContain('<table>')
    expect(html).toContain('<pre><code>')
  })

  it('removes executable HTML, event handlers, images, and unsafe links', () => {
    const html = renderAgentMarkdown([
      '<script>alert(1)</script>',
      '<img src="https://tracker.example/pixel" onerror="alert(2)">',
      '<a href="javascript:alert(3)" onclick="alert(4)">unsafe</a>',
      '',
      '[safe](https://example.com/docs)'
    ].join('\n'))

    expect(html).not.toContain('<script')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('javascript:')
    expect(html).not.toContain('onclick')
    expect(html).toContain('href="https://example.com/docs"')
  })
})
