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

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
        tooltipClass: '!py-2',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')
    expect(tooltip.classList.contains('!py-2')).toBe(true)

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('supports a delayed hover without delaying keyboard focus', async () => {
    vi.useFakeTimers()
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'delayed details',
        openDelayMs: 500,
      },
      slots: { trigger: '<button type="button">balance</button>' },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(499)
    expect(tooltip.style.display).toBe('none')

    await vi.advanceTimersByTimeAsync(1)
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await trigger.trigger('focusin')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

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

  it('opens on keyboard focus and clamps fixed positioning to the viewport', async () => {
    const originalInnerWidth = window.innerWidth
    const originalInnerHeight = window.innerHeight
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 320 })
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'quota details' },
      slots: { trigger: '<button type="button">quota</button>' },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    const triggerRect = vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({
      x: 360,
      y: 120,
      top: 120,
      right: 380,
      bottom: 144,
      left: 360,
      width: 20,
      height: 24,
      toJSON: () => ({}),
    })
    const tooltipRect = vi.spyOn(tooltip, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      right: 300,
      bottom: 100,
      left: 0,
      width: 300,
      height: 100,
      toJSON: () => ({}),
    })

    await trigger.trigger('focusin')
    await nextTick()
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.style.left).toBe('232px')
    expect(tooltip.style.top).toBe('112px')
    expect(tooltip.style.maxHeight).toBe('104px')
    expect(tooltip.classList.contains('-translate-y-full')).toBe(true)

    triggerRect.mockReturnValue({
      x: 360,
      y: 8,
      top: 8,
      right: 380,
      bottom: 32,
      left: 360,
      width: 20,
      height: 24,
      toJSON: () => ({}),
    })
    tooltipRect.mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      right: 300,
      bottom: 500,
      left: 0,
      width: 300,
      height: 500,
      toJSON: () => ({}),
    })
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(tooltip.style.top).toBe('40px')
    expect(tooltip.style.maxHeight).toBe('272px')
    expect(tooltip.classList.contains('-translate-y-full')).toBe(false)

    await trigger.trigger('focusout')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalInnerWidth })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: originalInnerHeight })
  })
})
