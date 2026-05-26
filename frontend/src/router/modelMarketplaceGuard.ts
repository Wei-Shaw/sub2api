export interface ModelMarketplacePublicSettingsLike {
  model_marketplace_public_enabled?: boolean
}

export interface ModelMarketplacePublicSettingsStoreLike {
  cachedPublicSettings?: ModelMarketplacePublicSettingsLike | null
  fetchPublicSettings: (force?: boolean) => Promise<unknown>
}

export function isPublicModelMarketplaceRoute(path: string): boolean {
  return path === '/models'
}

export function isModelMarketplacePublicEnabled(
  settings?: ModelMarketplacePublicSettingsLike | null,
): boolean {
  return settings?.model_marketplace_public_enabled === true
}

export async function ensureModelMarketplacePublicSettingLoaded(
  path: string,
  appStore: ModelMarketplacePublicSettingsStoreLike,
): Promise<void> {
  if (!isPublicModelMarketplaceRoute(path)) return
  if (typeof appStore.cachedPublicSettings?.model_marketplace_public_enabled === 'boolean') return
  await appStore.fetchPublicSettings(true)
}
