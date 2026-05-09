<template>
  <AppLayout>
    <div class="image-generator-shell">
      <section class="generator-panel">
        <div class="panel-heading">
          <div>
            <h1>{{ t('imageGenerator.title') }}</h1>
            <p>{{ t('imageGenerator.subtitle') }}</p>
          </div>
          <span class="model-pill">gpt-image-2</span>
        </div>

        <div class="mode-switch" role="tablist" :aria-label="t('imageGenerator.mode')">
          <button
            type="button"
            :class="{ active: mode === 'text' }"
            @click="mode = 'text'"
          >
            {{ t('imageGenerator.textToImage') }}
          </button>
          <button
            type="button"
            :class="{ active: mode === 'image' }"
            @click="mode = 'image'"
          >
            {{ t('imageGenerator.imageToImage') }}
          </button>
        </div>

        <label class="field-label" for="image-key">
          <span>{{ t('imageGenerator.currentApiKey') }}</span>
          <RouterLink to="/keys">{{ t('imageGenerator.manageKeys') }}</RouterLink>
        </label>
        <select id="image-key" v-model="selectedKeyId" class="input">
          <option value="" disabled>{{ t('imageGenerator.selectKey') }}</option>
          <option v-for="item in keyOptions" :key="item.id" :value="String(item.id)">
            {{ formatKeyOption(item) }}
          </option>
        </select>
        <p v-if="keysLoading" class="input-hint">{{ t('imageGenerator.loadingKeys') }}</p>
        <p v-else-if="keyOptions.length === 0" class="input-hint">
          {{ t('imageGenerator.noKeysHint') }}
        </p>

        <div v-if="mode === 'image'" class="upload-block">
          <label class="field-label" for="reference-image">{{ t('imageGenerator.referenceImage') }}</label>
          <label
            class="upload-dropzone"
            :class="{ 'has-file': !!referencePreview }"
            for="reference-image"
          >
            <input
              id="reference-image"
              type="file"
              accept="image/png,image/jpeg,image/webp"
              class="sr-only"
              @change="handleReferenceChange"
            />
            <img v-if="referencePreview" :src="referencePreview" alt="" />
            <span v-else>
              <Icon name="upload" size="lg" />
              {{ t('imageGenerator.uploadReference') }}
            </span>
          </label>
          <button v-if="referenceImage" type="button" class="btn btn-ghost btn-sm" @click="clearReference">
            <Icon name="x" size="sm" />
            {{ t('imageGenerator.removeReference') }}
          </button>
        </div>

        <label class="field-label" for="image-prompt">{{ t('imageGenerator.prompt') }}</label>
        <textarea
          id="image-prompt"
          v-model="prompt"
          class="input prompt-input"
          rows="5"
          :placeholder="t('imageGenerator.promptPlaceholder')"
        ></textarea>

        <div class="field-group">
          <div class="field-label">
            <label for="image-size">{{ t('imageGenerator.size') }}</label>
            <div class="size-help">
              <button
                type="button"
                class="size-help-trigger"
                aria-describedby="image-size-requirements"
              >
                <Icon name="infoCircle" size="sm" />
                {{ t('imageGenerator.sizeRequirements') }}
              </button>
              <div id="image-size-requirements" class="size-help-popover" role="tooltip">
                <strong>{{ t('imageGenerator.customSizeRequirementTitle') }}</strong>
                <ul>
                  <li>{{ t('imageGenerator.customSizeRequirementMaxSide') }}</li>
                  <li>{{ t('imageGenerator.customSizeRequirementMultiple') }}</li>
                  <li>{{ t('imageGenerator.customSizeRequirementRatio') }}</li>
                  <li>{{ t('imageGenerator.customSizeRequirementPixels') }}</li>
                </ul>
              </div>
            </div>
          </div>
          <select id="image-size" v-model="size" class="input">
            <option v-for="item in sizeOptions" :key="item.value" :value="item.value">
              {{ item.label }}
            </option>
          </select>
          <div v-if="size === CUSTOM_SIZE_VALUE" class="custom-size-grid">
            <label class="custom-size-field" for="custom-width">
              <span>{{ t('imageGenerator.customWidth') }}</span>
              <input
                id="custom-width"
                v-model.number="customWidth"
                class="input"
                type="number"
                min="16"
                max="3840"
                step="16"
              />
            </label>
            <span class="custom-size-separator">x</span>
            <label class="custom-size-field" for="custom-height">
              <span>{{ t('imageGenerator.customHeight') }}</span>
              <input
                id="custom-height"
                v-model.number="customHeight"
                class="input"
                type="number"
                min="16"
                max="3840"
                step="16"
              />
            </label>
          </div>
          <p class="input-hint" :class="{ error: !!customSizeError }">{{ selectedSizeHint }}</p>
        </div>

        <div class="field-group">
          <span class="field-label">{{ t('imageGenerator.quality') }}</span>
          <div class="segmented-grid four">
            <button
              v-for="item in qualityOptions"
              :key="item"
              type="button"
              :class="{ active: quality === item }"
              @click="quality = item"
            >
              {{ itemLabel(item) }}
            </button>
          </div>
        </div>

        <div class="field-group">
          <span class="field-label">{{ t('imageGenerator.count') }}</span>
          <div class="segmented-grid three">
            <button
              v-for="item in countOptions"
              :key="item"
              type="button"
              :class="{ active: count === item }"
              @click="count = item"
            >
              {{ t('imageGenerator.countUnit', { count: item }) }}
            </button>
          </div>
        </div>

        <button
          type="button"
          class="generate-button"
          :disabled="!canGenerate || generating"
          @click="generate"
        >
          <LoadingSpinner v-if="generating" size="sm" color="white" />
          <Icon v-else name="sparkles" size="md" />
          {{ generating ? t('imageGenerator.generating') : t('imageGenerator.generateNow') }}
        </button>
      </section>

      <section class="result-panel">
        <div class="result-header">
          <div>
            <h2>{{ t('imageGenerator.resultTitle') }}</h2>
            <p>{{ selectedKeyLabel }} · {{ selectedSizeLabel }}</p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="history.length === 0" @click="showHistory = !showHistory">
            <Icon name="clock" size="sm" />
            {{ t('imageGenerator.history') }}
          </button>
        </div>

        <div v-if="errorMessage" class="error-banner">
          <Icon name="exclamationTriangle" size="md" />
          <span>{{ errorMessage }}</span>
        </div>

        <div v-if="showHistory && history.length > 0" class="history-strip">
          <button
            v-for="item in history"
            :key="item.id"
            type="button"
            class="history-thumb"
            @click="restoreHistory(item)"
          >
            <img :src="item.images[0]?.url" alt="" />
          </button>
        </div>

        <div class="result-workspace" :class="{ filled: images.length > 0 }">
          <div v-if="generating" class="empty-workspace">
            <LoadingSpinner size="lg" />
            <h3>{{ t('imageGenerator.generatingTitle') }}</h3>
            <p>{{ t('imageGenerator.generatingHint') }}</p>
          </div>

          <div v-else-if="images.length === 0" class="empty-workspace">
            <div class="empty-icon">
              <Icon name="sparkles" size="xl" />
            </div>
            <h3>{{ t('imageGenerator.emptyTitle') }}</h3>
            <p>{{ t('imageGenerator.emptyDescription') }}</p>
          </div>

          <div v-else class="image-grid" :class="`count-${images.length}`">
            <figure v-for="(image, index) in images" :key="image.url" class="result-card">
              <img :src="image.url" :alt="t('imageGenerator.resultAlt', { index: index + 1 })" />
              <figcaption>
                <span>{{ image.revisedPrompt || prompt }}</span>
                <div class="image-actions">
                  <a class="btn btn-secondary btn-sm" :href="image.url" :download="`ci-image-${Date.now()}-${index + 1}.png`">
                    <Icon name="download" size="sm" />
                    {{ t('imageGenerator.download') }}
                  </a>
                </div>
              </figcaption>
            </figure>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { imageGenerationAPI, type GeneratedImage, type ImageGenerationKeyOption } from '@/api/imageGeneration'
