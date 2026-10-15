/**
 * 提示词模板：把常用的提示词存下来，下次一键套用。
 *
 * 和密钥一样只存在用户自己的浏览器里（localStorage），不上传服务器——
 * 平台不需要知道用户写了什么提示词，也不该为此多存一份数据。
 */

export interface PromptTemplate {
  id: string
  name: string
  prompt: string
  createdAt: number
  updatedAt: number
}

export const TEMPLATE_STORAGE_KEY = 'sub2api_ai_image_templates'

/** 最多存这么多条，避免 localStorage 被写爆。 */
export const MAX_TEMPLATES = 50

/** 模板名字太长会把下拉框撑坏，截断。 */
export const MAX_TEMPLATE_NAME = 40

function newId(): string {
  return `tpl_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
}

/**
 * 从 localStorage 读模板。
 * 读到坏数据（手改过、旧版本残留）就当作没有：绝不能因为几条模板把整页搞崩。
 */
export function loadTemplates(): PromptTemplate[] {
  try {
    const raw = window.localStorage.getItem(TEMPLATE_STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []

    return parsed
      .filter(
        (item): item is Record<string, unknown> =>
          Boolean(item) && typeof item === 'object' && typeof (item as PromptTemplate).name === 'string',
      )
      .filter((item) => typeof item.prompt === 'string' && (item.prompt as string).trim() !== '')
      .map((item) => {
        const now = Date.now()
        return {
          id: typeof item.id === 'string' && item.id ? item.id : newId(),
          name: (item.name as string).slice(0, MAX_TEMPLATE_NAME),
          prompt: item.prompt as string,
          createdAt: typeof item.createdAt === 'number' ? item.createdAt : now,
          updatedAt: typeof item.updatedAt === 'number' ? item.updatedAt : now,
        }
      })
      .slice(0, MAX_TEMPLATES)
  } catch {
    return []
  }
}

/**
 * 存一条模板。**同名视为覆盖**——
 * 用户想改已有模板的内容时，不该被迫先删掉再重建。
 * 最新的排在前面，用户刚存的马上能在下拉框顶部看到。
 */
export function upsertTemplate(
  templates: PromptTemplate[],
  name: string,
  prompt: string,
): PromptTemplate[] {
  const cleanName = name.trim().slice(0, MAX_TEMPLATE_NAME)
  const cleanPrompt = prompt.trim()
  if (!cleanName || !cleanPrompt) return templates

  const now = Date.now()
  const exists = templates.some((item) => item.name === cleanName)
  if (exists) {
    return templates.map((item) =>
      item.name === cleanName ? { ...item, prompt: cleanPrompt, updatedAt: now } : item,
    )
  }

  return [
    { id: newId(), name: cleanName, prompt: cleanPrompt, createdAt: now, updatedAt: now },
    ...templates,
  ].slice(0, MAX_TEMPLATES)
}

export function removeTemplate(templates: PromptTemplate[], id: string): PromptTemplate[] {
  return templates.filter((item) => item.id !== id)
}

/** 按名字找模板，用于下拉框选中后套用。 */
export function findTemplate(templates: PromptTemplate[], id: string): PromptTemplate | undefined {
  return templates.find((item) => item.id === id)
}

/**
 * 写回 localStorage。
 * 配额满或隐私模式下会失败，此时**不抛异常**，只返回 false 让界面提示一句，
 * 因为模板存不上不该妨碍用户正常生图。
 */
export function persistTemplates(templates: PromptTemplate[]): boolean {
  try {
    window.localStorage.setItem(TEMPLATE_STORAGE_KEY, JSON.stringify(templates))
    return true
  } catch {
    return false
  }
}
