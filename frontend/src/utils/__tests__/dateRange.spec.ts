import { describe, expect, it } from 'vitest'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import { formatLocalMinute, getDatePresetRange, parseDateBoundary, parseLocalMinute } from '../dateRange'

describe('minute ranges', () => {
  it('rounds up without jumping to the earlier DST overlap hour', () => {
    // A subprocess fixes the timezone independently of the test worker's TZ.
    const source = readFileSync('src/utils/dateRange.ts', 'utf8')
    const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
    const script = compiled + '\nconsole.log(JSON.stringify(exports.getDatePresetRange("last24Hours", new Date("2026-11-01T06:30:45Z"))))'
    const range = JSON.parse(execFileSync(process.execPath, ['-e', script], {
      encoding: 'utf8', env: { ...process.env, TZ: 'America/New_York' }
    }))
    expect(range).toEqual({ start: '2026-10-31T06:31:00.000Z', end: '2026-11-01T06:31:00.000Z' })
  })

  it('uses identical exact 24-hour windows within a minute', () => {
    const first = getDatePresetRange('last24Hours', new Date('2026-09-10T05:12:01.123Z'))!
    const second = getDatePresetRange('last24Hours', new Date('2026-09-10T05:12:59.999Z'))!
    expect(first).toEqual(second)
    expect(first.end).toBe('2026-09-10T05:13:00.000Z')
    expect(new Date(first.end).getTime() - new Date(first.start).getTime()).toBe(86400000)
  })
  it('includes the current partial minute, retains exact boundaries, and crosses midnight', () => {
    for (const [now, end] of [
      ['2026-09-10T17:17:50Z', '2026-09-10T17:18:00.000Z'],
      ['2026-09-10T17:17:00Z', '2026-09-10T17:17:00.000Z'],
      ['2026-09-10T17:17:00.001Z', '2026-09-10T17:18:00.000Z'],
      ['2026-09-10T23:59:50Z', '2026-09-11T00:00:00.000Z']
    ]) {
      const range = getDatePresetRange('last24Hours', new Date(now))!
      expect(range.end).toBe(end)
      expect(new Date(range.end).getTime() - new Date(range.start).getTime()).toBe(86400000)
      expect(new Date(range.end).getTime()).toBeGreaterThanOrEqual(new Date(now).getTime())
    }
  })
  it('keeps calendar-day boundaries and advances presets across midnight', () => {
    expect(getDatePresetRange('today', new Date(2026, 8, 10, 23, 59))).toEqual({ start: '2026-09-10', end: '2026-09-10' })
    expect(getDatePresetRange('today', new Date(2026, 8, 11, 0, 1))).toEqual({ start: '2026-09-11', end: '2026-09-11' })
    expect(formatLocalMinute(parseDateBoundary('2026-09-10', true))).toBe('2026-09-11T00:00')
    expect(getDatePresetRange('custom')).toBeNull()
  })
  it('rejects invalid local minutes without silently normalizing', () => {
    expect(parseLocalMinute('2026-02-30T10:00')).toBeNull()
    expect(parseLocalMinute('2026-09-10T10:00:30')).toBeNull()
    expect(parseLocalMinute('')).toBeNull()
    expect(formatLocalMinute(parseLocalMinute('2026-09-10T14:37')!)).toBe('2026-09-10T14:37')
  })
})
