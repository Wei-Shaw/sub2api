import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CodexIdentityTemplateWriteRequest } from '@/types/codexIdentity'

const { get, post, put, deleteRequest } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  deleteRequest: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: deleteRequest },
}))

import codexIdentityTemplatesAPI from '@/api/admin/codexIdentityTemplates'

const payload: CodexIdentityTemplateWriteRequest = {
  name: 'Desktop and CLI',
  description: 'Shared egress policy',
  session_policy: { mode: 'conversation_isolated' },
  affinity_ttl_seconds: 3600,
  unsupported_policy: 'reject',
  profiles: [{
    os_class: 'windows',
    canonical_surface: 'desktop',
    architecture: 'x86_64',
    slot_count: 1,
    proxy_mode: 'inherit',
    catalog_version: 1,
  }],
}

describe('Codex identity template admin API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    deleteRequest.mockReset()
  })

  it('unwraps the template list and reads one current revision', async () => {
    get.mockResolvedValueOnce({ data: { items: [{ id: 7 }] } })
    get.mockResolvedValueOnce({ data: { id: 7, revision: 3 } })

    await expect(codexIdentityTemplatesAPI.list()).resolves.toEqual([{ id: 7 }])
    await expect(codexIdentityTemplatesAPI.getByID(7)).resolves.toMatchObject({ id: 7, revision: 3 })
    expect(get).toHaveBeenNthCalledWith(1, '/admin/settings/codex-identity-templates')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/settings/codex-identity-templates/7')
  })

  it('uses full create and optimistic update payloads', async () => {
    post.mockResolvedValueOnce({ data: { id: 7 } })
    put.mockResolvedValueOnce({ data: { id: 7, revision: 4 } })

    await codexIdentityTemplatesAPI.create(payload)
    await codexIdentityTemplatesAPI.update(7, {
      ...payload,
      expected_revision: 3,
      confirm_assigned_accounts: false,
    })

    expect(post).toHaveBeenCalledWith('/admin/settings/codex-identity-templates', payload)
    expect(put).toHaveBeenCalledWith('/admin/settings/codex-identity-templates/7', {
      ...payload,
      expected_revision: 3,
      confirm_assigned_accounts: false,
    })
  })

  it('deletes through the scoped settings endpoint', async () => {
    deleteRequest.mockResolvedValueOnce({ data: { message: 'deleted' } })
    await expect(codexIdentityTemplatesAPI.delete(7)).resolves.toEqual({ message: 'deleted' })
    expect(deleteRequest).toHaveBeenCalledWith('/admin/settings/codex-identity-templates/7')
  })
})
