import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import { dialogStackDepth, resetDialogStack } from '@/composables/useDialogStack'

const useI18nMock = vi.fn(() => ({ t: (key: string) => key }))

vi.mock('vue-i18n', () => ({
  useI18n: () => useI18nMock()
}))

const mountDialog = (title: string, body = '<button class="dialog-action">action</button>') =>
  mount(BaseDialog, {
    attachTo: document.body,
    props: { show: false, title },
    slots: { default: body },
    global: { stubs: { Icon: true } }
  })

const overlayOf = (title: string): HTMLElement => {
  const overlay = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-overlay')).find(
    (element) => element.querySelector('.modal-title')?.textContent?.trim() === title
  )
  expect(overlay).toBeTruthy()
  return overlay as HTMLElement
}

const pressKey = async (key: string, init: KeyboardEventInit = {}) => {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...init }))
  await nextTick()
}

const isLocked = () => document.body.classList.contains('modal-open')

describe('BaseDialog', () => {
  const wrappers: VueWrapper[] = []

  const open = async (title: string, body?: string) => {
    const wrapper = mountDialog(title, body)
    wrappers.push(wrapper)
    await wrapper.setProps({ show: true })
    await nextTick()
    return wrapper
  }

  afterEach(() => {
    while (wrappers.length > 0) {
      wrappers.pop()?.unmount()
    }
    resetDialogStack()
    document.body.innerHTML = ''
    document.body.classList.remove('modal-open')
    useI18nMock.mockReset()
    useI18nMock.mockImplementation(() => ({ t: (key: string) => key }))
  })

  it('resets body scroll position when reopened', async () => {
    const wrapper = mountDialog('Details', '<div style="height: 2000px">content</div>')
    wrappers.push(wrapper)

    await wrapper.setProps({ show: true })
    await nextTick()
    const body = document.body.querySelector<HTMLElement>('.modal-body')
    expect(body).not.toBeNull()
    body!.scrollTop = 480

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await nextTick()

    expect(document.body.querySelector<HTMLElement>('.modal-body')?.scrollTop).toBe(0)
  })

  it('translates the close button label', async () => {
    await open('Details')

    expect(overlayOf('Details').querySelector('button')?.getAttribute('aria-label')).toBe('common.close')
  })

  it('falls back to an English close label when no i18n instance is installed', async () => {
    useI18nMock.mockImplementationOnce(() => {
      throw new Error('Need to install with `app.use` function')
    })

    await open('Details')

    expect(overlayOf('Details').querySelector('button')?.getAttribute('aria-label')).toBe('Close')
  })

  describe('nested dialogs', () => {
    it('closes only the topmost dialog on Escape', async () => {
      const outer = await open('Outer')
      const inner = await open('Inner')

      await pressKey('Escape')

      expect(inner.emitted('close')).toHaveLength(1)
      expect(outer.emitted('close')).toBeUndefined()

      // The consumer reacts to the emit by hiding the inner dialog.
      await inner.setProps({ show: false })
      await pressKey('Escape')

      expect(outer.emitted('close')).toHaveLength(1)
      expect(inner.emitted('close')).toHaveLength(1)
    })

    it('marks only the topmost dialog as a modal', async () => {
      await open('Outer')
      await open('Inner')

      expect(overlayOf('Outer').getAttribute('aria-modal')).toBe('false')
      expect(overlayOf('Outer').hasAttribute('inert')).toBe(true)
      expect(overlayOf('Inner').getAttribute('aria-modal')).toBe('true')
      expect(overlayOf('Inner').hasAttribute('inert')).toBe(false)
    })

    it('keeps the scroll lock until the last dialog closes', async () => {
      const outer = await open('Outer')
      const inner = await open('Inner')
      expect(isLocked()).toBe(true)

      await inner.setProps({ show: false })
      expect(isLocked()).toBe(true)
      expect(dialogStackDepth()).toBe(1)

      await outer.setProps({ show: false })
      expect(isLocked()).toBe(false)
      expect(dialogStackDepth()).toBe(0)
    })

    it('handles the outer dialog being closed by code while the inner is open', async () => {
      const outer = await open('Outer')
      const inner = await open('Inner')

      // e.g. closeDetail() runs while the image preview is still open.
      await outer.setProps({ show: false })

      expect(isLocked()).toBe(true)
      expect(dialogStackDepth()).toBe(1)

      // The inner dialog is still the topmost entry and still owns Escape.
      await pressKey('Escape')
      expect(inner.emitted('close')).toHaveLength(1)
      expect(outer.emitted('close')).toBeUndefined()

      await inner.setProps({ show: false })
      expect(isLocked()).toBe(false)
      expect(dialogStackDepth()).toBe(0)
    })

    it('leaves no orphan entry when a dialog unmounts while open', async () => {
      const outer = await open('Outer')
      const inner = await open('Inner')
      expect(dialogStackDepth()).toBe(2)

      inner.unmount()
      wrappers.splice(wrappers.indexOf(inner), 1)
      expect(dialogStackDepth()).toBe(1)
      expect(isLocked()).toBe(true)

      outer.unmount()
      wrappers.splice(wrappers.indexOf(outer), 1)
      expect(dialogStackDepth()).toBe(0)
      expect(isLocked()).toBe(false)

      // Escape after everything unmounted must not reach a destroyed dialog.
      await pressKey('Escape')
      expect(outer.emitted('close')).toBeUndefined()
    })

    it('restores focus to the outer dialog when the inner dialog closes', async () => {
      await open('Outer')
      const outerAction = overlayOf('Outer').querySelector<HTMLElement>('.dialog-action')
      expect(outerAction).not.toBeNull()
      outerAction!.focus()
      expect(document.activeElement).toBe(outerAction)

      const inner = await open('Inner')
      expect(overlayOf('Inner').contains(document.activeElement)).toBe(true)

      await inner.setProps({ show: false })

      // Back into the outer dialog — not to whatever was focused before it opened.
      expect(document.activeElement).toBe(outerAction)
      expect(overlayOf('Outer').contains(document.activeElement)).toBe(true)
    })

    it('traps Tab inside the topmost dialog only', async () => {
      await open('Outer')
      await open('Inner', '<button class="inner-first">first</button><button class="inner-last">last</button>')

      const innerOverlay = overlayOf('Inner')
      innerOverlay.querySelector<HTMLElement>('.inner-last')!.focus()

      await pressKey('Tab')

      // Wrapped to the inner dialog's first focusable (its close button).
      expect(innerOverlay.contains(document.activeElement)).toBe(true)
      expect(document.activeElement).toBe(innerOverlay.querySelector('button'))
    })
  })
})
