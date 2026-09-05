import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MetricCell from '../MetricCell.vue'

describe('MetricCell', () => {
  it('renders label/value/detail through shipped template with design-system classes', () => {
    const wrapper = mount(MetricCell, {
      props: {
        label: '请求',
        value: '1,234',
        detail: '12.5 RPM',
        state: 'healthy',
      },
    })

    expect(wrapper.classes().join(' ') + wrapper.html()).toContain('stat-card')
    expect(wrapper.text()).toContain('请求')
    expect(wrapper.text()).toContain('1,234')
    expect(wrapper.text()).toContain('12.5 RPM')
    expect(wrapper.find('strong').classes().join(' ')).toMatch(/emerald/)
  })

  it('maps warning and critical health states to distinct colors', () => {
    const warning = mount(MetricCell, {
      props: { label: '错误', value: '10%', detail: '1 次', state: 'warning' },
    })
    expect(warning.find('strong').classes().join(' ')).toMatch(/amber/)

    const critical = mount(MetricCell, {
      props: { label: '错误', value: '50%', detail: '5 次', state: 'critical' },
    })
    expect(critical.find('strong').classes().join(' ')).toMatch(/red/)
  })

  it('renders multi-part detail as non-truncated chips (AVG · P90 fully visible)', () => {
    const wrapper = mount(MetricCell, {
      props: {
        label: '首 Token P50',
        value: '400ms',
        detail: 'AVG 475ms · P90 800ms',
        state: 'healthy',
      },
    })
    expect(wrapper.find('small.truncate').exists()).toBe(false)
    expect(wrapper.find('.whitespace-nowrap').exists()).toBe(true)
    expect(wrapper.text()).toContain('AVG 475ms')
    expect(wrapper.text()).toContain('P90 800ms')
    expect(wrapper.text()).not.toContain('…')
  })
})
