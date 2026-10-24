export function supportsGroupOpenAIFast(platform: string): boolean {
  return platform === "openai" || platform === "composite";
}

export function normalizeGroupOpenAIFast(
  platform: string,
  enabled: boolean,
): boolean {
  return supportsGroupOpenAIFast(platform) && enabled;
}

export function supportsGroupOpenAIFastDowngrade(platform: string): boolean {
  return platform === "composite";
}

export function normalizeGroupOpenAIFastDowngrade(
  platform: string,
  enabled: boolean,
  forceFast: boolean,
): boolean {
  return supportsGroupOpenAIFastDowngrade(platform) && !forceFast && enabled;
}
