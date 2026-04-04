<template>
  <div class="sora-creator-bar-wrapper">
    <div class="sora-creator-bar">
      <div class="sora-creator-bar-inner" :class="{ focused: isFocused }">
        <!-- 模型选择行 -->
        <div class="sora-creator-model-row">
          <div class="sora-model-select-wrapper">
            <select
              v-model="selectedFamily"
              class="sora-model-select"
              @change="onFamilyChange"
            >
              <optgroup v-if="videoFamilies.length" :label="t('sora.videoModels')">
                <option v-for="f in videoFamilies" :key="f.id" :value="f.id">{{ f.name }}</option>
              </optgroup>
              <optgroup v-if="imageFamilies.length" :label="t('sora.imageModels')">
                <option v-for="f in imageFamilies" :key="f.id" :value="f.id">{{ f.name }}</option>
              </optgroup>
            </select>
            <span class="sora-model-select-arrow">▼</span>
          </div>
          <!-- 凭证选择器 -->
          <div class="sora-credential-select-wrapper">
            <select v-model="selectedCredentialId" class="sora-model-select">
              <option :value="0" disabled>{{ t('sora.selectCredential') }}</option>
              <optgroup v-if="apiKeyOptions.length" :label="t('sora.apiKeys')">
                <option v-for="k in apiKeyOptions" :key="'k'+k.id" :value="k.id">
                  {{ k.name }}{{ k.group ? ' · ' + k.group.name : '' }}
                </option>
              </optgroup>
              <optgroup v-if="subscriptionOptions.length" :label="t('sora.subscriptions')">
                <option v-for="s in subscriptionOptions" :key="'s'+s.id" :value="-s.id">
                  {{ s.group?.name || t('sora.subscription') }}
                </option>
              </optgroup>
            </select>
            <span class="sora-model-select-arrow">▼</span>
          </div>
          <!-- 无凭证提示 -->
          <span v-if="soraCredentialEmpty" class="sora-no-storage-badge">
            ⚠ {{ t('sora.noCredentialHint') }}
          </span>
          <!-- 无存储提示 -->
          <span v-if="!hasStorage" class="sora-no-storage-badge">
            ⚠ {{ t('sora.noStorageConfigured') }}
          </span>
        </div>

        <!-- 参考图预览 -->
        <div v-if="imagePreview" class="sora-image-preview-row">
          <div class="sora-image-preview-thumb">
            <img :src="imagePreview" alt="" />
            <button class="sora-image-preview-remove" @click="removeImage">✕</button>
          </div>
          <span class="sora-image-preview-label">{{ t('sora.referenceImage') }}</span>
        </div>

        <!-- 输入框 -->
        <div class="sora-creator-input-wrapper">
          <textarea
            ref="textareaRef"
            v-model="prompt"
            class="sora-creator-textarea"
            :placeholder="t('sora.creatorPlaceholder')"
            rows="1"
            @input="autoResize"
            @focus="isFocused = true"
            @blur="isFocused = false"
            @keydown.enter.ctrl="submit"
            @keydown.enter.meta="submit"
          />
        </div>

        <!-- 底部工具行 -->
        <div class="sora-creator-tools-row">
          <div class="sora-creator-tools-left">
            <!-- 方向选择（根据所选模型家族支持的方向动态渲染） -->
            <template v-if="availableAspects.length > 0">
              <button
                v-for="a in availableAspects"
                :key="a.value"
                class="sora-tool-btn"
                :class="{ active: currentAspect === a.value }"
                @click="currentAspect = a.value"
              >
                <span class="sora-tool-btn-icon">{{ a.icon }}</span> {{ a.label }}
              </button>

              <span v-if="availableDurations.length > 0" class="sora-tool-divider" />
            </template>

            <!-- 时长选择（根据所选模型家族支持的时长动态渲染） -->
            <template v-if="availableDurations.length > 0">
              <button
                v-for="d in availableDurations"
                :key="d"
                class="sora-tool-btn"
                :class="{ active: currentDuration === d }"
                @click="currentDuration = d"
              >
                {{ d }}s
              </button>

              <span class="sora-tool-divider" />
            </template>

            <!-- 视频数量（官方 Videos 1/2/3） -->
            <template v-if="availableVideoCounts.length > 0">
              <button
                v-for="count in availableVideoCounts"
                :key="count"
                class="sora-tool-btn"
                :class="{ active: currentVideoCount === count }"
                @click="currentVideoCount = count"
              >
                {{ count }}
              </button>

              <span class="sora-tool-divider" />
            </template>

            <!-- 图片上传 -->
            <button class="sora-upload-btn" :title="t('sora.uploadReference')" @click="triggerFileInput">
              📎
            </button>
            <input
              ref="fileInputRef"
              type="file"
              accept="image/png,image/jpeg,image/webp"
              style="display: none"
              @change="onFileChange"
            />
          </div>

          <!-- 活跃任务计数 -->
          <span v-if="activeTaskCount > 0" class="sora-active-tasks-label">
            <span class="sora-pulse-indicator" />
            <span>{{ t('sora.generatingCount', { current: activeTaskCount, max: maxConcurrentTasks }) }}</span>
          </span>

          <!-- 生成按钮 -->
          <button
            class="sora-generate-btn"
            :class="{ 'max-reached': isMaxReached }"
            :disabled="!canSubmit || generating || isMaxReached"
            @click="submit"
          >
            <span class="sora-generate-btn-icon">✨</span>
            <span>{{ generating ? t('sora.generating') : t('sora.generate') }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 文件大小错误 -->
    <p v-if="imageError" class="sora-image-error">{{ imageError }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import soraAPI, { type SoraModelFamily, type GenerateRequest } from '@/api/sora'
import keysAPI from '@/api/keys'
import { useSubscriptionStore } from '@/stores/subscriptions'
import type { ApiKey, UserSubscription } from '@/types'

const MAX_IMAGE_SIZE = 20 * 1024 * 1024

/** 方向显示配置 */
const ASPECT_META: Record<string, { icon: string; label: string }> = {
  landscape: { icon: '▬', label: '横屏' },
  portrait:  { icon: '▮', label: '竖屏' },
  square:    { icon: '◻', label: '方形' }
}

const props = defineProps<{
  generating: boolean
  activeTaskCount: number
  maxConcurrentTasks: number
}>()

const emit = defineEmits<{
  generate: [req: GenerateRequest]
  fillPrompt: [prompt: string]
}>()

const { t } = useI18n()

const prompt = ref('')
const families = ref<SoraModelFamily[]>([])
const selectedFamily = ref('')
const currentAspect = ref('landscape')
const currentDuration = ref(10)
const currentVideoCount = ref(1)
const isFocused = ref(false)
const imagePreview = ref<string | null>(null)
const imageError = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const hasStorage = ref(true)

// 凭证相关状态
const apiKeyOptions = ref<ApiKey[]>([])
const subscriptionOptions = ref<UserSubscription[]>([])
const selectedCredentialId = ref<number>(0) // >0 = api_key.id, <0 = -subscription.id

const soraCredentialEmpty = computed(() =>
  apiKeyOptions.value.length === 0 && subscriptionOptions.value.length === 0
)

// 按类型分组
const videoFamilies = computed(() => families.value.filter(f => f.type === 'video'))
const imageFamilies = computed(() => families.value.filter(f => f.type === 'image'))

// 当前选中的家族对象
const currentFamily = computed(() => families.value.find(f => f.id === selectedFamily.value))

// 当前家族支持的方向列表
const availableAspects = computed(() => {
  const fam = currentFamily.value
  if (!fam?.orientations?.length) return []
  return fam.orientations
    .map(o => ({ value: o, ...(ASPECT_META[o] || { icon: '?', label: o }) }))
})

// 当前家族支持的时长列表
const availableDurations = computed(() => currentFamily.value?.durations ?? [])
const availableVideoCounts = computed(() => (currentFamily.value?.type === 'video' ? [1, 2, 3] : []))

const isMaxReached = computed(() => props.activeTaskCount >= props.maxConcurrentTasks)
const canSubmit = computed(() =>
  prompt.value.trim().length > 0 && selectedFamily.value && selectedCredentialId.value !== 0
)

/** 构建最终 model ID（family + orientation + duration） */
function buildModelID(): string {
  const fam = currentFamily.value
  if (!fam) return selectedFamily.value

  if (fam.type === 'image') {
    // 图像模型: "gpt-image"（方形）或 "gpt-image-landscape"
    return currentAspect.value === 'square'
      ? fam.id
      : `${fam.id}-${currentAspect.value}`
  }
  // 视频模型: "sora2-landscape-10s"
  return `${fam.id}-${currentAspect.value}-${currentDuration.value}s`
}

/** 切换家族时自动调整方向和时长为首个可用值 */
function onFamilyChange() {
  const fam = families.value.find(f => f.id === selectedFamily.value)
  if (!fam) return
  // 若当前方向不在新家族支持列表中，重置为首个
  if (fam.orientations?.length && !fam.orientations.includes(currentAspect.value)) {
    currentAspect.value = fam.orientations[0]
  }
  // 若当前时长不在新家族支持列表中，重置为首个
  if (fam.durations?.length && !fam.durations.includes(currentDuration.value)) {
    currentDuration.value = fam.durations[0]
  }
  if (fam.type !== 'video') {
    currentVideoCount.value = 1
  }
}

async function loadModels() {
  try {
    families.value = await soraAPI.getModels()
    if (families.value.length > 0 && !selectedFamily.value) {
      selectedFamily.value = families.value[0].id
      onFamilyChange()
    }
  } catch (e) {
    console.error('Failed to load models:', e)
  }
}

async function loadStorageStatus() {
  try {
    const status = await soraAPI.getStorageStatus()
    hasStorage.value = status.s3_enabled && status.s3_healthy
  } catch {
    hasStorage.value = false
  }
}

async function loadSoraCredentials() {
  try {
    // 加载 API Keys，筛选 sora 平台 + active 状态
    const keysRes = await keysAPI.list(1, 100)
    apiKeyOptions.value = (keysRes.items || []).filter(
      (k: ApiKey) => k.status === 'active' && k.group?.platform === 'sora'
    )
    // 加载活跃订阅，筛选 sora 平台
    const subStore = useSubscriptionStore()
    const subs = await subStore.fetchActiveSubscriptions()
    subscriptionOptions.value = subs.filter(
      (s: UserSubscription) => s.status === 'active' && s.group?.platform === 'sora'
    )
    // 自动选择第一个
    if (apiKeyOptions.value.length > 0) {
      selectedCredentialId.value = apiKeyOptions.value[0].id
    } else if (subscriptionOptions.value.length > 0) {
      selectedCredentialId.value = -subscriptionOptions.value[0].id
    }
  } catch (e) {
    console.error('Failed to load sora credentials:', e)
  }
}

function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 120) + 'px'
}