import { useAppStore } from '@/stores/app'

type Mode = 'text' | 'image'
type Quality = 'auto' | 'low' | 'medium' | 'high'

const CUSTOM_SIZE_VALUE = 'custom'
const MIN_CUSTOM_SIZE_PIXELS = 655360
const MAX_CUSTOM_SIZE_PIXELS = 8294400
const MAX_CUSTOM_SIDE = 3840
const MAX_CUSTOM_RATIO = 3

interface HistoryItem {
  id: string
  prompt: string
  size: string
  quality: Quality
  count: number
  mode: Mode
  images: GeneratedImage[]
}

const { t } = useI18n()
const appStore = useAppStore()

const mode = ref<Mode>('text')
const keyOptions = ref<ImageGenerationKeyOption[]>([])
const selectedKeyId = ref('')
const keysLoading = ref(false)
const generating = ref(false)
const prompt = ref('')
const size = ref('1024x1024')
const customWidth = ref(1024)
const customHeight = ref(1024)
const quality = ref<Quality>('auto')
const count = ref(1)
const images = ref<GeneratedImage[]>([])
const history = ref<HistoryItem[]>([])
const showHistory = ref(false)
const errorMessage = ref('')
const referenceImage = ref<File | null>(null)
const referencePreview = ref('')

const qualityOptions: Quality[] = ['auto', 'low', 'medium', 'high']
const countOptions = [1, 2, 3]
const sizeOptions = computed(() => [
  { value: '1024x1024', label: t('imageGenerator.sizeSquare1K'), hint: t('imageGenerator.sizeSquare1KHint') },
  { value: '1536x1024', label: t('imageGenerator.sizeLandscape2K'), hint: t('imageGenerator.sizeLandscape2KHint') },
  { value: '1024x1536', label: t('imageGenerator.sizePortrait2K'), hint: t('imageGenerator.sizePortrait2KHint') },
  { value: '2048x2048', label: t('imageGenerator.sizeSquare2K'), hint: t('imageGenerator.sizeSquare2KHint') },
  { value: '3840x2160', label: t('imageGenerator.sizeLandscape4K'), hint: t('imageGenerator.sizeLandscape4KHint') },
  { value: '2160x3840', label: t('imageGenerator.sizePortrait4K'), hint: t('imageGenerator.sizePortrait4KHint') },
  { value: CUSTOM_SIZE_VALUE, label: t('imageGenerator.sizeCustom'), hint: t('imageGenerator.sizeCustomHint') },
])

