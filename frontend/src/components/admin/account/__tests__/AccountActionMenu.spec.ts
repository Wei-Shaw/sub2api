import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountActionMenu from '../AccountActionMenu.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function mountMenu(account: Record<string, unknown>) {
  return mount(AccountActionMenu, {
    props: {
      show: true,
      account,
      position: { top: 10, left: 10 }
    } as any,
    global: {
      stubs: {
        Teleport: true,
        Icon: true
      }
    }
  })
}

describe('AccountActionMenu', () => {
  it('disables privacy action when OpenAI privacy is already set', async () => {
    const wrapper = mountMenu({
      id: 1,
      platform: 'openai',
      type: 'oauth',
      extra: { privacy_mode: 'training_off' }
    })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.accounts.privacyAlreadySet'))
    expect(button).toBeTruthy()
    expect(button!.attributes('disabled')).toBeDefined()

    await button!.trigger('click')
    expect(wrapper.emitted('set-privacy')).toBeUndefined()
  })

  it('keeps privacy action enabled for failed OpenAI states', async () => {
    const wrapper = mountMenu({
      id: 2,
      platform: 'openai',
      type: 'oauth',
      extra: { privacy_mode: 'training_set_failed' }
    })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.accounts.setPrivacy'))
    expect(button).toBeTruthy()
    expect(button!.attributes('disabled')).toBeUndefined()

    await button!.trigger('click')
    expect(wrapper.emitted('set-privacy')).toHaveLength(1)
  })
})
