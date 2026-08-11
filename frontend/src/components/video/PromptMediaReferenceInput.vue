<template>
  <div ref="rootEl" class="prompt-media-reference-input relative">
    <div
      ref="editorEl"
      class="prompt-media-editor w-full overflow-auto rounded border border-gray-300 bg-white px-3 py-2 text-sm leading-6 text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100"
      :class="disabled ? 'cursor-not-allowed bg-gray-100 opacity-70 dark:bg-gray-900' : ''"
      :contenteditable="disabled ? 'false' : 'true'"
      :style="{ minHeight: `${Math.max(3, rows) * 1.5 + 1}rem` }"
      role="textbox"
      aria-multiline="true"
      @input="onInput"
      @click="syncMentionState"
      @keyup="syncMentionState"
      @keydown="onKeydown"
      @paste="onPaste"
      @blur="onBlur"
    ></div>

    <div
      v-if="mentionOpen && filteredReferences.length > 0"
      data-testid="prompt-media-menu"
      class="absolute z-30 max-h-72 w-80 max-w-[calc(100vw-2rem)] overflow-auto rounded border border-gray-200 bg-white p-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
      :style="menuStyle"
    >
      <div v-for="group in referenceGroups" :key="group.kind" class="py-1">
        <div
          class="px-2 pb-1 text-[10px] font-semibold uppercase text-gray-400"
          :data-testid="`prompt-media-group-${group.kind}`"
        >
          {{ groupTitle(group.kind) }}
        </div>
        <button
          v-for="reference in group.references"
          :key="reference.label"
          type="button"
          class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs"
          :class="referenceIndex(reference) === activeIndex
            ? 'bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-200'
            : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-gray-700'"
          @mousedown.prevent="selectReference(reference)"
        >
          <img
            v-if="reference.kind === 'image'"
            :src="reference.url"
            alt=""
            class="h-10 w-10 shrink-0 rounded object-cover"
          />
          <video
            v-else-if="reference.kind === 'video'"
            :src="reference.url"
            muted
            preload="metadata"
            class="h-10 w-14 shrink-0 rounded bg-black object-cover"
          />
          <span
            v-else
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded bg-gray-100 text-[10px] font-semibold text-gray-500 dark:bg-gray-700 dark:text-gray-300"
          >
            AUDIO
          </span>
          <span class="min-w-0 flex-1">
            <span class="block truncate font-medium">{{ mediaFileName(reference.url) }}</span>
            <span class="block font-mono text-[10px] text-gray-400">{{ reference.label }}</span>
          </span>
        </button>
      </div>
    </div>

    <img
      v-if="hoveredImageUrl"
      data-testid="prompt-media-image-preview"
      :src="hoveredImageUrl"
      alt=""
      class="pointer-events-none fixed z-50 h-72 w-72 rounded border border-gray-200 bg-white object-contain p-1 shadow-xl dark:border-gray-700 dark:bg-gray-900"
      :style="hoveredImageStyle"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  PromptMediaKind,
  PromptMediaReference,
} from './promptMediaReferences'

