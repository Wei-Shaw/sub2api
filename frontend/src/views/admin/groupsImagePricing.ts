export const imagePricingPlatforms = new Set([
  "antigravity",
  "gemini",
  "grok",
  "openai",
]);

export const supportsImagePricingPlatform = (platform: string): boolean =>
  imagePricingPlatforms.has(platform);

export const imagePricingI18nKey = (platform: string, key: string): string =>
  platform === "grok"
    ? `admin.groups.mediaPricing.${keyREDACTED`
    : `admin.groups.imagePricing.${keyREDACTED`;