const selectedKey = computed(() => keyOptions.value.find((item) => String(item.id) === selectedKeyId.value) || null)
const selectedKeyLabel = computed(() => selectedKey.value ? formatKeyOption(selectedKey.value) : t('imageGenerator.noKeySelected'))
const selectedSizeOption = computed(() => sizeOptions.value.find((item) => item.value === size.value) || sizeOptions.value[0])
const selectedImageSize = computed(() => size.value === CUSTOM_SIZE_VALUE ? `${customWidth.value}x${customHeight.value}` : size.value)
const selectedSizeLabel = computed(() => size.value === CUSTOM_SIZE_VALUE ? selectedImageSize.value : selectedSizeOption.value?.label || size.value)
const customSizeError = computed(() => validateCustomSize())
const selectedSizeHint = computed(() => customSizeError.value || selectedSizeOption.value?.hint || '')

const canGenerate = computed(() => {
  if (!selectedKey.value) return false
  if (!prompt.value.trim()) return false
  if (mode.value === 'image' && !referenceImage.value) return false
  if (customSizeError.value) return false
  return true
})

watch(mode, () => {
  errorMessage.value = ''
})

onMounted(loadKeys)
onBeforeUnmount(() => {
  revokeReferencePreview()
})

async function loadKeys() {
  keysLoading.value = true
  try {
    keyOptions.value = await imageGenerationAPI.listImageGenerationKeys()
    if (!selectedKeyId.value && keyOptions.value.length > 0) {
      selectedKeyId.value = String(keyOptions.value[0].id)
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : t('imageGenerator.loadKeysFailed')
    appStore.showError(message)
  } finally {
    keysLoading.value = false
  }
}

async function generate() {
  if (!selectedKey.value || !canGenerate.value) return

  generating.value = true
  errorMessage.value = ''
  images.value = []

  try {
    const result = await imageGenerationAPI.generateImage({
      apiKey: selectedKey.value.key,
      prompt: prompt.value.trim(),
      size: selectedImageSize.value,
      quality: quality.value,
      count: count.value,
      referenceImage: mode.value === 'image' ? referenceImage.value : null
    })

    images.value = result
    history.value = [
      {
        id: `${Date.now()}`,
        prompt: prompt.value.trim(),
        size: selectedImageSize.value,
        quality: quality.value,
        count: count.value,
        mode: mode.value,
        images: result
      },
      ...history.value
    ].slice(0, 8)
    appStore.showSuccess(t('imageGenerator.generateSuccess'))
  } catch (error) {
    const message = error instanceof Error ? error.message : t('imageGenerator.generateFailed')
    errorMessage.value = message
    appStore.showError(message)
  } finally {
    generating.value = false
  }
}

function handleReferenceChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    appStore.showError(t('imageGenerator.invalidImage'))
    return
  }
  revokeReferencePreview()
  referenceImage.value = file
  referencePreview.value = URL.createObjectURL(file)
}

