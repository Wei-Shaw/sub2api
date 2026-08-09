<template>
  <div class="relative" ref="containerRef">
    <!-- Tags display -->
    <div class="flex flex-wrap gap-1.5 rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800 min-h-[2.5rem]">
      <span
        v-for="(model, idx) in models"
        :key="idx"
        class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm"
        :class="getPlatformTagClass(props.platform || '')"
      >
        {{ model }}
        <button
          type="button"
          @click="removeModel(idx)"
          class="ml-0.5 rounded-full p-0.5 hover:bg-primary-200 dark:hover:bg-primary-800"
        >
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        ref="inputRef"
        v-model="inputValue"
        type="text"
        class="flex-1 min-w-[120px] border-none bg-transparent text-sm outline-none placeholder:text-gray-400 dark:text-white"
        :placeholder="models.length === 0 ? placeholder : ''"
        @keydown.enter.prevent="addModel"
        @keydown.tab.prevent="addModel"
        @keydown.delete="handleBackspace"
        @keydown.esc="closeDropdown"
        @paste="handlePaste"
        @focus="openDropdown"
        @blur="onBlur"
      />
    </div>
    <p class="mt-1 text-xs text-gray-400">
      {{ hintText }}
    </p>

    <!-- 候选模型下拉建议：来源于当前渠道该平台分组下所有账号支持的模型集合。
         鼠标点击选中即添加为标签；仍可继续输入自由值并回车/Tab 添加。
         使用 Teleport 挂到 body + fixed 定位，避免被祖先 overflow:hidden（比如
         PricingEntryCard.collapsible-inner）裁掉底部选项。 -->
    <Teleport to="body">
      <div
        v-if="dropdownOpen && filteredCandidates.length > 0"
        :class="[instanceId, 'model-tag-input-dropdown']"
        :style="dropdownStyle"
        @mousedown.prevent
      >
        <button
          v-for="(candidate, idx) in filteredCandidates"
          :key="candidate"
          type="button"
          class="flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left text-sm hover:bg-primary-50 dark:hover:bg-primary-900/30"
          :class="{ 'bg-primary-50 dark:bg-primary-900/30': idx === activeIndex }"
          @mouseenter="activeIndex = idx"
          @click="selectCandidate(candidate)"
        >
          <span class="truncate text-gray-800 dark:text-gray-200">{{ candidate }}</span>
          <span class="flex-shrink-0 text-xs text-gray-400">
            {{ t('admin.channels.form.modelCandidateAdd', '添加') }}
          </span>
        </button>
        <div
          v-if="hasMoreCandidates"
          class="px-3 py-1 text-xs italic text-gray-400"
        >
          {{ moreCandidatesHint }}
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { getPlatformTagClass } from './types'

const { t } = useI18n()

// 唯一实例 ID，用于 click-outside 判定（Teleport 后下拉不在原容器 DOM 树里）。
const instanceId = `model-tag-input-${Math.random().toString(36).slice(2, 9)}`

const props = defineProps<{
  models: string[]
  placeholder?: string
  platform?: string
  /**
   * 候选模型清单：来自当前渠道该平台分组下所有账号 model_mapping 的 key。
   * 传空数组表示无候选源（此时下拉不展示，退化为纯自由输入）。
   */
  candidates?: string[]
}>()

const emit = defineEmits<{
  'update:models': [models: string[]]
}>()

const inputValue = ref('')
const containerRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement>()
const dropdownOpen = ref(false)
const activeIndex = ref(-1)

// Teleport 到 body 后需要用 fixed + JS 定位。下面几个 ref 记录容器位置与翻转方向。
const containerRect = ref<DOMRect | null>(null)
const dropdownPlacement = ref<'bottom' | 'top'>('bottom')
const dropdownViewportPadding = 8
const dropdownMaxHeight = 240 // 与旧样式 max-h-60 保持一致

// 下拉可视上限：候选可能上百，一次全渲染会有性能与滚动体验问题。
// 用户继续输入即可缩小到目标。
const MAX_VISIBLE_CANDIDATES = 50

// filteredCandidates: 未被选中的候选 ∩ 前缀/子串匹配输入值。
// 输入为空时展示全部（截取前 MAX_VISIBLE_CANDIDATES）。
const filteredCandidates = computed(() => {
  const src = props.candidates || []
  if (src.length === 0) return []
  const q = inputValue.value.trim().toLowerCase()
  const selected = new Set(props.models)
  const result: string[] = []
  for (const c of src) {
    if (selected.has(c)) continue
    if (q && !c.toLowerCase().includes(q)) continue
    result.push(c)
    if (result.length >= MAX_VISIBLE_CANDIDATES) break
  }
  return result
})

// 是否还有未展示的候选（提示用户可以继续输入过滤）。
const hasMoreCandidates = computed(() => {
  const src = props.candidates || []
  if (src.length === 0) return false
  const q = inputValue.value.trim().toLowerCase()
  const selected = new Set(props.models)
  let total = 0
  for (const c of src) {
    if (selected.has(c)) continue
    if (q && !c.toLowerCase().includes(q)) continue
    total++
    if (total > MAX_VISIBLE_CANDIDATES) return true
  }
  return false
})

// 提示语：有候选时提示"回车添加，或从下拉选择"；否则退回原提示。
const hintText = computed(() => {
  const hasCandidates = (props.candidates?.length ?? 0) > 0
  if (hasCandidates) {
    return t(
      'admin.channels.form.modelInputHintWithCandidates',
      '回车添加自定义值，或从下拉中选择账号已支持的模型；支持粘贴批量导入。'
    )
  }
  return t('admin.channels.form.modelInputHint', 'Press Enter to add, supports paste for batch import.')
})

