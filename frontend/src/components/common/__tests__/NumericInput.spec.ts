import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import NumericInput from '@/components/common/NumericInput.vue'

describe('NumericInput', () => {
  it('integer=true 时 input 1000000 emit 1000000（无浮点误差）', async () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 0, integer: true },
    })
    const input = wrapper.get('input')
    await input.setValue('1000000')
    const events = wrapper.emitted('update:modelValue') as unknown[][]
    expect(events.at(-1)).toEqual([1000000])
  })

  it('integer=true 时禁用 keydown "."（preventDefault 阻止小数点）', () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 100, integer: true },
    })
    const input = wrapper.get('input')
    const event = new KeyboardEvent('keydown', { key: '.', cancelable: true })
    const prevent = vi.spyOn(event, 'preventDefault')
    input.element.dispatchEvent(event)
    expect(prevent).toHaveBeenCalled()
  })

  it('integer=true 时不阻止 Backspace / 方向键 / Ctrl+V', () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 100, integer: true },
    })
    const input = wrapper.get('input')
    for (const key of ['Backspace', 'ArrowLeft', 'Tab', 'Delete']) {
      const event = new KeyboardEvent('keydown', { key, cancelable: true })
      const prevent = vi.spyOn(event, 'preventDefault')
      input.element.dispatchEvent(event)
      expect(prevent).not.toHaveBeenCalled()
    }
    // Ctrl+V 放行
    const ev = new KeyboardEvent('keydown', { key: 'v', ctrlKey: true, cancelable: true })
    const prevent2 = vi.spyOn(ev, 'preventDefault')
    input.element.dispatchEvent(ev)
    expect(prevent2).not.toHaveBeenCalled()
  })

  it('integer=true 时数字键 0-9 放行', () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 0, integer: true },
    })
    const input = wrapper.get('input')
    for (const key of ['0', '5', '9']) {
      const event = new KeyboardEvent('keydown', { key, cancelable: true })
      const prevent = vi.spyOn(event, 'preventDefault')
      input.element.dispatchEvent(event)
      expect(prevent).not.toHaveBeenCalled()
    }
  })

  it('integer=true + 粘贴 "1e6" parseInt 截断到 1', async () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 0, integer: true },
    })
    await wrapper.get('input').setValue('1e6')
    const events = wrapper.emitted('update:modelValue') as unknown[][]
    expect(events.at(-1)).toEqual([1])
  })

  it('integer=true + blur 时非整数 value 强制 round', async () => {
    const wrapper = mount(NumericInput, {
      // 模拟外部传入毛刺值（来自老 DB 数据）
      props: { modelValue: 999999.999997, integer: true },
    })
    await wrapper.get('input').trigger('blur')
    const events = wrapper.emitted('update:modelValue') as unknown[][]
    expect(events.at(-1)).toEqual([1000000])
  })

  it('integer=false（默认）允许小数', async () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 0, step: 0.01 },
    })
    await wrapper.get('input').setValue('12.34')
    const events = wrapper.emitted('update:modelValue') as unknown[][]
    expect(events.at(-1)).toEqual([12.34])
  })

  it('空字符串 emit null（保持必填语义）', async () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 100, integer: true },
    })
    await wrapper.get('input').setValue('')
    const events = wrapper.emitted('update:modelValue') as unknown[][]
    expect(events.at(-1)).toEqual([null])
  })

  it('blur clamp 到 [min, max]', async () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: -5, min: 0, max: 100 },
    })
    await wrapper.get('input').trigger('blur')
    const events = wrapper.emitted('update:modelValue') as unknown[][]
    expect(events.at(-1)).toEqual([0])

    const wrapper2 = mount(NumericInput, {
      props: { modelValue: 200, min: 0, max: 100 },
    })
    await wrapper2.get('input').trigger('blur')
    const events2 = wrapper2.emitted('update:modelValue') as unknown[][]
    expect(events2.at(-1)).toEqual([100])
  })

  it('integer 模式 effectiveStep 强制为 1（覆盖父组件传入的小数 step）', () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 0, integer: true, step: 0.000001 },
    })
    expect(wrapper.get('input').attributes('step')).toBe('1')
  })

  it('非 integer 模式尊重父组件 step', () => {
    const wrapper = mount(NumericInput, {
      props: { modelValue: 0, step: 0.01 },
    })
    expect(wrapper.get('input').attributes('step')).toBe('0.01')
  })
})
