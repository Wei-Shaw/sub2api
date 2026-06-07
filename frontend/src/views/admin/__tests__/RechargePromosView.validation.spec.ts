import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RechargePromosView from '../RechargePromosView.vue'
import {
  validateRechargePromoForm,
  type RechargePromoFormState,
} from '../rechargePromoFormValidation'

// 1) 纯校验器单测：稳定靶点，覆盖 backend 规则的全部 6 个 error key。
describe('validateRechargePromoForm', () => {
  function baseForm(overrides: Partial<RechargePromoFormState> = {}): RechargePromoFormState {
    return {
      name: 'Spring Promo',
      enabled: true,
      valid_from: '',
      valid_until: '',
      tiers: [
        { min_amount: 100, bonus_rate: 0.05 },
        { min_amount: 500, bonus_rate: 0.08 },
        { min_amount: 1000, bonus_rate: 0.12 },
      ],
      ...overrides,
    }
  }

  it('returns null for a valid 3-tier ascending form', () => {
    expect(validateRechargePromoForm(baseForm())).toBeNull()
  })

  it('flags missing campaign name', () => {
    expect(validateRechargePromoForm(baseForm({ name: '   ' }))).toBe('nameRequired')
  })

  it('flags enabled-with-empty tiers', () => {
    expect(
      validateRechargePromoForm(baseForm({ enabled: true, tiers: [] })),
    ).toBe('tiersRequiredWhenEnabled')
  })

  it('allows disabled-with-empty tiers (admin draft state)', () => {
    expect(
      validateRechargePromoForm(baseForm({ enabled: false, tiers: [] })),
    ).toBeNull()
  })

  it('rejects min_amount = 0 or negative', () => {
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: 0, bonus_rate: 0.05 }] }),
      ),
    ).toBe('minAmountInvalid')
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: -10, bonus_rate: 0.05 }] }),
      ),
    ).toBe('minAmountInvalid')
  })

  it('rejects non-finite min_amount', () => {
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: Number.NaN, bonus_rate: 0.05 }] }),
      ),
    ).toBe('minAmountInvalid')
  })

  it('rejects bonus_rate < 0 or >= 1 (matches backend [0, 1) interval)', () => {
    // < 0
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: 100, bonus_rate: -0.01 }] }),
      ),
    ).toBe('bonusRateOutOfRange')
    // 边界 1.0：必须被拒（半开区间）
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: 100, bonus_rate: 1 }] }),
      ),
    ).toBe('bonusRateOutOfRange')
    // > 1
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: 100, bonus_rate: 1.5 }] }),
      ),
    ).toBe('bonusRateOutOfRange')
    // 0 是合法边界
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: 100, bonus_rate: 0 }] }),
      ),
    ).toBeNull()
    // 0.99 也合法
    expect(
      validateRechargePromoForm(
        baseForm({ tiers: [{ min_amount: 100, bonus_rate: 0.99 }] }),
      ),
    ).toBeNull()
  })

  it('rejects tiers not strictly ascending in min_amount', () => {
    // 反序
    expect(
      validateRechargePromoForm(
        baseForm({
          tiers: [
            { min_amount: 500, bonus_rate: 0.08 },
            { min_amount: 100, bonus_rate: 0.05 },
          ],
        }),
      ),
    ).toBe('tiersNotAscending')
    // 重复（"严格"升序也禁止 ==）
    expect(
      validateRechargePromoForm(
        baseForm({
          tiers: [
            { min_amount: 100, bonus_rate: 0.05 },
            { min_amount: 100, bonus_rate: 0.08 },
          ],
        }),
      ),
    ).toBe('tiersNotAscending')
    // 中间倒挂
    expect(
      validateRechargePromoForm(
        baseForm({
          tiers: [
            { min_amount: 100, bonus_rate: 0.05 },
            { min_amount: 90, bonus_rate: 0.08 },
            { min_amount: 1000, bonus_rate: 0.12 },
          ],
        }),
      ),
    ).toBe('tiersNotAscending')
  })

  it('rejects valid_until <= valid_from when both are present', () => {
    expect(
      validateRechargePromoForm(
        baseForm({
          valid_from: '2026-06-10T00:00',
          valid_until: '2026-06-01T00:00',
        }),
      ),
    ).toBe('validUntilBeforeFrom')
    // 相等也禁止（活动窗口必须有正长度）
    expect(
      validateRechargePromoForm(
        baseForm({
          valid_from: '2026-06-01T00:00',
          valid_until: '2026-06-01T00:00',
        }),
      ),
    ).toBe('validUntilBeforeFrom')
  })

  it('accepts a form where only one of valid_from / valid_until is set', () => {
    expect(
      validateRechargePromoForm(baseForm({ valid_from: '2026-06-01T00:00' })),
    ).toBeNull()
    expect(
      validateRechargePromoForm(baseForm({ valid_until: '2026-12-31T23:59' })),
    ).toBeNull()
  })
})

