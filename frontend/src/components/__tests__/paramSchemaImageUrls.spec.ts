/**
 * 媒体 URL 组控件 + array maxItems 的读写往返测试。
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
import { buildDefaultBody, fieldSpecToDefaultValue } from '@/components/video/paramSpec'
import type { MediaUrlWidget } from '@/utils/mediaUrlWidget'

/** 造一个媒体 URL 组字段行。 */
function makeMediaUrlsRow(maxItems: number, widget: MediaUrlWidget = 'ImageUrls') {
  return makeSchemaRow({
    key: 'image_urls',
    type: 'array',
    widget,
    maxItems,
    required: true,
    description: '参考图片',
    descriptionEn: 'Reference images',
    items: makeSchemaRow({ key: '', type: 'string', widget: 'image' }),
  })
}

describe('paramSchemaRow: array media URL widgets + maxItems', () => {
  it('stores and restores multiple array default values', () => {
    const row = makeMediaUrlsRow(4)
    row.arrayDefaults = ['https://cdn.example.com/a.png', 'https://cdn.example.com/b.png']

    const stored = rowsToMap([row]).image_urls as Record<string, unknown>
    expect(stored.value).toEqual(row.arrayDefaults)

    const restored = mapToRows({ image_urls: stored })[0]
    expect(restored.arrayDefaults).toEqual(row.arrayDefaults)
  })

  it('omits empty defaults and caps saved defaults at maxItems', () => {
    const empty = rowsToMap([makeMediaUrlsRow(3)]).image_urls as Record<string, unknown>
    expect('value' in empty).toBe(false)

    const row = makeMediaUrlsRow(2)
    row.arrayDefaults = ['a', 'b', 'c']
    const stored = rowsToMap([row]).image_urls as Record<string, unknown>
    expect(stored.value).toEqual(['a', 'b'])
  })

  it('serializes widget and maxItems onto the array spec', () => {
    const map = rowsToMap([makeMediaUrlsRow(4)])
    const spec = map.image_urls as Record<string, unknown>
    expect(spec.widget).toBe('ImageUrls')
    expect(spec.maxItems).toBe(4)
    expect(spec.required).toBe(true)
    expect(spec.description).toBe('参考图片')
    expect(spec.description_en).toBe('Reference images')
    // 元素 schema 仍然被写出，且是一个 string 图片叶子
    expect(spec.items).toMatchObject({ value: '', widget: 'image' })
  })

  it('omits maxItems when unlimited and omits widget when default', () => {
    const noMax = rowsToMap([makeMediaUrlsRow(0)]).image_urls as Record<string, unknown>
    expect(noMax.widget).toBe('ImageUrls')
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
    const rows = mapToRows(rowsToMap([makeMediaUrlsRow(6)]))
    expect(rows).toHaveLength(1)
    const row = rows[0]
    expect(row.type).toBe('array')
    expect(row.widget).toBe('ImageUrls')
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

  it.each(['ImageUrls', 'VideoUrls', 'AudioUrls'] as const)(
    'round-trips the canonical %s widget',
    (widget) => {
      const stored = rowsToMap([makeMediaUrlsRow(2, widget)]).image_urls as Record<string, unknown>
      expect(stored.widget).toBe(widget)
      expect(mapToRows({ media: stored })[0].widget).toBe(widget)
    }
  )

  it('reads legacy imageUrls and saves it back as ImageUrls', () => {
    const row = mapToRows({ imgs: { items: { value: '' }, widget: 'imageUrls' } })[0]
    expect(row.widget).toBe('ImageUrls')
    expect((rowsToMap([row]).imgs as Record<string, unknown>).widget).toBe('ImageUrls')
  })
})

describe('paramSpec: array media URL widgets + maxItems (playground read side)', () => {
  it('uses the full configured array as playground and curl defaults', () => {
    const defaults = ['https://cdn.example.com/a.png', 'https://cdn.example.com/b.png']
    const params = {
      image_urls: {
        items: { value: '', widget: 'image' },
        widget: 'ImageUrls',
        value: defaults,
      },
    }
    const spec = extractFieldSpecs(params)[0]

    expect(fieldSpecToDefaultValue(spec)).toEqual(defaults)
    expect(buildDefaultBody(params)).toEqual({ image_urls: defaults })

    const initialized = fieldSpecToDefaultValue(spec) as string[]
    initialized.push('local mutation')
    expect(spec.rawDefaultValue).toEqual(defaults)
  })

  it('parses widget and maxItems into FieldSpec', () => {
    const specs = extractFieldSpecs(rowsToMap([makeMediaUrlsRow(3)]))
    expect(specs).toHaveLength(1)
    const f = specs[0]
    expect(f.key).toBe('image_urls')
    expect(f.rawType).toBe('array')
    expect(f.widget).toBe('ImageUrls')
    expect(f.maxItems).toBe(3)
    expect(f.required).toBe(true)
  })

  it.each(['ImageUrls', 'VideoUrls', 'AudioUrls'] as const)(
    'parses %s into FieldSpec',
    (widget) => {
      const specs = extractFieldSpecs({ media: { items: { value: '' }, widget } })
      expect(specs[0].widget).toBe(widget)
    }
  )

  it('normalizes legacy imageUrls in FieldSpec', () => {
    const specs = extractFieldSpecs({ media: { items: { value: '' }, widget: 'imageUrls' } })
    expect(specs[0].widget).toBe('ImageUrls')
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
