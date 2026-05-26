<template>
  <div class="space-y-3">
    <!-- Terminal Output -->
    <div class="group relative">
      <div
        ref="terminalRef"
        class="max-h-[240px] min-h-[120px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
      >
        <!-- Idle -->
        <div v-if="status === 'idle'" class="flex items-center gap-2 text-gray-500">
          <Icon name="play" size="sm" :stroke-width="2" />
          <span>{{ t('admin.accounts.readyToTest') }}</span>
        </div>

        <!-- Connecting -->
        <div v-else-if="status === 'connecting'" class="flex items-center gap-2 text-yellow-400">
          <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <span>{{ t('admin.accounts.connectingToApi') }}</span>
        </div>

        <!-- Output lines -->
        <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
          {{ line.text }}
        </div>

        <!-- Streaming content with cursor -->
        <div v-if="streamingContent" class="text-green-400">
          {{ streamingContent }}<span class="animate-pulse">_</span>
        </div>

        <!-- Success -->
        <div
          v-if="status === 'success'"
          class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-green-400"
        >
          <Icon name="check" size="sm" :stroke-width="2" />
          <span>{{ t('admin.accounts.testCompleted') }}</span>
        </div>

        <!-- Error -->
        <div
          v-else-if="status === 'error'"
          class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-red-400"
        >
          <Icon name="x" size="sm" :stroke-width="2" />
          <span>{{ errorMessage }}</span>
        </div>
      </div>

      <!-- Copy button -->
      <button
        v-if="outputLines.length > 0"
        @click="copyOutput"
        class="absolute right-2 top-2 rounded-lg bg-gray-800/80 p-1.5 text-gray-400 opacity-0 transition-all hover:bg-gray-700 hover:text-white group-hover:opacity-100"
        :title="t('admin.accounts.copyOutput')"
      >
        <Icon name="copy" size="sm" :stroke-width="2" />
      </button>
    </div>

    <!-- Image preview grid -->
    <div v-if="images && images.length > 0" class="space-y-2">
      <div class="text-xs font-medium text-gray-600 dark:text-gray-300">
        {{ t('admin.accounts.imagePreview') }}
      </div>
      <div class="flex flex-wrap justify-center gap-3">
        <div
          v-for="(image, index) in images"
          :key="`${image.url}-${index}`"
          class="group/img relative cursor-pointer overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-500 dark:bg-dark-700"
          @click="previewUrl = image.url"
        >
          <img
            :src="image.url"
            :alt="`test-image-${index + 1}`"
            class="max-h-[360px] w-full object-contain"
          />
          <div
            class="absolute inset-0 flex items-center justify-center bg-black/0 transition-colors group-hover/img:bg-black/20"
          >
            <Icon
              name="eye"
              size="lg"
              class="text-white opacity-0 drop-shadow-lg transition-opacity group-hover/img:opacity-100"
              :stroke-width="2"
            />
          </div>
          <div
            class="border-t border-gray-100 px-3 py-1.5 text-xs text-gray-500 dark:border-dark-500 dark:text-gray-300"
          >
            {{ image.mimeType || 'image/*' }}
          </div>
        </div>
      </div>
    </div>

    <!-- Lightbox (teleported to body for z-index) -->
    <Teleport to="body">
      <Transition name="fade">
        <div
          v-if="previewUrl"
          class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
          @click.self="previewUrl = ''"
        >
          <button
            class="absolute right-4 top-4 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
            @click="previewUrl = ''"
          >
            <Icon name="x" size="lg" :stroke-width="2" />
          </button>
          <img
            :src="previewUrl"
            alt="preview"
            class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl"
          />
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../Icon.vue'
import type { TestOutputLine, TestImage } from '../../composables/useAccountTest'

const props = defineProps<{
  status: 'idle' | 'connecting' | 'success' | 'error'
  outputLines: TestOutputLine[]
  streamingContent: string
  errorMessage: string
  images?: TestImage[]
}>()

const { t } = useI18n()

const terminalRef = ref<HTMLElement | null>(null)
const previewUrl = ref('')

// Auto-scroll terminal on new content
const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  }
}

watch(() => props.outputLines.length, scrollToBottom)
watch(() => props.streamingContent, scrollToBottom)

const copyOutput = async () => {
  const text = props.outputLines.map((l) => l.text).join('\n')
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    // silent fallback – clipboard may be unavailable in some contexts
  }
}
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
