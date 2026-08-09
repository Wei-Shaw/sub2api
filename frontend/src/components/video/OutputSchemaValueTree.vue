<!--
  OutputSchemaValueTree：递归渲染管理端声明的"输出参数"schema 树，并在每个字段
  旁展示对应的值。

  给定：
    - schema：一个"节点"，可以是叶子（含 value）、object（含 properties）或
              array（含 items）。**顶层节点由 VideoPlaygroundView 从 OutputFieldSpec
              适配而来**，把 OutputFieldSpec.{key,description,required,type,...}
              统一转成本组件的 SchemaNode 形态。
    - value：该节点对应的"实际值"。null/undefined 表示"当前没有值可展示"（例如
              尚未提交任务）。
    - source：'payload' | 'default' | 'none'，用于左上角的角标显示。

  渲染规则：
    - 每个节点画一个块，头部展示：key（数组元素显示 [i]） · type 徽章 · required 星号
    - description 作为 helper 文字
    - 叶子：按 rawType 渲染
        · 值形如 URL 字符串（http(s):// 开头）→ 可点击链接
        · 其它 string/number/boolean → 原文
        · undefined/null → 灰色占位"—"
    - object：递归展开 properties 中每个子节点，值 = value[childKey]
    - array：遍历 value 数组，每个元素再递归渲染（元素 schema 用 items）；
             value 不是数组或为空时显示占位。

  该组件通过 defineOptions({ name: 'OutputSchemaValueTree' }) 支持模板内自递归。
