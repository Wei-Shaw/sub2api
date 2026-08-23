<template>
  <!-- 全屏遮罩弹窗 -->
  <div
    v-if="open"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
    @click.self="tryClose"
  >
    <div
      class="max-h-[92vh] w-full max-w-4xl overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900"
    >
      <!-- 头 -->
      <header
        class="flex items-start justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800"
      >
        <div class="min-w-0">
          <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ t('videoModels.playground.title') }}
          </h2>
          <p class="mt-1 truncate font-mono text-xs text-gray-500 dark:text-gray-400">
            {{ displaySlug(slug) }}
          </p>
        </div>
        <button
          @click="tryClose"
          class="ml-4 shrink-0 rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
          :aria-label="t('common.close', 'Close')"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path d="M6.28 5.22a.75.75 0 0 0-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 1 0 1.06 1.06L10 11.06l3.72 3.72a.75.75 0 1 0 1.06-1.06L11.06 10l3.72-3.72a.75.75 0 0 0-1.06-1.06L10 8.94 6.28 5.22Z" />
          </svg>
        </button>
      </header>

      <!-- 主体 -->
      <div class="max-h-[calc(92vh-8rem)] overflow-y-auto px-6 py-5">
        <!-- Key 选择区 -->
        <section class="mb-5">
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('videoModels.playground.apiKeyLabel') }}
            <span class="text-red-500">*</span>
          </label>
          <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('videoModels.playground.apiKeyHelper') }}
          </p>
          <div v-if="keysLoading" class="text-sm text-gray-500">
            {{ t('common.loading', 'Loading...') }}
          </div>
          <div v-else-if="!compositeKeys.length" class="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950/40 dark:text-yellow-200">
            {{ t('videoModels.playground.noCompositeKey') }}
          </div>
          <select
            v-else
            v-model="selectedKeyId"
            :disabled="playground.isBusy.value"
            class="w-full rounded border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:disabled:bg-gray-800/60"
          >
            <option value="">{{ t('videoModels.playground.selectKey') }}</option>
            <option v-for="k in compositeKeys" :key="k.id" :value="k.id">
              {{ k.name }} · {{ k.group?.name || '-' }} · {{ maskKey(k.key) }}
            </option>
          </select>
        </section>

        <!-- 参数模式切换：
             - 只有当管理员为该模型配置了"字段声明"（fieldSpecs 非空）时，才提供 form 模式。
             - 否则只展示 JSON 模式，避免误导用户以为有表单可填。 -->
        <div class="mb-4 flex items-center gap-3 border-b border-gray-200 pb-2 dark:border-gray-800">
          <button
            v-if="fieldSpecs.length > 0"
            @click="mode = 'form'"
            :class="tabClass(mode === 'form')"
          >
            {{ t('videoModels.playground.tabForm') }}
          </button>
          <button
            @click="switchToJsonMode"
            :class="tabClass(mode === 'json')"
          >
            {{ t('videoModels.playground.tabJson') }}
          </button>
        </div>

        <!-- 表单模式（仅当有字段声明时可见）：递归渲染 FieldSpec 树（支持 object / array 嵌套） -->
        <section v-if="mode === 'form' && fieldSpecs.length > 0" class="mb-5 space-y-3">
          <VideoPlaygroundSchemaField
            v-for="f in fieldSpecs"
            :key="f.key"
            :spec="f"
            :model-value="formData[f.key]"
            :disabled="playground.isBusy.value"
            :media-references="promptMediaReferences"
            @update:model-value="onFieldValueChange(f.key, $event)"
          />
        </section>

        <!-- JSON 模式（无字段声明时的唯一模式） -->
        <section v-else class="mb-5">
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('videoModels.playground.jsonLabel') }}
          </label>
          <p
            v-if="fieldSpecs.length === 0"
            class="mb-1 text-xs text-gray-500 dark:text-gray-400"
          >
            {{ t('videoModels.playground.jsonOnlyHint') }}
          </p>
          <textarea
            v-model="jsonInput"
            :disabled="playground.isBusy.value"
            rows="12"
            spellcheck="false"
            class="w-full rounded border border-gray-300 bg-gray-50 px-3 py-2 font-mono text-xs focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
          />
          <p v-if="jsonError" class="mt-1 text-xs text-red-600 dark:text-red-400">
            {{ jsonError }}
          </p>
        </section>

        <!-- curl 示例（可折叠） -->
        <details class="mb-5 rounded-lg border border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-950/50">
          <summary class="flex cursor-pointer items-center justify-between gap-2 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-900">
            <span>{{ t('videoModels.playground.curlLabel') }}</span>
            <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.curlHint') }}
            </span>
          </summary>
          <div class="border-t border-gray-200 p-3 dark:border-gray-800">
            <div class="mb-2 flex items-center justify-end">
              <button
                type="button"
                class="rounded border border-gray-300 bg-white px-2 py-1 text-xs text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700"
                @click="copyCurl"
              >
                {{ t('videoModels.playground.curlCopy') }}
              </button>
            </div>
            <pre class="max-h-72 overflow-auto rounded-md bg-gray-900 p-3 text-xs leading-relaxed text-gray-100 dark:bg-black"><code>{{ curlSnippet }}</code></pre>
          </div>
        </details>

        <!-- 运行状态区 -->
        <section
          v-if="playground.phase.value !== 'idle'"
          class="mb-4 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-800 dark:bg-gray-950/50"
        >
          <div class="mb-2 flex items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'inline-block h-2.5 w-2.5 rounded-full',
                  phaseIndicator.color,
                  playground.isBusy.value ? 'animate-pulse' : ''
                ]"
              />
              <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
                {{ phaseIndicator.label }}
              </span>
            </div>
            <span v-if="playground.displayElapsed.value > 0" class="text-xs text-gray-500 dark:text-gray-400">
              {{ playground.displayElapsed.value }}s
            </span>
          </div>

          <div v-if="playground.requestId.value" class="mb-2 truncate font-mono text-xs text-gray-500 dark:text-gray-400">
            request_id: {{ playground.requestId.value }}
          </div>

          <div v-if="queuePosition !== null" class="text-xs text-gray-600 dark:text-gray-400">
            {{ t('videoModels.playground.queuePosition', { pos: queuePosition }) }}
          </div>

          <div
            v-if="playground.errorMessage.value"
            class="mt-2 rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-950/40 dark:text-red-300"
          >
            {{ playground.errorMessage.value }}
          </div>

          <!-- 结果展示：按 output_fields 声明遍历，逐字段按 effectiveType 渲染。
               - isPrimary=true 的字段以大尺寸展示（由 result_field 或默认 video/image 自动推导）；
               - video / image / url 需要 URL；text / number 渲染文本；json 展示预格式化块。 -->
          <div v-if="playground.phase.value === 'completed' && resolvedOutputs.length" class="mt-3 space-y-4">
            <div
              v-for="(item, idx) in resolvedOutputs"
              :key="idx"
              class="space-y-1"
            >
              <div class="flex items-baseline justify-between gap-2">
                <span class="text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ item.spec.label || item.spec.key }}
                </span>
                <span class="font-mono text-[10px] text-gray-400">
                  {{ item.effectiveType }}<span v-if="item.isPrimary"> · primary</span>
                </span>
              </div>

              <!-- 每个字段可能对应多个叶子（如 images[*]），按 effectiveType 分别渲染 -->
              <div class="space-y-2">
                <template v-for="(leaf, j) in item.values" :key="j">
                  <!-- video -->
                  <div v-if="item.effectiveType === 'video' && leafToUrl(leaf)" class="space-y-1">
                    <video
                      :src="leafToUrl(leaf)"
                      controls
                      preload="metadata"
                      :class="[
                        'mx-auto block h-auto max-w-full rounded border border-gray-200 bg-black dark:border-gray-700',
                        item.isPrimary ? 'max-h-[520px]' : 'max-h-[240px]'
                      ]"
                    />
                    <a
                      :href="leafToUrl(leaf)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="inline-block break-all text-xs text-blue-600 hover:underline dark:text-blue-400"
                    >
                      {{ leafToUrl(leaf) }}
                    </a>
                  </div>

                  <!-- image -->
                  <div v-else-if="item.effectiveType === 'image' && leafToUrl(leaf)" class="space-y-1">
                    <img
                      :src="leafToUrl(leaf)"
                      :alt="item.spec.label || item.spec.key"
                      loading="lazy"
                      :class="[
                        'w-full rounded border border-gray-200 object-contain dark:border-gray-700',
                        item.isPrimary ? 'max-h-[520px]' : 'max-h-[240px]'
                      ]"
                    />
                    <a
                      :href="leafToUrl(leaf)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="inline-block break-all text-xs text-blue-600 hover:underline dark:text-blue-400"
                    >
                      {{ leafToUrl(leaf) }}
                    </a>
                  </div>

                  <!-- url：可点击链接 -->
                  <a
                    v-else-if="item.effectiveType === 'url' && leafToUrl(leaf)"
                    :href="leafToUrl(leaf)"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="block break-all rounded bg-gray-50 px-2 py-1 text-xs text-blue-600 hover:underline dark:bg-gray-900 dark:text-blue-400"
                  >
                    {{ leafToUrl(leaf) }}
                  </a>

                  <!-- object / array（含历史遗留的 json）：预格式化 JSON 块。
                       按 JSON Schema 标准 object 和 array 都以结构化数据展示；
                       旧数据里的 "json" 类型也走同一分支，保证渲染兼容。 -->
                  <pre
                    v-else-if="item.effectiveType === 'object' || item.effectiveType === 'array' || item.effectiveType === 'json'"
                    class="max-h-64 overflow-auto rounded bg-gray-900 p-2 text-xs text-gray-100"
                  >{{ leafToPrettyJson(leaf) }}</pre>

                  <!-- text / number（默认）：单行/多行文本 -->
                  <div
                    v-else
                    class="break-all rounded bg-gray-50 px-2 py-1 text-xs text-gray-800 dark:bg-gray-900 dark:text-gray-200"
                  >
                    {{ leafToText(leaf, item.spec.max_chars) }}
                  </div>
                </template>

                <p v-if="item.spec.description" class="text-[11px] text-gray-500 dark:text-gray-400">
                  {{ item.spec.description }}
                </p>
              </div>
            </div>
          </div>
          <div
            v-else-if="playground.phase.value === 'completed' && outputFields.length"
            class="mt-2 text-xs text-gray-600 dark:text-gray-400"
          >
            {{ t('videoModels.playground.noOutputMatched') }}
          </div>
          <div
            v-else-if="playground.phase.value === 'completed'"
            class="mt-2 text-xs text-gray-600 dark:text-gray-400"
          >
            {{ t('videoModels.playground.noOutputConfigured') }}
          </div>

          <!-- 原始 payload（可折叠） -->
          <details v-if="playground.resultPayload.value" class="mt-3">
            <summary class="cursor-pointer text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">
              {{ t('videoModels.playground.rawPayload') }}
            </summary>
            <pre class="mt-2 max-h-64 overflow-auto rounded bg-gray-900 p-2 text-xs text-gray-100">{{ prettyResult }}</pre>
          </details>
        </section>
      </div>

      <!-- 底部操作栏 -->
      <footer class="flex items-center justify-between gap-3 border-t border-gray-200 px-6 py-3 dark:border-gray-800">
        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('videoModels.playground.pollingHint') }}
        </div>
        <div class="flex items-center gap-2">
          <button
            v-if="playground.isBusy.value"
            @click="onCancel"
            class="rounded border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700"
          >
            {{ t('videoModels.playground.btnCancel') }}
          </button>
          <button
            v-if="playground.isTerminal.value"
            @click="playground.reset"
            class="rounded border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700"
          >
            {{ t('videoModels.playground.btnReset') }}
          </button>
          <button
            @click="onSubmit"
            :disabled="!canSubmit"
            class="rounded bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 disabled:hover:bg-blue-300 dark:disabled:bg-blue-900"
          >
            {{ t('videoModels.playground.btnSubmit') }}
          </button>
        </div>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ApiKey } from '@/types'
