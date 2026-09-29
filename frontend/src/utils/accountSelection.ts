interface AccountIDRow {
  id: number
}

interface AccountListPage<T extends AccountIDRow = AccountIDRow> {
  items: T[]
  total: number
  pages?: number
}

type AccountPageFetcher<T extends AccountIDRow = AccountIDRow> = (
  page: number,
  pageSize: number,
  filters: Record<string, unknown>
) => Promise<AccountListPage<T>>

const SELECT_ALL_PAGE_SIZE = 1000

// 拉取筛选条件下的全部账号行（按 id 去重），结果不完整时抛错
export async function fetchAllAccounts<T extends AccountIDRow>(
  fetchPage: AccountPageFetcher<T>,
  filters: Record<string, unknown>
): Promise<T[]> {
  const requestFilters = {
    ...filters,
    lite: '1',
    include_scheduler_score: '0'
  }
  const firstPage = await fetchPage(1, SELECT_ALL_PAGE_SIZE, requestFilters)
  const pageCount = Math.max(
    firstPage.pages ?? 0,
    Math.ceil(firstPage.total / SELECT_ALL_PAGE_SIZE)
  )
  const rows = [...firstPage.items]

  for (let page = 2; page <= pageCount; page++) {
    const result = await fetchPage(page, SELECT_ALL_PAGE_SIZE, requestFilters)
    rows.push(...result.items)
  }

  const uniqueRows = Array.from(new Map(rows.map(account => [account.id, account])).values())
  if (uniqueRows.length !== firstPage.total) {
    throw new Error('账号列表结果不完整')
  }
  return uniqueRows
}

export async function fetchAllAccountIds(
  fetchPage: AccountPageFetcher,
  filters: Record<string, unknown>
): Promise<number[]> {
  const rows = await fetchAllAccounts(fetchPage, filters)
  return rows.map(account => account.id)
}
