export default {
  groupsStatus: {
    title: 'Group Status',
    description: 'Live account availability and billing rates for every public group',
    overview: '{groups} public groups, {available} currently available',
    lastUpdated: 'Updated at {time}',
    refresh: 'Refresh status',
    retry: 'Try again',
    loading: 'Loading group status',
    loadFailed: 'Group status could not be loaded. Please try again shortly.',
    empty: 'There are no public groups',
    noResults: 'No groups match the current filters',
    filters: {
      title: 'Filter groups',
      resultSummary: 'Showing {shown} of {total} groups',
      searchLabel: 'Search',
      searchPlaceholder: 'Search group names or descriptions',
      clearSearch: 'Clear search',
      channelLabel: 'Channel',
      statusLabel: 'Status',
      allChannels: 'All channels',
      allStatuses: 'All statuses',
      reset: 'Reset filters'
    },
    summary: {
      accounts: 'Total accounts',
      available: 'Available accounts',
      rateLimited: 'Rate-limited accounts',
      availabilityRate: 'Overall availability'
    },
    table: {
      group: 'Group',
      channel: 'Channel',
      rate: 'Rate',
      accounts: 'Total accounts',
      available: 'Available',
      rateLimited: 'Rate limited',
      status: 'Availability'
    },
    status: {
      available: 'Available',
      degraded: 'Partially limited',
      rate_limited: 'Rate limited',
      unavailable: 'Unavailable'
    },
    statusHint: {
      available: 'This group has schedulable accounts',
      degraded: 'The group is available, but some accounts are rate limited',
      rate_limited: 'No account is currently available because accounts are temporarily limited',
      unavailable: 'The group is disabled or has no schedulable accounts'
    },
    accountBreakdown: '{available} available · {limited} limited · {other} otherwise unavailable'
  }
}