import keysAPI from '@/api/keys'
import { buildGatewayUrl } from '@/api/url'
import { useAppStore } from '@/stores/app'
import { useVideoPlayground } from '@/composables/useVideoPlayground'
import {
  extractFieldSpecs,
  buildDefaultBody,
  fieldSpecToDefaultValue,
  pickByPath,
  type FieldSpec,
} from './paramSpec'
import VideoPlaygroundSchemaField from './VideoPlaygroundSchemaField.vue'
import { collectPromptMediaReferences } from './promptMediaReferences'
import type { OutputFieldSpec } from '@/api/videoModels'

const props = defineProps<{
  open: boolean
  slug: string
  /**
   * 管理员在"模型介绍"里配置的 default_params（可能包含新格式的字段声明，也可能是老格式的 KV）。
   *   - 表单模式的字段列表由 extractFieldSpecs(defaultParams) 计算；无字段声明时不展示表单模式；
   *   - curl / 兜底 body 由 buildDefaultBody(defaultParams) 计算。
   */
  defaultParams?: Record<string, unknown> | null
  /**
   * 管理员声明的输出字段列表（新）。任务完成后演练台会逐字段从 result payload
   * 中提取并按 type 渲染。为空时（未配置）不展示专门的结果字段区，仅展示原始 payload。
   */
  outputFields?: OutputFieldSpec[] | null
  /**
   * 主结果字段：指向 outputFields 中的某个 key。非空时演练台会将该字段
   * 以大尺寸媒体展示，其余字段以小尺寸作为附属展示。
   */
  resultField?: string | null
  /**
   * 主结果媒体类型：'video' | 'image'（默认 'video'）。
   * 仅当 resultField 能匹配到 outputFields[].key 时才生效；
   * 匹配时会覆盖该字段声明的 type 来选择 <video> / <img>。
   */
  resultType?: 'video' | 'image' | null
}>()

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const playground = useVideoPlayground()

