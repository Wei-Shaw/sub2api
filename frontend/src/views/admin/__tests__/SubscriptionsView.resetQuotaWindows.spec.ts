import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SubscriptionsView from '../SubscriptionsView.vue'

const { list, resetQuota, listUsers, showError } = vi.hoisted(() => ({
  list: vi.fn(),
  resetQuota: vi.fn(),
  listUsers: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { list, resetQuota, bulkAction: vi.fn(), bulkAssign: vi.fn() },
    groups: { getAll: vi.fn().mockResolvedValue([]) },
    users: { list: listUsers }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const rows = [
  { id: 1, user_id: 11, group_id: 1, status: 'active', user: { email: 'active@example.com' } }
]

// ConfirmDialog 需要渲染默认 slot（分项勾选框在里面），故不能用 `true` 简写
const ConfirmDialogStub = {
  name: 'ConfirmDialog',
  props: ['show', 'title', 'message', 'confirmText', 'cancelText'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="reset-quota-dialog">
      <slot />
      <button data-test="confirm" @click="$emit('confirm')">confirm</button>
      <button data-test="cancel" @click="$emit('cancel')">cancel</button>
    </div>
  `
}

// DataTable 需要渲染每行的 cell-actions slot（行内「重置额度」按钮在里面）
const DataTableStub = {
  name: 'DataTable',
  props: ['data', 'selectedKeys'],
  emits: ['update:selectedKeys', 'sort'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}

function mountView() {
  return mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        Pagination: true, Select: true, Icon: true,
        GroupBadge: true, GroupOptionItem: true, EmptyState: true, Teleport: true, RouterLink: true,
        ConfirmDialog: ConfirmDialogStub
      }
    }
  })
}

let wrapper: ReturnType<typeof mountView>

beforeEach(async () => {
  vi.clearAllMocks()
  localStorage.clear()
  sessionStorage.clear()
  localStorage.setItem('auth_user', JSON.stringify({ id: 777 }))
  list.mockResolvedValue({ items: rows, total: 1, pages: 1 })
  resetQuota.mockResolvedValue({})
  wrapper = mountView()
  await flushPromises()
})

afterEach(() => {
  wrapper.unmount()
})

/** 通过行内「重置额度」按钮打开弹窗 */
async function openResetDialog() {
  const buttons = wrapper.findAll('button')
  const resetBtn = buttons.find((b) => b.text().includes('admin.subscriptions.resetQuota'))
  expect(resetBtn).toBeDefined()
  await resetBtn!.trigger('click')
  await flushPromises()
}

function windowCheckboxes() {
  return wrapper.findAll('[data-test="reset-quota-dialog"] input[type="checkbox"]')
}

describe('SubscriptionsView 单订阅额度重置的窗口选择', () => {
  it('打开弹窗时三个窗口默认全选（与既有行为一致）', async () => {
    await openResetDialog()
    const boxes = windowCheckboxes()
    expect(boxes).toHaveLength(3)
    for (const box of boxes) {
      expect((box.element as HTMLInputElement).checked).toBe(true)
    }
  })

  it('取消勾选某个窗口后，提交只带被勾选的窗口', async () => {
    await openResetDialog()
    // 取消 weekly
    await windowCheckboxes()[1].setValue(false)
    await flushPromises()

    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(resetQuota).toHaveBeenCalledTimes(1)
    expect(resetQuota).toHaveBeenCalledWith(1, { daily: true, weekly: false, monthly: true })
  })

  it('全部取消时拒绝提交并给出提示', async () => {
    await openResetDialog()
    for (const box of windowCheckboxes()) {
      await box.setValue(false)
    }
    await flushPromises()

    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(resetQuota).not.toHaveBeenCalled()
    const alert = wrapper.find('[role="alert"]')
    expect(alert.exists()).toBe(true)
    expect(alert.text()).toContain('admin.subscriptions.bulk.selectWindow')
  })

  it('重新打开弹窗时恢复为全选', async () => {
    await openResetDialog()
    await windowCheckboxes()[0].setValue(false)
    await flushPromises()
    await wrapper.get('[data-test="cancel"]').trigger('click')
    await flushPromises()

    await openResetDialog()
    const boxes = windowCheckboxes()
    for (const box of boxes) {
      expect((box.element as HTMLInputElement).checked).toBe(true)
    }
  })

  it('全选时提交的行为与改动前一致（三个窗口均为 true）', async () => {
    await openResetDialog()
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(resetQuota).toHaveBeenCalledWith(1, { daily: true, weekly: true, monthly: true })
  })
})