function clearReference() {
  revokeReferencePreview()
  referenceImage.value = null
}

function revokeReferencePreview() {
  if (referencePreview.value) {
    URL.revokeObjectURL(referencePreview.value)
    referencePreview.value = ''
  }
}

function restoreHistory(item: HistoryItem) {
  prompt.value = item.prompt
  if (sizeOptions.value.some((option) => option.value === item.size)) {
    size.value = item.size
  } else {
    const [width, height] = item.size.split('x').map((part) => Number(part))
    if (Number.isFinite(width) && Number.isFinite(height)) {
      customWidth.value = width
      customHeight.value = height
    }
    size.value = CUSTOM_SIZE_VALUE
  }
  quality.value = item.quality
  count.value = item.count
  mode.value = item.mode
  images.value = item.images
}

function itemLabel(value: Quality): string {
  return t(`imageGenerator.qualityOptions.${value}`)
}

function formatKeyOption(item: ImageGenerationKeyOption): string {
  return item.name?.trim() || item.maskedKey
}

function validateCustomSize(): string {
  if (size.value !== CUSTOM_SIZE_VALUE) return ''

  const width = Number(customWidth.value)
  const height = Number(customHeight.value)
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) {
    return t('imageGenerator.customSizeInvalidNumber')
  }
  if (width > MAX_CUSTOM_SIDE || height > MAX_CUSTOM_SIDE) {
    return t('imageGenerator.customSizeInvalidSide')
  }
  if (width % 16 !== 0 || height % 16 !== 0) {
    return t('imageGenerator.customSizeInvalidMultiple')
  }

  const longSide = Math.max(width, height)
  const shortSide = Math.min(width, height)
  if (longSide / shortSide > MAX_CUSTOM_RATIO) {
    return t('imageGenerator.customSizeInvalidRatio')
  }

  const pixels = width * height
  if (pixels < MIN_CUSTOM_SIZE_PIXELS || pixels > MAX_CUSTOM_SIZE_PIXELS) {
    return t('imageGenerator.customSizeInvalidPixels')
  }

  return ''
}
</script>

<style scoped>
.image-generator-shell {
  display: grid;
  grid-template-columns: minmax(320px, 420px) minmax(0, 1fr);
  gap: 20px;
  min-height: calc(100vh - 7rem);
}

.generator-panel,
.result-panel {
  border: 1px solid rgb(226 232 240);
  background: rgb(255 255 255);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgb(15 23 42 / 0.08);
}

.dark .generator-panel,
.dark .result-panel {
  border-color: rgb(51 65 85);
  background: rgb(15 23 42 / 0.78);
}

.generator-panel {
  align-self: start;
  padding: 20px;
}

.panel-heading,
.result-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.panel-heading h1,
.result-header h2 {
  margin: 0;
  color: rgb(15 23 42);
  font-size: 22px;
  font-weight: 800;
  line-height: 1.2;
}

.dark .panel-heading h1,
.dark .result-header h2 {
  color: white;
}

.panel-heading p,
.result-header p {
  margin-top: 6px;
  color: rgb(100 116 139);
  font-size: 13px;
}

.model-pill {
  flex-shrink: 0;
  border-radius: 999px;
  background: rgb(239 246 255);
  color: rgb(37 99 235);
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 700;
}