// ============ API Keys 加载 ============
const keysLoading = ref(false)
const allKeys = ref<ApiKey[]>([])
const selectedKeyId = ref<number | ''>('')

const compositeKeys = computed(() =>
  allKeys.value.filter((k) => k.group?.platform === 'composite' && k.status === 'active')
)

const selectedKey = computed<ApiKey | null>(() => {
  if (selectedKeyId.value === '') return null
  return compositeKeys.value.find((k) => k.id === selectedKeyId.value) ?? null
})

async function loadKeys() {
  keysLoading.value = true
  try {
    const resp = await keysAPI.list(1, 100)
    allKeys.value = resp.items ?? []
    if (!selectedKeyId.value && compositeKeys.value.length) {
      selectedKeyId.value = compositeKeys.value[0].id
    }
  } catch {
    allKeys.value = []
  } finally {
    keysLoading.value = false
  }
}

// ============ 参数表单 ============
// 展示用：剔离 fal-ai/ 前缀。真实 slug 仍用于请求提交。
const displaySlug = (slug: string): string =>
  slug.startsWith('fal-ai/') ? slug.slice('fal-ai/'.length) : slug

// 字段声明列表：来自 defaultParams 中的新格式条目。为空数组时演练台退化为 JSON 模式。
const fieldSpecs = computed<FieldSpec[]>(() => extractFieldSpecs(props.defaultParams))

