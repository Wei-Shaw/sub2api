import { describe, expect, it REDACTED from 'vitest'
import {
  normalizeGenerationListResponse,
  normalizeModelFamiliesResponse
REDACTED from '../sora'

describe('sora api normalizers', () => {
  it('normalizes generation list from data shape', () => {
    const result = normalizeGenerationListResponse({
      data: [{ id: 1, status: 'pending' REDACTED],
      total: 9,
      page: 2
    REDACTED)

    expect(result.data).toHaveLength(1)
    expect(result.total).toBe(9)
    expect(result.page).toBe(2)
  REDACTED)

  it('normalizes generation list from items shape', () => {
    const result = normalizeGenerationListResponse({
      items: [{ id: 1, status: 'completed' REDACTED],
      total: 1
    REDACTED)

    expect(result.data).toHaveLength(1)
    expect(result.total).toBe(1)
    expect(result.page).toBe(1)
  REDACTED)

  it('falls back to empty generation list on invalid payload', () => {
    const result = normalizeGenerationListResponse(null)
    expect(result).toEqual({ data: [], total: 0, page: 1 REDACTED)
  REDACTED)

  it('normalizes family model payload', () => {
    const result = normalizeModelFamiliesResponse({
      data: [
        {
          id: 'sora2',
          name: 'Sora 2',
          type: 'video',
          orientations: ['landscape', 'portrait'],
          durations: [10, 15]
        REDACTED
      ]
    REDACTED)

    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('sora2')
    expect(result[0].orientations).toEqual(['landscape', 'portrait'])
    expect(result[0].durations).toEqual([10, 15])
  REDACTED)

  it('normalizes legacy flat model list into families', () => {
    const result = normalizeModelFamiliesResponse({
      items: [
        { id: 'sora2-landscape-10s', type: 'video' REDACTED,
        { id: 'sora2-portrait-15s', type: 'video' REDACTED,
        { id: 'gpt-image-square', type: 'image' REDACTED
      ]
    REDACTED)

    const sora2 = result.find((m) => m.id === 'sora2')
    expect(sora2).toBeTruthy()
    expect(sora2?.orientations).toEqual(['landscape', 'portrait'])
    expect(sora2?.durations).toEqual([10, 15])

    const image = result.find((m) => m.id === 'gpt-image')
    expect(image).toBeTruthy()
    expect(image?.type).toBe('image')
    expect(image?.orientations).toEqual(['square'])
  REDACTED)

  it('falls back to empty families on invalid payload', () => {
    expect(normalizeModelFamiliesResponse(undefined)).toEqual([])
    expect(normalizeModelFamiliesResponse({REDACTED)).toEqual([])
  REDACTED)
REDACTED)