.dark .model-pill {
  background: rgb(30 58 138 / 0.35);
  color: rgb(147 197 253);
}

.mode-switch {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
  padding: 6px;
  margin-bottom: 18px;
  border-radius: 14px;
  background: rgb(241 245 249);
}

.dark .mode-switch {
  background: rgb(30 41 59);
}

.mode-switch button,
.segmented-grid button {
  min-height: 42px;
  border-radius: 11px;
  color: rgb(71 85 105);
  font-weight: 700;
  transition: background-color 0.2s ease, color 0.2s ease, box-shadow 0.2s ease;
}

.dark .mode-switch button,
.dark .segmented-grid button {
  color: rgb(203 213 225);
}

.mode-switch button.active,
.segmented-grid button.active {
  background: white;
  color: rgb(37 99 235);
  box-shadow: 0 8px 20px rgb(15 23 42 / 0.08);
}

.dark .mode-switch button.active,
.dark .segmented-grid button.active {
  background: rgb(15 23 42);
  color: rgb(147 197 253);
}

.field-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 16px 0 8px;
  color: rgb(51 65 85);
  font-size: 14px;
  font-weight: 700;
}

.dark .field-label {
  color: rgb(226 232 240);
}

.field-label a {
  color: rgb(37 99 235);
  font-size: 13px;
  font-weight: 700;
}

.prompt-input {
  min-height: 126px;
  resize: vertical;
}

.field-group {
  margin-top: 16px;
}

.size-help {
  position: relative;
  margin-left: auto;
}

.size-help-trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 30px;
  border: 1px solid rgb(245 158 11);
  border-radius: 999px;
  background: rgb(255 251 235);
  color: rgb(180 83 9);
  padding: 0 10px;
  font-size: 12px;
  font-weight: 800;
}

.dark .size-help-trigger {
  border-color: rgb(245 158 11 / 0.55);
  background: rgb(120 53 15 / 0.22);
  color: rgb(253 186 116);
}

.size-help-popover {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  z-index: 20;
  width: min(430px, calc(100vw - 64px));
  border: 1px solid rgb(253 186 116);
  border-radius: 16px;
  background: rgb(255 251 235);
  color: rgb(71 85 105);
  padding: 16px 18px;
  box-shadow: 0 24px 60px rgb(120 53 15 / 0.18);
  opacity: 0;
  pointer-events: none;
  transform: translateY(-4px);
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.size-help-popover::before {
  position: absolute;
  top: -7px;
  right: 36px;
  width: 12px;
  height: 12px;
  border-top: 1px solid rgb(253 186 116);
  border-left: 1px solid rgb(253 186 116);
  background: rgb(255 251 235);
  content: '';
  transform: rotate(45deg);
}

.dark .size-help-popover,
.dark .size-help-popover::before {
  border-color: rgb(245 158 11 / 0.6);
  background: rgb(30 41 59);
}

.size-help:hover .size-help-popover,
.size-help:focus-within .size-help-popover {
  opacity: 1;
  pointer-events: auto;
  transform: translateY(0);
}

.size-help-popover strong {
  display: block;
  margin-bottom: 10px;
  color: rgb(15 23 42);
  font-size: 14px;
}

.dark .size-help-popover strong {
  color: white;
}

.size-help-popover ul {
  display: grid;
  gap: 7px;
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
  line-height: 1.45;
}

.custom-size-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: end;
  gap: 10px;
  margin-top: 10px;
}

.custom-size-field {
  display: grid;
  gap: 6px;
  color: rgb(71 85 105);
  font-size: 12px;
  font-weight: 700;
}

.dark .custom-size-field {
  color: rgb(203 213 225);
}

.custom-size-field .input {
  min-height: 42px;
}

.custom-size-separator {
  padding-bottom: 12px;
  color: rgb(100 116 139);
  font-weight: 800;
}

.input-hint.error {
  color: rgb(220 38 38);
  font-weight: 700;
}

.dark .input-hint.error {
  color: rgb(252 165 165);
}

.segmented-grid {
  display: grid;
  gap: 8px;
}