function triggerFileInput() {
  fileInputRef.value?.click()
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  imageError.value = ''
  if (file.size > MAX_IMAGE_SIZE) {
    imageError.value = t('sora.imageTooLarge')
    input.value = ''
    return
  }
  const reader = new FileReader()
  reader.onload = (e) => {
    imagePreview.value = e.target?.result as string
  }
  reader.readAsDataURL(file)
  input.value = ''
}

function removeImage() {
  imagePreview.value = null
  imageError.value = ''
}

function submit() {
  if (!canSubmit.value || props.generating || isMaxReached.value) return
  const modelID = buildModelID()
  const req: GenerateRequest = {
    model: modelID,
    prompt: prompt.value.trim(),
    media_type: currentFamily.value?.type || 'video'
  }
  if ((currentFamily.value?.type || 'video') === 'video') {
    req.video_count = currentVideoCount.value
  }
  if (imagePreview.value) {
    req.image_input = imagePreview.value
  }
  if (selectedCredentialId.value > 0) {
    req.api_key_id = selectedCredentialId.value
  }
  emit('generate', req)
  prompt.value = ''
  imagePreview.value = null
  imageError.value = ''
  if (textareaRef.value) {
    textareaRef.value.style.height = 'auto'
  }
}

/** 外部调用：填充提示词 */
function fillPrompt(text: string) {
  prompt.value = text
  setTimeout(autoResize, 0)
  textareaRef.value?.focus()
}