// 2) SFC 集成断言：表单非法时 submitForm 应 toast + 跳过 API；
//    合法时应调用 update API。这一层只验证 wiring，规则细节交给上面的纯函数测试。
const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const showInfo = vi.hoisted(() => vi.fn())
const listMock = vi.hoisted(() => vi.fn())
const createMock = vi.hoisted(() => vi.fn())
const updateMock = vi.hoisted(() => vi.fn())

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    rechargePromos: {
      list: listMock,
      create: createMock,
      update: updateMock,
      delete: vi.fn(),
      toggle: vi.fn(),
    },
  },
}))

describe('RechargePromosView · client-side validation wiring', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()
    listMock.mockReset().mockResolvedValue({ items: [], total: 0 })
    createMock.mockReset().mockResolvedValue(undefined)
    updateMock.mockReset().mockResolvedValue(undefined)
  })

  async function mountAndOpenCreate() {
    const wrapper = mount(RechargePromosView, {
      global: {
        stubs: {
          // AppLayout / TablePageLayout 等容器组件透明渲染 slot。
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
          },
          DataTable: true,
          Pagination: true,
          // BaseDialog 必须渲染默认 slot 才能让 submitForm / form 输入起作用。
          BaseDialog: {
            props: ['show', 'title'],
            template: '<div v-if="show"><slot /><slot name="footer" /></div>',
          },
          ConfirmDialog: true,
          Icon: true,
        },
      },
    })
    await flushPromises()
    // 打开 Create 对话框：组件会塞一档默认 (100, 0.05)，name 留空。
    const createBtn = wrapper
      .findAll('button')
      .find((b) => b.text().includes('admin.rechargePromos.createBtn'))
    expect(createBtn).toBeTruthy()
    await createBtn!.trigger('click')
    await flushPromises()
    return wrapper
  }

  async function clickSave(wrapper: Awaited<ReturnType<typeof mountAndOpenCreate>>) {
    const buttons = wrapper.findAll('button')
    const saveBtn = buttons.find((b) => b.text().includes('common.save'))
    expect(saveBtn).toBeTruthy()
    await saveBtn!.trigger('click')
    await flushPromises()
  }

  it('shows nameRequired error and skips API when name is blank', async () => {
    const wrapper = await mountAndOpenCreate()
    // 默认 form.name = ''，直接提交。
    await clickSave(wrapper)
    expect(showError).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('admin.rechargePromos.errors.nameRequired')
    expect(createMock).not.toHaveBeenCalled()
    expect(updateMock).not.toHaveBeenCalled()
  })

  it('shows tiersNotAscending error and skips API when tiers are reverse-ordered', async () => {
    const wrapper = await mountAndOpenCreate()
    // 填 name，然后把唯一一档替换成 [500, 100] 反序。
    const nameInput = wrapper.find('input[type="text"]')
    await nameInput.setValue('Reverse Order')
    const numberInputs = wrapper.findAll('input[type="number"]')
    expect(numberInputs.length).toBeGreaterThanOrEqual(2)
    // 当前只有一档 → 通过 addTier 按钮加第二档。
    const addBtn = wrapper
      .findAll('button')
      .find((b) => b.text().includes('admin.rechargePromos.fields.addTier'))
    expect(addBtn).toBeTruthy()
    await addBtn!.trigger('click')
    await flushPromises()
    const allNumberInputs = wrapper.findAll('input[type="number"]')
    // 顺序为 [tier0.min, tier0.rate, tier1.min, tier1.rate]；
    // addTier 默认把 tier1 设成 (200, 0.05)，所以已经是升序。改成反序。
    await allNumberInputs[0].setValue(500)
    await allNumberInputs[2].setValue(100)
    await flushPromises()

    await clickSave(wrapper)
    expect(showError).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('admin.rechargePromos.errors.tiersNotAscending')
    expect(createMock).not.toHaveBeenCalled()
  })

  it('shows bonusRateOutOfRange error when a tier rate is >= 1', async () => {
    const wrapper = await mountAndOpenCreate()
    const nameInput = wrapper.find('input[type="text"]')
    await nameInput.setValue('Rate Too High')
    // 默认一档 (100, 0.05) → 改成 (100, 1.5)。
    const numberInputs = wrapper.findAll('input[type="number"]')
    await numberInputs[1].setValue(1.5)
    await flushPromises()

    await clickSave(wrapper)
    expect(showError).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith(
      'admin.rechargePromos.errors.bonusRateOutOfRange',
    )
    expect(createMock).not.toHaveBeenCalled()
  })

  it('proceeds to create API when the form is valid', async () => {
    const wrapper = await mountAndOpenCreate()
    const nameInput = wrapper.find('input[type="text"]')
    await nameInput.setValue('Valid Promo')
    // 默认 (100, 0.05) 已经合法，直接提交。
    await clickSave(wrapper)
    expect(showError).not.toHaveBeenCalled()
    expect(createMock).toHaveBeenCalledTimes(1)
    expect(createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Valid Promo',
        tiers: [{ min_amount: 100, bonus_rate: 0.05 }],
      }),
    )
  })
})
