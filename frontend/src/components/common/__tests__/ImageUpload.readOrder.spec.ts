import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import ImageUpload from '../ImageUpload.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

class ControlledReader {
  static instances: ControlledReader[] = []
  onload: ((event: { target: { result: string } }) => void) | null = null
  onerror: (() => void) | null = null
  aborted = false
  constructor() { ControlledReader.instances.push(this) }
  readAsText = vi.fn()
  readAsDataURL = vi.fn()
  abort() { this.aborted = true }
  complete(result: string) {
    if (!this.aborted) this.onload?.({ target: { result } })
  }
}

beforeEach(() => {
  ControlledReader.instances = []
  vi.stubGlobal('FileReader', ControlledReader)
})
afterEach(() => vi.unstubAllGlobals())

function uploader(mode: 'image' | 'svg' = 'image') {
  const wrapper = mount(ImageUpload, { props: { modelValue: 'existing', mode } })
  const input = wrapper.get('input')
  return {
    wrapper,
    async choose(type = 'image/png') {
      Object.defineProperty(input.element, 'files', {
        configurable: true, value: [new File(['image'], 'upload', { type })]
      })
      await input.trigger('change')
    }
  }
}

describe('image upload read lifecycle', () => {
  it.each(['image', 'svg'] as const)('keeps the newest %s selection when reads finish out of order', async mode => {
    const { wrapper, choose } = uploader(mode)
    await choose()
    await choose()
    ControlledReader.instances[1].complete('new image')
    ControlledReader.instances[0].complete('old image')
    expect(wrapper.emitted('update:modelValue')).toEqual([['new image']])
  })

  it('does not restore an image after the remove button is clicked', async () => {
    const { wrapper, choose } = uploader()
    await choose()
    await wrapper.get('button').trigger('click')
    ControlledReader.instances[0].complete('removed image')
    expect(wrapper.emitted('update:modelValue')).toEqual([['']])
  })

  it('does not apply a pending image after a later invalid selection', async () => {
    const { wrapper, choose } = uploader()
    await choose()
    await choose('text/plain')
    ControlledReader.instances[0].complete('old image')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('common.selectImageFile')
  })

  it('aborts a pending read when the uploader unmounts', async () => {
    const { wrapper, choose } = uploader()
    await choose()
    wrapper.unmount()
    expect(ControlledReader.instances[0].aborted).toBe(true)
  })
})
