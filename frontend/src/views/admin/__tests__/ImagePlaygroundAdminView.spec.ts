import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { listTasks, getPreview, deleteTask, deleteImage, showSuccess, showError } = vi.hoisted(() => ({
  listTasks: vi.fn(),
  getPreview: vi.fn(),
  deleteTask: vi.fn(),
  deleteImage: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/imagePlayground', () => ({
  listAdminImagePlaygroundTasks: listTasks,
  getAdminImagePlaygroundPreview: getPreview,
  deleteAdminImagePlaygroundTask: deleteTask,
  deleteAdminImagePlaygroundImage: deleteImage,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en-US' },
    }),
  }
})

import ImagePlaygroundAdminView from '@/views/admin/ImagePlaygroundAdminView.vue'

let observedElements: Element[] = []
let intersectionCallback: IntersectionObserverCallback

function revealObservedPreviews(): void {
  intersectionCallback(observedElements.map((target) => ({
    target,
    isIntersecting: true,
  } as IntersectionObserverEntry)), {} as IntersectionObserver)
}

describe('ImagePlaygroundAdminView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:admin-preview') })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
    getPreview.mockResolvedValue(new Blob(['image'], { type: 'image/png' }))
    observedElements = []
    vi.stubGlobal('IntersectionObserver', class {
      constructor(callback: IntersectionObserverCallback) {
        intersectionCallback = callback
      }

      observe(target: Element): void { observedElements.push(target) }
      unobserve(): void {}
      disconnect(): void { observedElements = [] }
      takeRecords(): IntersectionObserverEntry[] { return [] }
      readonly root = null
      readonly rootMargin = ''
      readonly thresholds = []
    })
    listTasks.mockResolvedValue({
      tasks: [{
        task: {
          id: 'imgtask_1',
          status: 'completed',
          group_id: 7,
          platform: 'openai',
          model: 'gpt-image-2',
          prompt_preview: 'A glass observatory',
          images: [
            { index: 0, url: '', download_url: '' },
            { index: 1, url: '', download_url: '' },
          ],
          created_at: 1_700_000_000,
          poll_url: '',
        },
        user_id: 42,
        api_key_id: 9,
        user_email: 'user@example.com',
        storage_bytes: 1536,
        image_sizes: [512, 1024],
      }],
      page: 1,
      page_size: 24,
      total: 1,
      total_images: 2,
      storage_bytes: 1536,
    })
  })

  it('shows global task ownership, previews, and storage totals', async () => {
    const wrapper = mount(ImagePlaygroundAdminView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          Pagination: true,
          ConfirmDialog: true,
        },
      },
    })
    await flushPromises()

    expect(listTasks).toHaveBeenCalledWith(1, 24)
		expect(getPreview).not.toHaveBeenCalled()
		revealObservedPreviews()
		await flushPromises()
    expect(getPreview).toHaveBeenCalledTimes(2)
    expect(getPreview).toHaveBeenCalledWith('imgtask_1', 0)
    expect(getPreview).toHaveBeenCalledWith('imgtask_1', 1)
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('API Key #9')
    expect(wrapper.text()).toContain('A glass observatory')
    expect(wrapper.text()).toContain('1.5 KB')
    expect(wrapper.findAll('img')).toHaveLength(2)

    wrapper.unmount()
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2)
  })

	it('limits preview downloads when many images enter the viewport together', async () => {
		listTasks.mockResolvedValue({
			tasks: [{
				task: {
					id: 'imgtask_many', status: 'completed', platform: 'openai', model: 'gpt-image-2',
					images: Array.from({ length: 12 }, (_, index) => ({ id: `img_${index}`, index, url: '', download_url: '' })),
					created_at: 1_700_000_000,
				},
				user_id: 42, api_key_id: 9, storage_bytes: 12, image_sizes: Array(12).fill(1),
			}],
			page: 1, page_size: 24, total: 1, total_images: 12, storage_bytes: 12,
		})
		getPreview.mockImplementation(() => new Promise(() => {}))

		const wrapper = mount(ImagePlaygroundAdminView, {
			global: {
				stubs: {
					AppLayout: { template: '<main><slot /></main>' }, Icon: true, Pagination: true, ConfirmDialog: true,
				},
			},
		})
		await flushPromises()
		revealObservedPreviews()
		await flushPromises()

		expect(getPreview).toHaveBeenCalledTimes(4)
		wrapper.unmount()
	})
})
