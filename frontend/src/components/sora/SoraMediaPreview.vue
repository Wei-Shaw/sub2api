<template>
  <Teleport to="body">
    <Transition name="sora-modal">
      <div
        v-if="visible && generation"
        class="sora-preview-overlay"
        @keydown.esc="emit('close')"
      >
        <!-- 背景遮罩 -->
        <div class="sora-preview-backdrop" @click="emit('close')" />

        <!-- 内容区 -->
        <div class="sora-preview-modal">
          <!-- 顶部栏 -->
          <div class="sora-preview-header">
            <h3 class="sora-preview-title">{{ t('sora.previewTitle') }}</h3>
            <button class="sora-preview-close" @click="emit('close')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- 媒体区 -->
          <div class="sora-preview-media-area">
            <video
              v-if="generation.media_type === 'video'"
              :src="generation.media_url"
              class="sora-preview-media"
              controls
              autoplay
            />
            <img
              v-else
              :src="generation.media_url"
              class="sora-preview-media"
              alt=""
            />
          </div>

          <!-- 详情 + 操作 -->
          <div class="sora-preview-footer">
            <!-- 模型 + 时间 -->
            <div class="sora-preview-meta">
              <span class="sora-preview-model-tag">{{ generation.model }}</span>
              <span>{{ formatDateTime(generation.created_at) }}</span>
            </div>
            <!-- 提示词 -->
            <p class="sora-preview-prompt">{{ generation.prompt }}</p>
            <!-- 操作按钮 -->
            <div class="sora-preview-actions">
              <button
                v-if="generation.storage_type === 'upstream'"
                class="sora-preview-btn primary"
                @click="emit('save', generation.id)"
              >
                ☁️ {{ t('sora.save') }}
              </button>
              <a
                v-if="generation.media_url"
                :href="generation.media_url"
                target="_blank"
                download
                class="sora-preview-btn secondary"
                @click="emit('download', generation.media_url)"
              >
                📥 {{ t('sora.download') }}
              </a>
              <button class="sora-preview-btn ghost" @click="emit('close')">
                {{ t('sora.closePreview') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SoraGeneration } from '@/api/sora'

defineProps<{
  visible: boolean
  generation: SoraGeneration | null
}>()

const emit = defineEmits<{
  close: []
  save: [id: number]
  download: [url: string]
}>()

const { t } = useI18n()

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => document.addEventListener('keydown', handleKeydown))
onUnmounted(() => document.removeEventListener('keydown', handleKeydown))
</script>

<style scoped>
.sora-preview-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
}

.sora-preview-backdrop {
  position: absolute;
  inset: 0;
  background: var(--sora-modal-backdrop, rgba(0, 0, 0, 0.4));
  backdrop-filter: blur(4px);
}

.sora-preview-modal {
  position: relative;
  z-index: 10;
  display: flex;
  flex-direction: column;
  max-height: 90vh;
  max-width: 90vw;
  overflow: hidden;
  border-radius: 20px;
  background: var(--sora-bg-secondary, #FFF);
  border: 1px solid var(--sora-border-color, #E5E7EB);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  animation: sora-modal-in 0.3s ease;
}

@keyframes sora-modal-in {
  from { transform: scale(0.95); opacity: 0; }
  to { transform: scale(1); opacity: 1; }
}

.sora-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--sora-border-color, #E5E7EB);
}

.sora-preview-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--sora-text-primary, #111827);
}

.sora-preview-close {
  padding: 6px;
  border-radius: 8px;
  color: var(--sora-text-tertiary, #9CA3AF);
  background: none;
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
}

.sora-preview-close:hover {
  background: var(--sora-bg-tertiary, #F3F4F6);
  color: var(--sora-text-secondary, #6B7280);
}

.sora-preview-media-area {
  flex: 1;
  overflow: auto;
  background: var(--sora-bg-primary, #F9FAFB);
  padding: 8px;
}

.sora-preview-media {
  max-height: 70vh;
  width: 100%;
  border-radius: 8px;
  object-fit: contain;
}

.sora-preview-footer {
  padding: 16px 20px;
  border-top: 1px solid var(--sora-border-color, #E5E7EB);
}

.sora-preview-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  color: var(--sora-text-tertiary, #9CA3AF);
  margin-bottom: 8px;
}

.sora-preview-model-tag {
  padding: 2px 8px;
  background: var(--sora-bg-tertiary, #F3F4F6);
  border-radius: 9999px;
  font-family: "SF Mono", "Fira Code", monospace;
  font-size: 11px;
  color: var(--sora-text-secondary, #6B7280);
}

.sora-preview-prompt {
  font-size: 13px;
  color: var(--sora-text-secondary, #6B7280);
  line-height: 1.5;
  margin-bottom: 16px;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.sora-preview-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sora-preview-btn {
  padding: 8px 16px;
  border-radius: 9999px;
  font-size: 13px;
  font-weight: 500;
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.sora-preview-btn.primary {
  background: var(--sora-accent-gradient);
  color: white;
}

.sora-preview-btn.primary:hover {
  box-shadow: var(--sora-shadow-glow);
}

.sora-preview-btn.secondary {
  background: var(--sora-bg-tertiary, #F3F4F6);
  color: var(--sora-text-secondary, #6B7280);
}

.sora-preview-btn.secondary:hover {
  background: var(--sora-bg-hover, #E5E7EB);
  color: var(--sora-text-primary, #111827);
}

.sora-preview-btn.ghost {
  background: transparent;
  color: var(--sora-text-tertiary, #9CA3AF);
  margin-left: auto;
}

.sora-preview-btn.ghost:hover {
  color: var(--sora-text-secondary, #6B7280);
}

/* 过渡动画 */
.sora-modal-enter-active,
.sora-modal-leave-active {
  transition: opacity 0.2s ease;
}
.sora-modal-enter-from,
.sora-modal-leave-to {
  opacity: 0;
}
</style>
