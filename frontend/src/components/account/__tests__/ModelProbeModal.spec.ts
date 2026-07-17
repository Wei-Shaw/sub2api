import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import ModelProbeModal from '../ModelProbeModal.vue'
import { adminAPI } from '@/api/admin'

const discoveredModelCount = 25

const { probeModelListMock, probeModelsMock } = vi.hoisted(() => ({
  probeModelListMock: vi.fn(),
  probeModelsMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      probeModelList: probeModelListMock,
      probeModels: probeModelsMock
    }
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

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function mountModal() {
  return mount(ModelProbeModal, {
    props: {
      show: true,
      defaultPlatform: 'openai'
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ModelIcon: true,
        Icon: true
      }
    }
  })
}

function findButton(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find(button => button.text().includes(text))!
}

describe('ModelProbeModal', () => {
  beforeEach(() => {
    probeModelListMock.mockReset()
    probeModelsMock.mockReset()
    probeModelListMock.mockResolvedValue({
      models: Array.from({ length: discoveredModelCount }, (_, index) => ({ id: `model-${index + 1}` }))
    })
    probeModelsMock.mockResolvedValue({
      results: [
        { model: 'model-1', mode: 'responses', ok: true, status: 200 },
        { model: 'model-2', mode: 'responses', ok: false, status: 404, error: 'not found' }
      ]
    })
  })

  it('发现、验证并只应用验证通过的模型', async () => {
    const wrapper = mountModal()

    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('sk-test')

    await findButton(wrapper, 'admin.accounts.modelProbe.discover').trigger('click')
    await flushPromises()

    expect(adminAPI.accounts.probeModelList).toHaveBeenCalledWith({
      platform: 'openai',
      base_url: '',
      api_key: 'sk-test'
    })
    expect(wrapper.text()).toContain('model-25')

    await findButton(wrapper, 'admin.accounts.modelProbe.testSelected').trigger('click')
    await flushPromises()

    expect(adminAPI.accounts.probeModels).toHaveBeenCalledWith({
      platform: 'openai',
      base_url: '',
      api_key: 'sk-test',
      mode: 'responses',
      models: Array.from({ length: 20 }, (_, index) => `model-${index + 1}`)
    })

    await findButton(wrapper, 'admin.accounts.modelProbe.applyModels').trigger('click')

    expect(wrapper.emitted('apply')?.[0]).toEqual([['model-1']])
  })

  it('验证全部失败后不允许应用选中模型', async () => {
    probeModelsMock.mockResolvedValue({
      results: [
        { model: 'gpt-5.4', mode: 'responses', ok: false, status: 404, error: 'not found' }
      ]
    })
    const wrapper = mountModal()

    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('sk-test')
    await findButton(wrapper, 'admin.accounts.modelProbe.discover').trigger('click')
    await flushPromises()
    await findButton(wrapper, 'admin.accounts.modelProbe.testSelected').trigger('click')
    await flushPromises()

    const applyButton = findButton(wrapper, 'admin.accounts.modelProbe.applyModels')
    expect(applyButton.attributes('disabled')).toBeDefined()
  })

  it('验证后只应用当前仍选中的成功模型', async () => {
    probeModelListMock.mockResolvedValue({
      models: [
        { id: 'model-1' },
        { id: 'model-2' }
      ]
    })
    probeModelsMock.mockResolvedValue({
      results: [
        { model: 'model-1', mode: 'responses', ok: true, status: 200 },
        { model: 'model-2', mode: 'responses', ok: true, status: 200 }
      ]
    })
    const wrapper = mountModal()

    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('sk-test')
    await findButton(wrapper, 'admin.accounts.modelProbe.discover').trigger('click')
    await flushPromises()
    await findButton(wrapper, 'admin.accounts.modelProbe.testSelected').trigger('click')
    await flushPromises()
    await findButton(wrapper, 'model-2').trigger('click')
    await findButton(wrapper, 'admin.accounts.modelProbe.applyModels').trigger('click')

    expect(wrapper.emitted('apply')?.[0]).toEqual([['model-1']])
  })

  it('手动选择不会超过最大探测数量', async () => {
    const wrapper = mountModal()

    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('sk-test')
    await findButton(wrapper, 'admin.accounts.modelProbe.discover').trigger('click')
    await flushPromises()

    await findButton(wrapper, 'model-21').trigger('click')
    expect(wrapper.text()).toContain('admin.accounts.modelProbe.maxSelectionHint:20')

    await findButton(wrapper, 'admin.accounts.modelProbe.testSelected').trigger('click')
    await flushPromises()

    expect(adminAPI.accounts.probeModels).toHaveBeenCalledWith({
      platform: 'openai',
      base_url: '',
      api_key: 'sk-test',
      mode: 'responses',
      models: Array.from({ length: 20 }, (_, index) => `model-${index + 1}`)
    })
  })
})
