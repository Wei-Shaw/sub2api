import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

enableAutoUnmount(afterEach)

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('keeps the existing hover interaction by default', async () => {
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
    expect(tooltip.style.display).toBe('none')
  })

  it('keeps a hover tooltip open while the pointer moves between the trigger and the tooltip', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'copyable details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave', { relatedTarget: tooltip })
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave', { relatedTarget: trigger.element }))
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave', { relatedTarget: null }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')
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
  })

  it.each([
    { scrollX: 120, scrollY: 0 },
    { scrollX: 0, scrollY: 240 },
    { scrollX: 120, scrollY: 240 },
  ])('uses viewport coordinates when opening with scroll ($scrollX, $scrollY)', async ({ scrollX, scrollY }) => {
    vi.stubGlobal('scrollX', scrollX)
    vi.stubGlobal('scrollY', scrollY)
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'positioned details' },
    })
    const trigger = wrapper.get('.group')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(new DOMRect(80, 160, 24, 24))

    await trigger.trigger('mouseenter')
    await nextTick()

    const tooltip = getTooltipElement()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.classList.contains('fixed')).toBe(true)
    expect(tooltip.style.left).toBe('92px')
    expect(tooltip.style.top).toBe('calc(152px)')
  })

  it.each(['window scroll', 'nested scroll', 'resize'] as const)(
    'recalculates viewport coordinates for an open tooltip on %s',
    async (change) => {
      const container = document.createElement('div')
      document.body.appendChild(container)
      const wrapper = mount(HelpTooltip, {
        attachTo: container,
        props: { content: 'moving details', trigger: 'click' },
      })
      const trigger = wrapper.get('.group')
      const getBoundingClientRect = vi.spyOn(trigger.element, 'getBoundingClientRect')
        .mockReturnValue(new DOMRect(80, 160, 24, 24))

      await trigger.trigger('click')
      await nextTick()

      const tooltip = getTooltipElement()
      expect(tooltip.style.left).toBe('92px')
      expect(tooltip.style.top).toBe('calc(152px)')

      vi.stubGlobal('scrollX', 120)
      vi.stubGlobal('scrollY', 240)
      getBoundingClientRect.mockReturnValue(new DOMRect(40, 100, 32, 24))

      if (change === 'nested scroll') {
        container.dispatchEvent(new Event('scroll', { bubbles: false }))
      } else {
        window.dispatchEvent(new Event(change === 'resize' ? 'resize' : 'scroll'))
      }
      await nextTick()

      expect(tooltip.style.display).not.toBe('none')
      expect(getBoundingClientRect).toHaveBeenCalledTimes(2)
      expect(tooltip.style.left).toBe('56px')
      expect(tooltip.style.top).toBe('calc(92px)')
    },
  )
})
