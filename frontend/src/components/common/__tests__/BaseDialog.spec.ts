import { afterEach, describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import { nextTick REDACTED from 'vue'
import BaseDialog from '../BaseDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key REDACTED)
REDACTED))

describe('BaseDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    document.body.classList.remove('modal-open')
  REDACTED)

  it('resets body scroll position when reopened', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: false, title: 'Details' REDACTED,
      slots: { default: '<div style="height: 2000px">content</div>' REDACTED,
      global: { stubs: { Icon: true REDACTED REDACTED
    REDACTED)

    await wrapper.setProps({ show: true REDACTED)
    await nextTick()
    const body = document.body.querySelector<HTMLElement>('.modal-body')
    expect(body).not.toBeNull()
    body!.scrollTop = 480

    await wrapper.setProps({ show: false REDACTED)
    await wrapper.setProps({ show: true REDACTED)
    await nextTick()

    expect(document.body.querySelector<HTMLElement>('.modal-body')?.scrollTop).toBe(0)
    wrapper.unmount()
  REDACTED)
REDACTED)
