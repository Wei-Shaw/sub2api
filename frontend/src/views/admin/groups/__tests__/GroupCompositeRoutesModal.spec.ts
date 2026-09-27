import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'

import type { CompositeRouteDecision } from '@/types'
import GroupCompositeRoutesModal from '../GroupCompositeRoutesModal.vue'
import { GROUPS_VIEW_CONTEXT } from '../context'
import type { GroupsViewContext } from '../useGroupsView'

const mountWithDecision = (decision: Partial<CompositeRouteDecision>) => {
  const identity = (value: string) => value
  const ctx = {
    closeCompositeRoutesModal: vi.fn(),
    compositePreviewDecision: ref({
      matched: true,
      source: 'route',
      group_id: 7,
      public_model: 'all/claude',
      target_platform: 'anthropic',
      upstream_model: 'claude-sonnet-4-6',
      endpoint: 'any',
      ...decision
    }),
    compositePreviewEndpoint: ref('any'),
    compositePreviewLoading: ref(false),
    compositePreviewModel: ref('all/claude'),
    compositeRouteEditingId: ref(null),
    compositeRouteEndpointOptions: [],
    compositeRouteForm: reactive({
      public_model: '', match_type: 'exact', endpoint: 'any', target_platform: 'anthropic',
      priority: 100, upstream_model: '', notes: '', enabled: true
    }),
    compositeRouteMatchLabel: identity,
    compositeRouteMatchOptions: [],
    compositeRoutePlatformOptions: [],
    compositeRouteSaving: ref(false),
    compositeRouteSourceLabel: identity,
    compositeRoutes: ref([]),
    compositeRoutesGroup: ref({ name: 'composite' }),
    compositeRoutesLoading: ref(false),
    deleteCompositeRoute: vi.fn(),
    editCompositeRoute: vi.fn(),
    formatCompositeEndpoint: identity,
    formatCompositePlatform: identity,
    loadCompositeRoutes: vi.fn(),
    previewCompositeRoute: vi.fn(),
    resetCompositeRouteForm: vi.fn(),
    saveCompositeRoute: vi.fn(),
    showCompositeRoutesModal: ref(true),
    t: (key: string) => key
  } as unknown as GroupsViewContext

  return mount(GroupCompositeRoutesModal, {
    global: {
      provide: { [GROUPS_VIEW_CONTEXT as symbol]: ctx },
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        Icon: true,
        PlatformIcon: true,
        Select: true
      }
    }
  })
}

describe('GroupCompositeRoutesModal preview', () => {
  it('warns when a matched route has no available account', () => {
    const wrapper = mountWithDecision({ available_accounts: 0 })
    expect(wrapper.text()).toContain('admin.groups.compositeRoutes.matched')
    expect(wrapper.text()).toContain('admin.groups.compositeRoutes.noAvailableAccounts')
    expect(wrapper.text()).not.toContain('admin.groups.compositeRoutes.availableAccounts')
  })

  it('shows the available account count when accounts exist', () => {
    const wrapper = mountWithDecision({ available_accounts: 3 })
    expect(wrapper.text()).toContain('admin.groups.compositeRoutes.availableAccounts: 3')
    expect(wrapper.text()).not.toContain('admin.groups.compositeRoutes.noAvailableAccounts')
  })

  it('shows neither line when the backend omits the count', () => {
    const wrapper = mountWithDecision({})
    expect(wrapper.text()).not.toContain('admin.groups.compositeRoutes.noAvailableAccounts')
    expect(wrapper.text()).not.toContain('admin.groups.compositeRoutes.availableAccounts')
  })
})
