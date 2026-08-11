import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

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

import PlaygroundTaskCard from '@/components/imagePlayground/PlaygroundTaskCard.vue'
import PlaygroundDetailDialog from '@/components/imagePlayground/PlaygroundDetailDialog.vue'
import type { ImagePlaygroundTask } from '@/api/imagePlayground'

const failedTask: ImagePlaygroundTask = {
  id: 'imgtask_failed',
  object: 'image.generation.task',
  status: 'failed',
  group_id: 14,
  platform: 'openai',
  model: 'gpt-image-2',
  prompt_preview: 'A test prompt',
  images: [],
  error: {
    type: 'image_generation_user_error',
    code: 'content_policy_violation',
    message: 'The prompt may contain sexual content or nudity.',
  },
  created_at: 1_700_000_000,
  poll_url: '',
}

describe('image task failure display', () => {
  it('lets the user open a failed task from its card', async () => {
    const wrapper = mount(PlaygroundTaskCard, {
      props: { task: failedTask, now: Date.now() },
      global: { stubs: { Icon: true, ExpiryCountdown: true } },
    })

    const previewButton = wrapper.find('article > button')
    expect(previewButton.attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).toContain('imagePlayground.errors.contentPolicyRejected')
    await previewButton.trigger('click')
    expect(wrapper.emitted('open')?.[0]?.[0]).toEqual(failedTask)
  })

  it('shows the complete provider error in the user detail dialog', () => {
    const wrapper = mount(PlaygroundDetailDialog, {
      props: { show: true, task: failedTask, now: Date.now() },
      global: {
        stubs: {
          BaseDialog: { template: '<section><slot /></section>' },
          Icon: true,
          ExpiryCountdown: true,
        },
      },
    })

    expect(wrapper.find('[data-test="image-task-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('content_policy_violation')
    expect(wrapper.text()).toContain('sexual content or nudity')
    expect(wrapper.text()).toContain('imagePlayground.detail.fullError')
    expect(wrapper.text()).not.toContain('imagePlayground.actions.download')
    expect(wrapper.text()).not.toContain('imagePlayground.actions.deleteImage')
  })
})
