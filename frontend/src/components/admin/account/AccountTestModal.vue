<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.testAccountConnection')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-4">
      <!-- Account Info Card -->
      <div
        v-if="account"
        class="flex items-center justify-between rounded-xl border border-gray-200 bg-gradient-to-r from-gray-50 to-gray-100 p-3 dark:border-dark-500 dark:from-dark-700 dark:to-dark-600"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
          >
            <Icon name="play" size="md" class="text-white" :stroke-width="2" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
              <span
                class="rounded bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase dark:bg-dark-500"
              >
                {{ account.type }}
              </span>
              <span>{{ t('admin.accounts.account') }}</span>
            </div>
          </div>
        </div>
        <span
          :class="[
            'rounded-full px-2.5 py-1 text-xs font-semibold',
            account.status === 'active'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
          ]"
        >
          {{ account.status }}
        </span>
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.selectTestModel') }}
        </label>
        <Select
          v-model="selectedModelId"
          :options="availableModels"
          :disabled="loadingModels || status === 'connecting'"
          value-key="id"
          label-key="display_name"
          :placeholder="loadingModels ? t('common.loading') + '...' : t('admin.accounts.selectTestModel')"
        />
      </div>

      <div v-if="supportsMediaTest" class="space-y-1.5">
        <TextArea
          v-model="testPrompt"
          :label="supportsGrokVideoTest ? t('admin.accounts.videoPromptLabel') : t('admin.accounts.imagePromptLabel')"
          :placeholder="supportsGrokVideoTest ? t('admin.accounts.videoPromptPlaceholder') : t('admin.accounts.imagePromptPlaceholder')"
          :hint="supportsGrokVideoTest ? t('admin.accounts.videoTestHint') : t('admin.accounts.imageTestHint')"
          :disabled="status === 'connecting'"
          rows="3"
        />
      </div>

      <!-- Terminal Output -->
      <div class="group relative">
        <div
          ref="terminalRef"
          class="max-h-[240px] min-h-[120px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
        >
          <!-- Status Line -->
          <div v-if="status === 'idle'" class="flex items-center gap-2 text-gray-500">
            <Icon name="play" size="sm" :stroke-width="2" />
            <span>{{ t('admin.accounts.readyToTest') }}</span>
          </div>
          <div v-else-if="status === 'connecting'" class="flex items-center gap-2 text-yellow-400">
            <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
            <span>{{ t('admin.accounts.connectingToApi') }}</span>
          </div>

          <!-- Output Lines -->
          <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
            {{ line.text }}
          </div>

          <!-- Streaming Content -->
          <div v-if="streamingContent" class="text-green-400">
            {{ streamingContent }}<span class="animate-pulse">_</span>
          </div>

          <!-- Result Status -->
          <div
            v-if="status === 'success'"
            class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-green-400"
          >
            <Icon name="check" size="sm" :stroke-width="2" />
            <span>{{ t('admin.accounts.testCompleted') }}</span>
          </div>
          <div
            v-else-if="status === 'error'"
            class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-red-400"
          >
            <Icon name="x" size="sm" :stroke-width="2" />
            <span>{{ errorMessage }}</span>
          </div>
        </div>

        <!-- Copy Button -->
        <button
          v-if="outputLines.length > 0"
          @click="copyOutput"
          class="absolute right-2 top-2 rounded-lg bg-gray-800/80 p-1.5 text-gray-400 opacity-0 transition-all hover:bg-gray-700 hover:text-white group-hover:opacity-100"
          :title="t('admin.accounts.copyOutput')"
        >
          <Icon name="link" size="sm" :stroke-width="2" />
        </button>
      </div>

      <div v-if="generatedImages.length > 0" class="space-y-2">
        <div class="text-xs font-medium text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.imagePreview') }}
        </div>
        <div class="flex flex-wrap justify-center gap-3">
          <div
            v-for="(image, index) in generatedImages"
            :key="`${image.url}-${index}`"
            class="group/img relative cursor-pointer overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-500 dark:bg-dark-700"
            @click="previewImageUrl = image.url"
          >
            <img :src="image.url" :alt="`test-image-${index + 1}`" class="max-h-[360px] w-full object-contain" />
            <div class="absolute inset-0 flex items-center justify-center bg-black/0 transition-colors group-hover/img:bg-black/20">
              <Icon name="eye" size="lg" class="text-white opacity-0 drop-shadow-lg transition-opacity group-hover/img:opacity-100" :stroke-width="2" />
            </div>
            <div class="border-t border-gray-100 px-3 py-1.5 text-xs text-gray-500 dark:border-dark-500 dark:text-gray-300">
              {{ image.mimeType || 'image/*' }}
            </div>
          </div>
        </div>
      </div>

      <div v-if="generatedVideos.length > 0" class="space-y-2">
        <div class="text-xs font-medium text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.videoPreview') }}
        </div>
        <div class="space-y-3">
          <div
            v-for="(video, index) in generatedVideos"
            :key="`${video.url}-${index}`"
            class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-500 dark:bg-dark-700"
          >
            <video
              :src="video.url"
              controls
              preload="metadata"
              playsinline
              class="max-h-[360px] w-full bg-black"
              :aria-label="`test-video-${index + 1}`"
              @error="video.error = true"
            />
            <div class="space-y-1 border-t border-gray-100 px-3 py-1.5 text-xs text-gray-500 dark:border-dark-500 dark:text-gray-300">
              <div>{{ video.mimeType || 'video/mp4' }}</div>
              <div v-if="video.error" class="text-red-600 dark:text-red-300">
                {{ t('admin.accounts.videoPlaybackError') }}
              </div>
              <a
                v-if="video.sourceUrl"
                :href="video.sourceUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex text-primary-600 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200"
              >
                {{ t('admin.accounts.openVideo') }}
              </a>
            </div>
          </div>
        </div>
      </div>

      <!-- Image Lightbox -->
      <Teleport to="body">
        <Transition name="fade">
          <div
            v-if="previewImageUrl"
            class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
            @click.self="previewImageUrl = ''"
          >
            <button
              class="absolute right-4 top-4 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
              @click="previewImageUrl = ''"
            >
              <Icon name="x" size="lg" :stroke-width="2" />
            </button>
            <img
              :src="previewImageUrl"
              alt="preview"
              class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl"
            />
          </div>
        </Transition>
      </Teleport>

      <!-- Test Info -->
      <div class="flex items-center justify-between px-1 text-xs text-gray-500 dark:text-gray-400">
        <div class="flex items-center gap-3">
          <span class="flex items-center gap-1">
            <Icon name="grid" size="sm" :stroke-width="2" />
            {{ t('admin.accounts.testModel') }}
          </span>
        </div>
        <span class="flex items-center gap-1">
          <Icon name="chat" size="sm" :stroke-width="2" />
          {{
            supportsGrokVideoTest
              ? t('admin.accounts.videoTestMode')
              : supportsImageTest
              ? t('admin.accounts.imageTestMode')
              : t('admin.accounts.testPrompt')
          }}
        </span>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
        <button
          @click="startTest"
          :disabled="status === 'connecting' || !selectedModelId"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            status === 'connecting' || !selectedModelId
              ? 'cursor-not-allowed bg-primary-400 text-white'
              : status === 'success'
                ? 'bg-green-500 text-white hover:bg-green-600'
                : status === 'error'
                  ? 'bg-orange-500 text-white hover:bg-orange-600'
                  : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <Icon
            v-if="status === 'connecting'"
            name="refresh"
            size="sm"
            class="animate-spin"
            :stroke-width="2"
          />
          <Icon v-else-if="status === 'idle'" name="play" size="sm" :stroke-width="2" />
          <Icon v-else name="refresh" size="sm" :stroke-width="2" />
          <span>
            {{
              status === 'connecting'
                ? t('admin.accounts.testing')
                : status === 'idle'
                  ? t('admin.accounts.startTest')
                  : t('admin.accounts.retry')
            }}
          </span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import { Icon } from '@/components/icons'
