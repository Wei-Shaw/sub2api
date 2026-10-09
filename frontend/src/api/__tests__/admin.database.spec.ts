import { beforeEach, describe, expect, it, vi } from 'vitest'
import { startDatabaseMaintenance } from '../admin/database'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

beforeEach(() => vi.clearAllMocks())
describe('database maintenance API', () => {
  it('sends the operation idempotency key so verification retries cannot duplicate a job', async () => {
    const request = { operation: 'cleanup' as const, tables: ['usage_logs'], retention_days: 30, confirm: true }
    const job = { id: 'job-1', status: 'running' }
    post.mockResolvedValue({ data: job })
    expect(await startDatabaseMaintenance(request, 'maintenance-key')).toEqual(job)
    expect(post).toHaveBeenCalledWith('/admin/system/database/maintenance', request, {
      headers: { 'Idempotency-Key': 'maintenance-key' }
    })
  })
})
