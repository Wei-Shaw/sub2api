<template>
  <AppLayout>
    <div class="mx-auto max-w-[1680px] space-y-6">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">教程文档</h1>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            固定公开路径为 <span class="font-mono">/docs/tutorial</span>。这里使用富文本工具栏维护完整教程内容，适合软件接入、命令示例和图文说明。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <a href="/docs/tutorial" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">打开公开页</a>
          <button type="button" class="btn btn-primary" :disabled="saving || loading" @click="saveDocument">
            {{ saving ? '保存中...' : '保存教程文档' }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-10 w-10 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <div v-else class="grid gap-6 xl:grid-cols-[300px_minmax(0,1fr)_minmax(0,1fr)]">
        <aside class="space-y-4">
          <div class="card p-4">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h2 class="text-sm font-semibold text-gray-900 dark:text-white">教程图片</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  文件会保存到 <span class="font-mono">data/pages/tutorial/</span>
                </p>
              </div>
              <label class="btn btn-secondary btn-sm cursor-pointer">
                <input
                  ref="fileInput"
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/gif"
                  multiple
                  class="hidden"
                  @change="handleFileSelect"
                />
                上传图片
              </label>
            </div>
            <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
              支持多图上传、拖拽到编辑区、以及直接粘贴图片。上传后会自动插入到当前光标位置。
            </p>
            <p v-if="uploadError" class="mt-3 text-xs text-red-500">{{ uploadError }}</p>
          </div>

          <div class="card p-4">
            <div class="flex items-center justify-between">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">图片列表</h2>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ images.length }} 张</span>
            </div>
            <div v-if="images.length === 0" class="mt-4 rounded-lg border border-dashed border-gray-300 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-300">
              还没有上传图片
            </div>
            <div v-else class="mt-4 space-y-3">
              <div
                v-for="image in images"
                :key="image.name"
                class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
              >
                <img
                  :src="image.url"
                  :alt="image.name"
                  class="h-28 w-full rounded-md object-contain bg-gray-50 dark:bg-dark-800"
                />
                <p class="mt-3 truncate text-sm font-medium text-gray-900 dark:text-white">{{ image.name }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ formatBytes(image.size) }}</p>
                <div class="mt-3 flex gap-2">
                  <button type="button" class="btn btn-secondary btn-sm flex-1" @click="insertImage(image.url)">
                    插入图片
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm flex-1" @click="copyImageURL(image.url)">
                    复制地址
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="card p-4">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">编辑提示</h2>
            <ul class="mt-3 space-y-2 text-sm text-gray-600 dark:text-dark-300">
              <li>工具栏支持标题、加粗、颜色、链接、代码块、对齐等能力</li>
              <li>右侧预览区会显示最终公开页内容效果</li>
              <li>代码块预览会自动带复制按钮</li>
            </ul>
          </div>
        </aside>

        <section class="card flex min-h-[820px] flex-col overflow-hidden">
          <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <div class="flex flex-wrap items-center gap-2">
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('heading', { level: 1 }) }" @click="toggleHeading(1)">H1</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('heading', { level: 2 }) }" @click="toggleHeading(2)">H2</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('heading', { level: 3 }) }" @click="toggleHeading(3)">H3</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('bold') }" @click="editor?.chain().focus().toggleBold().run()">加粗</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('italic') }" @click="editor?.chain().focus().toggleItalic().run()">斜体</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('underline') }" @click="editor?.chain().focus().toggleUnderline().run()">下划线</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('bulletList') }" @click="editor?.chain().focus().toggleBulletList().run()">列表</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('orderedList') }" @click="editor?.chain().focus().toggleOrderedList().run()">编号</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('blockquote') }" @click="editor?.chain().focus().toggleBlockquote().run()">引用</button>
              <button type="button" class="editor-btn" :class="{ 'editor-btn-active': editor?.isActive('codeBlock') }" @click="editor?.chain().focus().toggleCodeBlock().run()">代码块</button>
              <button type="button" class="editor-btn" @click="insertLink">链接</button>
              <label class="editor-color">
                <span>颜色</span>
                <input type="color" :value="currentColor" @input="setColor(($event.target as HTMLInputElement).value)" />
              </label>
              <button type="button" class="editor-btn" @click="setAlign('left')">左对齐</button>
              <button type="button" class="editor-btn" @click="setAlign('center')">居中</button>
              <button type="button" class="editor-btn" @click="setAlign('right')">右对齐</button>
              <button type="button" class="editor-btn" @click="setAlign('justify')">两端</button>
              <button type="button" class="editor-btn" @click="clearFormatting">清除格式</button>
            </div>
          </div>
          <div
            class="relative flex-1 overflow-hidden"
            :class="{ 'ring-2 ring-primary-400 ring-offset-2 ring-offset-white dark:ring-offset-dark-900': dragActive }"
            @dragenter.prevent="dragActive = true"
            @dragover.prevent="dragActive = true"
            @dragleave.prevent="dragActive = false"
            @drop.prevent="handleDrop"
            @paste="handlePaste"
          >
            <EditorContent v-if="editor" :editor="editor as any" class="tutorial-editor h-full overflow-auto p-6" />
            <div
              v-if="dragActive"
              class="pointer-events-none absolute inset-0 flex items-center justify-center bg-primary-50/90 text-sm font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-200"
            >
              松开鼠标即可上传图片并插入文档
            </div>
          </div>
        </section>

        <section class="card flex min-h-[820px] flex-col overflow-hidden">
          <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <div class="flex items-center justify-between">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">实时预览</h2>
              <span class="text-xs text-gray-500 dark:text-gray-400">公开页效果预览</span>
            </div>
          </div>
          <div
            ref="previewRef"
            class="tutorial-preview flex-1 overflow-auto p-6 md:p-8"
            v-html="previewHTML"
          ></div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, shallowRef, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import pagesAPI, { type AdminPageImage } from '@/api/admin/pages'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import DOMPurify from 'dompurify'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Underline from '@tiptap/extension-underline'
