export default {
  title: 'Upstream quota',
  description: 'Read-only quota snapshots for accounts shared by the administrator.',
  refresh: 'Refresh', loading: 'Loading…', disabledPage: 'Upstream quota sharing is disabled.',
  modelNote: 'Antigravity shows model quota snapshots. Identical quota/reset values within a model family are grouped; different values are numbered separately. These snapshots do not establish separate 5-hour or weekly windows.',
  platforms: 'Platforms', cacheNote: 'Quota snapshots are shared and cached for 3 minutes. Refreshing does not force an upstream probe. No automatic polling.',
  loadError: 'Unable to load quota information. Please try again later.', empty: 'No accounts shared for this platform.',
  unknown: 'Quota data unavailable', unavailable: 'Quota unavailable', concurrency: 'Current requests',
  remaining: 'Remaining', resets: 'resets', sampleTime: 'Quota sampled at',
  status: { available: 'Available', rate_limited: 'Rate limited', disabled: 'Disabled', error: 'Unavailable' },
  windows: {
    five_hour: '5 hours', seven_day: '7 days',
    gemini_shared_minute: 'Shared / minute', gemini_pro_minute: 'Pro / minute', gemini_flash_minute: 'Flash / minute',
    gemini_shared_daily: 'Shared / day', gemini_pro_daily: 'Pro / day', gemini_flash_daily: 'Flash / day',
    gemini_model: 'Gemini model quota', claude_model: 'Claude model quota', gpt_model: 'GPT model quota'
  },
  admin: {
    title: 'User-facing upstream quota', description: 'Disabled by default. Selected accounts are visible to all signed-in users, even outside their assigned groups. Only anonymous aliases, quotas, reset times and concurrency are shared.',
    enable: 'Enable quota sharing', accounts: 'Account IDs to share', hint: 'Enter comma-separated account IDs from the account management page (maximum 200). Account names and credentials are never shared.',
    save: 'Save quota sharing', saved: 'Saved. Reload the user page to update navigation.', error: 'Unable to load or save quota sharing settings.', invalid: 'Enter up to 200 positive account IDs.'
  }
}
