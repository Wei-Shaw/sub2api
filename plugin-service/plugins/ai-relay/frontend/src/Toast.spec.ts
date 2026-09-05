import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Toast from './Toast.vue'

afterEach(() => {
  vi.useRealTimers()
})

describe('Toast', () => {
  it('renders a success notification and dismisses after its duration', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Toast, {
      props: { type: 'success', message: 'Plugin URL copied', duration: 3000 },
    })

    expect(wrapper.get('[role="status"]').text()).toContain('Plugin URL copied')
    expect(wrapper.get('[role="status"]').classes()).toContain('relay-toast-success')
    expect(wrapper.find('[aria-label="Close notification"]').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(3000)
    expect(wrapper.emitted('dismiss')).toHaveLength(1)
  })

  it('renders an error notification and supports manual dismissal', async () => {
    const wrapper = mount(Toast, {
      props: { type: 'error', message: 'Failed to copy Plugin URL', duration: 5000 },
    })

    expect(wrapper.get('[role="status"]').text()).toContain('Failed to copy Plugin URL')
    expect(wrapper.get('[role="status"]').classes()).toContain('relay-toast-error')

    await wrapper.get('[aria-label="Close notification"]').trigger('click')
    expect(wrapper.emitted('dismiss')).toHaveLength(1)
  })
})
