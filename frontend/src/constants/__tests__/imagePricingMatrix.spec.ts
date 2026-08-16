import { describe, expect, it } from 'vitest'

import {
  cloneDefaultImagePricingMatrix,
  createEmptyImagePricingMatrix,
  DEFAULT_IMAGE_PRICING_MATRIX,
  IMAGE_PRICING_QUALITY_KEYS,
  IMAGE_PRICING_TIER_KEYS,
  loadEditableImagePricingMatrix,
  toMatrixDTO,
  toMatrixUpdateDTO,
  validateEditableMatrix,
  type EditableImagePricingMatrix,
} from '@/constants/imagePricingMatrix'

describe('imagePricingMatrix constants', () => {
  it('DEFAULT_IMAGE_PRICING_MATRIX 包含全部 3×3 = 9 个格子', () => {
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
    expect(count).toBe(9)
  })

  it('cloneDefaultImagePricingMatrix 不与常量共享引用', () => {
    const cloned = cloneDefaultImagePricingMatrix()
    cloned['1K'].high = 999
    expect(DEFAULT_IMAGE_PRICING_MATRIX['1K'].high).not.toBe(999)
  })

  it('createEmptyImagePricingMatrix 返回 3 行全 null 结构', () => {
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
      '1K': { low: 0.006, medium: -1, high: 0.211 } as Record<string, number>,
      'unknown_tier': { high: 999 } as Record<string, number>,
    })
    expect(editable['1K'].low).toBe(0.006)
    expect(editable['1K'].medium).toBeNull() // 负数被丢弃
    expect(editable['1K'].high).toBe(0.211)
    // 未知 tier 被忽略；其他 tier 仍为 null 骨架
    expect(editable['2K'].high).toBeNull()
  })

  it('兼容将旧六档矩阵映射为 1K/2K/4K', () => {
    const editable = loadEditableImagePricingMatrix({
      '1024x1024': { high: 0.211 },
      '2560x1440': { high: 0.222 },
      '3840x2160': { high: 0.401 },
    })
    expect(editable['1K'].high).toBe(0.211)
    expect(editable['2K'].high).toBe(0.222)
    expect(editable['4K'].high).toBe(0.401)
  })
})

describe('toMatrixDTO', () => {
  it('全空时返回 null（表示未配置矩阵）', () => {
    expect(toMatrixDTO(createEmptyImagePricingMatrix())).toBeNull()
  })

  it('过滤 null/NaN/负数，只保留 ≥ 0 的合法值', () => {
    const editable: EditableImagePricingMatrix = createEmptyImagePricingMatrix()
    editable['1K'].low = 0.006
    editable['1K'].medium = null
    editable['1K'].high = -0.5 // 应被丢弃
    editable['4K'].high = 0.401
    const dto = toMatrixDTO(editable)
    expect(dto).toEqual({
      '1K': { low: 0.006 },
      '4K': { high: 0.401 },
    })
  })
})

describe('toMatrixUpdateDTO', () => {
  it('全空时返回空对象，用于更新接口显式清空矩阵', () => {
    expect(toMatrixUpdateDTO(createEmptyImagePricingMatrix())).toEqual({})
  })

  it('有值时与 toMatrixDTO 保持一致', () => {
    const editable: EditableImagePricingMatrix = createEmptyImagePricingMatrix()
    editable['1K'].high = 0.211
    expect(toMatrixUpdateDTO(editable)).toEqual({
      '1K': { high: 0.211 },
    })
  })
})

describe('validateEditableMatrix', () => {
  it('全空或全合法返回空数组', () => {
    expect(validateEditableMatrix(createEmptyImagePricingMatrix())).toEqual([])
    const editable = createEmptyImagePricingMatrix()
    editable['1K'].high = 0.211
    expect(validateEditableMatrix(editable)).toEqual([])
  })

  it('负数和 NaN 都被标记', () => {
    const editable = createEmptyImagePricingMatrix()
    editable['1K'].low = -1
    editable['2K'].medium = NaN as unknown as number
    const errors = validateEditableMatrix(editable)
    expect(errors).toContain('1K/low')
    expect(errors).toContain('2K/medium')
    expect(errors.length).toBe(2)
  })
})
