import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'

import Badge from '../Badge.vue'
import Button from '../Button.vue'
import FormField from '../FormField.vue'
import Meter from '../Meter.vue'
import Metric from '../Metric.vue'
import NumCell from '../NumCell.vue'
import StatusDot from '../StatusDot.vue'
import Surface from '../Surface.vue'
import { TONE_SOLID, TONE_TINT } from '../primitives'

/**
 * These assert the DESIGN CONTRACT, not the markup.
 *
 * Each case below corresponds to a rule the old system broke, so a regression
 * here is a regression in the design system rather than a cosmetic diff:
 * pill badges, colour-only status, `0` standing in for "no data", decorative
 * semantic colour, and forms that jump when validation fires.
 */

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} } })
const global = { plugins: [i18n] }

describe('Button', () => {
  it('is the only accent-filled control, and only when asked', () => {
    const plain = mount(Button, { global, slots: { default: 'Save' } })
    expect(plain.classes().join(' ')).not.toContain('bg-accent')

    const primary = mount(Button, {
      global,
      props: { variant: 'solid', tone: 'accent' },
      slots: { default: 'Save' },
    })
    expect(primary.classes().join(' ')).toContain('bg-accent-solid')
  })

  it('inverts the focus ring on an accent fill so it cannot vanish into it', () => {
    const primary = mount(Button, { global, props: { variant: 'solid', tone: 'accent' } })
    expect(primary.classes().join(' ')).toContain('focus-visible:outline-focus-contrast')
  })

  it('never presses in or lifts', () => {
    const cls = mount(Button, { global }).classes().join(' ')
    expect(cls).not.toMatch(/active:scale-\[/)
    expect(cls).not.toMatch(/hover:-translate-y-/)
    expect(cls).not.toMatch(/shadow-(glow|card)/)
  })

  it('keeps the label box while loading so toolbars do not reflow', () => {
    const w = mount(Button, { global, props: { loading: true }, slots: { default: 'Save' } })
    // Label still rendered, just hidden — not removed from layout.
    expect(w.text()).toContain('Save')
    expect(w.find('.invisible').exists()).toBe(true)
    expect(w.attributes('aria-busy')).toBe('true')
    expect(w.attributes('disabled')).toBeDefined()
  })

  it('renders as an anchor when given an href', () => {
    expect(mount(Button, { global, props: { href: '/docs' } }).element.tagName).toBe('A')
  })
})

describe('Badge', () => {
  it('is squared, never a pill', () => {
    const cls = mount(Badge, { global, slots: { default: 'anthropic' } })
      .classes()
      .join(' ')
    expect(cls).toContain('rounded-sm')
    expect(cls).not.toContain('rounded-full')
  })

  it('always pairs its tint with a border, so colour is never the only channel', () => {
    for (const tone of Object.keys(TONE_TINT) as Array<keyof typeof TONE_TINT>) {
      expect(TONE_TINT[tone], `${tone} tint must carry a border`).toMatch(/border-/)
      expect(TONE_SOLID[tone], `${tone} solid must carry a border`).toMatch(/border-/)
    }
  })
})

describe('StatusDot', () => {
  it('renders the label alongside the dot', () => {
    const w = mount(StatusDot, { global, props: { tone: 'danger', label: 'down' } })
    expect(w.text()).toBe('down')
    expect(w.find('.rounded-full').exists()).toBe(true)
  })

  it('hides the dot from assistive tech, since the label carries the meaning', () => {
    const w = mount(StatusDot, { global, props: { label: 'ok' } })
    expect(w.find('.rounded-full').attributes('aria-hidden')).toBe('true')
  })
})

describe('NumCell', () => {
  it('distinguishes "no measurement" from zero', () => {
    expect(mount(NumCell, { global, props: { value: null } }).text()).toBe('–')
    expect(mount(NumCell, { global, props: { value: 0 } }).text()).toBe('0')
  })

  it('renders tabular mono figures so columns align', () => {
    const cls = mount(NumCell, { global, props: { value: 1284902 } })
      .classes()
      .join(' ')
    expect(cls).toContain('font-mono')
    expect(cls).toContain('tabular-nums')
  })

  it('groups thousands and keeps the unrounded value reachable', () => {
    const w = mount(NumCell, { global, props: { value: 1284902.456, precision: 1 } })
    expect(w.text()).toContain('1,284,902.5')
    expect(w.find('span[title]').attributes('title')).toBe('1284902.456')
  })

  it('subordinates the unit to the number', () => {
    const w = mount(NumCell, { global, props: { value: 318, unit: 'ms' } })
    const unit = w.findAll('span').find((s) => s.text() === 'ms')
    expect(unit?.classes().join(' ')).toContain('text-ink-tertiary')
  })

  it('treats a non-numeric string as no measurement', () => {
    expect(mount(NumCell, { global, props: { value: 'n/a' } }).text()).toBe('–')
  })
})

describe('Metric', () => {
  it('leads with the value, not an icon tile', () => {
    const w = mount(Metric, { global, props: { label: 'Requests', value: 1284902 } })
    expect(w.text()).toContain('1,284,902')
    // The old StatCard rendered a 48px coloured square here.
    expect(w.find('.stat-icon').exists()).toBe(false)
    expect(w.find('.h-12').exists()).toBe(false)
  })

  it('pairs the delta colour with a direction glyph', () => {
    const up = mount(Metric, { global, props: { label: 'x', value: 1, delta: 0.124 } })
    expect(up.text()).toContain('▲')
    expect(up.html()).toContain('text-success')

    const down = mount(Metric, { global, props: { label: 'x', value: 1, delta: -0.05 } })
    expect(down.text()).toContain('▼')
    expect(down.html()).toContain('text-danger')
  })

  it('inverts good/bad for metrics where a rise is worse', () => {
    const w = mount(Metric, {
      global,
      props: { label: 'p95', value: 318, delta: 0.2, invertDelta: true },
    })
    expect(w.html()).toContain('text-danger')
  })
})

describe('Meter', () => {
  it('stays neutral below the warn threshold — colour is a signal, not decoration', () => {
    const w = mount(Meter, { global, props: { value: 40, max: 100 } })
    expect(w.html()).toContain('bg-ink-secondary')
    expect(w.html()).not.toContain('bg-warn')
    expect(w.html()).not.toContain('bg-danger')
  })

  it('escalates at the declared thresholds', () => {
    expect(mount(Meter, { global, props: { value: 85 } }).html()).toContain('bg-warn')
    expect(mount(Meter, { global, props: { value: 99 } }).html()).toContain('bg-danger')
  })

  it('always shows the number, since the bar is the redundant channel', () => {
    expect(mount(Meter, { global, props: { value: 62 } }).text()).toContain('62%')
    expect(mount(Meter, { global, props: { value: 3, max: 8, format: 'ratio' } }).text()).toContain(
      '3 / 8'
    )
  })

  it('exposes a meter role with a readable value', () => {
    const bar = mount(Meter, { global, props: { value: 62, label: 'Quota' } }).find('[role=meter]')
    expect(bar.attributes('aria-valuenow')).toBe('62')
    expect(bar.attributes('aria-valuemax')).toBe('100')
    expect(bar.attributes('aria-valuetext')).toContain('62%')
  })

  it('clamps out-of-range values instead of overflowing the track', () => {
    const over = mount(Meter, { global, props: { value: 150, max: 100 } })
    expect(over.html()).toContain('width: 100%')
    const under = mount(Meter, { global, props: { value: -5, max: 100 } })
    expect(under.html()).toContain('width: 0%')
  })
})

describe('FormField', () => {
  it('reserves the message line so validation does not reflow the form', () => {
    const quiet = mount(FormField, { global, props: { label: 'Name' } })
    const row = quiet.find('p')
    expect(row.exists()).toBe(true)
    expect(row.attributes('style')).toContain('min-height')
  })

  it('wires label, control and message together', () => {
    const w = mount(FormField, {
      global,
      props: { label: 'Name', error: 'Required' },
      slots: {
        default: `<template #default="{ id, describedBy, invalid }">
          <input :id="id" :aria-describedby="describedBy" :aria-invalid="invalid" />
        </template>`,
      },
    })
    const id = w.find('input').attributes('id')
    expect(w.find('label').attributes('for')).toBe(id)
    expect(w.find('input').attributes('aria-describedby')).toBe(`${id}-message`)
    expect(w.find('input').attributes('aria-invalid')).toBe('true')
    expect(w.find('p').text()).toBe('Required')
  })

  it('does not advertise a description when there is nothing to read', () => {
    const w = mount(FormField, {
      global,
      props: { label: 'Name' },
      slots: {
        default: `<template #default="{ describedBy }">
          <input :aria-describedby="describedBy" />
        </template>`,
      },
    })
    expect(w.find('input').attributes('aria-describedby')).toBeUndefined()
  })

  it('prefers the error over the hint', () => {
    const w = mount(FormField, { global, props: { hint: 'Optional', error: 'Too short' } })
    expect(w.find('p').text()).toBe('Too short')
    expect(w.find('p').classes()).toContain('text-danger')
  })
})

describe('Surface', () => {
  it('is a hairline box with no shadow', () => {
    const cls = mount(Surface, { global, slots: { default: 'x' } })
      .classes()
      .join(' ')
    expect(cls).toContain('border-line')
    expect(cls).toContain('rounded')
    expect(cls).not.toMatch(/shadow-/)
  })

  it('drops body padding when flush, so a table is not double-guttered', () => {
    const padded = mount(Surface, { global, slots: { default: '<div/>' } })
    expect(padded.html()).toContain('p-4')
    const flush = mount(Surface, { global, props: { flush: true }, slots: { default: '<div/>' } })
    expect(flush.find('.p-4').exists()).toBe(false)
  })

  it('only renders the header rule when there is a header', () => {
    expect(mount(Surface, { global, slots: { default: 'x' } }).find('.border-b').exists()).toBe(
      false
    )
    expect(
      mount(Surface, { global, props: { title: 'Groups' }, slots: { default: 'x' } })
        .find('.border-b')
        .exists()
    ).toBe(true)
  })
})