import { useClipboard } from '@/composables/useClipboard'
import { buildApiUrl } from '@/api/client'
import { adminAPI } from '@/api/admin'
import type { Account, ClaudeModel } from '@/types'

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

interface OutputLine {
  text: string
  class: string
}

interface PreviewImage {
  url: string
  mimeType?: string
}

interface PreviewVideo {
  url: string
  sourceUrl?: string
  mimeType?: string
  error?: boolean
}

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const terminalRef = ref<HTMLElement | null>(null)
const status = ref<'idle' | 'connecting' | 'success' | 'error'>('idle')
const outputLines = ref<OutputLine[]>([])
const streamingContent = ref('')
const errorMessage = ref('')
const availableModels = ref<ClaudeModel[]>([])
const selectedModelId = ref('')
const testPrompt = ref('')
const loadingModels = ref(false)
let abortController: AbortController | null = null
const generatedImages = ref<PreviewImage[]>([])
const generatedVideos = ref<PreviewVideo[]>([])
const previewImageUrl = ref('')
const prioritizedGeminiModels = [
  'gemini-3.1-flash-image',
  'gemini-2.5-flash-image',
  'gemini-3.5-flash',
  'gemini-2.5-flash',
  'gemini-2.5-pro',
  'gemini-3-flash-preview',
  'gemini-3-pro-preview',
  'gemini-2.0-flash'
]
const prioritizedGrokModels = [
  'grok-4.3',
  'grok-4.3-fast',
  'grok-4.20-0309-non-reasoning',
  'grok-4.20-0309-reasoning',
  'grok-4.20-multi-agent-0309',
  'grok-code-fast-1',
  'grok-build-0.1',
  'grok-3-mini',
  'grok-imagine-image',
  'grok-imagine-image-quality',
  'grok-imagine-video',
  'grok-imagine-video-1.5-preview'
]

