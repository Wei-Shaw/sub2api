<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import GeneratedImageCard from './GeneratedImageCard.vue'
import type { Conversation, GeneratedImage, ImageReference } from '../types'

const props = defineProps<{ conversation: Conversation | null; loading?: boolean; hasOlder?: boolean }>()
const emit = defineEmits<{ reference: [image: GeneratedImage]; referenceImage: [image: ImageReference]; refine: [image: GeneratedImage]; refinePrompt: [prompt: string]; repeat: [image: GeneratedImage, prompt: string]; retry: [messageId: string]; view: [src: string, alt: string]; loadOlder: [] }>()
const thread = ref<HTMLElement | null>(null)
const awaitingOlderMessages = ref(false)
const pendingInitialScroll = ref(true)
const loadedReferenceImages = ref(new Set<string>())
let previousScrollHeight = 0
let previousScrollTop = 0
let referenceObserver: IntersectionObserver | undefined

function referenceImageSource(id: string, source: string): string | undefined {
  return loadedReferenceImages.value.has(id) ? source : undefined
}

function observeReferenceImage(element: HTMLImageElement | null, id: string): void {
  if (!element || !referenceObserver || loadedReferenceImages.value.has(id)) return
  referenceObserver.observe(element)
}

onMounted(() => {
  if (typeof window.IntersectionObserver !== 'function') {
    loadedReferenceImages.value = new Set((props.conversation?.messages ?? []).flatMap(message => message.referenceImages?.map(reference => reference.id) ?? []))
    return
  }
  referenceObserver = new IntersectionObserver(entries => {
    const loaded = new Set(loadedReferenceImages.value)
    for (const entry of entries) {
      if (!entry.isIntersecting) continue
      const id = (entry.target as HTMLElement).dataset.referenceImageId
      if (id) loaded.add(id)
      referenceObserver?.unobserve(entry.target)
    }
    loadedReferenceImages.value = loaded
  }, { root: thread.value, rootMargin: '300px 0px' })
})

onBeforeUnmount(() => referenceObserver?.disconnect())

async function scrollToLatestMessage(): Promise<void> {
  await nextTick()
  const element = thread.value
  if (element) element.scrollTop = element.scrollHeight
  pendingInitialScroll.value = false
}

function loadOlderNearTop(): void {
  const element = thread.value
  if (!element || element.scrollTop > 48 || !props.hasOlder || props.loading || awaitingOlderMessages.value) return
  previousScrollHeight = element.scrollHeight
  previousScrollTop = element.scrollTop
  awaitingOlderMessages.value = true
  emit('loadOlder')
}

watch(() => props.loading, async (loading, wasLoading) => {
  if (!loading && wasLoading && awaitingOlderMessages.value) {
    await nextTick()
    const element = thread.value
    if (element) element.scrollTop = previousScrollTop + Math.max(0, element.scrollHeight - previousScrollHeight)
    awaitingOlderMessages.value = false
    return
  }
  if (!loading && pendingInitialScroll.value) await scrollToLatestMessage()
})

watch(() => props.conversation?.id, async (id, previousId) => {
  if (!id || id === previousId) return
  pendingInitialScroll.value = true
  if (!props.loading) await scrollToLatestMessage()
}, { immediate: true })

function formatModelLabel(value: string): string {
  return ({ 'gpt-image-2': 'GPT Image 2', 'gpt-image-1': 'GPT Image 1', 'gemini-2.5-flash-image': 'Gemini 2.5 Flash Image' } as Record<string, string>)[value] ?? value
}

function formatSizeLabel(value: string): string {
  return value.replace('x', ' × ')
}
</script>

