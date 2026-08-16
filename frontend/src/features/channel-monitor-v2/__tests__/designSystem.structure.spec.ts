/**
 * Structure contracts: channel-monitor-v2 + studio shells must use project
 * design-system utility classes rather than isolated flat RGB skins.
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../../..')

function read(rel: string) {
  return readFileSync(resolve(root, rel), 'utf8')
}

describe('channel-monitor-v2 design system structure', () => {
  it('user ChannelStatus V2 shell uses page-header, card, btn, tabs utilities', () => {
    // Route wrapper may switch V1/V2; design chrome lives on the V2 implementation.
    const src = read('views/user/ChannelStatusV2View.vue')
    expect(src).toContain('page-header')
    expect(src).toContain('page-title')
    expect(src).toContain('class="card')
    expect(src).toContain('btn btn-secondary')
    expect(src).toContain('class="tab')
    expect(src).toContain('tab-active')
    expect(src).toContain('badge badge-warning')
    // Compact single-row toolbar
    expect(src).toContain('monitor-toolbar')
    // Ops elevation: rounded-3xl + ring surfaces
    expect(src).toContain('rounded-3xl')
    expect(src).toContain('ring-1 ring-gray-900/5')
    // Overview-first KPI strip before primary viz
    expect(src.indexOf('summaryAria')).toBeLessThan(src.indexOf('RelayPulseMatrix'))
    expect(src).not.toContain('MonitorTrendChart')
    expect(src).not.toContain('FilterMultiSelect')
    expect(src).not.toContain('healthModeOptions')
    expect(src).not.toContain('trendView')
    // No page-level fixed min-width that forces viewport horizontal scroll
    expect(src).not.toMatch(/min-width:\s*980px/)
    expect(src).not.toMatch(/min-w-\[980px\]/)
    // Dense tables scroll internally
    expect(src).toMatch(/max-h-\[min\(52vh/)
    expect(src).toContain('overflow-auto')
    expect(src).toContain("'platform_group'")
  })

  it('RelayPulseMatrix uses card chrome, matrix scroll, and hover tooltips (no click modal)', () => {
    const src = read('features/channel-monitor-v2/RelayPulseMatrix.vue')
    expect(src).toContain('class="card')
    expect(src).toContain('card-header')
    expect(src).toContain('card-body')
    expect(src).toContain('matrix-scroll')
    expect(src).toMatch(/max-h-\[min\(42vh/)
    expect(src).toContain('overflow-auto')
    expect(src).toContain('pulse-tooltip')
    expect(src).toContain('rounded-3xl')
    expect(src).toContain('ring-1 ring-gray-900/5')
    expect(src).not.toContain('modal-overlay')
    expect(src).not.toContain('modal-content')
  })

  it('MetricCell uses stat-card utility', () => {
    const src = read('features/channel-monitor-v2/MetricCell.vue')
    expect(src).toContain('stat-card')
    expect(src).toContain('stat-label')
    expect(src).toContain('stat-value')
    expect(src).toContain('rounded-3xl')
  })

  it('MonitorSettingsPanel uses page-header, card, btn-primary, tabs', () => {
    const src = read('features/channel-monitor-v2/MonitorSettingsPanel.vue')
    expect(src).toContain('page-header')
    expect(src).toContain('btn btn-primary')
    expect(src).toContain('class="card')
    expect(src).toContain('tab-active')
    expect(src).toMatch(/max-h-\[min\(40vh/)
  })

  it('admin ChannelMonitorView V2 tab chrome uses project tabs', () => {
    const src = read('views/admin/ChannelMonitorView.vue')
    expect(src).toContain('page-header')
    expect(src).toContain('page-title')
    expect(src).toContain('class="tabs')
    expect(src).toContain('tab-active')
    expect(src).toContain('MonitorSettingsPanel')
  })
})
