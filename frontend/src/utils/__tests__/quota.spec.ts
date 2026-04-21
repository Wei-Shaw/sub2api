import { describe, expect, it } from 'vitest'
import { findOverlappingGroupID, normalizeGroupIDs } from '../quota'

describe('normalizeGroupIDs', () => {
  it('returns ascending unique positive IDs', () => {
    expect(normalizeGroupIDs([3, 1, 2])).toEqual([1, 2, 3])
  })

  it('drops duplicates preserving sorted order', () => {
    expect(normalizeGroupIDs([2, 1, 2, 3, 1])).toEqual([1, 2, 3])
  })

  it('filters out zero and negative IDs', () => {
    expect(normalizeGroupIDs([0, -1, 5, 3])).toEqual([3, 5])
  })

  it('handles empty input', () => {
    expect(normalizeGroupIDs([])).toEqual([])
  })
})

describe('findOverlappingGroupID', () => {
  it('returns null when no overlap', () => {
    expect(
      findOverlappingGroupID([
        { group_ids: [1, 2] },
        { group_ids: [3, 4] }
      ])
    ).toBeNull()
  })

  it('returns the first overlapping group id', () => {
    expect(
      findOverlappingGroupID([
        { group_ids: [1, 2] },
        { group_ids: [3, 2] }
      ])
    ).toBe(2)
  })

  it('detects duplicate within a single rule', () => {
    expect(findOverlappingGroupID([{ group_ids: [1, 1] }])).toBe(1)
  })

  it('handles empty rules list', () => {
    expect(findOverlappingGroupID([])).toBeNull()
  })
})
