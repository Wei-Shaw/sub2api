<template>
  <div
    ref="hostRef"
    class="js-code-editor overflow-hidden rounded-xl border border-gray-300 dark:border-dark-600"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Compartment, EditorState } from '@codemirror/state'
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  drawSelection,
  dropCursor,
  rectangularSelection,
  crosshairCursor,
} from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import {
  bracketMatching,
  defaultHighlightStyle,
  foldGutter,
  foldKeymap,
  indentOnInput,
  syntaxHighlighting,
} from '@codemirror/language'
import { javascript } from '@codemirror/lang-javascript'
import { oneDark } from '@codemirror/theme-one-dark'

const props = withDefaults(
  defineProps<{
    modelValue: string
    readonly?: boolean
    minHeight?: string
  }>(),
  {
    readonly: false,
    minHeight: 'min(50vh, 420px)',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const hostRef = ref<HTMLElement | null>(null)
let view: EditorView | null = null
let syncing = false
let currentDark = false
const themeCompartment = new Compartment()
const editableCompartment = new Compartment()

function isDarkMode(): boolean {
  return document.documentElement.classList.contains('dark')
}

function layoutTheme(dark: boolean) {
  return EditorView.theme(
    {
      '&': {
        minHeight: props.minHeight,
        fontSize: '13px',
      },
      '.cm-scroller': {
        overflow: 'auto',
        maxHeight: 'min(60vh, 520px)',
        fontFamily:
          'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
      },
      '.cm-content': {
        minHeight: props.minHeight,
        padding: '12px 0',
      },
      '&.cm-focused': {
        outline: 'none',
      },
    },
    { dark },
  )
}

function themeExtensions(dark: boolean) {
  return dark ? [layoutTheme(true), oneDark] : [layoutTheme(false)]
}

function editableExtensions(readonly: boolean) {
  return [EditorView.editable.of(!readonly), EditorState.readOnly.of(!!readonly)]
}

function baseExtensions() {
  return [
    lineNumbers(),
    highlightActiveLine(),
    highlightActiveLineGutter(),
    foldGutter(),
    drawSelection(),
    dropCursor(),
    rectangularSelection(),
    crosshairCursor(),
    history(),
    indentOnInput(),
    bracketMatching(),
    javascript(),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    keymap.of([...defaultKeymap, ...historyKeymap, ...foldKeymap, indentWithTab]),
    themeCompartment.of(themeExtensions(currentDark)),
    editableCompartment.of(editableExtensions(!!props.readonly)),
    EditorView.updateListener.of((update) => {
      if (!update.docChanged || syncing) return
      emit('update:modelValue', update.state.doc.toString())
    }),
  ]
}

function createEditor() {
  if (!hostRef.value) return
  if (view) {
    view.destroy()
    view = null
  }
  currentDark = isDarkMode()
  view = new EditorView({
    parent: hostRef.value,
    state: EditorState.create({
      doc: props.modelValue || '',
      extensions: baseExtensions(),
    }),
  })
}

function setDoc(value: string) {
  if (!view) return
  const current = view.state.doc.toString()
  if (current === value) return
  syncing = true
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: value || '' },
  })
  syncing = false
}

function applyDarkTheme(dark: boolean) {
  if (!view || dark === currentDark) return
  currentDark = dark
  view.dispatch({
    effects: themeCompartment.reconfigure(themeExtensions(dark)),
  })
}

function applyReadonly(readonly: boolean) {
  if (!view) return
  view.dispatch({
    effects: editableCompartment.reconfigure(editableExtensions(readonly)),
  })
}

let darkObserver: MutationObserver | null = null

onMounted(() => {
  createEditor()
  darkObserver = new MutationObserver(() => {
    applyDarkTheme(isDarkMode())
  })
  darkObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })
})

onBeforeUnmount(() => {
  darkObserver?.disconnect()
  darkObserver = null
  view?.destroy()
  view = null
})

watch(
  () => props.modelValue,
  (val) => {
    setDoc(val || '')
  },
)

watch(
  () => props.readonly,
  (readonly) => {
    applyReadonly(!!readonly)
  },
)
</script>

<style scoped>
.js-code-editor :deep(.cm-editor) {
  background: transparent;
}
.js-code-editor :deep(.cm-editor.cm-focused) {
  outline: none;
}
</style>
