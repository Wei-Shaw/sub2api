import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
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

    const closeButton = tooltip.querySelector('button[aria-label="common.close"]')
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

  it('opens a hover tooltip on keyboard focus, describes the trigger, and closes on blur or Escape', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'focus details' },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    expect(trigger.attributes('tabindex')).toBe('0')
    expect(trigger.attributes('role')).toBe('button')
    expect(trigger.attributes('aria-describedby')).toBe(tooltip.id)
    expect(tooltip.id).not.toBe('')

    ;(trigger.element as HTMLElement).focus()
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    ;(trigger.element as HTMLElement).blur()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    ;(trigger.element as HTMLElement).focus()
    await nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('toggles a click tooltip with Enter on the focused trigger', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'click details', trigger: 'click' },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('keydown', { key: 'Enter' })
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('keydown', { key: 'Enter' })
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('does not add a second tab stop around a trigger slot that is already focusable', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'https://example.com' },
      slots: { trigger: '<a href="https://example.com">site</a>' },
    })
    await nextTick()

    const trigger = wrapper.get('.group')
    expect(trigger.attributes('tabindex')).toBeUndefined()
    expect(trigger.attributes('role')).toBeUndefined()

    wrapper.unmount()
  })
})
