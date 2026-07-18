import { mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

import DataTable from '../DataTable.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  REDACTED)
REDACTED))

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
    REDACTED))
  REDACTED)
REDACTED

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
    REDACTED))
  REDACTED)
REDACTED

describe('DataTable', () => {
  beforeEach(() => {
    stubDesktopMatchMedia()
    localStorage.clear()
  REDACTED)

  it('renders paired sort arrows and highlights the active direction', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'name', label: 'Name', sortable: true REDACTED,
          { key: 'created_at', label: 'Created', sortable: true REDACTED
        ],
        data: [
          { id: 1, name: 'Beta', created_at: '2026-01-02T00:00:00Z' REDACTED,
          { id: 2, name: 'Alpha', created_at: '2026-01-01T00:00:00Z' REDACTED
        ],
        defaultSortKey: 'name',
        defaultSortOrder: 'asc'
      REDACTED,
      slots: {
        'header-name': '<span data-test="custom-name-header">Name</span>'
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const nameHeader = wrapper.findAll('th')[0]
    expect(nameHeader.find('[data-test="custom-name-header"]').exists()).toBe(true)
    expect(nameHeader.attributes('aria-sort')).toBe('ascending')
    expect(nameHeader.findAll('svg')).toHaveLength(2)
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-primary-600')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-gray-300')

    await nameHeader.trigger('click')
    await wrapper.vm.$nextTick()

    expect(nameHeader.attributes('aria-sort')).toBe('descending')
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-gray-300')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-primary-600')
  REDACTED)

  it('renders every row with no virtual padding spacer for small datasets (virtualization off)', async () => {
    const data = Array.from({ length: 8 REDACTED, (_, i) => ({ id: i + 1, name: `Row ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    // Virtualization is OFF for a small list…
    expect((wrapper.vm as any).shouldVirtualize).toBe(false)
    // …every row is in the DOM…
    expect(wrapper.findAll('tbody tr[data-index]')).toHaveLength(data.length)
    // …and there are no aria-hidden virtual padding spacer rows.
    expect(wrapper.findAll('tbody tr[aria-hidden="true"]')).toHaveLength(0)
  REDACTED)

  it('switches to windowed rendering once row count exceeds virtualizeThreshold', async () => {
    const data = Array.from({ length: 12 REDACTED, (_, i) => ({ id: i + 1, name: `Row ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data,
        virtualizeThreshold: 3
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    // Virtualization is ON: the mode-switch decision flipped…
    expect((wrapper.vm as any).shouldVirtualize).toBe(true)
    // …and the virtualizer drives off the full row count.
    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    expect(instance.options.count).toBe(data.length)
  REDACTED)

  it('keys the virtualizer size cache by row identity, not index (avoids stale heights on sort/filter)', async () => {
    const data = Array.from({ length: 12 REDACTED, (_, i) => ({ id: 100 + i, name: `Row ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data,
        rowKey: 'id',
        virtualizeThreshold: 3
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    // getItemKey must resolve to the row's stable key (id), not the positional index.
    expect(instance.options.getItemKey(0)).toBe(100)
    expect(instance.options.getItemKey(5)).toBe(105)
  REDACTED)

  it('clears stale row and element caches when pagination replaces the row ID set', async () => {
    const firstPage = Array.from({ length: 100 REDACTED, (_, i) => ({ id: i + 1, name: `First ${i + 1REDACTED` REDACTED))
    const secondPage = Array.from({ length: 100 REDACTED, (_, i) => ({ id: i + 101, name: `Second ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data: firstPage,
        rowKey: 'id',
        virtualizeThreshold: 1
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const firstPageIDs = firstPage.map(row => row.id)
    ;(instance as any).itemSizeCache = new Map(firstPageIDs.map(id => [id, 156]))
    instance.elementsCache.clear()
    for (const id of firstPageIDs) {
      instance.elementsCache.set(id, document.createElement('tr'))
    REDACTED
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage REDACTED)
    await wrapper.vm.$nextTick()

    const sizeCache = (instance as any).itemSizeCache as Map<number, number>
    expect(sizeCache.size).toBeLessThanOrEqual(secondPage.length)
    expect(instance.elementsCache.size).toBeLessThanOrEqual(secondPage.length)
    expect(firstPageIDs.some(id => sizeCache.has(id))).toBe(false)
    expect(firstPageIDs.some(id => instance.elementsCache.has(id))).toBe(false)
    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  REDACTED)

  it('clears stale caches when equal-length pages replace rows without stable keys', async () => {
    const firstPage = Array.from({ length: 12 REDACTED, (_, i) => ({ name: `First ${i + 1REDACTED` REDACTED))
    const secondPage = Array.from({ length: 12 REDACTED, (_, i) => ({ name: `Second ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data: firstPage,
        virtualizeThreshold: 1
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage REDACTED)
    await wrapper.vm.$nextTick()

    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  REDACTED)

  it('conservatively clears caches when duplicate row-key multiplicity changes', async () => {
    const firstPage = [
      { id: 1, name: 'First A' REDACTED,
      { id: 1, name: 'First B' REDACTED,
      { id: 2, name: 'First C' REDACTED
    ]
    const secondPage = [
      { id: 1, name: 'Second A' REDACTED,
      { id: 2, name: 'Second B' REDACTED,
      { id: 2, name: 'Second C' REDACTED
    ]
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data: firstPage,
        rowKey: 'id',
        virtualizeThreshold: 1
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage REDACTED)
    await wrapper.vm.$nextTick()

    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  REDACTED)

  it('preserves cache when rows without stable keys only reorder the same objects', async () => {
    const data = Array.from({ length: 12 REDACTED, (_, i) => ({ name: `Row ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data,
        virtualizeThreshold: 1
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureSpy = vi.spyOn(instance, 'measure')

    await wrapper.setProps({ data: [...data].reverse() REDACTED)
    await wrapper.vm.$nextTick()

    expect(measureSpy).not.toHaveBeenCalled()
  REDACTED)

  it('preserves stable row height cache when the same row IDs are only reordered', async () => {
    const data = Array.from({ length: 100 REDACTED, (_, i) => ({ id: i + 1, name: `Row ${i + 1REDACTED` REDACTED))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data,
        rowKey: 'id',
        virtualizeThreshold: 1
      REDACTED
    REDACTED)

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    ;(instance as any).itemSizeCache = new Map(data.map(row => [row.id, 156]))
    const measureSpy = vi.spyOn(instance, 'measure')

    await wrapper.setProps({ data: [...data].reverse() REDACTED)
    await wrapper.vm.$nextTick()

    const sizeCache = (instance as any).itemSizeCache as Map<number, number>
    expect(measureSpy).not.toHaveBeenCalled()
    expect(sizeCache.size).toBe(100)
  REDACTED)

  it('emits controlled current-page selection while preserving off-page keys', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data: [
          { id: 1, name: 'One' REDACTED,
          { id: 2, name: 'Two' REDACTED
        ],
        rowKey: 'id',
        selectable: true,
        selectedKeys: [99]
      REDACTED
    REDACTED)

    await wrapper.get('[data-test="select-all"]').setValue(true)

    const selectedAll = wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]
    expect(selectedAll).toEqual([99, 1, 2])

    await wrapper.setProps({ selectedKeys: selectedAll as number[] REDACTED)
    const rowCheckboxes = wrapper.findAll<HTMLInputElement>('[data-test="select-row"]')
    expect(rowCheckboxes.every((checkbox) => checkbox.element.checked)).toBe(true)

    await rowCheckboxes[0].setValue(false)

    expect(wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]).toEqual([99, 2])
    expect(wrapper.emitted('selectionChange')?.at(-1)?.[0]).toEqual([99, 2])
  REDACTED)

  it('offers current-page select all in the mobile card layout', async () => {
    stubMobileMatchMedia()
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' REDACTED],
        data: [
          { id: 1, name: 'One' REDACTED,
          { id: 2, name: 'Two' REDACTED
        ],
        rowKey: 'id',
        selectable: true,
        selectedKeys: [99]
      REDACTED
    REDACTED)

    await wrapper.get('[data-test="select-all-mobile"]').setValue(true)

    expect(wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]).toEqual([99, 1, 2])
  REDACTED)
REDACTED)
