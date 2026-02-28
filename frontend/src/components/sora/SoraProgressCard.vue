<template>
  <div
    class="sora-task-card"
    :class="{
      cancelled: generation.status === 'cancelled',
      'countdown-warning': isUpstream && !isExpired && remainingMs <= 2 * 60 * 1000
    REDACTED"
  >
    <!-- 头部：状态 + 模型 + 取消按钮 -->
    <div class="sora-task-header">
      <div class="sora-task-status">
        <span class="sora-status-dot" :class="statusDotClass" />
        <span class="sora-status-label" :class="statusLabelClass">{{ statusText REDACTEDREDACTED</span>
      </div>
      <div class="sora-task-header-right">
        <span class="sora-model-tag">{{ generation.model REDACTEDREDACTED</span>
        <button
          v-if="generation.status === 'pending' || generation.status === 'generating'"
          class="sora-cancel-btn"
          @click="emit('cancel', generation.id)"
        >
          ✕ {{ t('sora.cancel') REDACTEDREDACTED
        </button>
      </div>
    </div>

    <!-- 提示词 -->
    <div class="sora-task-prompt" :class="{ 'line-through': generation.status === 'cancelled' REDACTED">
      {{ generation.prompt REDACTEDREDACTED
    </div>

    <!-- 错误分类（失败时） -->
    <div v-if="generation.status === 'failed' && generation.error_message" class="sora-task-error-category">
      ⛔ {{ t('sora.errorCategory') REDACTEDREDACTED
    </div>
    <div v-if="generation.status === 'failed' && generation.error_message" class="sora-task-error-message">
      {{ generation.error_message REDACTEDREDACTED
    </div>

    <!-- 进度条（排队/生成/失败时） -->
    <div v-if="showProgress" class="sora-task-progress-wrapper">
      <div class="sora-task-progress-bar">
        <div
          class="sora-task-progress-fill"
          :class="progressFillClass"
          :style="{ width: progressWidth REDACTED"
        />
      </div>
      <div v-if="generation.status !== 'failed'" class="sora-task-progress-info">
        <span>{{ progressInfoText REDACTEDREDACTED</span>
        <span>{{ progressInfoRight REDACTEDREDACTED</span>
      </div>
    </div>

    <!-- 完成预览区 -->
    <div v-if="generation.status === 'completed' && generation.media_url" class="sora-task-preview">
      <video
        v-if="generation.media_type === 'video'"
        :src="generation.media_url"
        class="sora-task-preview-media"
        muted
        loop
        @mouseenter="($event.target as HTMLVideoElement).play()"
        @mouseleave="($event.target as HTMLVideoElement).pause()"
      />
      <img
        v-else
        :src="generation.media_url"
        class="sora-task-preview-media"
        alt=""
      />
    </div>

    <!-- 完成占位预览（无 media_url 时） -->
    <div v-else-if="generation.status === 'completed' && !generation.media_url" class="sora-task-preview">
      <div class="sora-task-preview-placeholder">🎨</div>
    </div>

    <!-- 操作按钮 -->
    <div v-if="showActions" class="sora-task-actions">
      <!-- 已完成 -->
      <template v-if="generation.status === 'completed'">
        <!-- 已保存标签 -->
        <span v-if="generation.storage_type !== 'upstream'" class="sora-saved-badge">
          ✓ {{ t('sora.savedToCloud') REDACTEDREDACTED
        </span>
        <!-- 保存到存储按钮（upstream 时） -->
        <button
          v-if="generation.storage_type === 'upstream'"
          class="sora-action-btn save-storage"
          @click="emit('save', generation.id)"
        >
          ☁️ {{ t('sora.save') REDACTEDREDACTED
        </button>
        <!-- 本地下载 -->
        <a
          v-if="generation.media_url"
          :href="generation.media_url"
          target="_blank"
          download
          class="sora-action-btn primary"
        >
          📥 {{ t('sora.downloadLocal') REDACTEDREDACTED
        </a>
        <!-- 倒计时文本（upstream） -->
        <span v-if="isUpstream && !isExpired" class="sora-countdown-text">
          ⏱ {{ t('sora.upstreamCountdown', { time: countdownText REDACTED) REDACTEDREDACTED {{ t('sora.canDownload') REDACTEDREDACTED
        </span>
        <span v-if="isUpstream && isExpired" class="sora-countdown-text expired">
          {{ t('sora.upstreamExpired') REDACTEDREDACTED
        </span>
      </template>

      <!-- 失败/取消 -->
      <template v-if="generation.status === 'failed' || generation.status === 'cancelled'">
        <button class="sora-action-btn primary" @click="emit('retry', generation)">
          🔄 {{ generation.status === 'cancelled' ? t('sora.regenrate') : t('sora.retry') REDACTEDREDACTED
        </button>
        <button class="sora-action-btn secondary" @click="emit('delete', generation.id)">
          🗑 {{ t('sora.delete') REDACTEDREDACTED
        </button>
      </template>
    </div>

    <!-- 倒计时进度条（upstream 已完成） -->
    <div v-if="isUpstream && !isExpired && generation.status === 'completed'" class="sora-countdown-bar-wrapper">
      <div class="sora-countdown-bar">
        <div class="sora-countdown-bar-fill" :style="{ width: countdownPercent + '%' REDACTED" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { SoraGeneration REDACTED from '@/api/sora'

const props = defineProps<{ generation: SoraGeneration REDACTED>()
const emit = defineEmits<{
  cancel: [id: number]
  delete: [id: number]
  save: [id: number]
  retry: [gen: SoraGeneration]
REDACTED>()
const { t REDACTED = useI18n()

// ==================== 状态样式 ====================

const statusDotClass = computed(() => {
  const s = props.generation.status
  return {
    queued: s === 'pending',
    generating: s === 'generating',
    completed: s === 'completed',
    failed: s === 'failed',
    cancelled: s === 'cancelled'
  REDACTED
REDACTED)

const statusLabelClass = computed(() => statusDotClass.value)

const statusText = computed(() => {
  const map: Record<string, string> = {
    pending: t('sora.statusPending'),
    generating: t('sora.statusGenerating'),
    completed: t('sora.statusCompleted'),
    failed: t('sora.statusFailed'),
    cancelled: t('sora.statusCancelled')
  REDACTED
  return map[props.generation.status] || props.generation.status
REDACTED)

// ==================== 进度条 ====================

const showProgress = computed(() => {
  const s = props.generation.status
  return s === 'pending' || s === 'generating' || s === 'failed'
REDACTED)

const progressFillClass = computed(() => {
  const s = props.generation.status
  return {
    generating: s === 'pending' || s === 'generating',
    completed: s === 'completed',
    failed: s === 'failed'
  REDACTED
REDACTED)

const progressWidth = computed(() => {
  const s = props.generation.status
  if (s === 'failed') return '100%'
  if (s === 'pending') return '0%'
  if (s === 'generating') {
    // 根据创建时间估算进度
    const created = new Date(props.generation.created_at).getTime()
    const elapsed = Date.now() - created
    // 假设平均 10 分钟完成，最多到 95%
    const progress = Math.min(95, (elapsed / (10 * 60 * 1000)) * 100)
    return `${Math.round(progress)REDACTED%`
  REDACTED
  return '100%'
REDACTED)

const progressInfoText = computed(() => {
  const s = props.generation.status
  if (s === 'pending') return t('sora.queueWaiting')
  if (s === 'generating') {
    const created = new Date(props.generation.created_at).getTime()
    const elapsed = Date.now() - created
    return `${t('sora.waited')REDACTED ${formatElapsed(elapsed)REDACTED`
  REDACTED
  return ''
REDACTED)

const progressInfoRight = computed(() => {
  const s = props.generation.status
  if (s === 'pending') return t('sora.waiting')
  return ''
REDACTED)

function formatElapsed(ms: number): string {
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  const sec = s % 60
  return `${mREDACTED:${sec.toString().padStart(2, '0')REDACTED`
REDACTED

// ==================== 操作按钮 ====================

const showActions = computed(() => {
  const s = props.generation.status
  return s === 'completed' || s === 'failed' || s === 'cancelled'
REDACTED)

// ==================== Upstream 倒计时 ====================

const UPSTREAM_TTL = 15 * 60 * 1000
const now = ref(Date.now())
let countdownTimer: ReturnType<typeof setInterval> | null = null

const isUpstream = computed(() =>
  props.generation.status === 'completed' && props.generation.storage_type === 'upstream'
)

const expireTime = computed(() => {
  if (!props.generation.completed_at) return 0
  return new Date(props.generation.completed_at).getTime() + UPSTREAM_TTL
REDACTED)

const remainingMs = computed(() => Math.max(0, expireTime.value - now.value))
const isExpired = computed(() => remainingMs.value <= 0)
const countdownPercent = computed(() => {
  if (isExpired.value) return 0
  return Math.round((remainingMs.value / UPSTREAM_TTL) * 100)
REDACTED)

const countdownText = computed(() => {
  const totalSec = Math.ceil(remainingMs.value / 1000)
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return `${mREDACTED:${s.toString().padStart(2, '0')REDACTED`
REDACTED)

onMounted(() => {
  if (isUpstream.value) {
    countdownTimer = setInterval(() => {
      now.value = Date.now()
      if (now.value >= expireTime.value && countdownTimer) {
        clearInterval(countdownTimer)
        countdownTimer = null
      REDACTED
    REDACTED, 1000)
  REDACTED
REDACTED)

onUnmounted(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  REDACTED
REDACTED)
</script>

<style scoped>
.sora-task-card {
  background: var(--sora-bg-secondary, #1A1A1A);
  border: 1px solid var(--sora-border-color, #2A2A2A);
  border-radius: var(--sora-radius-lg, 16px);
  padding: 24px;
  transition: all 250ms ease;
  animation: sora-fade-in 0.4s ease;
REDACTED

.sora-task-card:hover {
  border-color: var(--sora-bg-hover, #333);
REDACTED

.sora-task-card.cancelled {
  opacity: 0.6;
  border-color: var(--sora-border-subtle, #1F1F1F);
REDACTED

.sora-task-card.countdown-warning {
  border-color: var(--sora-error, #EF4444) !important;
  box-shadow: 0 0 12px rgba(239, 68, 68, 0.15);
REDACTED

@keyframes sora-fade-in {
  from { opacity: 0; transform: translateY(8px); REDACTED
  to { opacity: 1; transform: translateY(0); REDACTED
REDACTED

/* 头部 */
.sora-task-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
REDACTED

.sora-task-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
REDACTED

.sora-task-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
REDACTED

/* 状态指示点 */
.sora-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
REDACTED

.sora-status-dot.queued { background: var(--sora-text-tertiary, #666); REDACTED
.sora-status-dot.generating {
  background: var(--sora-warning, #F59E0B);
  animation: sora-pulse-dot 1.5s ease-in-out infinite;
REDACTED
.sora-status-dot.completed { background: var(--sora-success, #10B981); REDACTED
.sora-status-dot.failed { background: var(--sora-error, #EF4444); REDACTED
.sora-status-dot.cancelled { background: var(--sora-text-tertiary, #666); REDACTED

@keyframes sora-pulse-dot {
  0%, 100% { opacity: 1; REDACTED
  50% { opacity: 0.4; REDACTED
REDACTED

/* 状态标签 */
.sora-status-label.queued { color: var(--sora-text-secondary, #A0A0A0); REDACTED
.sora-status-label.generating { color: var(--sora-warning, #F59E0B); REDACTED
.sora-status-label.completed { color: var(--sora-success, #10B981); REDACTED
.sora-status-label.failed { color: var(--sora-error, #EF4444); REDACTED
.sora-status-label.cancelled { color: var(--sora-text-tertiary, #666); REDACTED

/* 模型标签 */
.sora-model-tag {
  font-size: 11px;
  padding: 3px 10px;
  background: var(--sora-bg-tertiary, #242424);
  border-radius: var(--sora-radius-full, 9999px);
  color: var(--sora-text-secondary, #A0A0A0);
  font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
REDACTED

/* 取消按钮 */
.sora-cancel-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 12px;
  color: var(--sora-text-secondary, #A0A0A0);
  background: var(--sora-bg-tertiary, #242424);
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
REDACTED

.sora-cancel-btn:hover {
  background: rgba(239, 68, 68, 0.15);
  color: var(--sora-error, #EF4444);
REDACTED

/* 提示词 */
.sora-task-prompt {
  font-size: 14px;
  color: var(--sora-text-secondary, #A0A0A0);
  margin-bottom: 16px;
  line-height: 1.6;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
REDACTED

.sora-task-prompt.line-through {
  text-decoration: line-through;
  color: var(--sora-text-tertiary, #666);
REDACTED

/* 错误分类 */
.sora-task-error-category {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: rgba(239, 68, 68, 0.1);
  border-radius: var(--sora-radius-sm, 8px);
  font-size: 12px;
  color: var(--sora-error, #EF4444);
  margin-bottom: 8px;
REDACTED

.sora-task-error-message {
  font-size: 13px;
  color: var(--sora-text-secondary, #A0A0A0);
  line-height: 1.5;
  margin-bottom: 12px;
REDACTED

/* 进度条 */
.sora-task-progress-wrapper {
  margin-bottom: 16px;
REDACTED

.sora-task-progress-bar {
  width: 100%;
  height: 4px;
  background: var(--sora-bg-hover, #333);
  border-radius: 2px;
  overflow: hidden;
REDACTED

.sora-task-progress-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 400ms ease;
REDACTED

.sora-task-progress-fill.generating {
  background: var(--sora-accent-gradient, linear-gradient(135deg, #14b8a6, #0d9488));
  animation: sora-progress-shimmer 2s ease-in-out infinite;
REDACTED

.sora-task-progress-fill.completed {
  background: var(--sora-success, #10B981);
REDACTED

.sora-task-progress-fill.failed {
  background: var(--sora-error, #EF4444);
REDACTED

@keyframes sora-progress-shimmer {
  0% { opacity: 1; REDACTED
  50% { opacity: 0.6; REDACTED
  100% { opacity: 1; REDACTED
REDACTED

.sora-task-progress-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 12px;
  color: var(--sora-text-tertiary, #666);
REDACTED

/* 预览 */
.sora-task-preview {
  margin-top: 16px;
  border-radius: var(--sora-radius-md, 12px);
  overflow: hidden;
  background: var(--sora-bg-tertiary, #242424);
REDACTED

.sora-task-preview-media {
  width: 100%;
  height: 280px;
  object-fit: cover;
  display: block;
REDACTED

.sora-task-preview-placeholder {
  width: 100%;
  height: 280px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--sora-placeholder-gradient, linear-gradient(135deg, #e0e7ff 0%, #dbeafe 50%, #cffafe 100%));
  font-size: 48px;
REDACTED

/* 操作按钮 */
.sora-task-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
  align-items: center;
REDACTED

.sora-action-btn {
  padding: 8px 20px;
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 13px;
  font-weight: 500;
  border: none;
  cursor: pointer;
  transition: all 150ms ease;
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
REDACTED

.sora-action-btn.primary {
  background: var(--sora-accent-gradient, linear-gradient(135deg, #14b8a6, #0d9488));
  color: white;
REDACTED

.sora-action-btn.primary:hover {
  background: var(--sora-accent-gradient-hover, linear-gradient(135deg, #2dd4bf, #14b8a6));
  box-shadow: var(--sora-shadow-glow, 0 0 20px rgba(20,184,166,0.3));
REDACTED

.sora-action-btn.secondary {
  background: var(--sora-bg-tertiary, #242424);
  color: var(--sora-text-secondary, #A0A0A0);
REDACTED

.sora-action-btn.secondary:hover {
  background: var(--sora-bg-hover, #333);
  color: var(--sora-text-primary, #FFF);
REDACTED

.sora-action-btn.save-storage {
  background: linear-gradient(135deg, #10B981 0%, #059669 100%);
  color: white;
REDACTED

.sora-action-btn.save-storage:hover {
  box-shadow: 0 0 16px rgba(16, 185, 129, 0.3);
REDACTED

/* 已保存标签 */
.sora-saved-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.25);
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 13px;
  font-weight: 500;
  color: var(--sora-success, #10B981);
REDACTED

/* 倒计时文本 */
.sora-countdown-text {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 500;
  color: var(--sora-warning, #F59E0B);
REDACTED

.sora-countdown-text.expired {
  color: var(--sora-error, #EF4444);
REDACTED

/* 倒计时进度条 */
.sora-countdown-bar-wrapper {
  margin-top: 12px;
REDACTED

.sora-countdown-bar {
  width: 100%;
  height: 3px;
  background: var(--sora-bg-hover, #333);
  border-radius: 2px;
  overflow: hidden;
REDACTED

.sora-countdown-bar-fill {
  height: 100%;
  background: var(--sora-warning, #F59E0B);
  border-radius: 2px;
  transition: width 1s linear;
REDACTED

.countdown-warning .sora-countdown-bar-fill {
  background: var(--sora-error, #EF4444);
REDACTED

.countdown-warning .sora-countdown-text {
  color: var(--sora-error, #EF4444);
REDACTED
</style>
