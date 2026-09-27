import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('BaseDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    document.body.classList.remove('modal-open')
  })

  it('resets body scroll position when reopened', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: false, title: 'Details' },
      slots: { default: '<div style="height: 2000px">content</div>' },
      global: { stubs: { Icon: true } }
    })

    await wrapper.setProps({ show: true })
    await nextTick()
    const body = document.body.querySelector<HTMLElement>('.modal-body')
    expect(body).not.toBeNull()
    body!.scrollTop = 480

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await nextTick()

    expect(document.body.querySelector<HTMLElement>('.modal-body')?.scrollTop).toBe(0)
    wrapper.unmount()
  })

  const pressKey = (target: EventTarget, key: string, shiftKey = false) =>
    target.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, bubbles: true }))

  it('closes only the topmost dialog on Escape', async () => {
    const lower = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Edit key' },
      global: { stubs: { Icon: true } }
    })
    const upper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Confirm reset' },
      global: { stubs: { Icon: true } }
    })
    await nextTick()

    pressKey(document, 'Escape')
    expect(upper.emitted('close')).toHaveLength(1)
    expect(lower.emitted('close')).toBeUndefined()

    await upper.setProps({ show: false })
    pressKey(document, 'Escape')
    expect(lower.emitted('close')).toHaveLength(1)

    upper.unmount()
    lower.unmount()
  })

  it('does not let Escape fall through a top dialog that ignores it', async () => {
    const lower = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Edit key' },
      global: { stubs: { Icon: true } }
    })
    const upper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Busy', closeOnEscape: false },
      global: { stubs: { Icon: true } }
    })
    await nextTick()

    pressKey(document, 'Escape')
    expect(upper.emitted('close')).toBeUndefined()
    expect(lower.emitted('close')).toBeUndefined()

    upper.unmount()
    lower.unmount()
  })

  it('cycles Tab and Shift+Tab inside the dialog', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Form' },
      slots: {
        default: '<input id="name" /><button id="disabled" disabled>x</button>',
        footer: '<button id="save">Save</button>'
      },
      global: { stubs: { Icon: true } }
    })
    await nextTick()

    const close = document.body.querySelector<HTMLElement>('.modal-header button')!
    const save = document.getElementById('save')!
    expect(close.getAttribute('aria-label')).toBe('common.close')
    expect(document.activeElement).toBe(close)

    save.focus()
    pressKey(save, 'Tab')
    expect(document.activeElement).toBe(close)

    pressKey(close, 'Tab', true)
    expect(document.activeElement).toBe(save)

    wrapper.unmount()
  })

  it('returns focus to the opener when closed', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()

    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: false, title: 'Details' },
      global: { stubs: { Icon: true } }
    })
    await wrapper.setProps({ show: true })
    await nextTick()
    expect(document.activeElement).not.toBe(opener)

    await wrapper.setProps({ show: false })
    expect(document.activeElement).toBe(opener)
    wrapper.unmount()
  })
})
