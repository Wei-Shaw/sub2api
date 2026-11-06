import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import OpsTrafficCards from '../OpsTrafficCards.vue'
import type { OpsTrafficDirection, OpsTrafficLink, OpsTrafficSummary } from '@/api/admin/ops'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key })
}))
enableAutoUnmount(afterEach)
afterEach(() => {
  document.body.innerHTML = ''
})

function bodyTestId(testId: string): HTMLElement {
  const element = document.body.querySelector<HTMLElement>(`[data-testid="${testId}"]`)
  if (!element) throw new Error(`Missing teleported element: ${testId}`)
  return element
}

function tooltipFor(testId: string): HTMLElement {
  const tooltip = bodyTestId(testId).closest<HTMLElement>('[role="tooltip"]')
  if (!tooltip) throw new Error(`Missing tooltip for: ${testId}`)
  return tooltip
}

function direction(overrides: Partial<OpsTrafficDirection> = {}): OpsTrafficDirection {
  return {
    original_bytes: 1024,
    actual_bytes: 512,
    original_sample_count: 2,
    actual_sample_count: 2,
    compression_percent: 50,
    ...overrides
  }
}

function link(overrides: Partial<OpsTrafficLink> = {}): OpsTrafficLink {
  return {
    original_bytes: 4096, actual_bytes: 2048,
    original_sample_count: 4, actual_sample_count: 4, paired_sample_count: 4,
    compression_percent: 50,
    request: direction(),
    response: direction({ original_bytes: 3072, actual_bytes: 1536 }),
    ...overrides
  }
}

function traffic(overrides: Partial<OpsTrafficSummary> = {}): OpsTrafficSummary {
  return { source: 'http_requests', request_count: 2, upstream: link(), downstream: link(), ...overrides }
}

const unknownLink = (): OpsTrafficLink => link({
  original_bytes: null,
  actual_bytes: null,
  original_sample_count: 0,
  actual_sample_count: 0,
  paired_sample_count: 0,
  compression_percent: null,
  request: null,
  response: null
})

const startTime = '2026-06-01T00:00:00Z'
const endTime = '2026-06-01T00:01:00Z'

function mountCards(value?: OpsTrafficSummary | null, loading = false) {
  return mount(OpsTrafficCards, { attachTo: document.body, props: { traffic: value, startTime, endTime, loading } })
}

