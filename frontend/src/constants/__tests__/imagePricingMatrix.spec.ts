import { describe, expect, it } from 'vitest'

import {
  cloneDefaultImagePricingMatrix,
  createEmptyImagePricingMatrix,
  DEFAULT_IMAGE_PRICING_MATRIX,
  IMAGE_PRICING_QUALITY_KEYS,
  IMAGE_PRICING_TIER_KEYS,
  loadEditableImagePricingMatrix,
  toMatrixDTO,
  validateEditableMatrix,
  type EditableImagePricingMatrix,
} from '@/constants/imagePricingMatrix'

describe('imagePricingMatrix constants', () => {
  it('DEFAULT_IMAGE_PRICING_MATRIX 包含全部 6×3 = 18 个格子', () => {
    let count = 0
    for (const tier of IMAGE_PRICING_TIER_KEYS) {
      const row = DEFAULT_IMAGE_PRICING_MATRIX[tier]
      expect(row).toBeTruthy()
      for (const quality of IMAGE_PRICING_QUALITY_KEYS) {
        expect(typeof row[quality]).toBe('number')
        expect(row[quality]).toBeGreaterThanOrEqual(0)
        count += 1
      }
    }
    expect(count).toBe(18)
  })

  it('cloneDefaultImagePricingMatrix 不与常量共享引用', () => {
    const cloned = cloneDefaultImagePricingMatrix()
    cloned['1024x1024'].high = 999
    expect(DEFAULT_IMAGE_PRICING_MATRIX['1024x1024'].high).not.toBe(999)
  })

  it('createEmptyImagePricingMatrix 返回 6 行全 null 结构', () => {
    const empty = createEmptyImagePricingMatrix()
    for (const tier of IMAGE_PRICING_TIER_KEYS) {
      for (const quality of IMAGE_PRICING_QUALITY_KEYS) {
        expect(empty[tier][quality]).toBeNull()
      }
    }
  })
})

describe('loadEditableImagePricingMatrix', () => {
  it('null/undefined/非对象都返回全空骨架', () => {
    expect(loadEditableImagePricingMatrix(null)).toEqual(createEmptyImagePricingMatrix())
    expect(loadEditableImagePricingMatrix(undefined)).toEqual(createEmptyImagePricingMatrix())
  })

  it('合法行被加载，非法值被丢弃为 null', () => {
    const editable = loadEditableImagePricingMatrix({
      '1024x1024': { low: 0.006, medium: -1, high: 0.211 } as Record<string, number>,
      'unknown_tier': { high: 999 } as Record<string, number>,
    })
    expect(editable['1024x1024'].low).toBe(0.006)
    expect(editable['1024x1024'].medium).toBeNull() // 负数被丢弃
    expect(editable['1024x1024'].high).toBe(0.211)
    // 未知 tier 被忽略；其他 tier 仍为 null 骨架
    expect(editable['1920x1080'].high).toBeNull()
  })
})

describe('toMatrixDTO', () => {
  it('全空时返回 null（表示未配置矩阵）', () => {
    expect(toMatrixDTO(createEmptyImagePricingMatrix())).toBeNull()
  })

  it('过滤 null/NaN/负数，只保留 ≥ 0 的合法值', () => {
    const editable: EditableImagePricingMatrix = createEmptyImagePricingMatrix()
    editable['1024x1024'].low = 0.006
    editable['1024x1024'].medium = null
    editable['1024x1024'].high = -0.5 // 应被丢弃
    editable['3840x2160'].high = 0.401
    const dto = toMatrixDTO(editable)
    expect(dto).toEqual({
      '1024x1024': { low: 0.006 },
      '3840x2160': { high: 0.401 },
    })
  })
})

describe('validateEditableMatrix', () => {
  it('全空或全合法返回空数组', () => {
    expect(validateEditableMatrix(createEmptyImagePricingMatrix())).toEqual([])
    const editable = createEmptyImagePricingMatrix()
    editable['1024x1024'].high = 0.211
    expect(validateEditableMatrix(editable)).toEqual([])
  })

  it('负数和 NaN 都被标记', () => {
    const editable = createEmptyImagePricingMatrix()
    editable['1024x1024'].low = -1
    editable['1920x1080'].medium = NaN as unknown as number
    const errors = validateEditableMatrix(editable)
    expect(errors).toContain('1024x1024/low')
    expect(errors).toContain('1920x1080/medium')
    expect(errors.length).toBe(2)
  })
})
