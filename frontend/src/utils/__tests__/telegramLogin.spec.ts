import { afterEach, describe, expect, it, vi } from 'vitest'
import { runTelegramPopup } from '../telegramLogin'

describe('runTelegramPopup', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('posts the Telegram auth_result (UTF-8 safe base64) to the backend callback', async () => {
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {})
    const popup = { location: { href: '' }, closed: false, close: vi.fn() } as unknown as Window

    const done = runTelegramPopup(popup, '/api/v1/auth/oauth/telegram/start?redirect=%2F')
    expect(popup.location.href).toBe('/api/v1/auth/oauth/telegram/start?redirect=%2F')

    // Messages from other origins are ignored.
    window.dispatchEvent(new MessageEvent('message', { origin: 'https://evil.example', data: '{"event":"auth_result","result":{"id":1}}' }))
    const user = { id: 42, first_name: 'Ánh', auth_date: 1700000000, hash: 'abc' }
    window.dispatchEvent(
      new MessageEvent('message', {
        origin: 'https://oauth.telegram.org',
        data: JSON.stringify({ event: 'auth_result', result: user })
      })
    )
    await done

    expect(submit).toHaveBeenCalledOnce()
    const form = document.querySelector('form') as HTMLFormElement
    expect(form.action).toContain('/auth/oauth/telegram/callback')
    const encoded = (form.querySelector('input[name="tg_auth_result"]') as HTMLInputElement).value
    const decoded = new TextDecoder().decode(Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0)))
    expect(JSON.parse(decoded)).toEqual(user)
    expect(popup.close).toHaveBeenCalled()
  })
})