const props = withDefaults(defineProps<{
  modelValue: unknown
  references: PromptMediaReference[]
  disabled?: boolean
  rows?: number
}>(), {
  disabled: false,
  rows: 3,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const { t } = useI18n()
const rootEl = ref<HTMLElement | null>(null)
const editorEl = ref<HTMLElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const mentionRange = ref<Range | null>(null)
const activeIndex = ref(0)
const menuLeft = ref(0)
const menuTop = ref(0)
const hoveredImageUrl = ref('')
const hoveredImageLeft = ref(0)
const hoveredImageTop = ref(0)

const stringValue = computed(() => typeof props.modelValue === 'string' ? props.modelValue : '')
const filteredReferences = computed(() => {
  const query = mentionQuery.value.toUpperCase()
  return props.references.filter((reference) => reference.label.slice(1).includes(query))
})
const referenceGroups = computed(() => {
  const kinds: PromptMediaKind[] = ['image', 'video', 'audio']
  return kinds
    .map((kind) => ({
      kind,
      references: filteredReferences.value.filter((reference) => reference.kind === kind),
    }))
    .filter((group) => group.references.length > 0)
})
const menuStyle = computed(() => ({
  left: `${menuLeft.value}px`,
  top: `${menuTop.value}px`,
}))
const hoveredImageStyle = computed(() => ({
  left: `${hoveredImageLeft.value}px`,
  top: `${hoveredImageTop.value}px`,
}))

function groupTitle(kind: PromptMediaKind): string {
  if (kind === 'video') return t('videoModels.playground.promptReferencesVideos')
  if (kind === 'audio') return t('videoModels.playground.promptReferencesAudio')
  return t('videoModels.playground.promptReferencesImages')
}

function mediaFileName(url: string): string {
  try {
    const path = new URL(url).pathname
    const name = path.split('/').filter(Boolean).pop()
    return name ? decodeURIComponent(name) : url
  } catch {
    const clean = url.split(/[?#]/, 1)[0]
    return clean.split('/').filter(Boolean).pop() || url
  }
}

function referenceIndex(reference: PromptMediaReference): number {
  return filteredReferences.value.findIndex((item) => item.label === reference.label)
}

function showImagePreview(url: string, token: HTMLElement) {
  const previewSize = 288
  const viewportGap = 8
  const tokenRect = token.getBoundingClientRect()
  const maxLeft = Math.max(viewportGap, window.innerWidth - previewSize - viewportGap)
  const maxTop = Math.max(viewportGap, window.innerHeight - previewSize - viewportGap)
  hoveredImageLeft.value = Math.min(Math.max(viewportGap, tokenRect.left), maxLeft)
  const preferredTop = tokenRect.top >= previewSize + viewportGap
    ? tokenRect.top - previewSize - viewportGap
    : tokenRect.bottom + viewportGap
  hoveredImageTop.value = Math.min(Math.max(viewportGap, preferredTop), maxTop)
  hoveredImageUrl.value = url
}

function hideImagePreview() {
  hoveredImageUrl.value = ''
}

function createReferenceToken(reference: PromptMediaReference): HTMLElement {
  const token = document.createElement('span')
  token.className = 'media-reference-token'
  token.contentEditable = 'false'
  token.dataset.mediaReference = reference.label

  if (reference.kind === 'image') {
    const image = document.createElement('img')
    image.src = reference.url
    image.alt = ''
    token.append(image)
    token.addEventListener('mouseenter', () => showImagePreview(reference.url, token))
    token.addEventListener('mouseleave', hideImagePreview)
  } else if (reference.kind === 'video') {
    const video = document.createElement('video')
    video.src = reference.url
    video.muted = true
    video.preload = 'metadata'
    token.append(video)
  } else {
    const mediaBadge = document.createElement('span')
    mediaBadge.className = 'media-reference-audio'
    mediaBadge.textContent = 'AUDIO'
    token.append(mediaBadge)
  }

  const alias = document.createElement('span')
  alias.className = 'media-reference-alias'
  alias.textContent = reference.label
  token.append(alias)
  return token
}

function appendPlainText(container: HTMLElement, value: string) {
  if (value) container.append(document.createTextNode(value))
}

function renderEditorValue() {
  const editor = editorEl.value
  if (!editor) return
  hideImagePreview()
  editor.replaceChildren()

  const byLabel = new Map(props.references.map((reference) => [reference.label, reference]))
  const parts = stringValue.value.split(/(@(?:IMAGE|VIDEO|AUDIO)\d+)/g)
  for (const part of parts) {
    const reference = byLabel.get(part)
    if (reference) editor.append(createReferenceToken(reference))
    else appendPlainText(editor, part)
  }
}

function serializeNode(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? ''
  if (!(node instanceof HTMLElement)) return ''
  if (node.dataset.mediaReference) return node.dataset.mediaReference
  if (node.tagName === 'BR') return '\n'
  const value = Array.from(node.childNodes).map(serializeNode).join('')
  return node.tagName === 'DIV' || node.tagName === 'P' ? `${value}\n` : value
}

function serializeEditor(): string {
  const editor = editorEl.value
  if (!editor) return ''
  return Array.from(editor.childNodes).map(serializeNode).join('').replace(/\n$/, '')
}

function emitEditorValue() {
  emit('update:modelValue', serializeEditor())
}

function positionMenuAtCaret(range: Range) {
  const root = rootEl.value
  const editor = editorEl.value
  if (!root || !editor) return
  const rangeRect = typeof range.getBoundingClientRect === 'function'
    ? range.getBoundingClientRect()
    : editor.getBoundingClientRect()
  const rootRect = root.getBoundingClientRect()
  const editorRect = editor.getBoundingClientRect()
  const rawLeft = rangeRect.right - rootRect.left + 4
  const maxLeft = Math.max(0, rootRect.width - 320)
  menuLeft.value = Math.max(0, Math.min(rawLeft, maxLeft))
  menuTop.value = Math.max(0, (rangeRect.bottom || editorRect.top + 24) - rootRect.top + 6)
}

function detectMention() {
  const editor = editorEl.value
  const selection = window.getSelection()
  if (!editor || !selection || selection.rangeCount === 0 || !selection.isCollapsed) {
    mentionOpen.value = false
    return
  }
  const range = selection.getRangeAt(0)
  const node = range.startContainer
  if (!editor.contains(node) || node.nodeType !== Node.TEXT_NODE) {
    mentionOpen.value = false
    return
  }
  const text = node.textContent ?? ''
  const cursor = range.startOffset
  const match = text.slice(0, cursor).match(/@([A-Za-z0-9_]*)$/)
  if (!match || props.references.length === 0) {
    mentionOpen.value = false
    return
  }

  const query = match[1]
  const start = cursor - query.length - 1
  const replacementRange = document.createRange()
  replacementRange.setStart(node, start)
  replacementRange.setEnd(node, cursor)
  mentionRange.value = replacementRange
  mentionQuery.value = query
  activeIndex.value = 0
  mentionOpen.value = true
  positionMenuAtCaret(range)
}

function onInput() {
  emitEditorValue()
  detectMention()
}

function syncMentionState() {
  detectMention()
}

function selectReference(reference: PromptMediaReference) {
  const range = mentionRange.value
  if (!range) return
  range.deleteContents()
  const token = createReferenceToken(reference)
  const spacer = document.createTextNode(' ')
  const fragment = document.createDocumentFragment()
  fragment.append(token, spacer)
  range.insertNode(fragment)

  const selection = window.getSelection()
  const nextRange = document.createRange()
  nextRange.setStartAfter(spacer)
  nextRange.collapse(true)
  selection?.removeAllRanges()
  selection?.addRange(nextRange)
  mentionOpen.value = false
  mentionRange.value = null
  emitEditorValue()
  editorEl.value?.focus()
}

function onKeydown(event: KeyboardEvent) {
  if (!mentionOpen.value || filteredReferences.value.length === 0) return
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    activeIndex.value = (activeIndex.value + 1) % filteredReferences.value.length
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value =
      (activeIndex.value - 1 + filteredReferences.value.length) % filteredReferences.value.length
  } else if (event.key === 'Enter' || event.key === 'Tab') {
    event.preventDefault()
    selectReference(filteredReferences.value[activeIndex.value])
  } else if (event.key === 'Escape') {
    event.preventDefault()
    mentionOpen.value = false
  }
}

function onPaste(event: ClipboardEvent) {
  event.preventDefault()
  const text = event.clipboardData?.getData('text/plain') ?? ''
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0) return
  const range = selection.getRangeAt(0)
  range.deleteContents()
  const textNode = document.createTextNode(text)
  range.insertNode(textNode)
  range.setStartAfter(textNode)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  emitEditorValue()
  detectMention()
}

function onBlur() {
  window.setTimeout(() => {
    mentionOpen.value = false
  }, 100)
}

onMounted(renderEditorValue)

watch(
  () => props.modelValue,
  () => {
    if (serializeEditor() !== stringValue.value) renderEditorValue()
  },
)

watch(
  () => props.references,
  () => {
    if (document.activeElement !== editorEl.value) renderEditorValue()
  },
  { deep: true },
)
</script>

<style scoped>
.prompt-media-editor {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.prompt-media-editor :deep(.media-reference-token) {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  max-width: min(100%, 20rem);
  margin: 0 0.15rem;
  padding: 0.2rem 0.4rem 0.2rem 0.2rem;
  vertical-align: middle;
  border: 1px solid rgb(191 219 254);
  border-radius: 0.375rem;
  background: rgb(239 246 255);
  color: rgb(30 64 175);
}

.prompt-media-editor :deep(.media-reference-token img),
.prompt-media-editor :deep(.media-reference-token video) {
  width: 2rem;
  height: 2rem;
  flex: none;
  border-radius: 0.25rem;
  background: #000;
  object-fit: cover;
}

.prompt-media-editor :deep(.media-reference-audio) {
  display: inline-flex;
  width: 2rem;
  height: 2rem;
  align-items: center;
  justify-content: center;
  flex: none;
  border-radius: 0.25rem;
  background: rgb(229 231 235);
  font-size: 0.55rem;
  font-weight: 700;
  color: rgb(75 85 99);
}

.prompt-media-editor :deep(.media-reference-alias) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.7rem;
  font-weight: 700;
}

.dark .prompt-media-editor :deep(.media-reference-token) {
  border-color: rgb(30 58 138);
  background: rgb(23 37 84 / 0.65);
  color: rgb(191 219 254);
}

</style>
