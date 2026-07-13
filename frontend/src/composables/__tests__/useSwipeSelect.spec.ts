import { describe, it, expect REDACTED from 'vitest'

import { findRowIndexByDomPosition REDACTED from '../useSwipeSelect'

/**
 * Build a fake scroll element whose `tbody tr[data-index]` rows expose stubbed
 * vertical rects, so we can exercise findRowIndexByDomPosition without a real DOM.
 * `index` is the value placed in the row's data-index attribute (its absolute
 * position in the sorted data), which need not equal the array position.
 */
function makeScrollEl(rows: Array<{ index: number; top: number; bottom: number REDACTED>): Element {
  const trs = rows.map((r) => ({
    getAttribute: (name: string) => (name === 'data-index' ? String(r.index) : null),
    getBoundingClientRect: () => ({
      top: r.top,
      bottom: r.bottom,
      left: 0,
      right: 0,
      width: 0,
      height: r.bottom - r.top,
      x: 0,
      y: r.top,
      toJSON: () => ({REDACTED)
    REDACTED)
  REDACTED)) as unknown as HTMLElement[]

  return {
    querySelectorAll: (sel: string) =>
      (sel === 'tbody tr[data-index]' ? trs : []) as unknown as NodeListOf<Element>
  REDACTED as unknown as Element
REDACTED

describe('findRowIndexByDomPosition (swipe-select full-render fallback)', () => {
  // Variable row heights on purpose — the third row is taller.
  const rows = [
    { index: 0, top: 100, bottom: 200 REDACTED,
    { index: 1, top: 200, bottom: 300 REDACTED,
    { index: 2, top: 300, bottom: 450 REDACTED
  ]
  const el = makeScrollEl(rows)

  it('returns -1 when no rows are rendered', () => {
    expect(findRowIndexByDomPosition(makeScrollEl([]), 250)).toBe(-1)
  REDACTED)

  it('locates the row whose rect contains the Y coordinate', () => {
    expect(findRowIndexByDomPosition(el, 150)).toBe(0)
    expect(findRowIndexByDomPosition(el, 250)).toBe(1)
    expect(findRowIndexByDomPosition(el, 400)).toBe(2) // inside the tall row
  REDACTED)

  it('clamps to the first/last row when Y is outside the rendered range', () => {
    expect(findRowIndexByDomPosition(el, 50)).toBe(0) // above the first row
    expect(findRowIndexByDomPosition(el, 999)).toBe(2) // below the last row
  REDACTED)

  it('picks the closer row when Y falls in a gap between rows', () => {
    const gapped = makeScrollEl([
      { index: 0, top: 100, bottom: 180 REDACTED,
      { index: 1, top: 220, bottom: 300 REDACTED
    ])
    expect(findRowIndexByDomPosition(gapped, 190)).toBe(0) // 10px from row0.bottom vs 30px from row1.top
    expect(findRowIndexByDomPosition(gapped, 215)).toBe(1) // 35px from row0.bottom vs 5px from row1.top
  REDACTED)

  it('returns the data-index attribute value, not the array position', () => {
    const remapped = makeScrollEl([
      { index: 5, top: 100, bottom: 200 REDACTED,
      { index: 9, top: 200, bottom: 300 REDACTED
    ])
    expect(findRowIndexByDomPosition(remapped, 150)).toBe(5)
    expect(findRowIndexByDomPosition(remapped, 250)).toBe(9)
  REDACTED)
REDACTED)
