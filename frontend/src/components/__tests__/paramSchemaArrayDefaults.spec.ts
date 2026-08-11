import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  defaultValueForSchemaRow,
  makeSchemaRow,
  mapToRows,
  normalizeValueForSchemaRow,
  rowsToMap,
} from '@/components/common/paramSchemaRow'
import { buildDefaultBody } from '@/components/video/paramSpec'

const componentSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../common/ParamSchemaEditor.vue'),
  'utf8',
)

describe('paramSchemaRow: typed array defaults', () => {
  it('preserves number, boolean, object, and nested array defaults', () => {
    const cases = [
      { type: 'number' as const, defaults: [1, 2.5] },
      { type: 'boolean' as const, defaults: [true, false] },
      {
        type: 'object' as const,
        defaults: [{ name: 'first', enabled: true }, { name: 'second', enabled: false }],
        children: [
          makeSchemaRow({ key: 'name', type: 'string' }),
          makeSchemaRow({ key: 'enabled', type: 'boolean' }),
        ],
      },
      {
        type: 'array' as const,
        defaults: [['a', 'b'], ['c']],
        items: makeSchemaRow({ key: '', type: 'string' }),
      },
    ]

    for (const testCase of cases) {
      const item = makeSchemaRow({
        key: '',
        type: testCase.type,
        children: 'children' in testCase ? testCase.children : [],
        items: 'items' in testCase ? testCase.items : null,
      })
      const row = makeSchemaRow({
        key: 'values',
        type: 'array',
        items: item,
        arrayDefaults: testCase.defaults,
      })
      const stored = rowsToMap([row])

      expect((stored.values as Record<string, unknown>).value).toEqual(testCase.defaults)
      expect(mapToRows(stored)[0].arrayDefaults).toEqual(testCase.defaults)
      expect(buildDefaultBody(stored)).toEqual({ values: testCase.defaults })
    }
  })

  it('creates a new default element from the item schema', () => {
    const item = makeSchemaRow({
      key: '',
      type: 'object',
      children: [
        makeSchemaRow({ key: 'prompt', type: 'string', value: 'hello' }),
        makeSchemaRow({ key: 'count', type: 'number', value: '2' }),
        makeSchemaRow({ key: 'enabled', type: 'boolean', boolValue: true }),
      ],
    })

    expect(defaultValueForSchemaRow(item)).toEqual({
      prompt: 'hello',
      count: 2,
      enabled: true,
    })
  })

  it('normalizes existing defaults when the item schema type changes', () => {
    const numberItem = makeSchemaRow({ key: '', type: 'number' })
    expect(['1', 2, 'invalid'].map((value) => normalizeValueForSchemaRow(numberItem, value)))
      .toEqual([1, 2, 0])

    const objectItem = makeSchemaRow({
      key: '',
      type: 'object',
      children: [
        makeSchemaRow({ key: 'name', type: 'string' }),
        makeSchemaRow({ key: 'enabled', type: 'boolean', boolValue: true }),
      ],
    })
    expect(normalizeValueForSchemaRow(objectItem, { name: 123 })).toEqual({
      name: '123',
      enabled: true,
    })
  })

  it('normalizes imported defaults to the declared item type and maxItems', () => {
    const row = mapToRows({
      values: {
        items: { value: 0 },
        value: ['1', 2, 'invalid'],
        maxItems: 2,
      },
    })[0]

    expect(row.arrayDefaults).toEqual([1, 2])
  })

  it('keeps array item defaults inside the array items editor', () => {
    expect(componentSource).toContain('<div v-if="!isArrayItem" class="space-y-1.5 pl-1">')
    expect(componentSource).toContain(
      `v-if="(node.type === 'string' || node.type === 'number') && !isArrayItem"`,
    )
    expect(componentSource).toContain(
      `<div v-else-if="node.type === 'boolean' && !isArrayItem" class="flex flex-col gap-1">`,
    )

    const arrayItemsStart = componentSource.indexOf('[ array items')
    const arrayItemsEnd = componentSource.indexOf('<!-- ============================================================', arrayItemsStart)
    const arrayItemsSource = componentSource.slice(arrayItemsStart, arrayItemsEnd)

    expect(arrayItemsSource).toContain("t('admin.modelIntros.fields.addArrayItem')")
    expect(arrayItemsSource).toContain('v-for="(value, index) in node.arrayDefaults"')
    expect(arrayItemsSource).toContain('<SchemaValueEditor')
    expect(arrayItemsSource).toContain('v-model="node.arrayDefaults"')
    expect(arrayItemsSource).toContain('handle=".array-default-drag-handle"')
    expect(arrayItemsSource).toContain('@click="moveArrayDefault(index, -1)"')
    expect(arrayItemsSource).toContain('@click="moveArrayDefault(index, 1)"')
    expect(componentSource).not.toContain("t('admin.modelIntros.fields.arrayDefaults')")
  })
})
