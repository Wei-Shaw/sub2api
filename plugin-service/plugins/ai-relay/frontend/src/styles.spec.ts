import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

function rule(source: string, selector: string) {
  const match = source.match(new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*\\{([^}]*)\\}`))
  return match?.[1] || ''
}

describe('AI Relay page layout styles', () => {
  it('keeps pagination at the bottom of the desktop content area', () => {
    expect(rule(styles, '.relay-shell')).toMatch(/display:\s*flex/)
    expect(rule(styles, '.relay-shell')).toMatch(/flex-direction:\s*column/)
    expect(rule(styles, '.relay-shell')).toMatch(/height:\s*100dvh/)
    expect(rule(styles, '.relay-shell')).toMatch(/overflow:\s*hidden/)
    expect(rule(styles, '.relay-table-wrap')).toMatch(/flex:\s*1/)
    expect(rule(styles, '.relay-table-wrap')).toMatch(/min-height:\s*0/)
    expect(rule(styles, '.relay-table-wrap')).toMatch(/overflow:\s*auto/)
    expect(rule(styles, '.relay-pagination')).toMatch(/flex-shrink:\s*0/)
    expect(rule(styles, '.relay-pagination')).toMatch(/border-top:/)
    expect(rule(styles, '.relay-pagination')).toMatch(/padding:\s*12px 14px 12px/)
    expect(rule(styles, '.relay-shell')).toMatch(/padding:\s*32px 24px 0/)
    expect(rule(styles, '.relay-table thead')).toMatch(/position:\s*sticky/)
    expect(rule(styles, '.relay-table thead')).toMatch(/top:\s*0/)
    expect(rule(styles, '.relay-table thead')).toMatch(/z-index:\s*2/)
    expect(rule(styles, '.relay-table thead')).toMatch(/background:/)
  })

  it('restores natural document flow on mobile', () => {
    const mobile = styles.slice(styles.indexOf('@media (max-width: 680px)'))
    expect(rule(mobile, '.relay-shell')).toMatch(/height:\s*auto/)
    expect(rule(mobile, '.relay-shell')).toMatch(/overflow:\s*visible/)
    expect(rule(mobile, '.relay-table-wrap')).toMatch(/flex:\s*none/)
    expect(rule(mobile, '.relay-table-wrap')).toMatch(/overflow-x:\s*auto/)
  })

  it('uses muted examples for mapping inputs', () => {
    expect(rule(styles, '.mapping-row input::placeholder')).toMatch(/color:\s*#94a3b8/)
  })
})
