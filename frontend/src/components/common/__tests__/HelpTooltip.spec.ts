import { afterEach, describe, expect, it } from 'vitest'
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

const initialViewport = {
  width: window.innerWidth,
  height: window.innerHeight,
  scrollX: window.scrollX,
  scrollY: window.scrollY,
}

function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height })
}

function mockRect(element: HTMLElement, rect: { top: number; bottom: number; left: number; width: number; height: number }) {
  element.getBoundingClientRect = () => ({
    ...rect,
    right: rect.left + rect.width,
    x: rect.left,
    y: rect.top,
    toJSON: () => ({}),
  }) as DOMRect
}

function getTooltipContentElement(tooltip: HTMLElement): HTMLElement {
  const content = tooltip.querySelector<HTMLElement>('.tooltip-content')
  if (!content) throw new Error('tooltip content element not found')
  return content
}

const TOOLTIP_CHROME_HEIGHT = 24

function mockTooltipSize(tooltip: HTMLElement, width: number, height: number) {
  const content = getTooltipContentElement(tooltip)
  Object.defineProperty(tooltip, 'offsetWidth', { configurable: true, value: width })
  Object.defineProperty(tooltip, 'offsetHeight', { configurable: true, value: height + TOOLTIP_CHROME_HEIGHT })
  Object.defineProperty(content, 'offsetHeight', { configurable: true, value: height })
  Object.defineProperty(content, 'scrollHeight', { configurable: true, value: height })
}

describe('HelpTooltip', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    setViewport(initialViewport.width, initialViewport.height)
    Object.defineProperty(window, 'scrollX', { configurable: true, value: initialViewport.scrollX })
    Object.defineProperty(window, 'scrollY', { configurable: true, value: initialViewport.scrollY })
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

    wrapper.unmount()
  })

  it('opens on keyboard focus, describes the trigger and stays open on mouse leave until focus leaves', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      attrs: { tabindex: 0 },
      props: { content: 'keyboard details' },
    })
    const trigger = wrapper.get('.group')
    const element = trigger.element as HTMLElement
    const tooltip = getTooltipElement()

    element.focus()
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(trigger.attributes('aria-describedby')).toBe(tooltip.id)
    expect(tooltip.id).not.toBe('')

    await trigger.trigger('mouseenter')
    await trigger.trigger('mouseleave')
    expect(tooltip.style.display).not.toBe('none')

    element.blur()
    await nextTick()
    expect(tooltip.style.display).toBe('none')
    expect(trigger.attributes('aria-describedby')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps hovered details open when focus leaves and supports Escape dismissal', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      attrs: { tabindex: 0 },
      props: { content: 'hover and focus details' },
    })
    const trigger = wrapper.get('.group')
    const element = trigger.element as HTMLElement
    const tooltip = getTooltipElement()

    await trigger.trigger('mouseenter')
    element.focus()
    element.blur()
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    await trigger.trigger('mouseleave')
    expect(tooltip.style.display).toBe('none')

    element.focus()
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')
    wrapper.unmount()
  })

  it('does not change click-only tooltips into focus or hover tooltips', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      attrs: { tabindex: 0 },
      props: { content: 'click-only details', trigger: 'click' },
    })
    const trigger = wrapper.get('.group')
    const element = trigger.element as HTMLElement
    const tooltip = getTooltipElement()

    element.focus()
    await nextTick()
    await trigger.trigger('mouseenter')
    expect(tooltip.style.display).toBe('none')
    await trigger.trigger('click')
    expect(tooltip.style.display).not.toBe('none')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')
    wrapper.unmount()
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

    wrapper.unmount()
  })

  it('keeps the hover bridge and arrow outside the scrollable content', async () => {
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'short details' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    const content = getTooltipContentElement(tooltip)

    await trigger.trigger('mouseenter')
    await nextTick()

    expect(tooltip.classList.contains('overflow-y-auto')).toBe(false)
    expect(tooltip.classList.contains('before:top-full')).toBe(true)
    expect(content.parentElement).toBe(tooltip)
    expect(content.classList.contains('overflow-y-auto')).toBe(true)
    const arrow = tooltip.querySelector('.-bottom-1')
    expect(arrow?.parentElement).toBe(tooltip)
    expect(arrow?.parentElement).not.toBe(content)
    wrapper.unmount()
  })

  it('uses viewport coordinates after scrolling and clamps the tooltip horizontally', async () => {
    setViewport(390, 844)
    Object.defineProperty(window, 'scrollX', { configurable: true, value: 40 })
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 2000 })
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'clamped details' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    mockRect(trigger.element, { top: 300, bottom: 320, left: 20, width: 20, height: 20 })
    mockTooltipSize(tooltip, 320, 200)

    await trigger.trigger('mouseenter')
    await nextTick()

    expect(tooltip.style.top).toBe('292px')
    expect(tooltip.style.left).toBe('168px')
    expect(getTooltipContentElement(tooltip).style.maxHeight).toBe(`${284 - TOOLTIP_CHROME_HEIGHT}px`)
    expect(tooltip.classList.contains('-translate-y-full')).toBe(true)
    wrapper.unmount()
  })

  it('flips below the trigger when the tooltip does not fit above', async () => {
    setViewport(390, 600)
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'flipped details' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    mockRect(trigger.element, { top: 100, bottom: 120, left: 185, width: 20, height: 20 })
    mockTooltipSize(tooltip, 200, 300)

    await trigger.trigger('mouseenter')
    await nextTick()

    expect(tooltip.style.top).toBe('128px')
    expect(tooltip.style.left).toBe('195px')
    expect(getTooltipContentElement(tooltip).style.maxHeight).toBe(`${464 - TOOLTIP_CHROME_HEIGHT}px`)
    expect(tooltip.classList.contains('-translate-y-full')).toBe(false)
    expect(tooltip.querySelector('.-top-1')).not.toBeNull()
    wrapper.unmount()
  })

  it('limits readable height to the larger available side when neither side fits', async () => {
    setViewport(390, 600)
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'scrollable details' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    mockRect(trigger.element, { top: 300, bottom: 320, left: 185, width: 20, height: 20 })
    mockTooltipSize(tooltip, 200, 500)

    await trigger.trigger('mouseenter')
    await nextTick()

    expect(tooltip.style.top).toBe('292px')
    expect(getTooltipContentElement(tooltip).style.maxHeight).toBe(`${284 - TOOLTIP_CHROME_HEIGHT}px`)
    expect(tooltip.classList.contains('overflow-y-auto')).toBe(false)
    expect(getTooltipContentElement(tooltip).classList.contains('overflow-y-auto')).toBe(true)
    expect(tooltip.classList.contains('-translate-y-full')).toBe(true)
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