.segmented-grid.four {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.segmented-grid.three {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.segmented-grid button {
  border: 1px solid rgb(226 232 240);
  background: rgb(248 250 252);
}

.dark .segmented-grid button {
  border-color: rgb(51 65 85);
  background: rgb(15 23 42 / 0.4);
}

.upload-block {
  margin-top: 16px;
}

.upload-dropzone {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 150px;
  overflow: hidden;
  border: 1px dashed rgb(148 163 184);
  border-radius: 14px;
  background: rgb(248 250 252);
  color: rgb(100 116 139);
  cursor: pointer;
}

.dark .upload-dropzone {
  border-color: rgb(71 85 105);
  background: rgb(15 23 42 / 0.4);
}

.upload-dropzone span {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  font-weight: 700;
}

.upload-dropzone img {
  width: 100%;
  max-height: 240px;
  object-fit: contain;
}

.generate-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  width: 100%;
  min-height: 52px;
  margin-top: 18px;
  border-radius: 14px;
  background: linear-gradient(135deg, rgb(59 130 246), rgb(20 184 166));
  color: white;
  font-weight: 800;
  box-shadow: 0 16px 34px rgb(20 184 166 / 0.22);
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.generate-button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
  transform: none;
}

.generate-button:not(:disabled):active {
  transform: scale(0.99);
}

.result-panel {
  padding: 18px;
}

.error-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  border-radius: 12px;
  background: rgb(254 242 242);
  color: rgb(185 28 28);
  padding: 10px 12px;
  font-size: 13px;
  font-weight: 700;
}

.dark .error-banner {
  background: rgb(127 29 29 / 0.24);
  color: rgb(252 165 165);
}

.history-strip {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
  overflow-x: auto;
  padding-bottom: 4px;
}

.history-thumb {
  width: 72px;
  height: 72px;
  flex: 0 0 auto;
  overflow: hidden;
  border-radius: 12px;
  border: 2px solid transparent;
  background: rgb(241 245 249);
}

.history-thumb:hover {
  border-color: rgb(59 130 246);
}

.history-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.result-workspace {
  min-height: calc(100vh - 14rem);
  border-radius: 18px;
  background: linear-gradient(135deg, rgb(241 245 249), rgb(226 232 240));
  padding: 18px;
}

.dark .result-workspace {
  background: linear-gradient(135deg, rgb(15 23 42), rgb(30 41 59));
}

.empty-workspace {
  display: flex;
  min-height: calc(100vh - 17rem);
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  color: rgb(100 116 139);
}

.empty-icon {
  display: flex;
  width: 76px;
  height: 76px;
  align-items: center;
  justify-content: center;
  margin-bottom: 22px;
  border-radius: 24px;
  background: white;
  color: rgb(37 99 235);
  box-shadow: 0 18px 45px rgb(15 23 42 / 0.08);
}

.dark .empty-icon {
  background: rgb(15 23 42);
  color: rgb(147 197 253);
}

.empty-workspace h3 {
  margin: 0;
  color: rgb(15 23 42);
  font-size: 20px;
  font-weight: 800;
}

.dark .empty-workspace h3 {
  color: white;
}

.empty-workspace p {
  max-width: 460px;
  margin-top: 10px;
  line-height: 1.7;
}

.image-grid {
  display: grid;
  gap: 16px;
}

.image-grid.count-1 {
  grid-template-columns: minmax(0, 1fr);
}

.image-grid.count-2,
.image-grid.count-3 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.result-card {
  overflow: hidden;
  border-radius: 16px;
  background: white;
  box-shadow: 0 18px 40px rgb(15 23 42 / 0.12);
}

.dark .result-card {
  background: rgb(15 23 42);
}

.result-card img {
  width: 100%;
  aspect-ratio: 1 / 1;
  object-fit: contain;
  background: rgb(15 23 42 / 0.04);
}

.result-card figcaption {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  color: rgb(71 85 105);
  font-size: 12px;
  line-height: 1.5;
}

.dark .result-card figcaption {
  color: rgb(203 213 225);
}

.result-card figcaption span {
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
}

.image-actions {
  flex-shrink: 0;
}

@media (max-width: 1100px) {
  .image-generator-shell {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .generator-panel,
  .result-panel {
    border-radius: 12px;
  }

  .segmented-grid.four {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .image-grid.count-2,
  .image-grid.count-3 {
    grid-template-columns: 1fr;
  }
}
</style>
