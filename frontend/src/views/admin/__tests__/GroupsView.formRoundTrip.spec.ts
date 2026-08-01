import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

// The create and edit dialogs are about 1,500 lines of form each and share
// roughly 70% of their markup, so they are the obvious thing to merge into one
// component. Nothing covered them: the other GroupsView specs stub BaseDialog
// away entirely, so the dialog bodies never render.
//
// These pin the part a merge would most easily break — which form field is
// bound to which property. Opening the edit dialog on a group whose fields all
// hold distinctive values and submitting it unchanged puts every field through
// group -> form -> payload in one go, so a single mis-bound field shows up as a
// changed payload rather than as a bug someone finds in production.

const {
  listGroups,
  updateGroup,
  createGroup,
  getModelsListCandidates,
  getUsageSummary,
  getCapacitySummary,
  getLiveCapability,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  updateGroup: vi.fn(),
  createGroup: vi.fn(),
  getModelsListCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      update: updateGroup,
      create: createGroup,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      getAll: vi.fn(),
      duplicate: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn(),
      listCompositeRoutes: vi.fn().mockResolvedValue([]),
      createCompositeRoute: vi.fn(),
      updateCompositeRoute: vi.fn(),
      deleteCompositeRoute: vi.fn(),
      getLiveCapability
    },
    accounts: {
      list: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      getById: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

// Every field carries a value that is not the default, so a field wired to the
// wrong property cannot come back looking right by accident.
const populatedGroup: AdminGroup = {
  id: 42,
  name: 'Primary',
  description: 'a description',
  platform: 'openai',
  rate_multiplier: 2.5,
  rpm_limit: 120,
  is_exclusive: true,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: 11,
  weekly_limit_usd: 22,
  monthly_limit_usd: 33,
  allow_image_generation: true,
  allow_batch_image_generation: true,
  image_rate_independent: true,
  image_rate_multiplier: 3.5,
  batch_image_discount_multiplier: 0.4,
  batch_image_hold_multiplier: 0.7,
  image_price_1k: 1.1,
  image_price_2k: 2.2,
  image_price_4k: 4.4,
  video_rate_independent: true,
  video_rate_multiplier: 5.5,
  video_price_480p: 0.48,
  video_price_720p: 0.72,
  video_price_1080p: 1.08,
  web_search_price_per_call: 0.09,
  peak_rate_enabled: true,
  peak_start: '09:00',
  peak_end: '18:00',
  peak_rate_multiplier: 1.75,
  claude_code_only: true,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  allow_live: true,
  default_mapped_model: '',
  messages_dispatch_model_config: undefined,
  require_oauth_only: true,
  require_privacy_set: true,
  created_at: '2026-07-16T00:00:00Z',
  updated_at: '2026-07-16T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  supported_model_scopes: [],
  account_count: 1,
  active_account_count: 1,
  rate_limited_account_count: 0,
  models_list_config: undefined,
  sort_order: 10
}

const AppLayoutStub = defineComponent({
  template: '<main><slot /></main>'
})

const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false }
  },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>'
})

// Unlike the other GroupsView specs, this one needs the dialog body. Without
// it the form never mounts and there is nothing to check.
const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show" class="dialog"><slot /><slot name="footer" /></div>'
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        BaseDialog: BaseDialogStub,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        PlatformIcon: true,
        Icon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: true
      }
    }
  })
}

