import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { AdminUsageLog } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import UsageTable from '../UsageTable.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/utils/ipGeoLookup', () => ({ getEntry: vi.fn(), fetchBatch: vi.fn() }))

const row = {
  id: 1,
  traffic: {
    upstream: {
      request: { original_bytes: 777, actual_bytes: 555, compressed: true },
      response: { original_bytes: 888, actual_bytes: 666, compressed: true },
    },
    downstream: {
      request: { original_bytes: 10, actual_bytes: 10, compressed: false },
      response: { original_bytes: 20, actual_bytes: 20, compressed: false },
    },
  },
} as AdminUsageLog

function textWithoutTooltips(element: Element): string {
  const clone = element.cloneNode(true) as HTMLElement
  clone.querySelectorAll('[role="tooltip"]').forEach(tooltip => tooltip.remove())
  return clone.textContent ?? ''
}

function mountTable(showUpstreamEndpoint?: boolean) {
  return mount(UsageTable, {
    props: {
      data: [row],
      columns: [{ key: 'traffic', label: 'Traffic', sortable: false }],
      ...(showUpstreamEndpoint === undefined ? {} : { showUpstreamEndpoint }),
    },
    global: { stubs: { Teleport: true } },
  })
}

describe('UsageTable direction icons', () => {
  it('shows input/request and output/response arrows without exchanging their values', () => {
    const wrapper = mount(UsageTable, {
      props: {
        data: [{ ...row, input_tokens: 123, output_tokens: 456 }],
        columns: [{ key: 'tokens', label: 'Tokens', sortable: false }],
      },
      global: { stubs: { Teleport: true } },
    })
    const directions = wrapper.get('td').findAllComponents(Icon)
      .filter(component => ['arrowUp', 'arrowDown'].includes(component.props('name')))
    expect(directions).toHaveLength(2)
    expect(directions.map(component => component.props('name'))).toEqual(['arrowUp', 'arrowDown'])
    expect(directions.map(component => component.element.parentElement?.textContent?.trim())).toEqual(['123', '456'])
    wrapper.unmount()
  })
})

describe('UsageTable traffic column', () => {
  it('renders the traffic slot in the actual DataTable with admin upstream and downstream rows', () => {
    const wrapper = mountTable()
    expect(wrapper.get('th').text()).toBe('Traffic')
    // The admin cell aligns upstream and downstream request/response values in one grid.
    expect(wrapper.get('td [data-testid="traffic-cell"]').classes())
      .toContain('grid-cols-[max-content_max-content_max-content_max-content_max-content_max-content_max-content]')
    const upstream = wrapper.get('td [data-testid="traffic-upstream"]')
    expect(upstream.text()).toContain('555 B')
    expect(upstream.findAll('[data-testid="traffic-compression"]').map(cell => cell.text())).toEqual(['28.6%', '25.0%'])
    expect(textWithoutTooltips(upstream.element)).not.toContain('777 B')
    expect(textWithoutTooltips(upstream.element)).not.toContain('888 B')
    expect(upstream.get('[data-testid="traffic-upstream-request-direction"]').attributes('aria-label')).toBe('usage.traffic.request')
    expect(upstream.get('[data-testid="traffic-upstream-response-direction"]').attributes('aria-label')).toBe('usage.traffic.response')
    expect(wrapper.get('td [data-testid="traffic-downstream"]').text()).toContain('10 B')
    expect(wrapper.findAll('[data-testid="traffic-actual"]')).toHaveLength(4)
    wrapper.unmount()
  })

  it('honors the user view upstream boundary even with a fully populated admin payload', () => {
    const wrapper = mountTable(false)
    expect(wrapper.findAll('[data-testid="traffic-actual"]').map(cell => cell.text())).toEqual(['10 B', '20 B'])
    expect(wrapper.find('[data-testid="traffic-upstream"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-client-upload"]').text()).toContain('10 B')
    expect(wrapper.get('[data-testid="traffic-client-download"]').text()).toContain('20 B')
    expect(wrapper.get('td [data-testid="traffic-cell"]').text()).not.toContain('usage.traffic.clientUpload')
    expect(wrapper.get('td [data-testid="traffic-cell"]').text()).not.toContain('usage.traffic.clientDownload')
    expect(wrapper.get('[data-testid="traffic-client-upload-request-direction"]').attributes('aria-label')).toBe('usage.traffic.clientUpload')
    expect(wrapper.get('[data-testid="traffic-client-download-response-direction"]').attributes('aria-label')).toBe('usage.traffic.clientDownload')
    expect(wrapper.html()).not.toMatch(/555 B|666 B|777 B|888 B|usage\.traffic\.upstream/)
    wrapper.unmount()
  })

  it('updates the traffic visibility boundary reactively', async () => {
    const wrapper = mountTable()
    await wrapper.setProps({ showUpstreamEndpoint: false })
    expect(wrapper.find('[data-testid="traffic-upstream"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-client-upload"]').text()).toContain('10 B')
    wrapper.unmount()
  })
})