const isGrokImageModel = (modelID: string) => modelID.toLowerCase().startsWith('grok-imagine-image')
const isGrokVideoModel = (modelID: string) => modelID.toLowerCase().startsWith('grok-imagine-video')

const supportsGeminiImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  if (!modelID.startsWith('gemini-') || !modelID.includes('-image')) return false

  return props.account?.platform === 'gemini' || (props.account?.platform === 'antigravity' && props.account?.type === 'apikey')
})

const supportsOpenAIImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  if (!modelID.startsWith('gpt-image-')) return false
  return props.account?.platform === 'openai'
})

const supportsGrokImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  return props.account?.platform === 'grok' && isGrokImageModel(modelID)
})

const supportsGrokVideoTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  return props.account?.platform === 'grok' && isGrokVideoModel(modelID)
})

const supportsImageTest = computed(
  () => supportsGeminiImageTest.value || supportsOpenAIImageTest.value || supportsGrokImageTest.value
)

const supportsMediaTest = computed(() => supportsImageTest.value || supportsGrokVideoTest.value)

const retiredOpenAIModelIDs = new Set(['gpt-5.3-codex', 'gpt-5.3-codex-spark'])

const getTestableModels = (models: ClaudeModel[]) => {
  if (props.account?.platform !== 'openai') return models
  return models.filter((model) => !retiredOpenAIModelIDs.has(model.id.trim().toLowerCase()))
}

const sortTestModels = (models: ClaudeModel[]) => {
  const priorityIDs = props.account?.platform === 'grok' ? prioritizedGrokModels : prioritizedGeminiModels
  const priorityMap = new Map(priorityIDs.map((id, index) => [id, index]))

  return [...models].sort((a, b) => {
    if (props.account?.platform === 'grok') {
      const aRank = isGrokVideoModel(a.id) ? 2 : isGrokImageModel(a.id) ? 1 : 0
      const bRank = isGrokVideoModel(b.id) ? 2 : isGrokImageModel(b.id) ? 1 : 0
      if (aRank !== bRank) return aRank - bRank
    }

    const aPriority = priorityMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const bPriority = priorityMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    if (aPriority !== bPriority) return aPriority - bPriority
    return 0
  })
}

