import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MetricCell from '../MetricCell.vue'

/**
 * These assertions used to pin `stat-card` and `/emerald|amber|red/` on the
 * value element. Both are gone with the old visual language: the cell is no
 * longer a boxed card (a row of them sits inside one bordered panel), and the
 * tones are the semantic Family B tokens, which flip with the theme instead of
 * naming a static hue.
 *
 * The behavioural contract that matters is unchanged and re-asserted below:
 * label / value / detail render, a crossed threshold tints the number, and a
 * healthy one does not.
 */
describe('MetricCell', () => {
  it('renders label, value and detail with no box of its own', () => {
    const wrapper = mount(MetricCell, {
      props: {
        label: '请求',
        value: '1,234',
        detail: '12.5 RPM',
        state: 'healthy',
        stateLabel: '健康',
      },
    })

    expect(wrapper.text()).toContain('请求')
    expect(wrapper.text()).toContain('1,234')
    expect(wrapper.text()).toContain('12.5 RPM')
    const html = wrapper.html()
    expect(html).not.toContain('stat-card')
    expect(html).not.toContain('rounded-3xl')
    // Values are mono tabular so a row of tiles aligns on the digits.
    expect(wrapper.find('strong').classes()).toContain('font-mono')
    expect(wrapper.find('strong').classes()).toContain('tabular-nums')
  })

  it('leaves a healthy metric in plain ink and tints only crossed thresholds', () => {
    const healthy = mount(MetricCell, {
      props: { label: '成功率', value: '99.9%', detail: '-', state: 'healthy', stateLabel: '健康' },
    })
    // Signal budget: a KPI strip where every tile is green has spent its whole
    // colour budget before anything is wrong.
    expect(healthy.find('strong').classes()).toContain('text-ink')

    const warning = mount(MetricCell, {
      props: { label: '错误', value: '10%', detail: '1 次', state: 'warning', stateLabel: '需关注' },
    })
    expect(warning.find('strong').classes()).toContain('text-warn')

    const critical = mount(MetricCell, {
      props: { label: '错误', value: '50%', detail: '5 次', state: 'critical', stateLabel: '异常' },
    })
    expect(critical.find('strong').classes()).toContain('text-danger')
  })

  it('pairs the state dot with a word, and omits the dot when there is no word', () => {
    const labelled = mount(MetricCell, {
      props: { label: '缓存率', value: '80%', detail: '-', state: 'critical', stateLabel: '异常' },
    })
    // Colour is never the only channel: the label ships with the dot.
    expect(labelled.text()).toContain('异常')

    const unlabelled = mount(MetricCell, {
      props: { label: '缓存率', value: '80%', detail: '-', state: 'critical' },
    })
    // A bare coloured dot is unreadable in grayscale, so it is not rendered.
    expect(unlabelled.html()).not.toContain('rounded-full')
  })

  it('renders multi-part detail as non-truncated chips (AVG · P90 fully visible)', () => {
    const wrapper = mount(MetricCell, {
      props: {
        label: '首 Token P50',
        value: '400ms',
        detail: 'AVG 475ms · P90 800ms',
        state: 'healthy',
        stateLabel: '健康',
      },
    })
    expect(wrapper.find('small.truncate').exists()).toBe(false)
    expect(wrapper.find('.whitespace-nowrap').exists()).toBe(true)
    expect(wrapper.text()).toContain('AVG 475ms')
    expect(wrapper.text()).toContain('P90 800ms')
    expect(wrapper.text()).not.toContain('…')
  })
})
