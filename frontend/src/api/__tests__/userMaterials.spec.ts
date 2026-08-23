import { beforeEach, describe, expect, it, vi } from 'vitest'

const { patch } = vi.hoisted(() => ({
  patch: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  default: { patch },
}))

import userMaterialsAPI from '@/api/userMaterials'

describe('user materials API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renames a material through the user-scoped route', async () => {
    patch.mockResolvedValue({ data: { id: 42, file_name: 'renamed.png' } })

    await userMaterialsAPI.rename(42, 'renamed.png')

    expect(patch).toHaveBeenCalledWith('/user/materials/42', { file_name: 'renamed.png' })
  })
})