// outputFields 声明：任务完成后逐个提取并渲染的输出字段列表。
const outputFields = computed<OutputFieldSpec[]>(() =>
  Array.isArray(props.outputFields) ? props.outputFields : []
)

// resolvedOutputs：任务完成后从 payload 中按声明提取的列表。
// values 为空时该字段不展示（避免堆一开空行）。
//
// isPrimary 与 effectiveType 的推导规则：
//   - 若 resultField 非空且匹配到某个 spec.key，则该行 isPrimary=true，
//     effectiveType 用 resultType 覆盖（仅 video / image 两种）；其余行 isPrimary=false，
//     effectiveType 保持 spec.type 不变（新 JSON Schema 五种）。
//   - 若 resultField 为空，则退化到默认：按声明顺序取第一个 object/array（新语义）
//     或 video/image（旧数据兼容）字段作为 primary。
// effectiveType 使用宽松 string：因为可能被覆写为 'video' / 'image'，也可能保留
// 旧数据里已存的 'url' / 'text' / 'json' 等，渲染层按字面量分支处理。
interface ResolvedOutput {
  spec: OutputFieldSpec
  values: unknown[]
  isPrimary: boolean
  effectiveType: string
}
const resolvedOutputs = computed<ResolvedOutput[]>(() => {
  const payload = playground.resultPayload.value
  if (!payload) return []
  const specs = outputFields.value
  const rf = (props.resultField || '').trim()
  const rt: 'video' | 'image' = props.resultType === 'image' ? 'image' : 'video'
  const matchedByResultField = rf.length > 0 && specs.some((s) => s.key === rf)

  // 若 resultField 未配置或未匹配到，预先找出默认主结果：第一个 object/array
  // 字段（新语义）；同时兼容旧数据里遗留的 video/image 类型。
  let fallbackPrimaryKey = ''
  if (!matchedByResultField) {
    for (const s of specs) {
      const st = s.type as string
      if (st === 'object' || st === 'array' || st === 'video' || st === 'image') {
        fallbackPrimaryKey = s.key
        break
      }
    }
  }

  const out: ResolvedOutput[] = []
  for (const spec of specs) {
    const values = pickByPath(payload, spec.key)
    if (values.length === 0) continue
    let isPrimary = false
    let effectiveType: string = spec.type
    if (matchedByResultField && spec.key === rf) {
      isPrimary = true
      effectiveType = rt
    } else if (!matchedByResultField && spec.key === fallbackPrimaryKey) {
      isPrimary = true
      // 默认主结果字段 effectiveType 就是 spec.type。
    }
    out.push({ spec, values, isPrimary, effectiveType })
  }
  return out
})

