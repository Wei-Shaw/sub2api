import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UserAgentDistributionChart from '../UserAgentDistributionChart.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({ Doughnut: { props: ['data'], template: '<div data-testid="chart">{{ JSON.stringify(data) }}</div>' } }))

describe('UserAgentDistributionChart', () => {
  const stats = [
    { user_agent: 'codex-tui/1.0 (Windows)', client: 'Codex', version: '1.0', requests: 2 },
    { user_agent: 'codex-tui/1.0 (Linux)', client: 'Codex', version: '1.0', requests: 1 },
    { user_agent: 'codex-tui/2.0-alpha.1', client: 'Codex', version: '2.0-alpha.1', requests: 3 },
    { user_agent: '', client: '__missing__', version: '', requests: 4 },
  ]

  it('groups clients and versions without losing full UA rows or requests', () => {
    const wrapper = mount(UserAgentDistributionChart, { props: { stats } })
    const clients = wrapper.findAll('[data-testid="ua-client"]')
    expect(clients).toHaveLength(2)
    expect(clients[0].find('summary').text()).toContain('Codex')
    expect(clients[0].find('summary').text()).toContain('60.0%')
    expect(clients[0].findAll('[data-testid="ua-version"]')).toHaveLength(2)
    expect(clients[0].findAll('[data-testid="ua-raw"]')).toHaveLength(3)
    expect(clients[0].text()).toContain('codex-tui/1.0 (Linux)')
    expect(clients[1].text()).toContain('usage.uaMissing')
    expect(clients[1].text()).toContain('usage.uaUnknownVersion')
    // 原生 details/summary 承担展开状态和键盘语义，不用不可聚焦的行点击替代。
    expect(clients[0].element.tagName).toBe('DETAILS')
    expect(clients[0].find('summary').element.tagName).toBe('SUMMARY')
    const chart = JSON.parse(wrapper.get('[data-testid="chart"]').text())
    expect(chart.datasets[0].data).toEqual([6, 4])
  })

  it('limits only chart sectors, keeping all clients and the correct Other total', () => {
    const wrapper = mount(UserAgentDistributionChart, { props: { stats: Array.from({ length: 10 }, (_, i) => ({
      user_agent: `client${i}/1`, client: `client${i}`, version: '1', requests: i + 1,
    })) } })
    const chart = JSON.parse(wrapper.get('[data-testid="chart"]').text())
    expect(chart.labels).toHaveLength(9)
    expect(chart.labels[8]).toBe('usage.uaOther')
    expect(chart.datasets[0].data[8]).toBe(3)
    expect(chart.datasets[0].data.reduce((a: number, b: number) => a + b, 0)).toBe(55)
    expect(wrapper.findAll('[data-testid="ua-client"]')).toHaveLength(10)
  })

  it('distinguishes loading, failure and empty states and supports retry', async () => {
    const wrapper = mount(UserAgentDistributionChart, { props: { stats: [], loading: true } })
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    await wrapper.setProps({ loading: false, error: true })
    expect(wrapper.find('[role="alert"]').text()).toContain('usage.uaLoadError')
    expect(wrapper.text()).not.toContain('usage.uaNoData')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    await wrapper.setProps({ error: false })
    expect(wrapper.text()).toContain('usage.uaNoData')
    expect(wrapper.find('[data-testid="chart"]').exists()).toBe(false)
  })

  it('renders product names as text even when they match object prototype keys', () => {
    const wrapper = mount(UserAgentDistributionChart, { props: { stats: [
      { client: 'constructor', version: '1', user_agent: 'constructor/1', requests: 1 },
      { client: '__missing__', version: '', user_agent: '   ', requests: 1 },
    ] } })
    const chart = JSON.parse(wrapper.get('[data-testid="chart"]').text())
    expect(chart.labels).toContain('constructor')
    expect(wrapper.findAll('[data-testid="ua-raw"]').some(row => row.text().includes('usage.uaMissing'))).toBe(true)
  })
})