const modelIDsToOptions = (modelIDs: string[]): ClaudeModel[] => {
  const seen = new Set<string>()
  const models: ClaudeModel[] = []
  for (const rawModelID of modelIDs) {
    const modelID = rawModelID.trim()
    if (!modelID || seen.has(modelID)) continue
    seen.add(modelID)
    models.push({
      id: modelID,
      type: 'model',
      display_name: modelID,
      created_at: ''
    })
  }
  return models
}

const hasModelMapping = (account: Account): boolean => {
  const mapping = account.credentials?.model_mapping
  if (!mapping || typeof mapping !== 'object' || Array.isArray(mapping)) return false
  return Object.keys(mapping as Record<string, unknown>).length > 0
}

const hasProjectID = (account: Account): boolean => {
  const projectID = account.credentials?.project_id
  return typeof projectID === 'string' && projectID.trim() !== ''
}

const canSyncUpstreamModelsForTest = (account: Account): boolean => {
  if (hasModelMapping(account)) return false

  switch (account.platform) {
    case 'anthropic':
      return account.type === 'oauth' || account.type === 'setup-token' || account.type === 'apikey'
    case 'openai':
      return account.type === 'apikey'
    case 'grok':
      return account.type === 'oauth' || account.type === 'apikey'
    case 'gemini':
      return account.type === 'apikey' || (account.type === 'oauth' && !hasProjectID(account))
    case 'antigravity':
      return true
    default:
      return false
  }
}

const fetchAvailableModelsForTest = async (account: Account): Promise<ClaudeModel[]> => {
  if (canSyncUpstreamModelsForTest(account)) {
    try {
      const result = await adminAPI.accounts.syncUpstreamModels(account.id)
      const upstreamModels = modelIDsToOptions(result.models || [])
      if (upstreamModels.length > 0) {
        return upstreamModels
      }
    } catch (error) {
      console.warn('Failed to sync upstream models for account test:', error)
    }
  }

  return adminAPI.accounts.getAvailableModels(account.id)
}

// Load available models when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      testPrompt.value = ''
      resetState()
      await loadAvailableModels()
    } else {
      abortStream()
    }
  }
)

watch(selectedModelId, () => {
  const currentPrompt = testPrompt.value.trim()
  const imagePrompt = t('admin.accounts.imagePromptDefault')
  const videoPrompt = t('admin.accounts.videoPromptDefault')
  if (supportsImageTest.value && (!currentPrompt || currentPrompt === videoPrompt)) {
    testPrompt.value = imagePrompt
  } else if (supportsGrokVideoTest.value && (!currentPrompt || currentPrompt === imagePrompt)) {
    testPrompt.value = videoPrompt
  }
})

const loadAvailableModels = async () => {
  if (!props.account) return

  loadingModels.value = true
  selectedModelId.value = '' // Reset selection before loading
  try {
    const models = getTestableModels(await fetchAvailableModelsForTest(props.account))
    const shouldSortModels = ['gemini', 'antigravity', 'grok'].includes(props.account.platform)
    availableModels.value = shouldSortModels ? sortTestModels(models) : models
    // Default selection by platform
    if (availableModels.value.length > 0) {
      if (props.account.platform === 'gemini') {
        selectedModelId.value = availableModels.value[0].id
      } else {
        // Try to select Sonnet as default, otherwise use first model
        const sonnetModel = availableModels.value.find((m) => m.id.includes('sonnet'))
        selectedModelId.value = sonnetModel?.id || availableModels.value[0].id
      }
    }
  } catch (error) {
    console.error('Failed to load available models:', error)
    // Fallback to empty list
    availableModels.value = []
    selectedModelId.value = ''
  } finally {
    loadingModels.value = false
  }
}

const resetState = () => {
  status.value = 'idle'
  outputLines.value = []
  streamingContent.value = ''
  errorMessage.value = ''
  generatedImages.value = []
  generatedVideos.value = []
  previewImageUrl.value = ''
}

