import { flushPromises, mount REDACTED from '@vue/test-utils'
import { createPinia REDACTED from 'pinia'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

import AdminPaymentPlansView from '../AdminPaymentPlansView.vue'

const { getPlans, getConfig, getGroups REDACTED = vi.hoisted(() => ({
  getPlans: vi.fn(),
  getConfig: vi.fn(),
  getGroups: vi.fn(),
REDACTED))

vi.mock('@/api/admin/payment', () => ({
  adminPaymentAPI: {
    getPlans,
    getConfig,
  REDACTED,
REDACTED))

vi.mock('@/api/admin', () => ({
  default: {
    groups: {
      getAll: getGroups,
    REDACTED,
  REDACTED,
REDACTED))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    REDACTED),
  REDACTED
REDACTED)

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-price" :value="row.price" :row="row" />
      </div>
    </div>
  `,
REDACTED

describe('AdminPaymentPlansView', () => {
  beforeEach(() => {
    getGroups.mockResolvedValue([])
    getConfig.mockResolvedValue({ data: {REDACTED REDACTED)
    getPlans.mockResolvedValue({
      data: [
        {
          id: 1,
          name: 'CNY plan',
          group_id: 1,
          price: 499,
          original_price: 599,
          currency: 'CNY',
          validity_days: 30,
          validity_unit: 'day',
          sort_order: 0,
          for_sale: true,
          features: [],
        REDACTED,
        {
          id: 2,
          name: 'Legacy plan',
          group_id: 1,
          price: 10,
          original_price: 0,
          currency: '',
          validity_days: 30,
          validity_unit: 'day',
          sort_order: 0,
          for_sale: true,
          features: [],
        REDACTED,
      ],
    REDACTED)
  REDACTED)

  it('uses the configured currency symbol and keeps legacy prices in USD', async () => {
    const wrapper = mount(AdminPaymentPlansView, {
      global: {
        plugins: [createPinia()],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          DataTable: DataTableStub,
          ConfirmDialog: true,
          GroupBadge: true,
          Icon: true,
          PlanEditDialog: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('¥499.00CNY')
    expect(wrapper.text()).toContain('¥599.00')
    expect(wrapper.text()).toContain('$10.00')
  REDACTED)
REDACTED)
