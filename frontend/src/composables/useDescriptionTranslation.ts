/**
 * useDescriptionTranslation：字段说明中英互译的 provide/inject 上下文 + 调用逻辑。
 *
 * 使用场景：管理员在"模型简介编辑"面板中维护每个参数字段的中英双文描述。
 * 由于 ParamSchemaEditor 是递归组件，可能嵌套多层（object.children、array.items），
 * 用 props 逐层透传"翻译需要的 API Key / 目标模型"会非常繁琐；因此这里改用
 * Vue provide/inject，让页面顶层一次性 provide，任意深度的 ParamSchemaEditor
 * 都能 inject 到同一份上下文（selectedKey / model / 翻译函数）。
 *
 * 关键设计：
 *   1) 上下文放的是 ref/computed，而不是快照值——用户在底部改选另一把 key，
 *      每个递归节点里的翻译按钮下一次点击就直接用新的 key，无需 re-mount。
 *   2) translate() 直接走同源 `/v1/chat/completions`，用户选的 key 作为 Bearer；
 *      整个网关本身就是一个 OpenAI-compatible 服务，无需再加代理层。
 *   3) 只做"文本 → 文本"的单轮无状态翻译，system prompt 严格限制"只输出译文本身"，
 *      避免大模型输出解释性废话污染字段内容。
 */

import { computed, inject, provide, type ComputedRef, type InjectionKey, type Ref } from 'vue'

/** 语言标签。当前仅支持中英互译，后续需要扩展也在这一处加就行。 */
export type TranslationLang = 'zh' | 'en'

/**
 * 上下文数据形态：暴露"当前用户选中的翻译凭据（key / model）"和"翻译能力可用性"。
 * 用 ref/computed 而非普通对象，是因为顶层选项改变时递归组件要立刻感知。
 */
export interface TranslationContext {
  /** 当前选中的 API Key 明文值；空串表示未选择。 */
  apiKey: Ref<string>
  /** 当前用于翻译的模型名（如 "gpt-4o-mini"）；空串表示未选择。 */
  model: Ref<string>
  /** 便捷判断：apiKey 和 model 都已填时才 true；组件端用它判断按钮是否可用。 */
  ready: ComputedRef<boolean>
  /**
   * translate：核心翻译方法。
   * - sourceText：源文本（即当前另一语言字段里的原文）
   * - sourceLang / targetLang：源语言 / 目标语言
   * - 返回目标语言译文，去除首尾空白。抛错时向调用方冒泡（组件端负责 UI 提示）。
   */
  translate: (sourceText: string, sourceLang: TranslationLang, targetLang: TranslationLang) => Promise<string>
}

/** Vue provide/inject 的类型安全 key。默认值为 null，组件端要判 null。 */
export const TRANSLATION_CONTEXT_KEY: InjectionKey<TranslationContext | null> = Symbol('TranslationContext')

/**
 * provideDescriptionTranslation：在页面顶层调用，把选中的 apiKey / model 注入到子树。
 * @param apiKey 顶层 ref，绑定到 API Key 下拉的 selectedKey.value.key
 * @param model  顶层 ref，绑定到"目标模型"输入框
 * @returns 同一份 TranslationContext；同一个 setup 内也可以复用它，
 *          用于"页面顶层字段"也想加翻译按钮的场景（Vue 的 inject 不能拿
 *          到当前组件自己 provide 的值，因此把 ctx 直接返回给调用方）。
 */
export function provideDescriptionTranslation(apiKey: Ref<string>, model: Ref<string>): TranslationContext {
  const ready = computed(() => apiKey.value.trim().length > 0 && model.value.trim().length > 0)
  const ctx: TranslationContext = {
    apiKey,
    model,
    ready,
    translate: (src, srcLang, tgtLang) => translateViaChatCompletions(src, srcLang, tgtLang, apiKey.value, model.value),
  }
  provide(TRANSLATION_CONTEXT_KEY, ctx)
  return ctx
}

/**
 * useDescriptionTranslation：在 ParamSchemaEditor 等子组件里调用，取回上下文。
 * 返回 null 表示"页面未提供翻译能力"，此时组件端应隐藏翻译按钮。
 */
export function useDescriptionTranslation(): TranslationContext | null {
  return inject(TRANSLATION_CONTEXT_KEY, null)
}

// ============ 实际调用实现 ============

/**
 * translateViaChatCompletions：走 OpenAI-compatible `/v1/chat/completions` 做单轮翻译。
 *
 * 网关地址：走同源相对路径 `/v1/chat/completions`（本项目 gateway 挂在 `/v1`）。
 * 认证：Bearer <apiKey>。
 *
 * Prompt 策略：
 *   - system：严格限定"只输出译文本身，不要解释、不要引号包裹、不要 markdown"
 *   - user：把原文粘进去，前面明确 source / target 语言
 *   非流式请求（stream=false），一次性拿完整 content。
 *
 * 错误处理：
 *   - HTTP 非 2xx / 网络错误 / 响应结构异常 / 空译文 都会抛 Error；
 *     错误 message 会尽量携带上游返回的 body 前 200 字符，便于用户定位是
 *     "key 无效 / model 无权 / 余额不足"哪种问题。
 */
