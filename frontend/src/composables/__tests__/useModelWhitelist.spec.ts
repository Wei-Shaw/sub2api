import { describe, expect, it REDACTED from 'vitest'
import { buildModelMappingObject, getModelsByPlatform REDACTED from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  REDACTED)

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    REDACTED)
  REDACTED)
REDACTED)
