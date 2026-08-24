import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createDefaultCodexIdentityPolicy,
  createDefaultCodexOSProfile
} from '@/utils/codexIdentityValidation'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get, post }
}))

import {
  finalizeCodexDrainingSlots,
  importData,
  listCodexDeviceSlots
} from '@/api/admin/accounts'

describe('admin account Codex identity API payloads', () => {
  beforeEach(() => {
    post.mockReset().mockResolvedValue({
      data: {
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0,
        account_created: 1,
        account_failed: 0
      }
    })
    get.mockReset()
  })

  it('sends policy overrides at the trusted data-import request boundary', async () => {
    const policy = createDefaultCodexIdentityPolicy()
    const data = {
      exported_at: '2026-08-24T00:00:00Z',
      proxies: [],
      accounts: []
    }

    await importData({
      data,
      skip_default_group_bind: true,
      codex_identity_policy_override: policy,
      override_imported_identity_policies: true
    })

    expect(post).toHaveBeenCalledWith('/admin/accounts/data', {
      data,
      skip_default_group_bind: true,
      codex_identity_policy_override: policy,
      override_imported_identity_policies: true
    })
  })

  it('omits override fields for imports without a reviewed policy', async () => {
    const data = {
      exported_at: '2026-08-24T00:00:00Z',
      proxies: [],
      accounts: []
    }

    await importData({ data, skip_default_group_bind: true })

    expect(post).toHaveBeenCalledWith('/admin/accounts/data', {
      data,
      skip_default_group_bind: true
    })
  })

  it('keeps explicit direct profile and slot modes in API payloads', async () => {
    const profile = createDefaultCodexOSProfile('linux')
    profile.proxy_mode = 'direct'
    profile.slots = [{ index: 0, proxy_mode: 'direct' }]
    const policy = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool' as const,
      profiles: [profile]
    }
    const data = { exported_at: '2026-08-24T00:00:00Z', proxies: [], accounts: [] }

    await importData({
      data,
      codex_identity_policy_override: policy,
      override_imported_identity_policies: true
    })

    expect(post).toHaveBeenCalledWith('/admin/accounts/data', expect.objectContaining({
      codex_identity_policy_override: expect.objectContaining({
        profiles: [expect.objectContaining({
          proxy_mode: 'direct',
          slots: [{ index: 0, proxy_mode: 'direct' }]
        })]
      })
    }))
  })

  it('uses the scoped device-slot lifecycle endpoints', async () => {
    const slots = [{
      os_class: 'linux',
      canonical_surface: 'cli',
      catalog_version: 1,
      epoch: 2,
      slot_index: 0,
      state: 'draining',
      binding_count: 0
    }]
    get.mockResolvedValueOnce({ data: { slots } })
    post.mockResolvedValueOnce({ data: { deleted: 1 } })

    await expect(listCodexDeviceSlots(7, true)).resolves.toEqual(slots)
    await expect(finalizeCodexDrainingSlots(7)).resolves.toEqual({ deleted: 1 })

    expect(get).toHaveBeenCalledWith('/admin/accounts/7/codex-device-slots', {
      params: { include_draining: true }
    })
    expect(post).toHaveBeenCalledWith('/admin/accounts/7/codex-device-slots/finalize-draining')
  })
})
