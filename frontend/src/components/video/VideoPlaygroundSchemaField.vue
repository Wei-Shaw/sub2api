<!--
  VideoPlaygroundSchemaField：视频演练台的"字段渲染器"。

  给定一个 FieldSpec 节点和一个双向绑定的 value（任意 JSON 值），根据 rawType 递归渲染：
    - string / number / boolean：叶子控件（select | textarea | input | checkbox）
    - object：展开 children，每个 child 一个子控件；父 value 保持为 { key: subValue, ... }
    - array：value 保持为数组；提供"+ 添加元素"按钮，元素 UI 由 items schema 递归产出

  只有叶子会走 required 校验；object/array 的 required 目前仅用于展示徽章。
-->
<template>
  <div class="video-playground-field space-y-1">
    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
      <span class="font-mono">{{ spec.key || label || '(item)' }}</span>
      <span v-if="spec.required" class="text-red-500">*</span>
      <span class="ml-2 font-mono text-[10px] text-gray-400">{{ spec.rawType }}</span>
    </label>

    <!-- 字段说明：紧贴 key 下方展示，便于用户先看含义再填值。
         使用 whitespace-pre-line 让 SchemaEditor 里手工写入的换行原样生效。 -->
    <p
      v-if="localizedDescription"
      class="mb-1 whitespace-pre-line text-xs leading-relaxed text-gray-500 dark:text-gray-400"
    >
      {{ localizedDescription }}
    </p>

    <p
      v-if="spec.rawType === 'array' && spec.maxItems > 0"
      class="array-max-items-hint text-[11px] text-amber-600 dark:text-amber-400"
    >
      {{ t('videoModels.playground.arrayMaxItemsHint', { n: spec.maxItems }) }}
    </p>

    <!-- 枚举：改用项目通用 Select（与 API Key 下拉、其它页面下拉视觉一致）
         - options 直接由 spec.options（string[]）映射成 {value,label} 对
         - clearable 让非必填字段可清空；placeholder 使用"未指定"的 i18n 文案
         - searchable='auto'：选项超过阈值自动出现搜索框 -->
    <Select
      v-if="spec.isEnum && spec.options.length > 0"
      :model-value="stringLeafValue"
      :options="enumOptions"
      :disabled="disabled"
      :placeholder="t('videoModels.playground.enumEmpty')"
      :clearable="!spec.required"
      @update:model-value="(v: string | number | boolean | null) => onLeafChange(v == null ? '' : String(v))"
    />

    <!-- 布尔：使用项目通用 Toggle（与全站开关风格一致）
         Toggle 组件本身不支持 disabled prop，这里用外层 wrapper 的
         pointer-events-none + opacity-50 组合来实现禁用视觉。 -->
    <div
      v-else-if="spec.rawType === 'boolean'"
      class="flex items-center gap-2 text-sm"
      :class="disabled ? 'pointer-events-none opacity-50' : ''"
    >
      <Toggle
        :model-value="modelValue === true"
        @update:model-value="onBoolChange"
      />
      <span class="text-gray-600 dark:text-gray-400">
        {{ modelValue === true ? 'true' : 'false' }}
      </span>
    </div>

    <!-- number -->
    <input
      v-else-if="spec.rawType === 'number'"
      :value="stringLeafValue"
      type="number"
      :disabled="disabled"
      class="w-full rounded border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100"
      @input="onLeafChange(($event.target as HTMLInputElement).value)"
    />

    <!-- string 图片控件（widget='image'）：声明为 image 时优先渲染 ImageInputField（URL / 本地上传 / 素材库三合一）。
         仅当 spec.rawType==='string' && spec.widget==='image' 时生效。 -->
    <ImageInputField
      v-else-if="spec.rawType === 'string' && spec.widget === 'image'"
      :model-value="stringLeafValue"
      :disabled="disabled"
      @update:model-value="(v: unknown) => onLeafChange(v == null ? '' : String(v))"
    />

    <PromptMediaReferenceInput
      v-else-if="spec.rawType === 'string' && usePromptTextArea"
      :model-value="modelValue"
      :references="promptReferences"
      :disabled="disabled"
      :rows="textareaRowsForRender"
      @update:model-value="onLeafChange"
    />
    <!-- string：控件类型由 schema 声明的 spec.widget 决定：
           - widget='textarea' → 多行 <textarea>，行数取 spec.textareaRows（默认 3）
           - widget='input'    → 单行 <input>（但若默认值本身很长或含换行，作为兜底
                                  仍启用 textarea，避免"长文本被塞进单行框"的糟糕体验）
         这一层显式声明来自管理端的 SchemaEditor，让"演练台里的表单形态"由管理员精确控制。 -->
    <textarea
      v-else-if="spec.rawType === 'string' && useTextarea"
      :value="stringLeafValue"
      :disabled="disabled"
      :rows="textareaRowsForRender"
      class="w-full rounded border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100"
      @input="onLeafChange(($event.target as HTMLTextAreaElement).value)"
    />
    <input
      v-else-if="spec.rawType === 'string'"
      :value="stringLeafValue"
      type="text"
      :disabled="disabled"
      class="w-full rounded border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100"
      @input="onLeafChange(($event.target as HTMLInputElement).value)"
    />

    <!-- object：递归子字段 -->
    <div
      v-else-if="spec.rawType === 'object'"
      class="ml-2 mt-1 space-y-2 border-l-2 border-gray-200 pl-3 dark:border-gray-700"
    >
      <VideoPlaygroundSchemaField
        v-for="child in spec.children"
        :key="child.key"
        :spec="child"
        :model-value="((modelValue as Record<string, unknown>) || {})[child.key]"
        :disabled="disabled"
        :media-references="mediaReferences"
        @update:model-value="onObjectChildChange(child.key, $event)"
      />
    </div>

    <!-- array 媒体组控件：把整个 URL 数组当成一个整体展示，支持上传、素材库、
         粘贴 URL 与拖拽排序。
         注意这一分支不要求 spec.items 存在：元素形态已被 widget 固定为字符串 URL。 -->
    <ImageUrlsField
      v-else-if="spec.rawType === 'array' && mediaKind"
      :model-value="modelValue"
      :disabled="disabled"
      :max-items="spec.maxItems"
      :media-kind="mediaKind"
      @update:model-value="(v: string[]) => emit('update:modelValue', v)"
    />

    <!-- array：一组元素，每个元素由 items schema 递归渲染 -->
    <div
      v-else-if="spec.rawType === 'array' && spec.items"
      class="ml-2 mt-1 space-y-2 border-l-2 border-gray-200 pl-3 dark:border-gray-700"
    >
      <div
        v-for="(_, i) in arrayValue"
        :key="i"
        class="flex items-start gap-2"
      >
        <div class="flex-1">
          <VideoPlaygroundSchemaField
            :spec="({ ...spec.items!, key: `[${i}]` })"
            :model-value="arrayValue[i]"
            :disabled="disabled"
            :media-references="mediaReferences"
            @update:model-value="onArrayItemChange(i, $event)"
          />
        </div>
        <button
          type="button"
          :disabled="disabled"
          class="mt-6 shrink-0 rounded border border-gray-300 bg-white px-2 py-1 text-xs text-red-500 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:bg-gray-700"
          :title="t('common.remove')"
          @click="onArrayRemove(i)"
        >
          ✕
        </button>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          :disabled="disabled || !canAddArrayItem"
          class="rounded border border-dashed border-gray-300 bg-white px-3 py-1 text-xs text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
          @click="onArrayAdd"
        >
          + {{ t('common.add', 'Add') }}
        </button>
        <!-- 当前数量与上限；完整的“最多 N 个”提示已统一展示在字段说明下方。 -->
        <span v-if="spec.maxItems > 0" class="text-[11px] text-gray-400">
          {{ arrayValue.length }} / {{ spec.maxItems }}
        </span>
      </div>
    </div>

    <p
      v-if="spec.rawType === 'string' && usePromptTextArea && promptReferencesEnabled"
      class="prompt-reference-hint text-[11px] text-gray-500 dark:text-gray-400"
    >
      {{ t('videoModels.playground.promptReferenceHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * VideoPlaygroundSchemaField 组件实现说明：
 *  - 通过 defineOptions({ name: 'VideoPlaygroundSchemaField' }) 支持模板内递归。
 *  - 单一 v-model 承载"任意 JSON 值"：叶子是 primitive，object 是 Record，array 是 Array。
 *  - 叶子的值统一按 rawType 处理：
 *      string → 用户原文
 *      number → 保留字符串态编辑（提交时由上层再走 coerceFieldValue 换回 number）
 *      boolean → 布尔
 *    string 内部使用 stringLeafValue 计算属性避免 undefined/null 反显时抛错。
 *  - object：把子字段的 update 事件合成到父 value 上，再向上 emit。
 *  - array：value 保持为数组；元素类型由 spec.items 决定，UI 递归。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import ImageInputField from '@/components/video/ImageInputField.vue'
import PromptMediaReferenceInput from '@/components/video/PromptMediaReferenceInput.vue'
// ImageUrlsField：array 媒体 URL 组的统一输入控件。
import ImageUrlsField from '@/components/video/ImageUrlsField.vue'
import type { FieldSpec } from '@/components/video/paramSpec'
import type { PromptMediaReference } from '@/components/video/promptMediaReferences'
import { mediaKindForWidget, normalizeMediaUrlWidget } from '@/utils/mediaUrlWidget'

defineOptions({ name: 'VideoPlaygroundSchemaField' })

const { t, locale } = useI18n()

const props = defineProps<{
  spec: FieldSpec
  modelValue: unknown
  disabled?: boolean
  /** 兜底展示 label（默认取 spec.key；数组元素时上层会传 `[i]` 覆盖） */
  label?: string
  /** 当前表单中可通过 @ 引用的媒体；由顶层按 schema 顺序统一编号。 */
  mediaReferences?: PromptMediaReference[]
}>()

const mediaReferences = computed(() => props.mediaReferences ?? [])
const usePromptTextArea = computed(() => props.spec.widget === 'PromptTextArea')
const promptReferencesEnabled = computed(() => props.spec.referenceFields.length > 0)
const promptReferences = computed(() => {
  const configured = new Set(props.spec.referenceFields.map((field) => field.trim()).filter(Boolean))
  if (configured.size === 0) return []
  return mediaReferences.value.filter((reference) => {
    for (const field of configured) {
      if (reference.fieldKey === field || reference.fieldKey.startsWith(`${field}.`) || reference.fieldKey.startsWith(`${field}[`)) return true
    }
    return false
  })
})

const mediaKind = computed(() => {
  const widget = normalizeMediaUrlWidget(props.spec.widget)
  if (widget) return mediaKindForWidget(widget)
  // 兼容以 items 声明图片控件的 array：这类 schema 表达的是一组图片 URL，
  // 演练页应直接显示图库，而不是为每个元素重复渲染单图输入框。
  if (
    props.spec.rawType === 'array' &&
    props.spec.items?.rawType === 'string' &&
    props.spec.items.widget === 'image'
  ) {
    return 'image'
  }
  return null
})

const emit = defineEmits<{
  (e: 'update:modelValue', v: unknown): void
}>()

/**
 * localizedDescription：按当前 i18n locale 挑选合适语种展示字段说明。
 *   - locale 以 'en' 前缀开头（如 'en'、'en-US'）优先英文；否则优先中文。
 *   - 目标语种为空时兜底另一种，避免"某个字段只填了英文，中文界面反而空"。
 */
const localizedDescription = computed<string>(() => {
  const isEnLocale = String(locale.value || '').toLowerCase().startsWith('en')
  const zh = props.spec.description?.trim() || ''
  const en = props.spec.descriptionEn?.trim() || ''
  if (isEnLocale) return en || zh
  return zh || en
})

/** 叶子的字符串化 value（供 input/textarea/select 反显）。 */
const stringLeafValue = computed<string>(() => {
  const v = props.modelValue
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return ''
  }
})

/** 数组当前值：非数组时给一个空数组占位，避免 v-for 报错。 */
const arrayValue = computed<unknown[]>(() => {
  return Array.isArray(props.modelValue) ? (props.modelValue as unknown[]) : []
})

/**
 * canAddArrayItem：通用 array 分支的"添加"按钮是否可用。
 * spec.maxItems > 0 时按上限限制；0 表示不限制。
 * （imageUrls 分支的上限判断在 ImageUrlsField 内部完成。）
 */
const canAddArrayItem = computed<boolean>(() => {
  const max = props.spec.maxItems
  if (!max || max <= 0) return true
  return arrayValue.value.length < max
})

/** 枚举选项：把 spec.options（string[]）映射成通用 Select 需要的 {value,label}。 */
const enumOptions = computed<SelectOption[]>(() =>
  props.spec.options.map((opt) => ({ value: opt, label: opt }))
)

/**
 * useTextarea：决定 string 叶子是否使用 <textarea>。
 * 优先看管理端在 SchemaEditor 里的显式声明 spec.widget：
 *   - 'textarea' → 强制 textarea
 *   - 'input'   → 强制 input（但对"内容明显是长文本"的旧数据做兜底：默认值 >80 字
 *                  或含换行 / 描述过长时依然改用 textarea，避免体验退化）
 * 未声明 widget 的老数据走原有启发式规则（保持向后兼容）。
 */
const useTextarea = computed<boolean>(() => {
  const w = props.spec.widget
  if (w === 'textarea') return true
  // widget='input' 或未声明：仍保留启发式兜底（中英取较长者的长度作为参考）
  const descLen = Math.max(
    (props.spec.description || '').length,
    (props.spec.descriptionEn || '').length,
  )
  return isLongString(props.spec.defaultValue) || descLen > 60
})

/**
 * textareaRowsForRender：textarea 渲染时使用的 rows 数。
 * 只有当 spec.widget='textarea' 且 spec.textareaRows 合法时才用声明值；
 * 兜底走的 textarea（widget=input 但被启发式识别为长文本）用默认 3 行。
 */
const textareaRowsForRender = computed<number>(() => {
  if (props.spec.widget === 'textarea' || props.spec.widget === 'PromptTextArea') {
    const r = Number(props.spec.textareaRows)
    if (Number.isFinite(r) && r > 0) return Math.min(20, Math.max(1, Math.trunc(r)))
  }
  return 3
})

function isLongString(s: string | undefined): boolean {
  if (!s) return false
  return s.length > 80 || s.includes('\n')
}

/**
 * onLeafChange：叶子（string/number）文本输入 → 按 rawType 简单转型后 emit。
 * number 空串还原为 undefined，让上层判定"未填"；boolean 走 onBoolChange。
 */
function onLeafChange(v: string) {
  const s = v ?? ''
  if (props.spec.rawType === 'number') {
    const trimmed = s.trim()
    if (trimmed === '') {
      emit('update:modelValue', undefined)
      return
    }
    const n = Number(trimmed)
    emit('update:modelValue', Number.isFinite(n) ? n : trimmed)
    return
  }
  emit('update:modelValue', s)
}

function onBoolChange(checked: boolean) {
  emit('update:modelValue', checked)
}

/** object 子字段变更：与旧 value 合并（浅合并）后 emit 新对象。 */
function onObjectChildChange(childKey: string, v: unknown) {
  const cur =
    props.modelValue && typeof props.modelValue === 'object' && !Array.isArray(props.modelValue)
      ? { ...(props.modelValue as Record<string, unknown>) }
      : {}
  if (v === undefined) {
    delete cur[childKey]
  } else {
    cur[childKey] = v
  }
  emit('update:modelValue', cur)
}

/** array 元素修改：拷贝后按 index 更新。 */
function onArrayItemChange(i: number, v: unknown) {
  const arr = Array.isArray(props.modelValue) ? [...(props.modelValue as unknown[])] : []
  arr[i] = v
  emit('update:modelValue', arr)
}

/** array 元素删除。 */
function onArrayRemove(i: number) {
  const arr = Array.isArray(props.modelValue) ? [...(props.modelValue as unknown[])] : []
  arr.splice(i, 1)
  emit('update:modelValue', arr)
}

/** array 元素新增：根据 items schema 生成一个默认值（叶子取 rawDefaultValue，object → {}，array → []）。 */
function onArrayAdd() {
  if (!canAddArrayItem.value) return
  const arr = Array.isArray(props.modelValue) ? [...(props.modelValue as unknown[])] : []
  arr.push(defaultForSpec(props.spec.items))
  emit('update:modelValue', arr)
}

/** defaultForSpec：给 FieldSpec 递归产生一个默认值。 */
function defaultForSpec(s: FieldSpec | null | undefined): unknown {
  if (!s) return ''
  if (s.rawType === 'object') return {}
  if (s.rawType === 'array') return []
  return s.rawDefaultValue ?? (s.rawType === 'boolean' ? false : s.rawType === 'number' ? 0 : '')
}
</script>

<style scoped>
.video-playground-field {
  width: 100%;
}
</style>
