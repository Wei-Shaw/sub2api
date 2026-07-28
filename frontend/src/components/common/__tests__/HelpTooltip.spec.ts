import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('keeps the hover tooltip open while the pointer crosses into the teleported panel', async () => {
    vi.useFakeTimers()
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseenter'))
    vi.advanceTimersByTime(200)
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave'))
    vi.advanceTimersByTime(120)
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('positions a fixed tooltip before showing it without adding page scroll offsets', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'positioned details',
      },
    })
    const trigger = wrapper.get('.group')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({
      x: 40,
      y: 120,
      top: 120,
      left: 40,
      right: 56,
      bottom: 136,
      width: 16,
      height: 16,
      toJSON: () => ({}),
    })
    vi.spyOn(window, 'scrollX', 'get').mockReturnValue(500)
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(1000)

    await trigger.trigger('mouseenter')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.style.top).toBe('calc(112px)')
    expect(tooltip.style.left).toBe('48px')

    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })
})
