import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getOptions, submitTask, getTask, deleteTask, showSuccess, showError } = vi.hoisted(() => ({
  getOptions: vi.fn(),
  submitTask: vi.fn(),
  getTask: vi.fn(),
  deleteTask: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/imagePlayground', () => ({
  getImagePlaygroundOptions: getOptions,
  submitImagePlaygroundTask: submitTask,
  getImagePlaygroundTask: getTask,
  deleteImagePlaygroundTask: deleteTask,
  downloadImagePlaygroundImage: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'zh' },
  }),
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<main><slot /></main>' },
}))

import ImagePlaygroundView from '@/views/user/ImagePlaygroundView.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const options = {
  enabled: true,
  retention_hours: 24,
  groups: [{
    id: 7,
    name: 'OpenAI Images',
    platform: 'openai',
    subscription_type: 'standard',
    available: true,
    models: [{
      id: 'gpt-image-1.5',
      sizes: ['1024x1024'],
      qualities: ['medium'],
      max_images: 2,
      output_formats: ['png'],
      backgrounds: ['auto'],
      output_compression: true,
    }],
  }],
}

describe('ImagePlaygroundView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
    getOptions.mockResolvedValue(options)
    deleteTask.mockResolvedValue(undefined)
    submitTask.mockResolvedValue({
      id: 'task-1',
      object: 'image.playground.task',
      status: 'processing',
      group_id: 7,
      platform: 'openai',
      model: 'gpt-image-1.5',
      prompt_preview: 'A paper sculpture',
      images: [],
      created_at: 1_700_000_000,
      expires_at: 1_700_086_400,
      poll_url: '/api/v1/image-playground/tasks/task-1',
    })
  })

  it('loads available models and submits a background generation task', async () => {
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: {
            props: ['modelValue', 'options'],
            emits: ['update:modelValue'],
            template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
          },
          PlaygroundTaskCard: { template: '<div data-test="task-card" />' },
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('OpenAI Images')
    expect(wrapper.text()).toContain('gpt-image-1.5')
    expect(wrapper.text()).toContain('imagePlayground.retention.current')

    await wrapper.get('textarea').setValue('A paper sculpture')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(submitTask).toHaveBeenCalledWith({
      group_id: 7,
      model: 'gpt-image-1.5',
      prompt: 'A paper sculpture',
      size: '1024x1024',
      quality: 'medium',
      n: 1,
      output_format: 'png',
      background: 'auto',
    })
    expect(wrapper.find('[data-test="task-card"]').exists()).toBe(true)
    expect(JSON.parse(localStorage.getItem('image_playground_history_v1') || '{}').ids).toEqual(['task-1'])

    wrapper.unmount()
  })

  it('requires confirmation before deleting records and stored files', async () => {
    localStorage.setItem('image_playground_history_v1', JSON.stringify({
      ids: ['task-1'],
      meta: { 'task-1': { prompt: 'A paper sculpture', payload: { group_id: 7, model: 'gpt-image-1.5', prompt: 'A paper sculpture' } } },
    }))
    getTask.mockResolvedValue({
      id: 'task-1',
      object: 'image.playground.task',
      status: 'completed',
      group_id: 7,
      platform: 'openai',
      model: 'gpt-image-1.5',
      images: [{ index: 0, url: 'https://cdn.example/image.png', download_url: '/download' }],
      created_at: 1_700_000_000,
      expires_at: 1_700_086_400,
      poll_url: '/api/v1/image-playground/tasks/task-1',
    })
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: true,
          PlaygroundTaskCard: { template: '<div data-test="task-card" />' },
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })
    await flushPromises()

    const deleteButton = wrapper.findAll('button').find(button => button.text().includes('imagePlayground.actions.deleteAll'))
    expect(deleteButton).toBeDefined()
    await deleteButton!.trigger('click')

    const dialog = wrapper.findComponent(ConfirmDialog)
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('message')).toBe('imagePlayground.deleteAll.message')
    expect(wrapper.find('[data-test="task-card"]').exists()).toBe(true)

    await dialog.vm.$emit('cancel')
    expect(dialog.props('show')).toBe(false)
    expect(localStorage.getItem('image_playground_history_v1')).not.toBeNull()

    await deleteButton!.trigger('click')
    await dialog.vm.$emit('confirm')
    await flushPromises()
    expect(deleteTask).toHaveBeenCalledWith('task-1')
    expect(dialog.props('show')).toBe(false)
    expect(localStorage.getItem('image_playground_history_v1')).toBeNull()
    expect(wrapper.find('[data-test="task-card"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('submits a validated custom size for gpt-image-2', async () => {
    getOptions.mockResolvedValue({
      ...options,
      groups: [{
        ...options.groups[0],
        models: [{
          ...options.groups[0].models[0],
          id: 'gpt-image-2',
          sizes: ['auto', '1024x1024', '1536x864'],
          custom_size_constraints: {
            max_edge: 3840,
            multiple_of: 16,
            max_aspect_ratio: 3,
            min_pixels: 655360,
            max_pixels: 8294400,
          },
          max_images: 10,
        }],
      }],
    })

    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: {
            props: ['modelValue', 'options'],
            emits: ['update:modelValue'],
            template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
          },
          PlaygroundTaskCard: { template: '<div data-test="task-card" />' },
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })

    await flushPromises()
    const selects = wrapper.findAll('select')
    expect(selects).toHaveLength(6)
    await selects[2].setValue('__custom__')
    await flushPromises()

    const sizeInputs = wrapper.findAll('input[type="number"]')
    expect(sizeInputs).toHaveLength(2)
    await sizeInputs[0].setValue('1537')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await sizeInputs[0].setValue('1536')
    await sizeInputs[1].setValue('864')
    await wrapper.get('textarea').setValue('A widescreen editorial illustration')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(submitTask).toHaveBeenCalledWith(expect.objectContaining({
      model: 'gpt-image-2',
      size: '1536x864',
      background: 'auto',
    }))
    wrapper.unmount()
  })

  it('submits an image-only request with reference files', async () => {
    getOptions.mockResolvedValue({
      ...options,
      groups: [{
        ...options.groups[0],
        models: [{
          ...options.groups[0].models[0],
          id: 'gpt-image-2',
          supports_image_input: true,
          max_input_images: 4,
          max_input_image_bytes: 10 * 1024 * 1024,
          input_image_formats: ['image/png'],
        }],
      }],
    })
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: {
            props: ['modelValue', 'options'],
            emits: ['update:modelValue'],
            template: '<select :value="modelValue"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
          },
          PlaygroundTaskCard: { template: '<div data-test="task-card" />' },
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })
    await flushPromises()

    const file = new File(['png'], 'reference.png', { type: 'image/png' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(submitTask).toHaveBeenCalledWith(expect.objectContaining({
      model: 'gpt-image-2',
      prompt: '',
    }), [file])
    wrapper.unmount()
  })

  it('notifies the user when no dedicated image group exists', async () => {
    getOptions.mockResolvedValue({ enabled: true, groups: [] })
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          PlaygroundTaskCard: true,
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('imagePlayground.unavailable.contactAdminForDedicatedGroup')
    expect(wrapper.text()).toContain('imagePlayground.unavailable.contactAdminForDedicatedGroup')
    wrapper.unmount()
  })

  it('prompts the user to create a dedicated image group API key', async () => {
    getOptions.mockResolvedValue({
      enabled: true,
      groups: [{
        ...options.groups[0],
        available: false,
        unavailable_reason: 'API_KEY_REQUIRED',
      }],
    })
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
          PlaygroundTaskCard: true,
          PlaygroundDetailDialog: true,
        },
      },
    })
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('imagePlayground.unavailable.createImageGroupAPIKey')
    expect(wrapper.text()).toContain('imagePlayground.unavailable.apiKeyTitle')
    expect(wrapper.text()).toContain('imagePlayground.actions.createAPIKey')
    expect(wrapper.get('a').attributes('href')).toBe('/keys')
    expect(wrapper.find('form').exists()).toBe(false)
    wrapper.unmount()
  })
})
