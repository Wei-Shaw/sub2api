import { describe, expect, it } from 'vitest'
import {
  hasAllowedMediaExtension,
  mediaFileAccept,
  mediaKindForWidget,
  normalizeMediaUrlWidget,
  normalizeSingleMediaWidget,
} from '@/utils/mediaUrlWidget'

describe('mediaUrlWidget', () => {
  it('normalizes only supported widget names', () => {
    expect(normalizeMediaUrlWidget('imageUrls')).toBe('ImageUrls')
    expect(normalizeMediaUrlWidget('VideoUrls')).toBe('VideoUrls')
    expect(normalizeMediaUrlWidget('bogus')).toBeNull()
  })

  it('checks extensions case-insensitively and ignores URL query strings', () => {
    expect(hasAllowedMediaExtension('https://cdn.test/a.JPG?token=1', 'image')).toBe(true)
    expect(hasAllowedMediaExtension('https://cdn.test/a.mp4#preview', 'video')).toBe(true)
    expect(hasAllowedMediaExtension('track.FLAC', 'audio')).toBe(true)
    expect(hasAllowedMediaExtension('track.mp3', 'video')).toBe(false)
  })

  it('exposes an extension-based file accept filter', () => {
    expect(mediaFileAccept('video')).toBe('.mp4,.mov,.webm,.m4v')
  })

  it('maps each widget to the material picker kind', () => {
    expect(mediaKindForWidget('ImageUrls')).toBe('image')
    expect(mediaKindForWidget('VideoUrls')).toBe('video')
    expect(mediaKindForWidget('AudioUrls')).toBe('audio')
  })

  it('normalizes single media widget names used by prompt references', () => {
    expect(normalizeSingleMediaWidget('Image')).toBe('image')
    expect(normalizeSingleMediaWidget('ImageUrl')).toBe('image')
    expect(normalizeSingleMediaWidget('Video')).toBe('video')
    expect(normalizeSingleMediaWidget('VideoUrl')).toBe('video')
    expect(normalizeSingleMediaWidget('Audio')).toBe('audio')
    expect(normalizeSingleMediaWidget('AudioUrl')).toBe('audio')
    expect(normalizeSingleMediaWidget('input')).toBeNull()
  })
})