defineExpose({ fillPrompt })

onMounted(() => {
  loadModels()
  loadStorageStatus()
  loadSoraCredentials()
})
</script>

<style scoped>
.sora-creator-bar-wrapper {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 40;
  background: linear-gradient(to top, var(--sora-bg-primary, #0D0D0D) 60%, transparent 100%);
  padding: 20px 24px 24px;
  pointer-events: none;
}

.sora-creator-bar {
  max-width: 780px;
  margin: 0 auto;
  pointer-events: all;
}

.sora-creator-bar-inner {
  background: var(--sora-bg-secondary, #1A1A1A);
  border: 1px solid var(--sora-border-color, #2A2A2A);
  border-radius: var(--sora-radius-xl, 20px);
  padding: 12px 16px;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}

.sora-creator-bar-inner.focused {
  border-color: var(--sora-accent-primary, #14b8a6);
  box-shadow: 0 0 0 1px var(--sora-accent-primary, #14b8a6), var(--sora-shadow-glow, 0 0 20px rgba(20,184,166,0.3));
}

/* 模型选择行 */
.sora-creator-model-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  padding: 0 4px;
}

.sora-model-select-wrapper {
  position: relative;
}

.sora-model-select {
  appearance: none;
  background: var(--sora-bg-tertiary, #242424);
  color: var(--sora-text-primary, #FFF);
  padding: 5px 28px 5px 10px;
  border-radius: var(--sora-radius-sm, 8px);
  font-size: 12px;
  font-family: "SF Mono", "Fira Code", monospace;
  cursor: pointer;
  border: 1px solid transparent;
  transition: all 150ms ease;
}

.sora-model-select:hover {
  border-color: var(--sora-bg-hover, #333);
}

.sora-model-select:focus {
  border-color: var(--sora-accent-primary, #14b8a6);
  outline: none;
}

.sora-model-select option {
  background: var(--sora-bg-secondary, #1A1A1A);
  color: var(--sora-text-primary, #FFF);
}

.sora-model-select-arrow {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  pointer-events: none;
  font-size: 10px;
  color: var(--sora-text-tertiary, #666);
}

.sora-credential-select-wrapper {
  position: relative;
  max-width: 200px;
}

/* 无存储提示 */
.sora-no-storage-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 10px;
  background: rgba(245, 158, 11, 0.1);
  border: 1px solid rgba(245, 158, 11, 0.2);
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 11px;
  color: var(--sora-warning, #F59E0B);
}

/* 参考图预览 */
.sora-image-preview-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 4px;
  margin-bottom: 8px;
}

.sora-image-preview-thumb {
  position: relative;
  width: 48px;
  height: 48px;
}

.sora-image-preview-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 8px;
  border: 1px solid var(--sora-border-color, #2A2A2A);
}

.sora-image-preview-remove {
  position: absolute;
  top: -6px;
  right: -6px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--sora-error, #EF4444);
  color: white;
  font-size: 10px;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
}

.sora-image-preview-label {
  font-size: 12px;
  color: var(--sora-text-tertiary, #666);
}

/* 输入框 */
.sora-creator-input-wrapper {
  position: relative;
}

.sora-creator-textarea {
  width: 100%;
  min-height: 44px;
  max-height: 120px;
  padding: 10px 4px;
  font-size: 14px;
  color: var(--sora-text-primary, #FFF);
  background: transparent;
  resize: none;
  line-height: 1.5;
  overflow-y: auto;
  border: none;
  outline: none;
  font-family: inherit;
}

.sora-creator-textarea::placeholder {
  color: var(--sora-text-muted, #4A4A4A);
}

/* 底部工具行 */
.sora-creator-tools-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 4px 0;
  border-top: 1px solid var(--sora-border-subtle, #1F1F1F);
  margin-top: 4px;
  padding-top: 10px;
  gap: 8px;
}

.sora-creator-tools-left {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.sora-tool-btn {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 6px 12px;
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 12px;
  color: var(--sora-text-secondary, #A0A0A0);
  background: var(--sora-bg-tertiary, #242424);
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
  white-space: nowrap;
}

.sora-tool-btn:hover {
  background: var(--sora-bg-hover, #333);
  color: var(--sora-text-primary, #FFF);
}

.sora-tool-btn.active {
  background: rgba(20, 184, 166, 0.15);
  color: var(--sora-accent-primary, #14b8a6);
  border: 1px solid rgba(20, 184, 166, 0.3);
}

.sora-tool-btn-icon {
  font-size: 14px;
  line-height: 1;
}

.sora-tool-divider {
  width: 1px;
  height: 20px;
  background: var(--sora-border-color, #2A2A2A);
  margin: 0 4px;
}

/* 上传按钮 */
.sora-upload-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: var(--sora-radius-sm, 8px);
  background: var(--sora-bg-tertiary, #242424);
  color: var(--sora-text-secondary, #A0A0A0);
  font-size: 16px;
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
}

.sora-upload-btn:hover {
  background: var(--sora-bg-hover, #333);
  color: var(--sora-text-primary, #FFF);
}

/* 活跃任务计数 */
.sora-active-tasks-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  background: rgba(20, 184, 166, 0.12);
  border: 1px solid rgba(20, 184, 166, 0.25);
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 12px;
  font-weight: 500;
  color: var(--sora-accent-primary, #14b8a6);
  white-space: nowrap;
  animation: sora-fade-in 0.3s ease;
}

.sora-pulse-indicator {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--sora-accent-primary, #14b8a6);
  animation: sora-pulse-dot 1.5s ease-in-out infinite;
}

@keyframes sora-pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

@keyframes sora-fade-in {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 生成按钮 */
.sora-generate-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 24px;
  background: var(--sora-accent-gradient, linear-gradient(135deg, #14b8a6, #0d9488));
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 13px;
  font-weight: 600;
  color: white;
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
  flex-shrink: 0;
}

.sora-generate-btn:hover:not(:disabled) {
  background: var(--sora-accent-gradient-hover, linear-gradient(135deg, #2dd4bf, #14b8a6));
  box-shadow: var(--sora-shadow-glow, 0 0 20px rgba(20,184,166,0.3));
  transform: translateY(-1px);
}

.sora-generate-btn:active:not(:disabled) {
  transform: translateY(0);
}

.sora-generate-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  transform: none;
  box-shadow: none;
}

.sora-generate-btn.max-reached {
  opacity: 0.4;
  cursor: not-allowed;
}

.sora-generate-btn-icon {
  font-size: 16px;
}

/* 图片错误 */
.sora-image-error {
  text-align: center;
  font-size: 12px;
  color: var(--sora-error, #EF4444);
  margin-top: 8px;
  pointer-events: all;
}

/* 响应式 */
@media (max-width: 600px) {
  .sora-creator-bar-wrapper {
    padding: 12px 12px 16px;
  }

  .sora-creator-tools-left {
    gap: 4px;
  }

  .sora-tool-btn {
    padding: 5px 8px;
    font-size: 11px;
  }
}
</style>