describe('GroupsView group form round trip', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    for (const fn of [
      listGroups,
      updateGroup,
      createGroup,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      getLiveCapability,
      showSuccess,
      showError
    ]) {
      fn.mockReset()
    }

    listGroups.mockResolvedValue({
      items: [populatedGroup],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    updateGroup.mockResolvedValue({ ...populatedGroup })
    createGroup.mockResolvedValue({ ...populatedGroup, id: 43 })
    getModelsListCandidates.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    getLiveCapability.mockResolvedValue({ supported: true })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  async function openEditDialog() {
    const wrapper = mountView()
    await flushPromises()
    const editButton = wrapper.findAll('button').find((b) => b.text().includes('common.edit'))
    expect(editButton, 'edit button should be reachable from the row actions').toBeTruthy()
    await editButton!.trigger('click')
    await flushPromises()
    return wrapper
  }

  it('opens the edit dialog and renders its form', async () => {
    const wrapper = await openEditDialog()
    const forms = wrapper.findAll('form')
    expect(forms.length, 'the edit dialog body must render, not be stubbed away').toBeGreaterThan(0)
  })

  it('submitting an untouched edit form sends every field back unchanged', async () => {
    const wrapper = await openEditDialog()

    const form = wrapper.findAll('form').at(-1)
    expect(form).toBeTruthy()
    await form!.trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledTimes(1)
    const [id, payload] = updateGroup.mock.calls[0] as [number, Record<string, unknown>]
    expect(id).toBe(populatedGroup.id)

    // Each of these is a separate section of the form. A merge that binds any
    // one of them to the wrong property changes exactly this object.
    expect(payload).toMatchObject({
      name: 'Primary',
      description: 'a description',
      platform: 'openai',
      rate_multiplier: 2.5,
      rpm_limit: 120,
      is_exclusive: true,
      daily_limit_usd: 11,
      weekly_limit_usd: 22,
      monthly_limit_usd: 33,
      allow_image_generation: true,
      image_rate_independent: true,
      image_rate_multiplier: 3.5,
      image_price_1k: 1.1,
      image_price_2k: 2.2,
      image_price_4k: 4.4,
      video_rate_independent: true,
      video_rate_multiplier: 5.5,
      video_price_480p: 0.48,
      video_price_720p: 0.72,
      video_price_1080p: 1.08,
      web_search_price_per_call: 0.09,
      peak_rate_enabled: true,
      peak_start: '09:00',
      peak_end: '18:00',
      peak_rate_multiplier: 1.75,
      claude_code_only: true,
      require_oauth_only: true,
      require_privacy_set: true,
      mcp_xml_inject: false
    })
  })

  it('resets batch image pricing for a platform that does not support it', async () => {
    // resetDisabledBatchImagePricing only lets batch image billing stand on
    // gemini. The group below asks for it on openai, and the rule takes it
    // away again. This is easy to lose when the two dialogs are merged,
    // because the rule lives in a watcher per form rather than in the markup.
    listGroups.mockResolvedValue({
      items: [{ ...populatedGroup, allow_batch_image_generation: true, batch_image_discount_multiplier: 0.4 }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const wrapper = await openEditDialog()
    await wrapper.findAll('form').at(-1)!.trigger('submit')
    await flushPromises()

    const [, payload] = updateGroup.mock.calls[0] as [number, Record<string, unknown>]
    expect(payload.allow_batch_image_generation).toBe(false)
    expect(payload.batch_image_discount_multiplier).toBe(0.5)
    expect(payload.batch_image_hold_multiplier).toBe(0.6)
  })

  it('keeps batch image pricing on gemini, where it applies', async () => {
    listGroups.mockResolvedValue({
      items: [
        {
          ...populatedGroup,
          platform: 'gemini',
          allow_image_generation: true,
          allow_batch_image_generation: true,
          batch_image_discount_multiplier: 0.4,
          batch_image_hold_multiplier: 0.7
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const wrapper = await openEditDialog()
    await wrapper.findAll('form').at(-1)!.trigger('submit')
    await flushPromises()

    const [, payload] = updateGroup.mock.calls[0] as [number, Record<string, unknown>]
    expect(payload.allow_batch_image_generation).toBe(true)
    expect(payload.batch_image_discount_multiplier).toBe(0.4)
    expect(payload.batch_image_hold_multiplier).toBe(0.7)
  })

  it('opening create after closing edit does not inherit the edited group', async () => {
    // The two dialogs now share one form object, which makes the reset
    // load-bearing: neither close handler clears everything — closeEditModal
    // leaves name, platform and rate_multiplier alone. Both entry points
    // therefore start from createDefaultGroupForm(). Without that, the last
    // edited group would quietly carry into the next create.
    const wrapper = await openEditDialog()

    const cancel = wrapper.findAll('button').find((b) => b.text().includes('common.cancel'))
    expect(cancel, 'edit dialog should have a cancel button').toBeTruthy()
    await cancel!.trigger('click')
    await flushPromises()

    const createButton = wrapper
      .findAll('button')
      .find((b) => b.text().includes('admin.groups.createGroup'))
    await createButton!.trigger('click')
    await flushPromises()

    const form = wrapper.findAll('form').at(-1)
    expect(form, 'the create dialog body must render').toBeTruthy()
    const nameInput = form!.find('input[type="text"]')
    await nameInput.setValue('Fresh group')
    await form!.trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenCalledTimes(1)
    const [payload] = createGroup.mock.calls[0] as [Record<string, unknown>]
    expect(payload.name).toBe('Fresh group')
    expect(payload.description).toBe('')
    expect(payload.rate_multiplier).toBe(1)
    expect(payload.is_exclusive).toBe(false)
    expect(payload.image_rate_multiplier).toBe(1)
    // populatedGroup has allow_live: true, so this one proves the Live toggle
    // is reset too — it is the newest field on the shared form and the one a
    // future edit is most likely to forget.
    expect(payload.allow_live).toBe(false)
  })

  it('both payloads carry copy_accounts_from_group_ids even though only create shows it', async () => {
    // Copying accounts from another group is rendered only in the create
    // dialog, but the field sits on both form objects and both payloads spread
    // it, so the edit request sends a key the dialog has no control for. That
    // is the behaviour today; a merged form must not quietly change it,
    // because the backend sees the difference and this spec would not.
    const wrapper = await openEditDialog()
    await wrapper.findAll('form').at(-1)!.trigger('submit')
    await flushPromises()
    const [, updatePayload] = updateGroup.mock.calls[0] as [number, Record<string, unknown>]
    expect(updatePayload).toHaveProperty('copy_accounts_from_group_ids')

    const createWrapper = mountView()
    await flushPromises()
    const createButton = createWrapper
      .findAll('button')
      .find((b) => b.text().includes('admin.groups.createGroup'))
    expect(createButton, 'create button should be reachable').toBeTruthy()
    await createButton!.trigger('click')
    await flushPromises()

    const createForm = createWrapper.findAll('form').at(-1)
    expect(createForm, 'the create dialog body must render').toBeTruthy()
    const nameInput = createForm!.find('input[type="text"]')
    if (nameInput.exists()) {
      await nameInput.setValue('New group')
    }
    await createForm!.trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenCalledTimes(1)
    const [createPayload] = createGroup.mock.calls[0] as [Record<string, unknown>]
    expect(createPayload).toHaveProperty('copy_accounts_from_group_ids')
  })
})
