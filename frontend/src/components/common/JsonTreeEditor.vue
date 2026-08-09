<!--
  JsonTreeEditor：可嵌套 JSON 值的树形编辑器。

  使用方式：
    <JsonTreeEditor v-model="value" />

  设计要点：
    - 组件通过 `name: 'JsonTreeEditor'` 自引用，实现对 object / array 子节点的递归渲染。
    - 内部 shape：以 `NodeType`（object|array|string|number|boolean|null）+ 各具体值组合
      表达一个节点，避免把 JS 原始类型直接塞进单一字段（Vue reactivity 对 union
      类型不友好，且 array/object 无法区分）。
    - `modelValue` 是任意 JSON 兼容值（Record|Array|string|number|boolean|null）。
      每次 `modelValue` 变化都会重建内部树；每次用户编辑树则通过 nodeToValue 序列化
      并 `emit('update:modelValue', ...)`。
    - 校验：所有子节点的字符串值都可被解析（number 用 Number()、boolean 用 checkbox），
      因此本组件本身**永远可以成功序列化**，不会产生非法 JSON。这也是选择"树形结构"
      优于"文本 JSON"的关键理由。
-->
<template>
  <div class="json-tree-editor">
    <!-- 叶子：string -->
    <template v-if="node.type === 'string'">
      <input
        v-model="node.stringValue"
        type="text"
        class="input h-8 text-xs"
        :placeholder="placeholder || 'string'"
        @input="emitChange"
      />
    </template>

    <!-- 叶子：number -->
    <template v-else-if="node.type === 'number'">
      <input
        v-model="node.numberText"
        type="text"
        inputmode="decimal"
        class="input h-8 w-40 font-mono text-xs"
        placeholder="0"
        @input="emitChange"
      />
    </template>

    <!-- 叶子：boolean -->
    <template v-else-if="node.type === 'boolean'">
      <label
        class="inline-flex h-8 items-center gap-2 rounded-xl border border-gray-200 px-3 dark:border-dark-600"
      >
        <input
          v-model="node.booleanValue"
          type="checkbox"
          class="h-4 w-4"
          @change="emitChange"
        />
        <span class="text-xs text-gray-500">{{ node.booleanValue ? 'true' : 'false' }}</span>
      </label>
    </template>

    <!-- 叶子：null -->
    <template v-else-if="node.type === 'null'">
      <span
        class="inline-flex h-8 items-center rounded-xl border border-dashed border-gray-300 px-3 font-mono text-xs text-gray-400 dark:border-dark-600"
      >null</span>
    </template>

    <!-- 分支：object
         每个子项对齐外层"输入参数"的填写风格：
           - 顶部一行由三列组成：key / 类型 / 值 + 删除按钮
           - 每列上方都挂"字段名" label，视觉与外层一致
           - 类型下拉使用项目通用 Select 组件（sm 尺寸），
             保证与项目内所有下拉框样式一致 -->
    <template v-else-if="node.type === 'object'">
      <div
        class="rounded-lg border border-gray-200 bg-gray-50/50 p-2 dark:border-dark-700 dark:bg-dark-800/40"
      >
        <div class="mb-2 flex items-center gap-2">
          <span class="font-mono text-[11px] text-gray-500">{{ '{' }} object · {{ node.objectChildren.length }} {{ '}' }}</span>
          <button type="button" class="btn btn-ghost btn-xs" @click="addObjectChild">+ key</button>
        </div>
        <div v-if="node.objectChildren.length === 0" class="pl-1 text-[11px] text-gray-400">
          （空对象，点击"+ key"新增字段）
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="(child, i) in node.objectChildren"
            :key="child.uid"
            class="rounded border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800"
          >
            <!-- 第一行：字段名 / 类型 / 删除，flex-wrap + items-end 对齐 -->
            <div class="flex flex-wrap items-end gap-2">
              <div class="flex w-32 flex-col gap-1">
                <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('common.name') }}</label>
                <input
                  v-model="child.key"
                  type="text"
                  class="input h-8 font-mono text-xs"
                  placeholder="key"
                  @input="emitChange"
                />
              </div>
              <div class="flex w-28 flex-col gap-1">
                <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('common.type') }}</label>
                <Select
                  :model-value="child.node.type"
                  :options="jsonTypeOptions"
                  :searchable="false"
                  size="sm"
                  @update:modelValue="(v: string | number | boolean | null) => onChildTypeSelectUpdate(child.node, v)"
                />
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs ml-auto self-end text-red-500"
                :title="'删除该字段'"
                @click="removeObjectChild(i)"
              >
                ✕
              </button>
            </div>

            <!-- 第二行：值。上方挂"值"字段名，与外层"默认值"呼应，
                 保证不同类型的值控件都有统一的字段说明。 -->
            <div class="mt-2 flex flex-col gap-1">
              <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('common.value') }}</label>
              <JsonTreeEditor
                v-model="child.node.__proxy"
                :internal-node="child.node"
                @__nodeChange="emitChange"
              />
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- 分支：array
         每个子项对齐外层"输入参数"的填写风格：
           - 顶部一行：序号（只读） / 类型 / 删除按钮
           - 每列上方都挂"字段名" label -->
    <template v-else-if="node.type === 'array'">
      <div
        class="rounded-lg border border-gray-200 bg-gray-50/50 p-2 dark:border-dark-700 dark:bg-dark-800/40"
      >
        <div class="mb-2 flex items-center gap-2">
          <span class="font-mono text-[11px] text-gray-500">[ array · {{ node.arrayChildren.length }} ]</span>
          <button type="button" class="btn btn-ghost btn-xs" @click="addArrayChild">+ item</button>
        </div>
        <div v-if="node.arrayChildren.length === 0" class="pl-1 text-[11px] text-gray-400">
          （空数组，点击"+ item"新增元素）
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="(child, i) in node.arrayChildren"
            :key="child.uid"
            class="rounded border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex flex-wrap items-end gap-2">
              <div class="flex w-16 flex-col gap-1">
                <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('common.index') }}</label>
                <div class="flex h-8 items-center rounded-xl border border-dashed border-gray-300 px-3 font-mono text-xs text-gray-500 dark:border-dark-600">
                  {{ i }}
                </div>
              </div>
              <div class="flex w-28 flex-col gap-1">
                <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('common.type') }}</label>
                <Select
                  :model-value="child.type"
                  :options="jsonTypeOptions"
                  :searchable="false"
                  size="sm"
                  @update:modelValue="(v: string | number | boolean | null) => onChildTypeSelectUpdate(child, v)"
                />
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs ml-auto self-end text-red-500"
                :title="'删除该元素'"
                @click="removeArrayChild(i)"
              >
                ✕
              </button>
            </div>

            <div class="mt-2 flex flex-col gap-1">
              <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('common.value') }}</label>
              <JsonTreeEditor
                v-model="child.__proxy"
                :internal-node="child"
                @__nodeChange="emitChange"
              />
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
// 该组件通过 defineOptions 声明自身名字，用于在 <template> 内递归调用。
// defineOptions 是 <script setup> 的编译宏，无需从 vue 中 import。
import { reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'

defineOptions({ name: 'JsonTreeEditor' })

const { t } = useI18n()

// ---------------- 类型定义 ----------------
export type JsonNodeType = 'string' | 'number' | 'boolean' | 'null' | 'object' | 'array'

// jsonTypeOptions：JSON 树内叶子/分支类型的候选下拉项。
// label 保持英文常量（string/number/…），因为它们直接对应 JSON 规范，无需 i18n。
const jsonTypeOptions: SelectOption[] = [
  { value: 'string', label: 'string' },
  { value: 'number', label: 'number' },
  { value: 'boolean', label: 'boolean' },
  { value: 'null', label: 'null' },
  { value: 'object', label: 'object' },
  { value: 'array', label: 'array' }
]

// JsonNode：树的通用节点。为便于 v-model 递归 + 类型切换保留旧值，
// 六种子类型的数据字段全部并列存在（未激活的字段忽略即可）。
// __proxy 是 modelValue 通道的一份占位（永远与 nodeToValue(node) 保持一致），
// 便于递归子组件透过 v-model 触发外层刷新（本组件也会自己 emit）。
// uid 用于 array v-for 的稳定 key，避免元素乱序时 DOM 复用导致输入失焦。
interface JsonNode {
  uid: number
  type: JsonNodeType
  stringValue: string
  numberText: string // 用文本保留，避免 "1." 被强转为 1
  booleanValue: boolean
  objectChildren: Array<{ uid: number; key: string; node: JsonNode }>
  arrayChildren: JsonNode[]
  // 递归子组件的 v-model 占位，同步更新。
  __proxy: unknown
}

// ---------------- Props / Emits ----------------
const props = defineProps<{
  // 外部值（任意合法 JSON 值）。
  modelValue?: unknown
  // 顶层 placeholder（仅 type=string 时用得上）。
  placeholder?: string
  // 递归时，父组件会传入自己的内部 JsonNode，避免重复构造/丢失焦点。
  internalNode?: JsonNode
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: unknown): void
  // 递归专用事件：子孙节点变化时逐级冒泡，最终触发顶层重新 emit modelValue。
  (e: '__nodeChange'): void
}>()