import { TextStyle } from '@tiptap/extension-text-style'
import Color from '@tiptap/extension-color'
import Image from '@tiptap/extension-image'
import TextAlign from '@tiptap/extension-text-align'
import Placeholder from '@tiptap/extension-placeholder'

const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loading = ref(true)
const saving = ref(false)
const dragActive = ref(false)
const uploadError = ref('')
const images = ref<AdminPageImage[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const previewRef = ref<HTMLElement | null>(null)
const previewHTML = ref('')
const currentColor = ref('#111827')
const editor = shallowRef<Editor | null>(null)

function normalizeHTML(raw: string) {
  return DOMPurify.sanitize(raw, {
    ADD_ATTR: ['style', 'target', 'rel', 'class'],
  })
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(2)} MB`
}

function updatePreview() {
  const html = editor.value?.getHTML() || ''
  previewHTML.value = normalizeHTML(html)
}

async function loadDocument() {
  loading.value = true
  try {
    const [doc, imageList] = await Promise.all([
      pagesAPI.getTutorialDocument(),
      pagesAPI.listPageImages('tutorial'),
    ])
    editor.value?.commands.setContent(doc.content_html || '')
    images.value = imageList
    uploadError.value = ''
    updatePreview()
  } finally {
    loading.value = false
  }
}

async function saveDocument() {
  saving.value = true
  try {
    const html = editor.value?.getHTML() || ''
    const result = await pagesAPI.updateTutorialDocument(html)
    editor.value?.commands.setContent(result.content_html || '')
    updatePreview()
    appStore.showSuccess('教程文档已保存')
  } catch (error: unknown) {
    appStore.showError(String((error as { message?: string })?.message || '保存失败'))
  } finally {
    saving.value = false
  }
}

function insertImage(url: string) {
  editor.value?.chain().focus().setImage({ src: url }).run()
  updatePreview()
}

async function copyImageURL(url: string) {
  await copyToClipboard(url, '图片地址已复制')
}

async function uploadFiles(files: FileList | File[]) {
  uploadError.value = ''
  const list = Array.from(files).filter((file) => file.type.startsWith('image/'))
  if (list.length === 0) {
    uploadError.value = '未检测到可上传的图片文件'
    return
  }
  for (const file of list) {
    try {
      const image = await pagesAPI.uploadPageImage('tutorial', file)
      editor.value?.chain().focus().setImage({ src: image.url, alt: image.name }).run()
    } catch (error: unknown) {
      uploadError.value = String((error as { message?: string })?.message || '图片上传失败')
      appStore.showError(uploadError.value)
      break
    }
  }
  images.value = await pagesAPI.listPageImages('tutorial')
  updatePreview()
}

async function handleFileSelect(event: Event) {
  const input = event.target as HTMLInputElement
  const files = input.files
  if (files && files.length > 0) {
    await uploadFiles(files)
  }
  input.value = ''
}

async function handlePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.items ?? [])
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((file): file is File => !!file)
  if (files.length === 0) {
    return
  }
  event.preventDefault()
  await uploadFiles(files)
}

async function handleDrop(event: DragEvent) {
  dragActive.value = false
  const files = Array.from(event.dataTransfer?.files ?? [])
  if (files.length === 0) {
    return
  }
  await uploadFiles(files)
}

function toggleHeading(level: 1 | 2 | 3) {
  editor.value?.chain().focus().toggleHeading({ level }).run()
  updatePreview()
}

function setColor(color: string) {
  currentColor.value = color
  editor.value?.chain().focus().setColor(color).run()
  updatePreview()
}

function setAlign(alignment: 'left' | 'center' | 'right' | 'justify') {
  editor.value?.chain().focus().setTextAlign(alignment).run()
  updatePreview()
}

function clearFormatting() {
  editor.value?.chain().focus().unsetAllMarks().clearNodes().run()
  updatePreview()
}

function insertLink() {
  const url = window.prompt('请输入链接地址')
  if (!url) return
  editor.value?.chain().focus().extendMarkRange('link').setLink({ href: url, target: '_blank', rel: 'noopener noreferrer nofollow' }).run()
  updatePreview()
}

function injectCopyButtons() {
  const root = previewRef.value
  if (!root) return
  root.querySelectorAll('pre').forEach((pre) => {
    if (pre.querySelector('.copy-btn')) return
    const btn = document.createElement('button')
    btn.className = 'copy-btn'
    btn.textContent = '复制'
    btn.addEventListener('click', async () => {
      const code = pre.querySelector('code')?.textContent ?? pre.textContent ?? ''
      try {
        await navigator.clipboard.writeText(code)
        btn.textContent = '已复制'
        setTimeout(() => { btn.textContent = '复制' }, 1500)
      } catch {
        btn.textContent = '失败'
        setTimeout(() => { btn.textContent = '复制' }, 1500)
      }
    })
    pre.appendChild(btn)
  })
}

watch(previewHTML, async () => {
  await nextTick()
  injectCopyButtons()
}, { immediate: true })

onMounted(() => {
  editor.value = new Editor({
    extensions: [
      StarterKit,
      Link.configure({ openOnClick: false, autolink: true }),
      Underline,
      TextStyle,
      Color,
      Image.configure({ inline: false }),
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      Placeholder.configure({ placeholder: '在这里编写教程文档内容…' }),
    ],
    content: '',
    onUpdate: () => updatePreview(),
  })
  loadDocument()
})

onBeforeUnmount(() => {
  editor.value?.destroy()
})
</script>

<style scoped>
.editor-btn {
  @apply rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm text-gray-700 transition hover:border-primary-300 hover:text-primary-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:border-primary-500 dark:hover:text-primary-300;
}

.editor-btn-active {
  @apply border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300;
}

.editor-color {
  @apply inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200;
}

.editor-color input {
  @apply h-7 w-8 cursor-pointer border-0 bg-transparent p-0;
}

.tutorial-editor :deep(.tiptap) {
  @apply min-h-[720px] outline-none text-gray-800 dark:text-dark-100 leading-7;
}

.tutorial-editor :deep(.tiptap h1) { @apply text-3xl font-bold mt-8 mb-4; }
.tutorial-editor :deep(.tiptap h2) { @apply text-2xl font-bold mt-6 mb-3; }
.tutorial-editor :deep(.tiptap h3) { @apply text-xl font-semibold mt-5 mb-2; }
.tutorial-editor :deep(.tiptap p) { @apply mb-4; }
.tutorial-editor :deep(.tiptap ul) { @apply list-disc pl-6 mb-4; }
.tutorial-editor :deep(.tiptap ol) { @apply list-decimal pl-6 mb-4; }
.tutorial-editor :deep(.tiptap blockquote) { @apply border-l-4 border-gray-300 dark:border-dark-500 pl-4 italic text-gray-600 dark:text-dark-300 my-4; }
.tutorial-editor :deep(.tiptap code) { @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-700; }
.tutorial-editor :deep(.tiptap pre) { @apply my-4 rounded-xl bg-gray-900 p-4 text-gray-100 overflow-x-auto; }
.tutorial-editor :deep(.tiptap pre code) { @apply bg-transparent p-0 text-inherit; }
.tutorial-editor :deep(.tiptap img) { @apply my-5 h-auto max-w-full rounded-lg border border-gray-200 dark:border-dark-700; }

.tutorial-preview {
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.tutorial-preview :deep(h1) { @apply mb-4 mt-8 border-b border-gray-200 pb-3 text-3xl font-bold dark:border-dark-700; }
.tutorial-preview :deep(h2) { @apply mb-3 mt-7 text-2xl font-bold; }
.tutorial-preview :deep(h3) { @apply mb-2 mt-6 text-xl font-semibold; }
.tutorial-preview :deep(p) { @apply mb-4 text-gray-700 dark:text-dark-200; }
.tutorial-preview :deep(a) { @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200; }
.tutorial-preview :deep(ul) { @apply mb-4 list-disc pl-6; }
.tutorial-preview :deep(ol) { @apply mb-4 list-decimal pl-6; }
.tutorial-preview :deep(img) { @apply my-5 h-auto max-w-full rounded-lg border border-gray-200 dark:border-dark-700; }
.tutorial-preview :deep(code) { @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-700; }
.tutorial-preview :deep(pre) { @apply relative my-4 overflow-x-auto rounded-xl bg-gray-900 p-4 text-gray-100; }
.tutorial-preview :deep(pre code) { @apply bg-transparent p-0 text-inherit; }
.tutorial-preview :deep(.copy-btn) {
  position: absolute;
  top: 10px;
  right: 10px;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid rgba(255,255,255,.16);
  background: rgba(255,255,255,.12);
  color: #e5e7eb;
  font-size: 12px;
  cursor: pointer;
}
</style>