async function translateViaChatCompletions(
  sourceText: string,
  sourceLang: TranslationLang,
  targetLang: TranslationLang,
  apiKey: string,
  model: string
): Promise<string> {
  const trimmedSrc = (sourceText ?? '').trim()
  if (!trimmedSrc) throw new Error('source text is empty')
  if (!apiKey.trim()) throw new Error('api key is empty')
  if (!model.trim()) throw new Error('model is empty')

  const langLabel: Record<TranslationLang, string> = { zh: 'Simplified Chinese', en: 'English' }

  const systemPrompt =
    'You are a professional technical translator specializing in API/SDK parameter documentation. ' +
    'Translate the user-provided text strictly and faithfully. ' +
    'Rules: (1) Return ONLY the translated text with no explanations, no quotes, no code fences, no leading/trailing labels. ' +
    '(2) Preserve inline code, placeholders like {var}, punctuation and structure. ' +
    '(3) Keep the tone concise and technical, suitable for a form field description.'
  const userPrompt = `Translate from ${langLabel[sourceLang]} to ${langLabel[targetLang]}. Only output the translation:\n\n${trimmedSrc}`

  const resp = await fetch('/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiKey}`,
    },
    body: JSON.stringify({
      model,
      messages: [
        { role: 'system', content: systemPrompt },
        { role: 'user', content: userPrompt },
      ],
      stream: false,
      temperature: 0.2,
    }),
  })

  if (!resp.ok) {
    const bodyText = await safeReadBody(resp)
    throw new Error(`HTTP ${resp.status}${bodyText ? ': ' + bodyText : ''}`)
  }

  const data = (await resp.json().catch(() => null)) as
    | { choices?: Array<{ message?: { content?: string } }> }
    | null
  const content = data?.choices?.[0]?.message?.content
  if (typeof content !== 'string' || !content.trim()) {
    throw new Error('empty translation from model')
  }
  return content.trim()
}

/** safeReadBody：尽力把上游 error body 读出来（限制 200 字符），失败返回空串。 */
async function safeReadBody(resp: Response): Promise<string> {
  try {
    const text = await resp.text()
    return text.slice(0, 200)
  } catch {
    return ''
  }
}

/**
 * fetchModelsForKey：走同源 `/v1/models` 拉取"当前 API Key 可用模型 ID 列表"。
 *
 * 使用场景：管理员在"模型介绍编辑"底部工具区选中一把 API Key 后，希望把
 *   "目标翻译模型"从手写输入切换成"根据当前 key 权限列出的下拉选择"。
 *   由于每把 key 归属的 group 可能配了不同的 model_mapping / custom_models_list，
 *   所以必须用该 key 作为 Bearer 请求 gateway 的 /v1/models，得到的才是
 *   "这把 key 真正能调用的模型清单"。
 *
 * 响应结构：与 OpenAI 兼容 —— `{ object: 'list', data: [{ id: string, ... }] }`。
 * 我们只保留每个 item 的 `id` 字段（去空、去重、稳定排序），列表直接喂给 Select。
 *
 * 错误处理：
 *   - key 为空：直接返回空数组（调用方通常在 key 未选时也不会调用这里）；
 *   - HTTP 非 2xx / 网络异常 / 响应结构异常：抛 Error 由调用方降级。
 */
export async function fetchModelsForKey(apiKey: string): Promise<string[]> {
  const trimmed = (apiKey ?? '').trim()
  if (!trimmed) return []

  const resp = await fetch('/v1/models', {
    method: 'GET',
    headers: { Authorization: `Bearer ${trimmed}` },
  })
  if (!resp.ok) {
    const bodyText = await safeReadBody(resp)
    throw new Error(`HTTP ${resp.status}${bodyText ? ': ' + bodyText : ''}`)
  }

  const data = (await resp.json().catch(() => null)) as
    | { data?: Array<{ id?: unknown }> }
    | null
  const list = Array.isArray(data?.data) ? data!.data : []
  const ids: string[] = []
  const seen = new Set<string>()
  for (const item of list) {
    const id = typeof item?.id === 'string' ? item.id.trim() : ''
    if (!id || seen.has(id)) continue
    seen.add(id)
    ids.push(id)
  }
  // 稳定字典序：让下拉呈现固定顺序，方便管理员用键盘检索/记忆。
  ids.sort((a, b) => a.localeCompare(b))
  return ids
}