// ---------------- 内部状态 ----------------
// 递增 uid，保证 object / array 子项 v-for 使用稳定 key（避免 DOM 复用导致失焦）。
// 注意：必须在 valueToNode 被调用之前完成初始化，否则 let 的 TDZ 会让
// `valueToNode(props.modelValue)` 内部访问 __uidSeq 时抛 ReferenceError，
// 表现为组件挂载后什么都不渲染。
let __uidSeq = 1
function nextUid(): number {
  return __uidSeq++
}

// valueToNode / nodeToValue 使用函数声明会自动 hoisting，但内部访问的
// __uidSeq（let）必须先于任何调用点完成初始化。此处的顺序至关重要。

// 顶层实例自己 own 一个 node；子实例复用父传下来的 node 引用（避免重复构造）。
const isRoot = props.internalNode === undefined
const node: JsonNode = isRoot ? reactive(valueToNode(props.modelValue)) : (props.internalNode as JsonNode)

// ---------------- 值 ⇄ 节点 转换 ----------------
// valueToNode：把任意 JSON 值转成 JsonNode 树。
function valueToNode(v: unknown): JsonNode {
  const base: JsonNode = {
    uid: nextUid(),
    type: 'null',
    stringValue: '',
    numberText: '',
    booleanValue: false,
    objectChildren: [],
    arrayChildren: [],
    __proxy: null
  }
  if (v === null || v === undefined) {
    base.type = 'null'
  } else if (typeof v === 'string') {
    base.type = 'string'
    base.stringValue = v
  } else if (typeof v === 'number' && Number.isFinite(v)) {
    base.type = 'number'
    base.numberText = String(v)
  } else if (typeof v === 'boolean') {
    base.type = 'boolean'
    base.booleanValue = v
  } else if (Array.isArray(v)) {
    base.type = 'array'
    base.arrayChildren = v.map((item) => valueToNode(item))
  } else if (typeof v === 'object') {
    base.type = 'object'
    base.objectChildren = Object.entries(v as Record<string, unknown>).map(([k, val]) => ({
      uid: nextUid(),
      key: k,
      node: valueToNode(val)
    }))
  }
  base.__proxy = nodeToValue(base)
  return base
}

