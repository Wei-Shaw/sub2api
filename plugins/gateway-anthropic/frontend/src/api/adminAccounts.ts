/**
 * Thin wrapper around admin accounts API for model routing.
 * Uses the plugin's injected axios client (host API interceptors included).
 */
import { getClient } from './client'

export interface SimpleAccount {
  id: number
  name: string
}

interface PaginatedAccountsResponse {
  items: SimpleAccount[]
  total: number
}

export async function searchAccounts(
  keyword: string,
  platform: string,
  options?: { signal?: AbortSignal },
): Promise<SimpleAccount[]> {
  const client = getClient()
  const { data } = await client.get<PaginatedAccountsResponse>(
    '/admin/accounts',
    {
      params: { page: 1, page_size: 20, search: keyword, platform },
      signal: options?.signal,
    },
  )
  return data.items.map((a) => ({ id: a.id, name: a.name }))
}

export async function getAccountById(id: number): Promise<SimpleAccount> {
  const client = getClient()
  const { data } = await client.get<SimpleAccount>(`/admin/accounts/${id}`)
  return { id: data.id, name: data.name }
}