describe('OpsTrafficCards', () => {
  it('shows the request body above the response body with each directional actual total and compression rate', () => {
    const wrapper = mountCards(traffic({
      downstream: link({
        request: direction({ actual_bytes: 2560, original_bytes: 2048, compression_percent: -25 }),
        response: direction({ actual_bytes: 1024, original_bytes: 4096, compression_percent: 75 })
      })
    }))
    const articles = wrapper.findAll('article')
    expect(articles).toHaveLength(2)
    for (const article of articles) {
      expect(article.classes()).toEqual(expect.arrayContaining(['min-w-0', 'rounded-2xl', 'bg-gray-50', 'p-4', 'dark:bg-dark-900']))
    }
    expect(wrapper.get('[data-testid="traffic-upstream-card"]').attributes('style')).toContain('order: 8')
    expect(wrapper.get('[data-testid="traffic-downstream-card"]').attributes('style')).toContain('order: 9')

    // Actual (encoded) body size is the headline value for each direction.
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('512 B')
    expect(wrapper.get('[data-testid="traffic-upstream-response"]').text()).toBe('1.5 KiB')
    expect(wrapper.get('[data-testid="traffic-downstream-request"]').text()).toBe('2.5 KiB')
    expect(wrapper.get('[data-testid="traffic-downstream-response"]').text()).toBe('1 KiB')

    // The rate appears only in the grey label; it is the directional ratio that
    // belongs to the headline value.
    expect(wrapper.get('[data-testid="traffic-upstream-request-compression"]').text()).toBe('50.0%')
    expect(wrapper.get('[data-testid="traffic-upstream-response-compression"]').text()).toBe('50.0%')
    expect(wrapper.get('[data-testid="traffic-downstream-request-compression"]').text()).toBe('-25.0%')
    expect(wrapper.get('[data-testid="traffic-downstream-response-compression"]').text()).toBe('75.0%')

    // The label keeps the direction name and appends the equivalent rate.
    const label = wrapper.get('[data-testid="traffic-upstream-request-label"]')
    expect(label.element.tagName).toBe('DT')
    expect(label.classes()).toContain('text-xs')
    expect(label.text()).toBe('admin.ops.traffic.request, 68.27 bps')
    expect(label.text()).not.toContain('admin.ops.traffic.bandwidth')
    expect(wrapper.get('[data-testid="traffic-upstream-response-label"]').text()).toBe('admin.ops.traffic.response, 204.8 bps')

    // Metric-per-row rendering is gone: no original/actual rows remain.
    for (const metric of ['original', 'actual']) {
      expect(wrapper.find(`[data-testid="traffic-upstream-${metric}"]`).exists()).toBe(false)
      expect(wrapper.find(`[data-testid="traffic-upstream-${metric}-label"]`).exists()).toBe(false)
    }

    for (const testId of ['traffic-upstream-request', 'traffic-upstream-response']) {
      const target = wrapper.get(`[data-testid="${testId}"]`)
      expect(target.find('svg').exists()).toBe(false)
      expect(target.classes()).toContain('cursor-help')
    }
    expect(wrapper.get('[data-testid="traffic-upstream-coverage-tooltip"] svg').exists()).toBe(true)
    expect(bodyTestId('traffic-upstream-scope').textContent).toBe('admin.ops.traffic.scope')
    expect(bodyTestId('traffic-upstream-compression-hint').textContent).toBe('admin.ops.traffic.compressionHint')
  })

  it('derives the rate after the comma from the actual bytes of that direction', async () => {
    const wrapper = mountCards(traffic({
      upstream: link({ request: direction({ original_bytes: 7_500_000, actual_bytes: 3_750_000 }) })
    }))
    // 3,750,000 B * 8 / 60 s = 500 kbps. Using the original bytes would show 1 Mbps.
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('500 kbps')
    expect(wrapper.get('[data-testid="traffic-upstream-request-label"]').text()).toBe('admin.ops.traffic.request, 500 kbps')
    await wrapper.setProps({ startTime: '2026-06-01T08:00:00+08:00', endTime: '2026-06-01T08:02:30+08:00' })
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('200 kbps')
    await wrapper.setProps({ traffic: traffic({ upstream: link({ request: direction({ actual_bytes: null }) }) }) })
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('—')
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('—')
  })

  it.each([
    [undefined, endTime],
    [startTime, undefined],
    [null, endTime],
    [startTime, null],
    ['', endTime],
    ['invalid', endTime],
    [startTime, 'invalid'],
    [startTime, startTime],
    [endTime, startTime]
  ])('keeps the equivalent rate unknown for a missing or invalid statistics window: %s / %s', async (from, to) => {
    const wrapper = mountCards(traffic())
    await wrapper.setProps({ startTime: from, endTime: to })
    for (const side of ['upstream', 'downstream']) {
      for (const row of ['request', 'response']) {
        expect(wrapper.get(`[data-testid="traffic-${side}-${row}-bandwidth"]`).text()).toBe('—')
      }
    }
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('512 B')
  })

  it('explains the equivalent rate on hover and keyboard focus for both rows of each card', async () => {
    const wrapper = mountCards(traffic())
    const tooltipIds = new Set<string>()
    for (const side of ['upstream', 'downstream']) {
      for (const row of ['request', 'response']) {
        const trigger = wrapper.get(`[data-testid="traffic-${side}-${row}-bandwidth-tooltip"]`)
        const element = trigger.element as HTMLElement
        const tooltip = tooltipFor(`traffic-${side}-${row}-bandwidth-hint`)
        expect(trigger.attributes('tabindex')).toBe('0')
        expect(trigger.attributes('aria-label')).toContain('admin.ops.traffic.bandwidth')
        expect(tooltip.style.display).toBe('none')
        expect(trigger.text()).not.toContain('admin.ops.traffic.bandwidthHint')

        await trigger.trigger('mouseenter')
        expect(tooltip.style.display).not.toBe('none')
        expect(tooltip.textContent).toBe('admin.ops.traffic.bandwidthHint')
        await trigger.trigger('mouseleave')
        expect(tooltip.style.display).toBe('none')

        element.focus()
        await nextTick()
        expect(tooltip.style.display).not.toBe('none')
        expect(trigger.attributes('aria-describedby')).toBe(tooltip.id)
        tooltipIds.add(tooltip.id)
        element.blur()
        await nextTick()
        expect(tooltip.style.display).toBe('none')
      }
    }
    expect(tooltipIds.size).toBe(4)
  })

  it('reveals the original traffic of that direction on hover instead of a request/response split', async () => {
    const wrapper = mountCards(traffic())

    const originalTooltip = tooltipFor('traffic-upstream-request-original')
    expect(originalTooltip.style.display).toBe('none')
    const trigger = wrapper.get('[data-testid="traffic-upstream-request"]')
    expect(trigger.attributes('aria-label')).toBeUndefined()
    const tooltipTrigger = wrapper.get('[data-testid="traffic-upstream-request-original-tooltip"]')
    expect(tooltipTrigger.attributes('aria-label')).toBe('admin.ops.traffic.originalHelp')

    trigger.element.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }))
    await nextTick()
    expect(originalTooltip.style.display).not.toBe('none')
    // Original bytes of the request body plus their equivalent rate.
    expect(bodyTestId('traffic-upstream-request-original').textContent).toBe('1 KiB, 136.53 bps')
    expect(bodyTestId('traffic-upstream-request-original-coverage').textContent).toBe('admin.ops.traffic.directionCoverage:{"count":2,"total":2}')
    expect(originalTooltip.textContent).not.toContain('admin.ops.traffic.actual')

    trigger.element.dispatchEvent(new MouseEvent('mouseleave', { bubbles: true }))
    await nextTick()
    expect(originalTooltip.style.display).toBe('none')

    expect(bodyTestId('traffic-upstream-response-original').textContent).toBe('3 KiB, 409.6 bps')
    expect(bodyTestId('traffic-downstream-request-original').textContent).toBe('1 KiB, 136.53 bps')
  })

  it('recalculates the original traffic rate from the returned statistics window', async () => {
    const wrapper = mountCards(traffic())
    expect(bodyTestId('traffic-upstream-request-original').textContent).toBe('1 KiB, 136.53 bps')
    await wrapper.setProps({ startTime: '2026-06-01T00:00:00Z', endTime: '2026-06-01T00:02:00Z' })
    expect(bodyTestId('traffic-upstream-request-original').textContent).toBe('1 KiB, 68.27 bps')
    expect(bodyTestId('traffic-upstream-response-original').textContent).toBe('3 KiB, 204.8 bps')
  })

  it('keeps compression rates unknown when the server did not pair the samples, never deriving them from totals', () => {
    const wrapper = mountCards(traffic({
      upstream: link({
        request: direction({ original_bytes: 1024, actual_bytes: 512, compression_percent: null }),
        response: direction({ original_bytes: 1024, actual_bytes: 512, compression_percent: Number.POSITIVE_INFINITY })
      })
    }))
    expect(wrapper.get('[data-testid="traffic-upstream-request-compression"]').text()).toBe('—')
    expect(wrapper.get('[data-testid="traffic-upstream-response-compression"]').text()).toBe('—')
    // The byte totals themselves stay visible.
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('512 B')
    expect(wrapper.get('[data-testid="traffic-upstream-response"]').text()).toBe('512 B')
  })

  it('renders measured zero compression instead of treating it as unknown', () => {
    const wrapper = mountCards(traffic({
      upstream: link({
        request: direction({ original_bytes: 0, actual_bytes: 0, compression_percent: 0 }),
        response: direction({ original_bytes: 100, actual_bytes: 125, compression_percent: -25 })
      })
    }))
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('0 B')
    expect(wrapper.get('[data-testid="traffic-upstream-request-compression"]').text()).toBe('0.0%')
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('0 bps')
    expect(wrapper.get('[data-testid="traffic-upstream-response-compression"]').text()).toBe('-25.0%')
  })

  it('falls back to the side totals with the side-level ratio when no directional split exists', () => {
    const wrapper = mountCards(traffic({
      upstream: link({ request: null, response: null, original_bytes: 4096, actual_bytes: 2048, compression_percent: 25 })
    }))
    expect(wrapper.find('[data-testid="traffic-upstream-request"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="traffic-upstream-response"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-upstream-combined"]').text()).toBe('2 KiB')
    // 2 KiB * 8 / 60 s = 273.07 kbps
    expect(wrapper.get('[data-testid="traffic-upstream-combined-label"]').text()).toBe('admin.ops.traffic.actual, 273.07 bps')
    expect(wrapper.get('[data-testid="traffic-upstream-combined-compression"]').text()).toBe('25.0%')
    expect(wrapper.get('[data-testid="traffic-upstream-combined-combined-badge"]').text()).toBe('admin.ops.traffic.noDirectionalSplit')
    // The combined row covers both bodies of every request.
    expect(bodyTestId('traffic-upstream-combined-original').textContent).toBe('4 KiB, 546.13 bps')
    expect(bodyTestId('traffic-upstream-combined-original-coverage').textContent).toBe('admin.ops.traffic.coverage:{"count":4,"total":4}')
    // A downstream card that still has both directions keeps the directional view.
    expect(wrapper.find('[data-testid="traffic-downstream-combined"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-downstream-request"]').text()).toBe('512 B')
  })

  it.each([
    ['a missing overview', undefined, 'admin.ops.traffic.coverageUnknown'],
    ['a null overview', null, 'admin.ops.traffic.coverageUnknown'],
    ['unknown measurements', traffic({ upstream: unknownLink(), downstream: unknownLink() }), 'admin.ops.traffic.coverage:{"count":0,"total":4}']
  ])('shows %s as unknown instead of zero or an invented split', (_name, value, coverage) => {
    const wrapper = mountCards(value)
    for (const side of ['upstream', 'downstream']) {
      expect(wrapper.get(`[data-testid="traffic-${side}-combined"]`).text()).toBe('—')
      expect(wrapper.get(`[data-testid="traffic-${side}-combined-compression"]`).text()).toBe('—')
      expect(wrapper.get(`[data-testid="traffic-${side}-combined-bandwidth"]`).text()).toBe('—')
      expect(wrapper.get(`[data-testid="traffic-${side}-coverage-state"]`).text()).toBe('admin.ops.traffic.unavailable')
      expect(bodyTestId(`traffic-${side}-combined-original`).textContent).toBe('—')
      expect(bodyTestId(`traffic-${side}-combined-original-coverage`).textContent).toContain(coverage)
      expect(wrapper.find(`[data-testid="traffic-${side}-request"]`).exists()).toBe(false)
    }
    expect(wrapper.text()).not.toContain('0 B')
  })

  it('keeps long scope, formula and coverage details hidden until their help icon is hovered', async () => {
    const wrapper = mountCards(traffic())
    const upstream = wrapper.get('[data-testid="traffic-upstream-card"]')
    expect(upstream.text()).not.toContain('admin.ops.traffic.scope')
    expect(upstream.text()).not.toContain('admin.ops.traffic.compressionHint')
    expect(upstream.text()).not.toContain('admin.ops.traffic.pairedCoverage')

    const detailsTooltip = tooltipFor('traffic-upstream-scope')
    expect(detailsTooltip.style.display).toBe('none')
    const detailsTrigger = wrapper.get('[data-testid="traffic-upstream-coverage-tooltip"]')
    await detailsTrigger.trigger('mouseenter')
    await nextTick()
    expect(detailsTooltip.style.display).not.toBe('none')
    expect(detailsTooltip.textContent).toContain('admin.ops.traffic.scope')
    expect(detailsTooltip.textContent).toContain('admin.ops.traffic.compressionHint')
    expect(detailsTooltip.textContent).toContain('admin.ops.traffic.pairedCoverage')
    await detailsTrigger.trigger('mouseleave')
    await nextTick()
    expect(detailsTooltip.style.display).toBe('none')
  })

  it('shows per-metric and paired coverage for the card and partial coverage when bodies are missing', () => {
    const wrapper = mountCards(traffic({
      request_count: 5,
      upstream: link({
        original_bytes: 100, actual_bytes: 500, original_sample_count: 4, actual_sample_count: 7,
        paired_sample_count: 2, compression_percent: 25, request: null, response: null
      })
    }))
    expect(bodyTestId('traffic-upstream-original-coverage').textContent).toContain('admin.ops.traffic.original')
    expect(bodyTestId('traffic-upstream-original-coverage').textContent).toContain('admin.ops.traffic.coverage:{"count":4,"total":10}')
    expect(bodyTestId('traffic-upstream-actual-coverage').textContent).toContain('admin.ops.traffic.actual')
    expect(bodyTestId('traffic-upstream-actual-coverage').textContent).toContain('admin.ops.traffic.coverage:{"count":7,"total":10}')
    expect(bodyTestId('traffic-upstream-paired-coverage').textContent).toContain('admin.ops.traffic.compression')
    expect(bodyTestId('traffic-upstream-paired-coverage').textContent).toContain('admin.ops.traffic.pairedCoverage:{"count":2,"total":10}')
    expect(wrapper.get('[data-testid="traffic-upstream-coverage-state"]').text()).toBe('admin.ops.traffic.partial')
    expect(wrapper.get('[data-testid="traffic-upstream-combined-compression"]').text()).toBe('25.0%')
  })

  it('uses binary units for large directional totals without overflowing card values', () => {
    const wrapper = mountCards(traffic({
      upstream: link({
        request: direction({ actual_bytes: 1024 ** 5 }),
        response: direction({ actual_bytes: 1024 ** 4 })
      }),
      downstream: link({
        request: direction({ actual_bytes: 1024 ** 3 }),
        response: direction({ actual_bytes: 1024 ** 2 })
      })
    }))
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('1 PiB')
    expect(wrapper.get('[data-testid="traffic-upstream-response"]').text()).toBe('1 TiB')
    expect(wrapper.get('[data-testid="traffic-downstream-request"]').text()).toBe('1 GiB')
    expect(wrapper.get('[data-testid="traffic-downstream-response"]').text()).toBe('1 MiB')
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').classes()).toContain('[overflow-wrap:anywhere]')
  })

  it('marks loading and hides stale totals, rates and ratios until the filtered overview arrives', async () => {
    const wrapper = mountCards(traffic(), true)
    expect(wrapper.get('[data-testid="traffic-upstream-card"]').attributes('aria-busy')).toBe('true')
    expect(wrapper.get('[data-testid="traffic-downstream-card"]').attributes('aria-busy')).toBe('true')
    expect(wrapper.get('[role="status"]').text()).toBe('admin.ops.loadingText')
    for (const side of ['upstream', 'downstream']) {
      for (const row of ['request', 'response']) {
        expect(wrapper.get(`[data-testid="traffic-${side}-${row}-label"]`).text()).toBe(`admin.ops.traffic.${row}`)
        expect(wrapper.find(`[data-testid="traffic-${side}-${row}-bandwidth"]`).exists()).toBe(false)
        expect(wrapper.find(`[data-testid="traffic-${side}-${row}"]`).exists()).toBe(false)
        expect(wrapper.find(`[data-testid="traffic-${side}-${row}-compression"]`).exists()).toBe(false)
      }
      expect(wrapper.find(`[data-testid="traffic-${side}-coverage-state"]`).exists()).toBe(false)
      expect(document.body.querySelector(`[data-testid="traffic-${side}-request-bandwidth-hint"]`)).toBeNull()
      expect(document.body.querySelector(`[data-testid="traffic-${side}-original-coverage"]`)).toBeNull()
    }
    expect(wrapper.findAll('.animate-pulse')).toHaveLength(4)
    await wrapper.setProps({ loading: false, traffic: traffic() })
    expect(wrapper.get('[data-testid="traffic-upstream-card"]').attributes('aria-busy')).toBe('false')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('512 B')
  })

  it('handles invalid byte values, nonfinite ratios and unavailable coverage without fabricated zeroes', () => {
    const wrapper = mountCards(traffic({
      request_count: Number.NaN,
      upstream: link({
        request: direction({ original_bytes: Number.NaN, actual_bytes: -1, compression_percent: Number.NaN }),
        response: direction({ original_bytes: Number.POSITIVE_INFINITY, actual_bytes: Number.POSITIVE_INFINITY })
      })
    }))
    for (const row of ['request', 'response']) {
      expect(wrapper.get(`[data-testid="traffic-upstream-${row}"]`).text()).toBe('—')
      expect(wrapper.get(`[data-testid="traffic-upstream-${row}-bandwidth"]`).text()).toBe('—')
    }
    expect(wrapper.get('[data-testid="traffic-upstream-request-compression"]').text()).toBe('—')
    expect(bodyTestId('traffic-upstream-original-coverage').textContent).toContain('admin.ops.traffic.coverageUnknown')
  })
})

