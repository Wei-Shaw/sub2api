/**
 * Resolve the customer-facing model vendor independently from the gateway protocol.
 * Many Chinese vendors use an OpenAI-compatible channel, so `platform=openai` alone
 * is not enough to choose the correct logo or provider filter.
 */
export function inferModelProvider(modelName: string, fallbackPlatform: string): string {
  const name = modelName.trim().toLowerCase()

  if (/^(claude|anthropic)[-_/]/.test(name)) return 'anthropic'
  if (/^(gemini|gemma)[-_/]/.test(name)) return 'gemini'
  if (/^(grok|xai)[-_/]/.test(name)) return 'grok'
  if (/^(deepseek)[-_/]/.test(name)) return 'deepseek'
  if (/^(kimi|moonshot)[-_/]/.test(name)) return 'kimi'
  if (/^(glm|cogview|cogvideo)[-_/]/.test(name)) return 'zhipu'
  if (/^(minimax)[-_/]/.test(name)) return 'minimax'
  if (/^(qwen(?:[-_/]|\d)|qwq[-_/])/.test(name)) return 'qwen'
  if (/^(mimo)[-_/]/.test(name)) return 'mimo'
  if (/^(hunyuan|hy\d|hy-)[-_/]?/.test(name)) return 'hunyuan'
  if (/^(gpt|chatgpt|o[1345]|text-embedding|dall-e|sora)[-_/]/.test(name)) return 'openai'
  if (/^auto(?:-|$)/.test(name)) return 'auto'

  return fallbackPlatform || 'auto'
}
