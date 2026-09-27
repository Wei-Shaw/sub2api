import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { ref } from 'vue'

const authState = vi.hoisted(() => ({ isSimpleMode: false }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({
    canUseBatchImage: ref(false),
    refreshBatchImageAccess: vi.fn().mockResolvedValue(false),
  }),
}))

import UserDashboardQuickActions from '../UserDashboardQuickActions.vue'

function actionTargets() {
  const wrapper = mount(UserDashboardQuickActions, {
    global: { stubs: { RouterLink: RouterLinkStub, Icon: true } },
  })
  return wrapper.findAllComponents(RouterLinkStub).map(link => link.props('to'))
}

describe('UserDashboardQuickActions', () => {
  beforeEach(() => {
    authState.isSimpleMode = false
  })

  it('offers redeem codes in standard mode', () => {
    expect(actionTargets()).toEqual(['/keys', '/usage', '/redeem'])
  })

  it('hides the redeem action in simple mode, where the router blocks /redeem', () => {
    authState.isSimpleMode = true
    expect(actionTargets()).toEqual(['/keys', '/usage'])
  })
})
