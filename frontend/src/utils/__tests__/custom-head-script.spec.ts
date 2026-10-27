import { beforeEach, describe, expect, it } from 'vitest'
import { injectCustomHeadScript } from '../custom-head-script'

describe('custom head script', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
  })

  it('inserts JavaScript once in the head', async () => {
    await injectCustomHeadScript('window.custom = true;')
    await injectCustomHeadScript('window.custom = true;')
    const scripts = document.head.querySelectorAll('script[data-custom-head-script]')
    expect(scripts).toHaveLength(1)
    expect(scripts[0].textContent).toBe('window.custom = true;')
  })

  it('does not repeat scripts rendered by the backend', async () => {
    document.head.innerHTML = '<script data-custom-head-script>window.custom = true;</script>'
    await injectCustomHeadScript('<script>window.custom = true;</script>')
    expect(document.head.querySelectorAll('script')).toHaveLength(1)
  })

  it('retains external script attributes and uses the current nonce', async () => {
    document.head.innerHTML = '<script nonce="current-nonce"></script>'
    await injectCustomHeadScript('<script async src="https://example.com/a.js" data-site="test" nonce="old"></script>')
    const script = document.head.querySelector<HTMLScriptElement>('script[data-custom-head-script]')!
    expect(script.src).toBe('https://example.com/a.js')
    expect(script.hasAttribute('async')).toBe(true)
    expect(script.dataset.site).toBe('test')
    expect(script.nonce).toBe('current-nonce')
  })

  it('waits for ordered external scripts before inserting inline initialization', async () => {
    const injected = injectCustomHeadScript('<script src="/a.js"></script><script>window.customOrder = true;</script>')
    const external = document.head.querySelector<HTMLScriptElement>('script')!
    expect(external.async).toBe(false)
    expect(document.head.querySelectorAll('script')).toHaveLength(1)
    external.dispatchEvent(new Event('load'))
    await injected
    expect(document.head.querySelectorAll('script')).toHaveLength(2)
    expect(document.head.lastElementChild?.textContent).toBe('window.customOrder = true;')
  })

  it('continues after an external script fails', async () => {
    const injected = injectCustomHeadScript('<script src="/a.js"></script><script>window.custom = true;</script>')
    document.head.querySelector('script')!.dispatchEvent(new Event('error'))
    await injected
    expect(document.head.querySelectorAll('script')).toHaveLength(2)
  })

  it('preserves external script load handlers', async () => {
    delete document.documentElement.dataset.customScriptLoaded
    const injected = injectCustomHeadScript('<script src="/a.js" onload="document.documentElement.dataset.customScriptLoaded = \'yes\'"></script>')
    document.head.querySelector('script')!.dispatchEvent(new Event('load'))
    await injected
    expect(document.documentElement.dataset.customScriptLoaded).toBe('yes')
    delete document.documentElement.dataset.customScriptLoaded
  })

  it('ignores empty settings and non-script HTML', async () => {
    await injectCustomHeadScript(' \n ')
    await injectCustomHeadScript('<script>window.custom = true;</script><img src="x">')
    expect(document.head.querySelectorAll('script')).toHaveLength(0)
    expect(document.head.querySelectorAll('img')).toHaveLength(0)
  })
})
