import { describe, expect, it } from 'vitest'

import { renderHomeTemplate } from '../homeTemplate'

describe('renderHomeTemplate', () => {
  const settings = {
    site_name: 'Demo <Site>',
    site_logo: '/logo.svg',
    site_subtitle: 'Fast & reliable',
    api_base_url: 'https://api.example.com',
    contact_info: 'admin@example.com',
    doc_url: 'https://docs.example.com',
    version: '1.2.3',
  } as any

  it('replaces supported variables in HTML mode', () => {
    expect(renderHomeTemplate('<h1>{{ site_name }}</h1><img src="{{site_logo}}">', settings))
      .toBe('<h1>Demo &lt;Site&gt;</h1><img src="/logo.svg">')
  })

  it('escapes values and leaves unknown variables unchanged', () => {
    expect(renderHomeTemplate('{{site_subtitle}} {{unknown}}', settings))
      .toBe('Fast &amp; reliable {{unknown}}')
  })

  it('does not render without public settings', () => {
    expect(renderHomeTemplate('{{site_name}}', null)).toBe('{{site_name}}')
  })
})
