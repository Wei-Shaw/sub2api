<template>
  <div class="chat-markdown min-w-0 max-w-full space-y-3 overflow-hidden">
    <template v-for="(part, index) in parts" :key="`${part.type}-${index}`">
      <div v-if="part.type === 'html'" v-html="part.html"></div>
      <div
        v-else
        data-testid="chat-code-block"
        class="group relative min-w-0 max-w-full overflow-hidden rounded-lg bg-gray-950 text-gray-50"
      >
        <div class="flex min-w-0 items-center justify-between gap-3 border-b border-white/10 px-3 py-2">
          <span class="min-w-0 truncate text-xs text-gray-400">{{ part.lang || t('chatCompletion.code') }}</span>
          <button
            type="button"
            class="rounded-md border border-white/10 bg-white/5 px-2 py-1 text-xs font-medium text-gray-200 opacity-80 transition hover:bg-white/10 hover:opacity-100"
            data-testid="copy-code-button"
            @click="copyCode(part.code)"
          >
            {{ t('chatCompletion.copyCode') }}
          </button>
        </div>
        <pre class="m-0 max-w-full overflow-x-auto p-3"><code class="block w-max min-w-full">{{ part.code }}</code></pre>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  content: string
}>()

type MarkdownPart =
  | { type: 'html'; html: string }
  | { type: 'code'; code: string; lang: string }

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const parts = computed<MarkdownPart[]>(() => {
  const tokens = marked.lexer(props.content || '')
  const result: MarkdownPart[] = []
  let htmlBuffer = ''

  for (const token of tokens) {
    if (token.type === 'code') {
      pushHtmlPart(result, htmlBuffer)
      htmlBuffer = ''
      result.push({
        type: 'code',
        code: token.text,
        lang: token.lang || '',
      })
      continue
    }
    htmlBuffer += marked.parser([token])
  }
  pushHtmlPart(result, htmlBuffer)
  return result
})

function pushHtmlPart(result: MarkdownPart[], html: string) {
  if (!html.trim()) return
  result.push({
    type: 'html',
    html: DOMPurify.sanitize(html),
  })
}

function copyCode(code: string) {
  void copyToClipboard(code)
}
</script>

<style scoped>
.chat-markdown :deep(p) {
  margin: 0.35rem 0;
}

.chat-markdown :deep(ul),
.chat-markdown :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.25rem;
}

.chat-markdown :deep(li + li) {
  margin-top: 0.25rem;
}

.chat-markdown :deep(code) {
  border-radius: 0.375rem;
  background: rgba(17, 24, 39, 0.08);
  padding: 0.125rem 0.25rem;
  font-size: 0.875em;
}

.chat-markdown :deep(a) {
  color: #2563eb;
  text-decoration: underline;
  text-underline-offset: 2px;
}
</style>