// 将任意叶子节点根据 output type 正常化为可渲染的字符串（多个 URL / 多个值逐个渲染）。
// - video / image / url：取叶子字符串或 leaf.url；不是字符串时返回空串。
// - text / number：取 String(leaf)；对象时 JSON 序列化。
// - json：直接返回美化 JSON。
function leafToText(v: unknown, maxChars?: number): string {
  if (v === null || v === undefined) return ''
  let text: string
  if (typeof v === 'string') text = v
  else if (typeof v === 'number' || typeof v === 'boolean') text = String(v)
  else {
    try {
      text = JSON.stringify(v)
    } catch {
      text = String(v)
    }
  }
  const max = typeof maxChars === 'number' && maxChars > 0 ? Math.trunc(maxChars) : 0
  return max > 0 ? Array.from(text).slice(0, max).join('') : text
}
function leafToUrl(v: unknown): string {
  if (typeof v === 'string' && v) return v
  if (v && typeof v === 'object') {
    const u = (v as Record<string, unknown>).url
    if (typeof u === 'string' && u) return u
  }
  return ''
}
function leafToPrettyJson(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

// formData：表单双向绑定值。现在值可以是任意 JSON（包括 object / array），
// 不再强制字符串。读写都通过 VideoPlaygroundSchemaField 的 v-model 自动处理。
const formData = reactive<Record<string, unknown>>({})
const promptMediaReferences = computed(() =>
  collectPromptMediaReferences(fieldSpecs.value, formData)
)

function initFormDefaults() {
  // 清空旧字段，避免 slug 切换后残留
  for (const k of Object.keys(formData)) delete formData[k]
  for (const f of fieldSpecs.value) {
    formData[f.key] = fieldSpecToDefaultValue(f)
  }
}

/**
 * onFieldValueChange：顶层字段变更时同步到 formData。
 * undefined 时删掉 key，避免 curl 预览/提交时包含 undefined。
 */
function onFieldValueChange(key: string, v: unknown) {
  if (v === undefined) {
    delete formData[key]
  } else {
    formData[key] = v
  }
}

// 模式（form / json）
const mode = ref<'form' | 'json'>('form')
const jsonInput = ref<string>('')
const jsonError = ref<string>('')

function switchToJsonMode() {
  if (mode.value === 'json') return
  // 若当前有表单字段，先把表单值序列化到 JSON 编辑框，避免用户丢失已填内容
  if (fieldSpecs.value.length > 0) {
    jsonInput.value = JSON.stringify(currentFormBody(), null, 2)
  } else if (!jsonInput.value) {
    // 无字段声明：把兜底 body 塞入 JSON 编辑框作为参考。
    const fallback = buildDefaultBody(props.defaultParams)
    jsonInput.value = JSON.stringify(
      Object.keys(fallback).length > 0 ? fallback : { prompt: 'A cinematic shot ...' },
      null,
      2
    )
  }
  jsonError.value = ''
  mode.value = 'json'
}

function tabClass(active: boolean): string {
  return [
    'rounded px-3 py-1.5 text-sm',
    active
      ? 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
      : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800',
  ].join(' ')
}

// currentFormBody：把 formData 按 fieldSpecs 直接取值拼成 body。
// 方案 C 下，VideoPlaygroundSchemaField 已完成递归保存，值本身已是合适类型。
// v === undefined 时（表示用户没填 & 无默认值）跳过该 key；空串同样视为未填。
function currentFormBody(): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  for (const f of fieldSpecs.value) {
    const v = formData[f.key]
    if (v === undefined) continue
    if (typeof v === 'string' && v.trim() === '') continue
    body[f.key] = v
  }
  return body
}

