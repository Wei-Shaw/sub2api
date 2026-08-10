import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getOptions, listTasks, submitTask, getTask, getPreview, deleteTask, deleteImage, showSuccess, showError } = vi.hoisted(() => ({
  getOptions: vi.fn(),
  listTasks: vi.fn(),
  submitTask: vi.fn(),
  getTask: vi.fn(),
  getPreview: vi.fn(),
  deleteTask: vi.fn(),
  deleteImage: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/imagePlayground', () => ({
  getImagePlaygroundOptions: getOptions,
  listImagePlaygroundTasks: listTasks,
  submitImagePlaygroundTask: submitTask,
  getImagePlaygroundTask: getTask,
  getImagePlaygroundImagePreview: getPreview,
  deleteImagePlaygroundTask: deleteTask,
  deleteImagePlaygroundImage: deleteImage,
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
      supports_image_input: true,
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
    listTasks.mockResolvedValue([])
    deleteTask.mockResolvedValue(undefined)
    deleteImage.mockResolvedValue(null)
    getPreview.mockResolvedValue(new Blob(['preview'], { type: 'image/png' }))
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

  it('collapses and restores the generation composer', async () => {
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: true,
          PlaygroundTaskCard: true,
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })
    await flushPromises()

    const toggle = wrapper.get('[data-test="composer-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-test="composer-content"]').isVisible()).toBe(true)

    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-test="composer-content"]').classes()).toContain('grid-rows-[0fr]')
    expect(wrapper.get('[data-test="composer-prompt-form"]').isVisible()).toBe(true)
    expect(wrapper.get('[data-test="composer-prompt-form"] textarea').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-test="composer-prompt-form"] [data-test="reference-image-picker"]').isVisible()).toBe(true)
    expect(wrapper.text()).toContain('OpenAI Images')
    expect(wrapper.text()).toContain('gpt-image-1.5')
    expect(wrapper.text()).toContain('1024×1024')

    await toggle.trigger('click')
    expect(wrapper.get('[data-test="composer-content"]').classes()).toContain('grid-rows-[1fr]')
    wrapper.unmount()
  })

  it('requires confirmation before deleting records and stored files', async () => {
    localStorage.setItem('image_playground_history_v1', JSON.stringify({
      ids: ['task-1'],
      meta: { 'task-1': { prompt: 'A paper sculpture', payload: { group_id: 7, model: 'gpt-image-1.5', prompt: 'A paper sculpture' } } },
    }))
    listTasks.mockResolvedValue([{
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
    }])
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

    const deleteButton = wrapper.get('[data-test="gallery-delete-all"]')
    await deleteButton.trigger('click')

    const dialog = wrapper.findComponent(ConfirmDialog)
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('message')).toBe('imagePlayground.deleteAll.message')
    expect(wrapper.find('[data-test="task-card"]').exists()).toBe(true)

    await dialog.vm.$emit('cancel')
    expect(dialog.props('show')).toBe(false)
    expect(localStorage.getItem('image_playground_history_v1')).not.toBeNull()

    await deleteButton.trigger('click')
    await dialog.vm.$emit('confirm')
    await flushPromises()
    expect(deleteTask).toHaveBeenCalledWith('task-1')
    expect(dialog.props('show')).toBe(false)
    expect(localStorage.getItem('image_playground_history_v1')).toBeNull()
    expect(wrapper.find('[data-test="task-card"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('confirms and deletes one generated image', async () => {
    listTasks.mockResolvedValue([{
      id: 'task-1',
      object: 'image.playground.task',
      status: 'completed',
      group_id: 7,
      platform: 'openai',
      model: 'gpt-image-1.5',
      images: [{ index: 0, url: '', download_url: '/download' }],
      created_at: 1_700_000_000,
      expires_at: 1_700_086_400,
      poll_url: '/api/v1/image-playground/tasks/task-1',
    }])
    deleteImage.mockResolvedValue(null)
    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: true,
          PlaygroundTaskCard: {
            props: ['task'],
            emits: ['deleteImage'],
            template: '<button data-test="delete-image" @click="$emit(\'deleteImage\', task, 0)" />',
          },
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-test="delete-image"]').trigger('click')
    const dialogs = wrapper.findAllComponents(ConfirmDialog)
    const dialog = dialogs.find((item) => item.props('show') && item.props('title') === 'imagePlayground.deleteImage.title')
    expect(dialog).toBeDefined()
    await dialog!.vm.$emit('confirm')
    await flushPromises()

    expect(deleteImage).toHaveBeenCalledWith('task-1', 0)
    expect(wrapper.find('[data-test="delete-image"]').exists()).toBe(false)
    expect(showSuccess).toHaveBeenCalledWith('imagePlayground.messages.imageDeleted')
    wrapper.unmount()
  })

  it('loads server history when the browser has no local task ids', async () => {
    listTasks.mockResolvedValue([{
      id: 'server-task',
      object: 'image.playground.task',
      status: 'completed',
      group_id: 7,
      platform: 'openai',
      model: 'gpt-image-1.5',
      images: [{ index: 0, url: 'https://cdn.example/server.png', download_url: '/download' }],
      created_at: 1_700_000_000,
      expires_at: 1_700_086_400,
      poll_url: '/api/v1/image-playground/tasks/server-task',
    }])

    const wrapper = mount(ImagePlaygroundView, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Select: true,
          PlaygroundTaskCard: {
            props: ['task'],
            template: '<div data-test="task-card" :data-image-url="task.images[0]?.url" />',
          },
          PlaygroundDetailDialog: true,
          RouterLink: true,
        },
      },
    })
    await flushPromises()

    expect(listTasks).toHaveBeenCalledTimes(1)
    expect(getPreview).toHaveBeenCalledWith('server-task', 0)
    expect(URL.createObjectURL).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="task-card"]').attributes('data-image-url')).toBe('blob:preview')
    expect(wrapper.find('[data-test="task-card"]').exists()).toBe(true)
    expect(JSON.parse(localStorage.getItem('image_playground_history_v1') || '{}').ids).toEqual(['server-task'])
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
