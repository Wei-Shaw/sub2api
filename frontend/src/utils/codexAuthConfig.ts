/**
 * Codex 0.164 checks a static actor header when registering its native image
 * extension for a custom API-key provider. The marker is not a credential;
 * requests authenticate with the provider's normal bearer token.
 */
export function buildCodexApiKeyAuthConfig(apiKey: string): string {
  return `requires_openai_auth = false
experimental_bearer_token = ${JSON.stringify(apiKey)}
http_headers = { "x-openai-actor-authorization" = "local-image-extension" }`
}