<template>
  <div ref="thread" class="chat-thread" data-testid="image-chat-thread" aria-live="polite" @scroll="loadOlderNearTop">
    <div v-if="!conversation?.messages.length" class="empty-state">
      <h1>No conversation yet</h1>
      <p>Send the first prompt and generated images will appear here.</p>
    </div>
    <div v-else class="message-list">
      <article v-for="message in conversation.messages" :key="message.id" class="message" :class="[`message-${message.role}`, { failed: message.status === 'failed' }]">
        <div class="avatar" aria-hidden="true">{{ message.role === 'user' ? 'U' : 'AI' }}</div>
        <div class="message-column">
          <span class="message-role">{{ message.role === 'user' ? 'Prompt' : 'Assistant' }}</span>
          <div class="message-body" :aria-busy="message.status === 'pending'">
          <div v-if="message.referenceImages?.length" class="reference-list">
            <div v-for="reference in message.referenceImages" :key="reference.id" class="reference-item" data-testid="history-reference-item">
              <button type="button" class="reference-open" aria-label="查看参考图原图" @click="$emit('view', reference.originalDataUrl || reference.dataUrl, reference.fileName)">
                <img
                  :ref="element => observeReferenceImage(element as HTMLImageElement | null, reference.id)"
                  :data-reference-image-id="reference.id"
                  :src="referenceImageSource(reference.id, reference.dataUrl)"
                  :alt="reference.fileName"
                  data-testid="user-message-reference-image"
                >
              </button>
              <div class="reference-actions">
                <button type="button" class="reference-use-button" data-testid="history-reference-action" @click="$emit('referenceImage', reference)">设为参考图</button>
                <button type="button" class="reference-use-button" data-testid="history-refine-action" @click="$emit('refinePrompt', message.content)">优化提示词</button>
              </div>
            </div>
          </div>
          <div v-if="message.role === 'user'" class="user-message-details">
            <section>
              <h3>创作描述</h3>
              <p>{{ message.content }}</p>
            </section>
            <section v-if="message.requestSettings?.length">
              <h3>生成参数</h3>
              <div class="request-settings">
                <span v-for="(setting, index) in message.requestSettings" :key="`${setting.modelLabel}-${index}`">
                  {{ formatModelLabel(setting.modelLabel) }} | {{ formatSizeLabel(setting.sizeLabel) }} | {{ setting.countLabel.startsWith('数量:') ? setting.countLabel : `数量: ${setting.countLabel}` }}<template v-if="setting.detailsLabel"> | {{ setting.detailsLabel }}</template>
                </span>
              </div>
            </section>
          </div>
          <p v-else>{{ message.content }}</p>
          <time>{{ message.createdAt }}</time>
          <div
            v-if="message.images?.length || message.generationSlots?.length"
            class="image-grid"
            :class="{ 'single-image': (message.generationSlots?.length ?? message.images?.length) === 1 }"
            data-testid="message-attachments"
          >
            <template v-if="message.generationSlots?.length">
              <template v-for="slot in message.generationSlots" :key="slot.id">
                <GeneratedImageCard
                  v-if="slot.image"
                  :image="slot.image"
                  :scroll-root="thread"
                  @reference="$emit('reference', $event)"
                  @refine="$emit('refine', $event)"
                  @repeat="$emit('repeat', $event, message.content)"
                  @view="$emit('view', $event.originalSrc || $event.src, $event.revisedPrompt || '生成图片')"
                />
                <figure v-else class="generation-slot" :class="`generation-slot-${slot.status}`" data-testid="generation-slot">
                  <div class="generation-slot-label">{{ slot.label || '图片' }}</div>
                  <template v-if="slot.status === 'pending'">
                    <div class="generation-progress" :style="{ '--generation-progress': `${slot.progress}%` }" aria-hidden="true" />
                    <strong>{{ slot.progress }}%</strong>
                    <figcaption>正在生成</figcaption>
                  </template>
                  <template v-else>
                    <strong>{{ slot.status === 'canceled' ? '已取消' : '生成失败' }}</strong>
                    <figcaption>{{ slot.error || '图片生成失败' }}</figcaption>
                  </template>
                </figure>
              </template>
            </template>
            <GeneratedImageCard
              v-else
              v-for="image in message.images"
              :key="image.id"
              :image="image"
              :scroll-root="thread"
              @reference="$emit('reference', $event)"
              @refine="$emit('refine', $event)"
              @repeat="$emit('repeat', $event, message.content)"
              @view="$emit('view', $event.originalSrc || $event.src, $event.revisedPrompt || '生成图片')"
            />
          </div>
          <button v-if="message.role === 'assistant' && message.status === 'failed'" type="button" class="retry-button" @click="$emit('retry', message.id)">
            重新生成
          </button>
          </div>
        </div>
      </article>
    </div>
  </div>
</template>