// nodeToValue：把 JsonNode 树递归序列化为普通 JSON 值。
// - number: 空串 → 0；非法 → 0（永不抛异常，保证外部值一直合法）
// - object 中若某 child.key 为空，直接跳过（避免生成 `"": ...`）
function nodeToValue(n: JsonNode): unknown {
  switch (n.type) {
    case 'string':
      return n.stringValue
    case 'number': {
      const t = (n.numberText || '').trim()
      if (t === '') return 0
      const num = Number(t)
      return Number.isFinite(num) ? num : 0
    }
    case 'boolean':
      return n.booleanValue
    case 'null':
      return null
    case 'array':
      return n.arrayChildren.map((c) => nodeToValue(c))
    case 'object': {
      const obj: Record<string, unknown> = {}
      for (const ch of n.objectChildren) {
        const k = (ch.key || '').trim()
        if (!k) continue
        obj[k] = nodeToValue(ch.node)
      }
      return obj
    }
    default:
      return null
  }
}

// ---------------- 变更冒泡 ----------------
// emitChange：任何叶子/结构变更后调用；顶层 emit modelValue，子层 emit __nodeChange。
function emitChange() {
  // 同步 __proxy，让父的 v-model 更新，父的 watch(modelValue) 不会误重建。
  node.__proxy = nodeToValue(node)
  if (isRoot) {
    emit('update:modelValue', node.__proxy)
  } else {
    emit('__nodeChange')
  }
}