describe('OpsTrafficCards ingestion health', () => {
  it.each([undefined, null, { scope: 'instance_lifetime' as const, pending: 0, failed: 0, flushed: 100 }])(
    'does not claim healthy/complete ingestion when there is no known problem: %s', (ingestion) => {
      const wrapper = mountCards(traffic({ ingestion }))
      expect(wrapper.find('[data-testid="traffic-ingestion-warning"]').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('healthy')
      expect(wrapper.text()).not.toContain('complete')
    }
  )

  it('prominently warns about sticky record loss despite full persisted measurement coverage', async () => {
    const wrapper = mountCards(traffic({
      ingestion: { scope: 'instance_lifetime', pending: 0, failed: 3, flushed: 100 }
    }))
    const warning = wrapper.get('[data-testid="traffic-ingestion-warning"]')
    expect(warning.attributes('role')).toBe('alert')
    expect(warning.classes()).toContain('bg-red-50')
    expect(warning.get('[data-testid="traffic-ingestion-instance"]').text()).toBe('admin.ops.traffic.instance')
    expect(warning.find('svg').exists()).toBe(true)
    expect(warning.get('[data-testid="traffic-ingestion-failed-short"]').text()).toBe('admin.ops.traffic.ingestionFailedShort:{"count":3}')
    expect(wrapper.get('[data-testid="traffic-upstream-card"]').find('[data-testid="traffic-ingestion-warning"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-downstream-card"]').find('[data-testid="traffic-ingestion-warning"]').exists()).toBe(true)
    expect(warning.text()).not.toContain('admin.ops.traffic.ingestionScope')
    const tooltip = tooltipFor('traffic-ingestion-failed')
    expect(tooltip.style.display).toBe('none')
    await warning.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(bodyTestId('traffic-ingestion-failed').textContent).toBe('admin.ops.traffic.ingestionFailed:{"count":3}')
    expect(bodyTestId('traffic-ingestion-scope').textContent).toBe('admin.ops.traffic.ingestionScope')
    expect(bodyTestId('traffic-ingestion-coverage').textContent).toBe('admin.ops.traffic.ingestionCoverage')
    expect(document.body.querySelector('[data-testid="traffic-ingestion-pending"]')).toBeNull()
    // Ingestion lifetime counts must not be added to the filtered denominator.
    expect(bodyTestId('traffic-upstream-original-coverage').textContent).toContain('admin.ops.traffic.coverage:{"count":4,"total":4}')
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('512 B')
  })

  it('shows pending persistence without claiming record loss and clears it on the next overview', async () => {
    const wrapper = mountCards(traffic({
      ingestion: { scope: 'instance_lifetime', pending: 2, failed: 0, flushed: 100 }
    }))
    const warning = wrapper.get('[data-testid="traffic-ingestion-warning"]')
    expect(warning.attributes('role')).toBe('status')
    expect(warning.classes()).toContain('bg-amber-50')
    expect(warning.get('[data-testid="traffic-ingestion-pending-short"]').text()).toBe('admin.ops.traffic.ingestionPendingShort:{"count":2}')
    expect(bodyTestId('traffic-ingestion-pending').textContent).toBe('admin.ops.traffic.ingestionPending:{"count":2}')
    expect(document.body.querySelector('[data-testid="traffic-ingestion-failed"]')).toBeNull()
    await wrapper.setProps({ traffic: traffic({ ingestion: { scope: 'instance_lifetime', pending: 0, failed: 0, flushed: 102 } }) })
    expect(wrapper.find('[data-testid="traffic-ingestion-warning"]').exists()).toBe(false)
  })

  it('keeps the failure warning after pending records flush, including for an empty selected range', async () => {
    const wrapper = mountCards(traffic({
      request_count: 0, upstream: unknownLink(), downstream: unknownLink(),
      ingestion: { scope: 'instance_lifetime', pending: 2, failed: 3, flushed: 100 }
    }))
    expect(wrapper.get('[data-testid="traffic-ingestion-warning"]').attributes('role')).toBe('alert')
    expect(bodyTestId('traffic-ingestion-failed').textContent).toContain('"count":3')
    expect(bodyTestId('traffic-ingestion-pending').textContent).toContain('"count":2')
    await wrapper.setProps({ traffic: traffic({
      request_count: 0, upstream: unknownLink(), downstream: unknownLink(),
      ingestion: { scope: 'instance_lifetime', pending: 0, failed: 3, flushed: 102 }
    }) })
    expect(wrapper.get('[data-testid="traffic-ingestion-warning"]').attributes('role')).toBe('alert')
    expect(document.body.querySelector('[data-testid="traffic-ingestion-pending"]')).toBeNull()
    expect(wrapper.get('[data-testid="traffic-upstream-combined"]').text()).toBe('—')
  })

  it('hides stale ingestion status while loading and uses the new overview health afterwards', async () => {
    const wrapper = mountCards(traffic({
      ingestion: { scope: 'instance_lifetime', pending: 2, failed: 3, flushed: 100 }
    }), true)
    expect(wrapper.find('[data-testid="traffic-ingestion-warning"]').exists()).toBe(false)
    await wrapper.setProps({ loading: false })
    expect(wrapper.get('[data-testid="traffic-ingestion-warning"]').attributes('role')).toBe('alert')
    await wrapper.setProps({ traffic: traffic({ ingestion: null }) })
    expect(wrapper.find('[data-testid="traffic-ingestion-warning"]').exists()).toBe(false)
  })
})