// 候选溢出提示：i18n 两参 API 只能 (key, params) 或 (key, fallback)，
// 无法同时传 fallback+params，因此走本地字符串拼接。
const moreCandidatesHint = computed(() =>
  `仅显示前 ${MAX_VISIBLE_CANDIDATES} 项，可继续输入以缩小范围`
)

// dropdownStyle：根据容器位置计算 fixed 坐标。
// - left/宽度对齐输入容器
// - 底部空间不足且顶部够放时翻转到上方
// - z-index 取一个大值，穿透任意 modal 层叠上下文
const dropdownStyle = computed<Record<string, string>>(() => {
  if (!containerRect.value) return { display: 'none' }
  const rect = containerRect.value
  const viewportRight = Math.max(dropdownViewportPadding, window.innerWidth - dropdownViewportPadding)
  const left = Math.min(Math.max(dropdownViewportPadding, rect.left), viewportRight)
  const availableWidth = Math.max(0, viewportRight - left)
  const width = Math.min(rect.width, availableWidth)
  const style: Record<string, string> = {
    position: 'fixed',
    left: `${left}px`,
    width: `${width}px`,
    maxHeight: `${dropdownMaxHeight}px`,
    zIndex: '100000020',
  }
  if (dropdownPlacement.value === 'top') {
    style.bottom = `${window.innerHeight - rect.top + 4}px`
  } else {
    style.top = `${rect.bottom + 4}px`
  }
  return style
})

function updateContainerRect() {
  if (containerRef.value) {
    containerRect.value = containerRef.value.getBoundingClientRect()
  }
}

function recalcPlacement() {
  updateContainerRect()
  nextTick(() => {
    if (!containerRect.value) return
    const spaceBelow = window.innerHeight - containerRect.value.bottom
    const spaceAbove = containerRect.value.top
    if (spaceBelow < dropdownMaxHeight && spaceAbove > dropdownMaxHeight) {
      dropdownPlacement.value = 'top'
    } else {
      dropdownPlacement.value = 'bottom'
    }
  })
}

function addModel() {
  const val = inputValue.value.trim()
  if (!val) {
    // blur 时空值触发的 addModel 不应关闭下拉之外做别的事
    return
  }
  if (!props.models.includes(val)) {
    emit('update:models', [...props.models, val])
  }
  inputValue.value = ''
  activeIndex.value = -1
}

function removeModel(idx: number) {
  const newModels = [...props.models]
  newModels.splice(idx, 1)
  emit('update:models', newModels)
}

function handleBackspace() {
  if (inputValue.value === '' && props.models.length > 0) {
    removeModel(props.models.length - 1)
  }
}

function handlePaste(e: ClipboardEvent) {
  e.preventDefault()
  const text = e.clipboardData?.getData('text') || ''
  const items = text.split(/[,\n;]+/).map(s => s.trim()).filter(Boolean)
  if (items.length === 0) return
  const unique = [...new Set([...props.models, ...items])]
  emit('update:models', unique)
  inputValue.value = ''
}

function openDropdown() {
  dropdownOpen.value = true
  recalcPlacement()
}

function closeDropdown() {
  dropdownOpen.value = false
  activeIndex.value = -1
}

// blur 时先尝试提交自由输入，再关闭下拉。用 setTimeout 让 mousedown 上的
// selectCandidate 有机会先执行（dropdown 用了 @mousedown.prevent，
// 这里再兜一层）。
function onBlur() {
  addModel()
  setTimeout(() => {
    closeDropdown()
  }, 100)
}

function selectCandidate(candidate: string) {
  if (!props.models.includes(candidate)) {
    emit('update:models', [...props.models, candidate])
  }
  inputValue.value = ''
  activeIndex.value = -1
  // 选中后保持焦点在输入框，方便连续多选。
  inputRef.value?.focus()
}

// 打开时监听滚动/resize，跟随容器位置更新；关闭时解绑。
// 用 capture 保证嵌套滚动容器（弹窗内部滚动）的 scroll 事件也能触发。
function onWindowScroll() {
  if (!dropdownOpen.value) return
  updateContainerRect()
}

function onWindowResize() {
  if (!dropdownOpen.value) return
  recalcPlacement()
}

watch(dropdownOpen, (open) => {
  if (open) {
    window.addEventListener('scroll', onWindowScroll, { capture: true, passive: true })
    window.addEventListener('resize', onWindowResize)
  } else {
    window.removeEventListener('scroll', onWindowScroll, { capture: true })
    window.removeEventListener('resize', onWindowResize)
  }
})

// 输入变化时候选集合可能改变，可能需要重新翻转（比如候选变少了）
watch(inputValue, () => {
  if (dropdownOpen.value) recalcPlacement()
})

onUnmounted(() => {
  window.removeEventListener('scroll', onWindowScroll, { capture: true })
  window.removeEventListener('resize', onWindowResize)
})
</script>

<style scoped>
/* Teleport 到 body 的元素在 scoped 中无法命中，样式放到下面的非 scoped 块。 */
</style>

<style>
.model-tag-input-dropdown {
  overflow-y: auto;
  border-radius: 0.375rem;
  border-width: 1px;
  --tw-border-opacity: 1;
  border-color: rgb(229 231 235 / var(--tw-border-opacity));
  background-color: #fff;
  padding-top: 0.25rem;
  padding-bottom: 0.25rem;
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -4px rgba(0, 0, 0, 0.1);
}
.dark .model-tag-input-dropdown {
  border-color: rgb(51 65 85 / 1); /* 与 dark:border-dark-600 类近似 */
  background-color: rgb(30 41 59 / 1); /* 与 dark:bg-dark-800 近似 */
}
</style>
