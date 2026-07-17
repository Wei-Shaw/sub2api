import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const { showSuccessMock, showInfoMock } = vi.hoisted(() => {
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      getItem: vi.fn(),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn()
    },
    configurable: true
  })

  return {
    showSuccessMock: vi.fn(),
    showInfoMock: vi.fn()
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: showSuccessMock,
    showInfo: showInfoMock
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      probeModelList: vi.fn(),
      probeModels: vi.fn()
    }
  }
}))

vi.mock('@/api/admin/index', () => ({
  adminAPI: {
    accounts: {
      probeModelList: vi.fn(),
      probeModels: vi.fn()
    }
  }
}))

vi.mock('@/api/admin/index.ts', () => ({
  adminAPI: {
    accounts: {
      probeModelList: vi.fn(),
      probeModels: vi.fn()
    }
  }
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback
}))

vi.mock('@/components/account/ModelProbeModal.vue', () => ({
  default: {
    name: 'ModelProbeModal',
    props: ['show', 'defaultPlatform'],
    emits: ['close', 'apply'],
    template: `
      <div v-if="show" data-test="probe-modal">
        <button type="button" @click="$emit('apply', ['gpt-5.4-openai-compact'])">apply-probed</button>
      </div>
    `
  }
}))

vi.mock('../ModelProbeModal.vue', () => ({
  default: {
    name: 'ModelProbeModal',
    props: ['show', 'defaultPlatform'],
    emits: ['close', 'apply'],
    template: `
      <div v-if="show" data-test="probe-modal">
        <button type="button" @click="$emit('apply', ['gpt-5.4-openai-compact'])">apply-probed</button>
      </div>
    `
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params?.count !== undefined ? `${key}:${params.count}` : key
    })
  }
})

describe('ModelWhitelistSelector', () => {
  it('通过探测弹窗把模型合并到白名单', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['gpt-5.4'],
        platform: 'openai'
      },
      global: {
        stubs: {
          ModelIcon: true,
          Icon: true
        }
      }
    })

    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.modelProbe.openButton'))!.trigger('click')
    await wrapper.get('[data-test="probe-modal"] button').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['gpt-5.4', 'gpt-5.4-openai-compact']])
    expect(showSuccessMock).toHaveBeenCalledWith('admin.accounts.modelProbe.addedModels:1')
  })

  it('切换平台后清空本地探测候选模型', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai'
      },
      global: {
        stubs: {
          ModelIcon: true,
          Icon: true
        }
      }
    })

    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.modelProbe.openButton'))!.trigger('click')
    await wrapper.get('[data-test="probe-modal"] button').trigger('click')
    await wrapper.setProps({ modelValue: ['gpt-5.4-openai-compact'] })

    expect(wrapper.text()).toContain('gpt-5.4-openai-compact')

    await wrapper.setProps({
      modelValue: [],
      platform: 'anthropic'
    })
    await wrapper.find('.cursor-pointer').trigger('click')
    await wrapper.find('input').setValue('gpt-5.4-openai-compact')

    expect(wrapper.text()).toContain('admin.accounts.noMatchingModels')
  })
})
