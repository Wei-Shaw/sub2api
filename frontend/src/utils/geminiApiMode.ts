export const GEMINI_AI_STUDIO_BASE_URL = 'https://generativelanguage.googleapis.com'
export const GEMINI_VERTEX_EXPRESS_BASE_URL = 'https://aiplatform.googleapis.com'

export type GeminiAPIMode = 'ai_studio' | 'vertex'

export const normalizeGeminiAPIMode = (value: unknown): GeminiAPIMode =>
  typeof value === 'string' && value.trim().toLowerCase() === 'vertex'
    ? 'vertex'
    : 'ai_studio'

export const geminiAPIKeyBaseURL = (mode: GeminiAPIMode): string =>
  mode === 'vertex' ? GEMINI_VERTEX_EXPRESS_BASE_URL : GEMINI_AI_STUDIO_BASE_URL
