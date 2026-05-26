/**
 * API Key 创建逻辑测试
 * 通过封装组件测试 API Key 创建的核心流程
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { defineComponent, ref, reactive } from 'vue'

// Mock keysAPI
const mockCreate = vi.fn()
const mockList = vi.fn()

vi.mock('@/api', () => ({
  keysAPI: {
    create: (...args: any[]) => mockCreate(...args),
    list: (...args: any[]) => mockList(...args),
  },
  authAPI: {
    getCurrentUser: vi.fn().mockResolvedValue({ data: {} }),
    logout: vi.fn(),
    refreshToken: vi.fn(),
  },
  isTotp2FARequired: () => false,
}))

vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(),
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn().mockResolvedValue({}),
}))

// Mock app store - 使用固定引用确保组件和测试共享同一对象
const mockShowSuccess = vi.fn()
const mockShowError = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
  }),
}))

import { useAppStore } from '@/stores/app'

/**
 * 简化的 API Key 创建测试组件
 */
const ApiKeyCreateTestComponent = defineComponent({
  setup() {
    const appStore = useAppStore()
    const loading = ref(false)
    const createdKey = ref('')
    const formData = reactive({
      name: '',
      group_id: null as number | null,
    })

    const handleCreate = async () => {
      if (!formData.name) return

      loading.value = true
      try {
        const result = await mockCreate({
          name: formData.name,
          group_id: formData.group_id,
        })
        createdKey.value = result.key
        appStore.showSuccess('API Key 创建成功')
      } catch (error: any) {
        appStore.showError(error.message || '创建失败')
      } finally {
        loading.value = false
      }
    }

    return { formData, loading, createdKey, handleCreate }
  },
  template: `
    <div>
      <form @submit.prevent="handleCreate">
        <input id="name" v-model="formData.name" placeholder="Key 名称" />
        <select id="group" v-model="formData.group_id">
          <option :value="null">默认</option>
          <option :value="1">Group 1</option>
        </select>
        <button type="submit" :disabled="loading">创建</button>
      </form>
      <div v-if="createdKey" class="created-key">{{ createdKey }}</div>
    </div>
  `,
})

describe('ApiKey 创建流程', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('创建 API Key 调用 API 并显示结果', async () => {
    mockCreate.mockResolvedValue({
      id: 1,
      key: 'sk-test-key-12345',
      name: 'My Test Key',
    })

    const wrapper = mount(ApiKeyCreateTestComponent)

    await wrapper.find('#name').setValue('My Test Key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockCreate).toHaveBeenCalledWith({
      name: 'My Test Key',
      group_id: null,
    })

    expect(wrapper.find('.created-key').text()).toBe('sk-test-key-12345')
  })

  it('选择分组后正确传参', async () => {
    mockCreate.mockResolvedValue({
      id: 2,
      key: 'sk-group-key',
      name: 'Group Key',
    })

    const wrapper = mount(ApiKeyCreateTestComponent)

    await wrapper.find('#name').setValue('Group Key')
    // 选择 group_id = 1
    await wrapper.find('#group').setValue('1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockCreate).toHaveBeenCalledWith({
      name: 'Group Key',
      group_id: 1,
    })
  })

  it('创建失败时显示错误', async () => {
    mockCreate.mockRejectedValue(new Error('配额不足'))

    const wrapper = mount(ApiKeyCreateTestComponent)

    await wrapper.find('#name').setValue('Fail Key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockShowError).toHaveBeenCalledWith('配额不足')
    expect(wrapper.find('.created-key').exists()).toBe(false)
  })

  it('名称为空时不提交', async () => {
    const wrapper = mount(ApiKeyCreateTestComponent)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockCreate).not.toHaveBeenCalled()
  })

  it('创建过程中按钮被禁用', async () => {
    let resolveCreate: (v: any) => void
    mockCreate.mockImplementation(
      () => new Promise((resolve) => { resolveCreate = resolve })
    )

    const wrapper = mount(ApiKeyCreateTestComponent)

    await wrapper.find('#name').setValue('Test Key')
    await wrapper.find('form').trigger('submit')

    expect(wrapper.find('button').attributes('disabled')).toBeDefined()

    resolveCreate!({ id: 1, key: 'sk-test', name: 'Test Key' })
    await flushPromises()

    expect(wrapper.find('button').attributes('disabled')).toBeUndefined()
  })
})