// ============ 提交/取消 ============
const canSubmit = computed(() => {
  if (playground.isBusy.value) return false
  if (!selectedKey.value) return false
  return true
})

function validateFormRequired(): string | null {
  // 仅校验叶子必填（object/array 的 required 当前仅作展示徽章使用）。
  for (const f of fieldSpecs.value) {
    if (!f.required) continue
    if (f.rawType === 'boolean') continue // boolean 必填无意义
    if (f.rawType === 'object' || f.rawType === 'array') continue
    const raw = formData[f.key]
    if (raw === undefined || String(raw).trim() === '') {
      return t('videoModels.playground.requiredMissing', { field: f.key })
    }
  }
  return null
}

function currentBody(): Record<string, unknown> | null {
  if (mode.value === 'json') {
    const raw = jsonInput.value.trim()
    if (!raw) {
      jsonError.value = t('videoModels.playground.jsonEmpty')
      return null
    }
    try {
      const parsed = JSON.parse(raw)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        jsonError.value = t('videoModels.playground.jsonMustBeObject')
        return null
      }
      jsonError.value = ''
      return parsed as Record<string, unknown>
    } catch (e) {
      jsonError.value = (e as Error).message
      return null
    }
  }
  // 表单模式：先做 required 校验
  const missing = validateFormRequired()
  if (missing) {
    appStore.showError(missing)
    return null
  }
  return currentFormBody()
}

async function onSubmit() {
  const body = currentBody()
  if (!body) return
  if (!selectedKey.value) return
  await playground.start(props.slug, body, selectedKey.value.key)
}

async function onCancel() {
  if (!selectedKey.value) {
    playground.reset()
    return
  }
  await playground.cancel(props.slug, selectedKey.value.key)
}

