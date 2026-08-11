/**
 * imageUrls 控件 + array maxItems 的读写往返测试。
 *
 * 覆盖两条链路：
 *   1) 管理端编辑器：SchemaRow ⇄ 存储 shape（rowsToMap / mapToRows）
 *   2) 演练台读侧：存储 shape → FieldSpec（extractFieldSpecs）
 *
 * 重点验证"默认值不落库"这条约定：widget='input' / maxItems=0 不应写进存储，
 * 否则历史数据在下次保存时会凭空多出两个键，diff 噪声很大。
 */
import { describe, expect, it } from 'vitest'
import {
  makeSchemaRow,
  mapToRows,
  rowsToMap,
} from '@/components/common/paramSchemaRow'
import { extractFieldSpecs } from '@/components/video/paramSpec'

/** 造一个 imageUrls 图片组字段行（元素为图片 URL 字符串）。 */
function makeImageUrlsRow(maxItems: number) {
  return makeSchemaRow({
    key: 'image_urls',
    type: 'array',
    widget: 'imageUrls',
    maxItems,
    required: true,
    description: '参考图片',
    descriptionEn: 'Reference images',
    items: makeSchemaRow({ key: '', type: 'string', widget: 'image' }),
  })
}

describe('paramSchemaRow: array imageUrls + maxItems', () => {
  it('serializes widget and maxItems onto the array spec', () => {
    const map = rowsToMap([makeImageUrlsRow(4)])
    const spec = map.image_urls as Record<string, unknown>
    expect(spec.widget).toBe('imageUrls')
    expect(spec.maxItems).toBe(4)
    expect(spec.required).toBe(true)
    expect(spec.description).toBe('参考图片')
    expect(spec.description_en).toBe('Reference images')
    // 元素 schema 仍然被写出，且是一个 string 图片叶子
    expect(spec.items).toMatchObject({ value: '', widget: 'image' })
  })

  it('omits maxItems when unlimited and omits widget when default', () => {
    const noMax = rowsToMap([makeImageUrlsRow(0)]).image_urls as Record<string, unknown>
    expect(noMax.widget).toBe('imageUrls')
    expect('maxItems' in noMax).toBe(false)

    const plainArray = rowsToMap([
      makeSchemaRow({
        key: 'tags',
        type: 'array',
        items: makeSchemaRow({ key: '', type: 'string' }),
      }),
    ]).tags as Record<string, unknown>
    expect('widget' in plainArray).toBe(false)
    expect('maxItems' in plainArray).toBe(false)
  })

  it('round-trips through mapToRows', () => {
    const rows = mapToRows(rowsToMap([makeImageUrlsRow(6)]))
    expect(rows).toHaveLength(1)
    const row = rows[0]
    expect(row.type).toBe('array')
    expect(row.widget).toBe('imageUrls')
    expect(row.maxItems).toBe(6)
    expect(row.required).toBe(true)
    expect(row.items?.type).toBe('string')
  })

  it('normalizes hostile maxItems values on read', () => {
    const read = (maxItems: unknown) =>
      mapToRows({ imgs: { items: { value: '' }, maxItems } })[0].maxItems
    // 负数 / 0 / 非数字 → 0（不限制）
    expect(read(-3)).toBe(0)
    expect(read(0)).toBe(0)
    expect(read('abc')).toBe(0)
    expect(read(null)).toBe(0)
    // 小数向下取整；超过 100 夹到 100，避免渲染出天量输入框
    expect(read(3.7)).toBe(3)
    expect(read(9999)).toBe(100)
  })

  it('ignores unknown array widget values', () => {
    expect(mapToRows({ imgs: { items: { value: '' }, widget: 'bogus' } })[0].widget).toBe('input')
  })
})

describe('paramSpec: array imageUrls + maxItems (playground read side)', () => {
  it('parses widget and maxItems into FieldSpec', () => {
    const specs = extractFieldSpecs(rowsToMap([makeImageUrlsRow(3)]))
    expect(specs).toHaveLength(1)
    const f = specs[0]
    expect(f.key).toBe('image_urls')
    expect(f.rawType).toBe('array')
    expect(f.widget).toBe('imageUrls')
    expect(f.maxItems).toBe(3)
    expect(f.required).toBe(true)
  })

  it('defaults widget/maxItems for legacy arrays without the new keys', () => {
    const specs = extractFieldSpecs({ tags: { items: { value: '' } } })
    expect(specs[0].widget).toBe('input')
    expect(specs[0].maxItems).toBe(0)
  })

  it('keeps maxItems at 0 for non-array fields', () => {
    const specs = extractFieldSpecs({ prompt: { value: '', widget: 'textarea', rows: 4 } })
    expect(specs[0].rawType).toBe('string')
    expect(specs[0].widget).toBe('textarea')
    expect(specs[0].maxItems).toBe(0)
  })
})
