/**
 * Opencode model catalog definitions for each platform.
 *
 * Separated from platformCliConfigs.ts to keep file sizes manageable.
 * Each export is a Record<modelId, OpencodeModelDef>.
 */
import type { OpencodeModelDef } from './platformCliConfigs'

export const openaiModels: Record<string, OpencodeModelDef> = {
  'gpt-5.2': { name: 'GPT-5.2', limit: { context: 400000, output: 128000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {}, xhigh: {} } },
  'gpt-5.5': { name: 'GPT-5.5', limit: { context: 1050000, output: 128000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {}, xhigh: {} } },
  'gpt-5.4': { name: 'GPT-5.4', limit: { context: 1050000, output: 128000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {}, xhigh: {} } },
  'gpt-5.4-mini': { name: 'GPT-5.4 Mini', limit: { context: 400000, output: 128000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {}, xhigh: {} } },
  'gpt-5.3-codex-spark': { name: 'GPT-5.3 Codex Spark', limit: { context: 128000, output: 32000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {}, xhigh: {} } },
  'gpt-5.3-codex': { name: 'GPT-5.3 Codex', limit: { context: 400000, output: 128000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {}, xhigh: {} } },
  'codex-mini-latest': { name: 'Codex Mini', limit: { context: 200000, output: 100000 }, options: { store: false }, variants: { low: {}, medium: {}, high: {} } }
}

export const geminiModels: Record<string, OpencodeModelDef> = {
  'gemini-2.0-flash': { name: 'Gemini 2.0 Flash', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] } },
  'gemini-2.5-flash': { name: 'Gemini 2.5 Flash', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] } },
  'gemini-2.5-pro': { name: 'Gemini 2.5 Pro', limit: { context: 2097152, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-3-flash-preview': { name: 'Gemini 3 Flash Preview', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] } },
  'gemini-3-pro-preview': { name: 'Gemini 3 Pro Preview', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-3.1-pro-preview': { name: 'Gemini 3.1 Pro Preview', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } }
}

export const claudeModels: Record<string, OpencodeModelDef> = {
  'claude-opus-4-6-thinking': { name: 'Claude 4.6 Opus (Thinking)', limit: { context: 200000, output: 128000 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'claude-sonnet-4-6': { name: 'Claude 4.6 Sonnet', limit: { context: 200000, output: 64000 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } }
}

export const antigravityGeminiModels: Record<string, OpencodeModelDef> = {
  'gemini-2.5-flash': { name: 'Gemini 2.5 Flash', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'disable' } } },
  'gemini-2.5-flash-lite': { name: 'Gemini 2.5 Flash Lite', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-2.5-flash-thinking': { name: 'Gemini 2.5 Flash (Thinking)', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-3-flash': { name: 'Gemini 3 Flash', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-3.1-pro-low': { name: 'Gemini 3.1 Pro Low', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-3.1-pro-high': { name: 'Gemini 3.1 Pro High', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-2.5-flash-image': { name: 'Gemini 2.5 Flash Image', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image'], output: ['image'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } },
  'gemini-3.1-flash-image': { name: 'Gemini 3.1 Flash Image', limit: { context: 1048576, output: 65536 }, modalities: { input: ['text', 'image'], output: ['image'] }, options: { thinking: { budgetTokens: 24576, type: 'enabled' } } }
}

export const openaiAgent = {
  build: { options: { store: false } },
  plan: { options: { store: false } }
}
