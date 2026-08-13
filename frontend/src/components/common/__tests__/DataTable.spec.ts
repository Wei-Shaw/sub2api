import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DataTable from '../DataTable.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const stubDesktopMatchMedia = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  })
}

const stubMobileMatchMedia = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  })
}

const readSortedData = (wrapper: ReturnType<typeof mount>): any[] => {
  const exposed = (wrapper.vm as any).sortedData
  return exposed?.value ?? exposed
}

const mountSorted = (
  columnKey: string,
  data: any[],
  order: 'asc' | 'desc'
) =>
  mount(DataTable, {
    props: {
      columns: [{ key: columnKey, label: columnKey, sortable: true }],
      data,
      rowKey: 'id',
      defaultSortKey: columnKey,
      defaultSortOrder: order
    }
  })

describe('DataTable', () => {
  beforeEach(() => {
    stubDesktopMatchMedia()
    localStorage.clear()
  })

  it('renders paired sort arrows and highlights the active direction', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'name', label: 'Name', sortable: true },
          { key: 'created_at', label: 'Created', sortable: true }
        ],
        data: [
          { id: 1, name: 'Beta', created_at: '2026-01-02T00:00:00Z' },
          { id: 2, name: 'Alpha', created_at: '2026-01-01T00:00:00Z' }
        ],
        defaultSortKey: 'name',
        defaultSortOrder: 'asc'
      },
      slots: {
        'header-name': '<span data-test="custom-name-header">Name</span>'
      }
    })

    await wrapper.vm.$nextTick()

    const nameHeader = wrapper.findAll('th')[0]
    // The arrows advertise their own state, so this asserts which direction is
    // active rather than which colour class happens to paint it.
    const arrows = () => nameHeader.findAll('[data-test="sort-arrow"]')

    expect(nameHeader.find('[data-test="custom-name-header"]').exists()).toBe(true)
    expect(nameHeader.attributes('aria-sort')).toBe('ascending')
    expect(arrows()).toHaveLength(2)
    expect(arrows().map((arrow) => arrow.attributes('data-sort-arrow'))).toEqual(['asc', 'desc'])
    expect(arrows().map((arrow) => arrow.attributes('data-active'))).toEqual(['true', 'false'])

    await nameHeader.trigger('click')
    await wrapper.vm.$nextTick()

    expect(nameHeader.attributes('aria-sort')).toBe('descending')
    expect(arrows().map((arrow) => arrow.attributes('data-active'))).toEqual(['false', 'true'])
  })

  it('renders every row with no virtual padding spacer for small datasets (virtualization off)', async () => {
    const data = Array.from({ length: 8 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data
      }
    })

    await wrapper.vm.$nextTick()

    // Virtualization is OFF for a small list…
    expect((wrapper.vm as any).shouldVirtualize).toBe(false)
    // …every row is in the DOM…
    expect(wrapper.findAll('tbody tr[data-index]')).toHaveLength(data.length)
    // …and there are no aria-hidden virtual padding spacer rows.
    expect(wrapper.findAll('tbody tr[aria-hidden="true"]')).toHaveLength(0)
  })

  it('switches to windowed rendering once row count exceeds virtualizeThreshold', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        virtualizeThreshold: 3
      }
    })

    await wrapper.vm.$nextTick()

    // Virtualization is ON: the mode-switch decision flipped…
    expect((wrapper.vm as any).shouldVirtualize).toBe(true)
    // …and the virtualizer drives off the full row count.
    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    expect(instance.options.count).toBe(data.length)
  })

  it('keys the virtualizer size cache by row identity, not index (avoids stale heights on sort/filter)', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ id: 100 + i, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        rowKey: 'id',
        virtualizeThreshold: 3
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    // getItemKey must resolve to the row's stable key (id), not the positional index.
    expect(instance.options.getItemKey(0)).toBe(100)
    expect(instance.options.getItemKey(5)).toBe(105)
  })

  it('clears stale row and element caches when pagination replaces the row ID set', async () => {
    const firstPage = Array.from({ length: 100 }, (_, i) => ({ id: i + 1, name: `First ${i + 1}` }))
    const secondPage = Array.from({ length: 100 }, (_, i) => ({ id: i + 101, name: `Second ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: firstPage,
        rowKey: 'id',
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const firstPageIDs = firstPage.map(row => row.id)
    ;(instance as any).itemSizeCache = new Map(firstPageIDs.map(id => [id, 156]))
    instance.elementsCache.clear()
    for (const id of firstPageIDs) {
      instance.elementsCache.set(id, document.createElement('tr'))
    }
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage })
    await wrapper.vm.$nextTick()

    const sizeCache = (instance as any).itemSizeCache as Map<number, number>
    expect(sizeCache.size).toBeLessThanOrEqual(secondPage.length)
    expect(instance.elementsCache.size).toBeLessThanOrEqual(secondPage.length)
    expect(firstPageIDs.some(id => sizeCache.has(id))).toBe(false)
    expect(firstPageIDs.some(id => instance.elementsCache.has(id))).toBe(false)
    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  })

  it('clears stale caches when equal-length pages replace rows without stable keys', async () => {
    const firstPage = Array.from({ length: 12 }, (_, i) => ({ name: `First ${i + 1}` }))
    const secondPage = Array.from({ length: 12 }, (_, i) => ({ name: `Second ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: firstPage,
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage })
    await wrapper.vm.$nextTick()

    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  })

  it('conservatively clears caches when duplicate row-key multiplicity changes', async () => {
    const firstPage = [
      { id: 1, name: 'First A' },
      { id: 1, name: 'First B' },
      { id: 2, name: 'First C' }
    ]
    const secondPage = [
      { id: 1, name: 'Second A' },
      { id: 2, name: 'Second B' },
      { id: 2, name: 'Second C' }
    ]
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: firstPage,
        rowKey: 'id',
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage })
    await wrapper.vm.$nextTick()

    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  })

  it('preserves cache when rows without stable keys only reorder the same objects', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureSpy = vi.spyOn(instance, 'measure')

    await wrapper.setProps({ data: [...data].reverse() })
    await wrapper.vm.$nextTick()

    expect(measureSpy).not.toHaveBeenCalled()
  })

  it('preserves stable row height cache when the same row IDs are only reordered', async () => {
    const data = Array.from({ length: 100 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        rowKey: 'id',
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    ;(instance as any).itemSizeCache = new Map(data.map(row => [row.id, 156]))
    const measureSpy = vi.spyOn(instance, 'measure')

    await wrapper.setProps({ data: [...data].reverse() })
    await wrapper.vm.$nextTick()

    const sizeCache = (instance as any).itemSizeCache as Map<number, number>
    expect(measureSpy).not.toHaveBeenCalled()
    expect(sizeCache.size).toBe(100)
  })

  it('emits controlled current-page selection while preserving off-page keys', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: [
          { id: 1, name: 'One' },
          { id: 2, name: 'Two' }
        ],
        rowKey: 'id',
        selectable: true,
        selectedKeys: [99]
      }
    })

    await wrapper.get('[data-test="select-all"]').setValue(true)

    const selectedAll = wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]
    expect(selectedAll).toEqual([99, 1, 2])

    await wrapper.setProps({ selectedKeys: selectedAll as number[] })
    const rowCheckboxes = wrapper.findAll<HTMLInputElement>('[data-test="select-row"]')
    expect(rowCheckboxes.every((checkbox) => checkbox.element.checked)).toBe(true)

    await rowCheckboxes[0].setValue(false)

    expect(wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]).toEqual([99, 2])
    expect(wrapper.emitted('selectionChange')?.at(-1)?.[0]).toEqual([99, 2])
  })

  it('keeps the single usage field shrinkable in a 320px mobile card', () => {
    stubMobileMatchMedia()
    const viewport = document.createElement('div')
    viewport.style.width = '320px'
    document.body.appendChild(viewport)
    const wrapper = mount(DataTable, {
      attachTo: viewport,
      props: {
        columns: [{ key: 'usage', label: 'Usage' }],
        data: [{ id: 1, usage: 'snapshot' }],
        rowKey: 'id'
      },
      slots: {
        'cell-usage': '<div data-test="usage-cell">snapshot</div>'
      }
    })

    expect(viewport.style.width).toBe('320px')
    expect(wrapper.findAll('[data-field="usage"]')).toHaveLength(1)
    expect(wrapper.find('[data-field="ollama_cloud_usage"]').exists()).toBe(false)
    const field = wrapper.get('[data-field="usage"]')
    expect(field.classes()).toContain('min-w-0')
    expect(field.get('div').classes()).toEqual(expect.arrayContaining(['min-w-0', 'max-w-full']))
    expect(wrapper.findAll('[data-test="usage-cell"]')).toHaveLength(1)

    wrapper.unmount()
    viewport.remove()
  })

  it('offers current-page select all in the mobile card layout', async () => {
    stubMobileMatchMedia()
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: [
          { id: 1, name: 'One' },
          { id: 2, name: 'Two' }
        ],
        rowKey: 'id',
        selectable: true,
        selectedKeys: [99]
      }
    })

    await wrapper.get('[data-test="select-all-mobile"]').setValue(true)

    expect(wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]).toEqual([99, 1, 2])
  })

  /*
   * Row height is the component's own contract, not something a page shell
   * lends it: the padded `py-4` cells this replaces rendered ~56px rows against
   * a 32px spec at all 21 call sites. jsdom does no layout, so these assert the
   * hooks that carry the token (`ds-row-cell` / `ds-header-cell`, sized from
   * `--ds-row-h` / `--ds-header-h` in the component's own stylesheet) and the
   * absence of the fixed vertical padding that used to defeat them.
   */
  describe('row geometry', () => {
    const geometryColumns = (count: number) =>
      Array.from({ length: count }, (_, i) => ({ key: `c${i}`, label: `C${i}` }))

    const mountGeometry = (columnCount: number, extra: Record<string, unknown> = {}) =>
      mount(DataTable, {
        props: {
          columns: geometryColumns(columnCount),
          data: [{ id: 1 }],
          rowKey: 'id',
          ...extra
        }
      })

    it('sizes body cells from the row token instead of hardcoded vertical padding', () => {
      const cells = mountGeometry(3, { selectable: true }).findAll('tbody td')

      expect(cells.length).toBe(4) // selection cell + three data cells
      for (const cell of cells) {
        expect(cell.classes()).toContain('ds-row-cell')
        expect(cell.classes()).not.toContain('py-4')
      }
    })

    it('sizes skeleton cells identically so loading does not change row height', () => {
      const cells = mountGeometry(3, { selectable: true, loading: true }).findAll('tbody td')

      expect(cells.length).toBeGreaterThan(0)
      for (const cell of cells) {
        expect(cell.classes()).toContain('ds-row-cell')
        expect(cell.classes()).not.toContain('py-4')
      }
    })

    it('sizes header cells from the header token', () => {
      const headers = mountGeometry(3, { selectable: true }).findAll('thead th')

      expect(headers.length).toBe(4)
      for (const header of headers) {
        expect(header.classes()).toContain('ds-header-cell')
        expect(header.classes()).not.toContain('py-3')
      }
    })

    it('leaves the adaptive horizontal padding untouched', () => {
      // Column count still drives horizontal padding; only the vertical axis moved.
      const expectations: Array<[number, string]> = [
        [3, 'px-6'],
        [5, 'px-4'],
        [7, 'px-3'],
        [10, 'px-2']
      ]

      for (const [columnCount, padding] of expectations) {
        const wrapper = mountGeometry(columnCount)

        expect(wrapper.findAll('tbody td')[0].classes()).toContain(padding)
        expect(wrapper.findAll('thead th')[0].classes()).toContain(padding)
      }
    })

    it('keeps the selection column on its own fixed horizontal padding', () => {
      const wrapper = mountGeometry(10, { selectable: true })
      const selectionCell = wrapper.findAll('tbody td')[0]

      expect(selectionCell.classes()).toEqual(
        expect.arrayContaining(['ds-row-cell', 'w-11', 'px-3'])
      )
      // The adaptive class belongs to data cells only.
      expect(selectionCell.classes()).not.toContain('px-2')
    })

    it('estimates virtualized rows at the compact row height, not the old padded one', async () => {
      const data = Array.from({ length: 12 }, (_, i) => ({ id: i + 1 }))
      const wrapper = mount(DataTable, {
        props: {
          columns: geometryColumns(2),
          data,
          rowKey: 'id',
          virtualizeThreshold: 3
        }
      })

      await wrapper.vm.$nextTick()

      const exposed = (wrapper.vm as any).virtualizer
      const instance = exposed?.value ?? exposed
      // A stale 56 here (the height `py-4` produced) would make the scrollbar
      // jump as real rows are measured against it.
      expect(instance.options.estimateSize(0)).toBe(32)
    })

    it('still honours an explicit estimateRowHeight override', async () => {
      const data = Array.from({ length: 12 }, (_, i) => ({ id: i + 1 }))
      const wrapper = mount(DataTable, {
        props: {
          columns: geometryColumns(2),
          data,
          rowKey: 'id',
          virtualizeThreshold: 3,
          estimateRowHeight: 44
        }
      })

      await wrapper.vm.$nextTick()

      const exposed = (wrapper.vm as any).virtualizer
      const instance = exposed?.value ?? exposed
      expect(instance.options.estimateSize(0)).toBe(44)
    })
  })

  describe('client-side sort ordering', () => {
    // isNullishOrEmpty treats null, undefined and '' as empty.
    const numericRows = [
      { id: 1, cost: 5 },
      { id: 2, cost: null },
      { id: 3, cost: 20 },
      { id: 4, cost: undefined },
      { id: 5, cost: 1 },
      { id: 6, cost: '' }
    ]
    const stringRows = [
      { id: 1, name: 'beta' },
      { id: 2, name: '' },
      { id: 3, name: 'alpha' },
      { id: 4, name: null },
      { id: 5, name: 'gamma' },
      { id: 6, name: undefined }
    ]

    it('sorts a numeric column ascending with empty cells last', () => {
      const wrapper = mountSorted('cost', numericRows, 'asc')

      expect(readSortedData(wrapper).map(row => row.id)).toEqual([5, 1, 3, 2, 4, 6])
    })

    it('sorts a numeric column descending with empty cells still last', () => {
      const wrapper = mountSorted('cost', numericRows, 'desc')

      const ids = readSortedData(wrapper).map(row => row.id)
      // Regression: negating the whole comparator used to float empty cells to the top.
      expect(ids).toEqual([3, 1, 5, 2, 4, 6])
      expect(ids.slice(0, 3)).toEqual([3, 1, 5])
    })

    it('sorts a string column ascending with empty cells last', () => {
      const wrapper = mountSorted('name', stringRows, 'asc')

      expect(readSortedData(wrapper).map(row => row.id)).toEqual([3, 1, 5, 2, 4, 6])
    })

    it('sorts a string column descending with empty cells still last', () => {
      const wrapper = mountSorted('name', stringRows, 'desc')

      const ids = readSortedData(wrapper).map(row => row.id)
      expect(ids).toEqual([5, 1, 3, 2, 4, 6])
      expect(ids.slice(0, 3)).toEqual([5, 1, 3])
    })

    it('keeps the original order among equally empty cells in both directions', () => {
      const rows = [
        { id: 1, cost: null },
        { id: 2, cost: 7 },
        { id: 3, cost: '' },
        { id: 4, cost: undefined }
      ]

      expect(readSortedData(mountSorted('cost', rows, 'asc')).map(row => row.id)).toEqual([
        2, 1, 3, 4
      ])
      expect(readSortedData(mountSorted('cost', rows, 'desc')).map(row => row.id)).toEqual([
        2, 1, 3, 4
      ])
    })

    it('leaves row order untouched when serverSideSort is enabled', () => {
      const wrapper = mount(DataTable, {
        props: {
          columns: [{ key: 'cost', label: 'Cost', sortable: true }],
          data: numericRows,
          rowKey: 'id',
          serverSideSort: true,
          defaultSortKey: 'cost',
          defaultSortOrder: 'desc'
        }
      })

      expect(readSortedData(wrapper).map(row => row.id)).toEqual([1, 2, 3, 4, 5, 6])
    })
  })
})
