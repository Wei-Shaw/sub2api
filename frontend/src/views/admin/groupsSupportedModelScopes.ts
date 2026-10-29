export const normalizeSupportedModelScopesForPlatform = (
  platform: string,
  scopes: string[] | undefined,
): string[] => {
  if (platform !== "antigravity") return [];
  return scopes ?? [];
};