// ============ 展示辅助 ============
function maskKey(key: string): string {
  if (!key) return '-'
  if (key.length <= 10) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

// ============ curl 示例 ============
function curlBody(): Record<string, unknown> {
  const cur = tryPeekBody()
  if (cur && Object.keys(cur).length > 0) return cur
  const fallback = buildDefaultBody(props.defaultParams)
  if (Object.keys(fallback).length > 0) return fallback
  return { prompt: 'A cinematic shot of a corgi surfing on a rainbow.' }
}

function tryPeekBody(): Record<string, unknown> | null {
  if (mode.value === 'json') {
    const raw = jsonInput.value.trim()
    if (!raw) return null
    try {
      const parsed = JSON.parse(raw)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
      return null
    } catch {
      return null
    }
  }
  if (fieldSpecs.value.length === 0) return null
  return currentFormBody()
}

const curlSnippet = computed(() => {
  const url = buildGatewayUrl(`/api/v1/model/${props.slug}`)
  const keyPart = selectedKey.value ? selectedKey.value.key : '<YOUR_API_KEY>'
  const body = JSON.stringify(curlBody(), null, 2)
  const indented = body
    .split('\n')
    .map((line, i) => (i === 0 ? line : '  ' + line))
    .join('\n')
  return [
    `curl -X POST '${url}' \\`,
    `  -H 'Authorization: Bearer ${keyPart}' \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -d '${indented}'`,
  ].join('\n')
})

async function copyCurl() {
  try {
    await navigator.clipboard.writeText(curlSnippet.value)
    appStore.showSuccess(t('videoModels.copied'))
  } catch {
    appStore.showError(t('videoModels.copyFailed'))
  }
}

const phaseIndicator = computed(() => {
  const p = playground.phase.value
  const map: Record<string, { label: string; color: string }> = {
    submitting: { label: t('videoModels.playground.phaseSubmitting'), color: 'bg-blue-500' },
    queued: { label: t('videoModels.playground.phaseQueued'), color: 'bg-blue-500' },
    running: { label: t('videoModels.playground.phaseRunning'), color: 'bg-blue-500' },
    completed: { label: t('videoModels.playground.phaseCompleted'), color: 'bg-green-500' },
    failed: { label: t('videoModels.playground.phaseFailed'), color: 'bg-red-500' },
    canceled: { label: t('videoModels.playground.phaseCanceled'), color: 'bg-gray-500' },
    idle: { label: '', color: 'bg-gray-400' },
  }
  return map[p] ?? map.idle
})

const queuePosition = computed<number | null>(() => {
  const s = playground.statusPayload.value
  if (!s || typeof s !== 'object') return null
  const q = (s as Record<string, unknown>).queue_position
  return typeof q === 'number' ? q : null
})

const prettyResult = computed(() => {
  try {
    return JSON.stringify(playground.resultPayload.value, null, 2)
  } catch {
    return String(playground.resultPayload.value)
  }
})

// ============ 生命周期与关闭 ============
function tryClose() {
  if (playground.isBusy.value) {
    const ok = window.confirm(t('videoModels.playground.confirmClose'))
    if (!ok) return
    if (selectedKey.value) {
      void playground.cancel(props.slug, selectedKey.value.key)
    } else {
      playground.reset()
    }
  }
  emit('update:open', false)
}

// 每次弹窗打开：加载 keys、重置表单、重置状态
watch(
  () => props.open,
  (val) => {
    if (val) {
      // 若无字段声明，直接进 json 模式
      mode.value = fieldSpecs.value.length > 0 ? 'form' : 'json'
      initFormDefaults()
      jsonInput.value = ''
      jsonError.value = ''
      playground.reset()
      void loadKeys()
    } else {
      playground.reset()
    }
  }
)

// slug / defaultParams 变更时重置默认表单（同一弹窗多次打开或父页面切换模型）
watch(
  () => props.slug,
  () => {
    if (props.open) {
      mode.value = fieldSpecs.value.length > 0 ? 'form' : 'json'
      initFormDefaults()
      playground.reset()
    }
  }
)
watch(
  () => props.defaultParams,
  () => {
    if (props.open) {
      mode.value = fieldSpecs.value.length > 0 ? 'form' : 'json'
      initFormDefaults()
    }
  }
)
</script>