-->
<template>
  <div class="output-schema-node space-y-1.5">
    <!-- 节点头部：key（或 [i]） · 类型徽章 · required · 描述 -->
    <div class="flex items-baseline gap-2">
      <span class="font-mono text-xs text-gray-700 dark:text-gray-300">
        {{ displayKey }}
      </span>
      <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
        {{ node.rawType }}
      </span>
      <span
        v-if="node.required"
        class="text-[10px] font-medium text-red-500"
        :title="t('videoModels.playground.requiredBadge')"
      >
        *
      </span>
      <span
        v-if="localizedDescription"
        class="truncate text-[11px] text-gray-500 dark:text-gray-400"
      >
        {{ localizedDescription }}
      </span>
    </div>

    <!-- 叶子节点：渲染值 -->
    <div v-if="isLeaf" class="pl-1">
      <!-- URL 形态：给一个可点击的链接 -->
      <a
        v-if="leafIsUrl"
        :href="leafText"
        target="_blank"
        rel="noopener noreferrer"
        class="block break-all rounded bg-gray-50 px-2 py-1 font-mono text-[11px] text-blue-600 hover:underline dark:bg-gray-900 dark:text-blue-400"
      >
        {{ leafText }}
      </a>
      <!-- 普通标量：原文 -->
      <div
        v-else-if="leafText !== ''"
        class="break-all rounded bg-gray-50 px-2 py-1 font-mono text-[11px] text-gray-800 dark:bg-gray-900 dark:text-gray-200"
      >
        {{ leafText }}
      </div>
      <!-- 无值 -->
      <div v-else class="pl-1 text-[11px] text-gray-400">
        {{ t('videoModels.playground.outputNoValue') }}
      </div>
    </div>

    <!-- object：递归展开 properties -->
    <div
      v-else-if="node.rawType === 'object'"
      class="nested-block nested-block--object mt-1"
    >
      <div v-if="node.children.length === 0" class="pl-1 text-[11px] text-gray-400">
        {{ t('videoModels.playground.outputEmptyObject') }}
      </div>
      <div v-else class="space-y-2">
        <OutputSchemaValueTree
          v-for="child in node.children"
          :key="child.key"
          :node="child"
          :value="pickChildValue(child.key)"
        />
      </div>
    </div>

    <!-- array：遍历 value 数组，每个元素递归渲染 -->
    <div
      v-else-if="node.rawType === 'array' && node.items"
      class="nested-block nested-block--array mt-1"
    >
      <!-- 有值：按值的实际长度渲染每个元素 -->
      <div v-if="Array.isArray(value) && value.length > 0" class="space-y-2">
        <OutputSchemaValueTree
          v-for="(el, i) in (value as unknown[])"
          :key="i"
          :node="itemNodeWithIndex(i)"
          :value="el"
        />
      </div>
      <!-- 无值：至少展示一个 items schema（值为 undefined）让用户看到"数组元素长什么样" -->
      <div v-else class="space-y-2">
        <OutputSchemaValueTree
          :node="itemNodeWithIndex(0)"
          :value="undefined"
        />
        <div class="pl-1 text-[11px] text-gray-400">
          {{ t('videoModels.playground.outputEmptyArray') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * OutputSchemaValueTree 组件实现说明：
 *  - defineOptions({ name: 'OutputSchemaValueTree' }) 让模板可以递归自引用；
 *  - 只做"展示"：所有值/schema 由父组件传入，本组件不修改。
 *  - 顶层的 SchemaNode 由父组件从 OutputFieldSpec 适配而来（见 VideoPlaygroundView.vue）。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

defineOptions({ name: 'OutputSchemaValueTree' })

/**
 * SchemaNode：适配后的"输出参数节点"。
 *   - rawType: 叶子 = 'string' | 'number' | 'boolean' | 'url'
 *              复合 = 'object' | 'array'
 *   - children: object 的子字段
 *   - items:    array 的元素 schema
 *
 * 与 OutputFieldSpec 的差别是：
 *   - 展平了 properties 到 children 数组，顺序稳定；
 *   - 展平了 items 到子 SchemaNode，方便递归；
 *   - 数组元素传入时会把 key 覆盖为 `[i]` 便于展示。
 */
export interface SchemaNode {
  key: string
  /** 展示用的 key，array 元素时应传 `[i]`；父层负责设置。 */
  displayKey?: string
  /**
   * rawType：与 OutputFieldType 对齐（string/number/boolean/object/array）。
   * URL 语义不再单独用 rawType 表达，而是叶子分支里通过"值是否 http(s):// 起始"
   * 做启发式判断，任意 string 叶子只要值像 URL 就自动渲染成可点链接。
   */
  rawType: 'string' | 'number' | 'boolean' | 'object' | 'array'
  required: boolean
  description: string
  /** 字段英文说明（可选）；与 description 构成中英双文说明。 */
  descriptionEn?: string
  children: SchemaNode[]
  items: SchemaNode | null
}

const { t, locale } = useI18n()

const props = defineProps<{
  node: SchemaNode
  /** 当前节点对应的实际值；顶层从 payload 或 default 传入；子节点由递归取值。 */
  value: unknown
}>()

const isLeaf = computed(
  () =>
    props.node.rawType !== 'object' &&
    props.node.rawType !== 'array'
)

const displayKey = computed(() => props.node.displayKey || props.node.key || '(item)')

/**
 * localizedDescription：按当前 i18n locale 选择字段说明的展示语种。
 * 与 VideoPlaygroundSchemaField 保持完全一致的挑选策略：
 *   - locale 以 'en' 前缀开头（'en'/'en-US'）→ 优先英文；否则优先中文。
 *   - 目标语种为空时兜底另一种，避免出现空白。
 */
const localizedDescription = computed<string>(() => {
  const isEnLocale = String(locale.value || '').toLowerCase().startsWith('en')
  const zh = props.node.description?.trim() || ''
  const en = props.node.descriptionEn?.trim() || ''
  if (isEnLocale) return en || zh
  return zh || en
})

/**
 * leafText：叶子的字符串化值。
 *   - null / undefined → ''（触发"无值"分支）
 *   - 其它标量 → String(v)
 *   - 对象/数组：本组件叶子分支不应出现这些类型（rawType 已由父层判定），
 *     这里做兜底 JSON.stringify。
 */
const leafText = computed<string>(() => {
  const v = props.value
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
})

/**
 * leafIsUrl：叶子是否应作为链接展示。
 * 启发式判断：string 叶子值以 http:// 或 https:// 起始时按链接展示。
 * 不引入独立的 'url' rawType，避免与新 OutputFieldType（string/number/boolean/
 * object/array）语义偏离。
 */
const leafIsUrl = computed<boolean>(() => {
  const s = leafText.value
  return typeof s === 'string' && /^https?:\/\//i.test(s)
})

/** object：按 child.key 从 value 里取子值。 */
function pickChildValue(childKey: string): unknown {
  const v = props.value
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    return (v as Record<string, unknown>)[childKey]
  }
  return undefined
}

/**
 * itemNodeWithIndex：为 array 的第 i 个元素派生一个 SchemaNode，
 * displayKey 设置为 `[i]`。原 items 节点复用其它元数据。
 */
function itemNodeWithIndex(i: number): SchemaNode {
  const it = props.node.items!
  return {
    ...it,
    displayKey: `[${i}]`,
  }
}
</script>

<style scoped>
/*
  嵌套色标：与 ParamSchemaEditor 保持一致视觉语言。
  - object 蓝色，array 紫色
  - 内嵌 padding + 左边框 4px 累积缩进感
*/
.nested-block {
  padding: 0.5rem 0.5rem 0.5rem 0.625rem;
  border-radius: 0.375rem;
  border: 1px solid rgb(229 231 235);
  border-left-width: 4px;
  background-color: rgb(249 250 251 / 0.6);
}
.dark .nested-block {
  border-color: rgb(55 65 81);
  background-color: rgb(31 41 55 / 0.4);
}
.nested-block--object {
  border-left-color: rgb(59 130 246);
}
.nested-block--array {
  border-left-color: rgb(139 92 246);
}
</style>
