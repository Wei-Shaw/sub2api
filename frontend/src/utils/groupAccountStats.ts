type GroupAccountCounts = {
  available_account_count?: number | null
  active_account_count?: number | null
  rate_limited_account_count?: number | null
}

export function getAvailableGroupAccountCount(group?: GroupAccountCounts | null): number {
  if (group?.available_account_count != null) {
    return Math.max(0, group.available_account_count)
  }
  return Math.max(0, (group?.active_account_count ?? 0) - (group?.rate_limited_account_count ?? 0))
}