// ---------------- object 子项 增删 ----------------
function addObjectChild() {
  node.objectChildren.push({ uid: nextUid(), key: '', node: valueToNode('') })
  emitChange()
}
function removeObjectChild(i: number) {
  node.objectChildren.splice(i, 1)
  emitChange()
}

// ---------------- array 子项 增删 ----------------
function addArrayChild() {
  node.arrayChildren.push(valueToNode(''))
  emitChange()
}
function removeArrayChild(i: number) {
  node.arrayChildren.splice(i, 1)
  emitChange()
}

// onChildTypeSelectUpdate：接收 Select 组件的 update:modelValue，把值写回 child.type
// 并复用 onChildTypeChange 的初始化/冒泡逻辑。避免直接给 child.type v-model 拿到 unknown。
function onChildTypeSelectUpdate(child: JsonNode, v: string | number | boolean | null) {
  const next = (v == null ? 'null' : String(v)) as JsonNodeType
  child.type = next
  onChildTypeChange(child)
}

// ---------------- 类型切换 ----------------
// 用户在 <select> 切换某个子节点类型时，清空/初始化对应字段，避免残留脏数据。
function onChildTypeChange(child: JsonNode) {
  switch (child.type) {
    case 'string':
      // 保留 stringValue（如果之前从 string 切走再切回）
      break
    case 'number':
      if (!child.numberText) child.numberText = ''
      break
    case 'boolean':
      break
    case 'null':
      break
    case 'object':
      if (!child.objectChildren) child.objectChildren = []
      break
    case 'array':
      if (!child.arrayChildren) child.arrayChildren = []
      break
  }
  emitChange()
}

// ---------------- 外部值 → 内部树 同步 ----------------
// 仅顶层实例监听 modelValue：当 modelValue 从外部替换（例如清空、导入）时重建整棵树。
// 判等使用 JSON.stringify，避免因引用不同触发多次重建；差异检测已过滤自己 emit 的回环。
if (isRoot) {
  watch(
    () => props.modelValue,
    (nv) => {
      try {
        const current = JSON.stringify(nodeToValue(node))
        const next = JSON.stringify(nv ?? null)
        if (current === next) return
      } catch {
        // stringify 失败（例如循环引用）时直接重建。
      }
      const rebuilt = valueToNode(nv)
      node.type = rebuilt.type
      node.stringValue = rebuilt.stringValue
      node.numberText = rebuilt.numberText
      node.booleanValue = rebuilt.booleanValue
      node.objectChildren = rebuilt.objectChildren
      node.arrayChildren = rebuilt.arrayChildren
      node.__proxy = rebuilt.__proxy
      // 顶层 uid 保持不变即可（顶层实例不放到 v-for 里）。
    },
    { deep: false }
  )
}
</script>

<style scoped>
/* 保持自身尽可能"透明"，样式跟随 .input 和项目全局 tailwind 变量。 */
.json-tree-editor {
  width: 100%;
}
</style>