const handleClose = () => {
  abortStream()
  emit('close')
}

const abortStream = () => {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

const addLine = (text: string, className: string = 'text-gray-300') => {
  outputLines.value.push({ text, class: className })
  scrollToBottom()
}

const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  }
}

const startTest = async () => {
  if (!props.account || !selectedModelId.value) return

  resetState()
  status.value = 'connecting'
  addLine(t('admin.accounts.startingTestForAccount', { name: props.account.name }), 'text-blue-400')
  addLine(t('admin.accounts.testAccountTypeLabel', { type: props.account.type }), 'text-gray-400')
  addLine('', 'text-gray-300')

  abortStream()

  abortController = new AbortController()

  try {
    // Use the configured API base; EventSource does not support POST.
    const url = buildApiUrl(`/admin/accounts/${props.account.id}/test`)

    // Use fetch with streaming for SSE since EventSource doesn't support POST
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        model_id: selectedModelId.value,
        prompt: supportsMediaTest.value ? testPrompt.value.trim() : ''
      }),
      signal: abortController.signal
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('No response body')
    }

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const jsonStr = line.slice(6).trim()
          if (jsonStr) {
            try {
              const event = JSON.parse(jsonStr)
              handleEvent(event)
            } catch (e) {
              console.error('Failed to parse SSE event:', e)
            }
          }
        }
      }
    }
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      status.value = 'idle'
      return
    }
    status.value = 'error'
    const msg = error instanceof Error ? error.message : 'Unknown error'
    errorMessage.value = msg
    addLine(`Error: ${msg}`, 'text-red-400')
  }
}

const handleEvent = (event: {
  type: string
  text?: string
  model?: string
  success?: boolean
  error?: string
  image_url?: string
  video_url?: string
  source_url?: string
  mime_type?: string
}) => {
  switch (event.type) {
    case 'test_start':
      addLine(t('admin.accounts.connectedToApi'), 'text-green-400')
      if (event.model) {
        addLine(t('admin.accounts.usingModel', { model: event.model }), 'text-cyan-400')
      }
      addLine(
        supportsGrokVideoTest.value
          ? t('admin.accounts.sendingVideoRequest')
          : supportsImageTest.value
            ? t('admin.accounts.sendingImageRequest')
            : t('admin.accounts.sendingTestMessage'),
        'text-gray-400'
      )
      addLine('', 'text-gray-300')
      addLine(t('admin.accounts.response'), 'text-yellow-400')
      break

    case 'content':
      if (event.text) {
        streamingContent.value += event.text
        scrollToBottom()
      }
      break

    case 'video':
      if (event.video_url) {
        generatedVideos.value.push({
          url: event.video_url,
          sourceUrl: event.source_url || event.video_url,
          mimeType: event.mime_type
        })
        addLine(t('admin.accounts.videoReceived', { count: generatedVideos.value.length }), 'text-purple-300')
      }
      break

    case 'status':
      if (event.text) {
        addLine(event.text, 'text-gray-400')
      }
      break

    case 'image':
      if (event.image_url) {
        generatedImages.value.push({
          url: event.image_url,
          mimeType: event.mime_type
        })
        addLine(t('admin.accounts.imageReceived', { count: generatedImages.value.length }), 'text-purple-300')
      }
      break

    case 'test_complete':
      // Move streaming content to output lines
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      }
      if (event.success) {
        status.value = 'success'
      } else {
        status.value = 'error'
        errorMessage.value = event.error || 'Test failed'
      }
      break

    case 'error':
      status.value = 'error'
      errorMessage.value = event.error || 'Unknown error'
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      }
      addLine(`Error: ${errorMessage.value}`, 'text-red-400')
      break
  }
}

const copyOutput = () => {
  const text = outputLines.value.map((l) => l.text).join('\n')
  copyToClipboard(text, t('admin.accounts.outputCopied'))
}
</script>

<style>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
