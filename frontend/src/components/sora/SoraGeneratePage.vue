<template>
  <div class="sora-generate-page">
    <div class="sora-task-area">
      <!-- 欢迎区域（无任务时显示） -->
      <div v-if="activeGenerations.length === 0" class="sora-welcome-section">
        <h1 class="sora-welcome-title">{{ t('sora.welcomeTitle') REDACTEDREDACTED</h1>
        <p class="sora-welcome-subtitle">{{ t('sora.welcomeSubtitle') REDACTEDREDACTED</p>
      </div>

      <!-- 示例提示词（无任务时显示） -->
      <div v-if="activeGenerations.length === 0" class="sora-example-prompts">
        <button
          v-for="(example, idx) in examplePrompts"
          :key="idx"
          class="sora-example-prompt"
          @click="fillPrompt(example)"
        >
          {{ example REDACTEDREDACTED
        </button>
      </div>

      <!-- 任务卡片列表 -->
      <div v-if="activeGenerations.length > 0" class="sora-task-cards">
        <SoraProgressCard
          v-for="gen in activeGenerations"
          :key="gen.id"
          :generation="gen"
          @cancel="handleCancel"
          @delete="handleDelete"
          @save="handleSave"
          @retry="handleRetry"
        />
      </div>

      <!-- 无存储提示 Toast -->
      <div v-if="showNoStorageToast" class="sora-no-storage-toast">
        <span>⚠️</span>
        <span>{{ t('sora.noStorageToastMessage') REDACTEDREDACTED</span>
      </div>
    </div>

    <!-- 底部创作栏 -->
    <SoraPromptBar
      ref="promptBarRef"
      :generating="generating"
      :active-task-count="activeTaskCount"
      :max-concurrent-tasks="3"
      @generate="handleGenerate"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import soraAPI, { type SoraGeneration, type GenerateRequest REDACTED from '@/api/sora'
import SoraProgressCard from './SoraProgressCard.vue'
import SoraPromptBar from './SoraPromptBar.vue'

const { t REDACTED = useI18n()

const emit = defineEmits<{
  'task-count-change': [counts: { active: number; generating: boolean REDACTED]
REDACTED>()

const activeGenerations = ref<SoraGeneration[]>([])
const generating = ref(false)
const showNoStorageToast = ref(false)
let pollTimers: Record<number, ReturnType<typeof setTimeout>> = {REDACTED
const promptBarRef = ref<InstanceType<typeof SoraPromptBar> | null>(null)

// 示例提示词
const examplePrompts = [
  '一只金色的柴犬在东京涩谷街头散步，镜头跟随，电影感画面，4K 高清',
  '无人机航拍视角，冰岛极光下的冰川湖面反射绿色光芒，慢速推进',
  '赛博朋克风格的未来城市，霓虹灯倒映在雨后积水中，夜景，电影级色彩',
  '水墨画风格，一叶扁舟在山水间漂泊，薄雾缭绕，中国古典意境'
]

// 活跃任务统计
const activeTaskCount = computed(() =>
  activeGenerations.value.filter(g => g.status === 'pending' || g.status === 'generating').length
)

const hasGeneratingTask = computed(() =>
  activeGenerations.value.some(g => g.status === 'generating')
)

// 通知父组件任务数变化
watch([activeTaskCount, hasGeneratingTask], () => {
  emit('task-count-change', {
    active: activeTaskCount.value,
    generating: hasGeneratingTask.value
  REDACTED)
REDACTED, { immediate: true REDACTED)

// ==================== 浏览器通知 ====================

function requestNotificationPermission() {
  if ('Notification' in window && Notification.permission === 'default') {
    Notification.requestPermission()
  REDACTED
REDACTED

function sendNotification(title: string, body: string) {
  if ('Notification' in window && Notification.permission === 'granted') {
    new Notification(title, { body, icon: '/favicon.ico' REDACTED)
  REDACTED
REDACTED

const originalTitle = document.title
let titleBlinkTimer: ReturnType<typeof setInterval> | null = null

function startTitleBlink(message: string) {
  stopTitleBlink()
  let show = true
  titleBlinkTimer = setInterval(() => {
    document.title = show ? message : originalTitle
    show = !show
  REDACTED, 1000)
  const onFocus = () => {
    stopTitleBlink()
    window.removeEventListener('focus', onFocus)
  REDACTED
  window.addEventListener('focus', onFocus)
REDACTED

function stopTitleBlink() {
  if (titleBlinkTimer) {
    clearInterval(titleBlinkTimer)
    titleBlinkTimer = null
  REDACTED
  document.title = originalTitle
REDACTED

function checkStatusTransition(oldGen: SoraGeneration, newGen: SoraGeneration) {
  const wasActive = oldGen.status === 'pending' || oldGen.status === 'generating'
  if (!wasActive) return
  if (newGen.status === 'completed') {
    const title = t('sora.notificationCompleted')
    const body = t('sora.notificationCompletedBody', { model: newGen.model REDACTED)
    sendNotification(title, body)
    if (document.hidden) startTitleBlink(title)
  REDACTED else if (newGen.status === 'failed') {
    const title = t('sora.notificationFailed')
    const body = t('sora.notificationFailedBody', { model: newGen.model REDACTED)
    sendNotification(title, body)
    if (document.hidden) startTitleBlink(title)
  REDACTED
REDACTED

// ==================== beforeunload ====================

const hasUpstreamRecords = computed(() =>
  activeGenerations.value.some(g => g.status === 'completed' && g.storage_type === 'upstream')
)

function beforeUnloadHandler(e: BeforeUnloadEvent) {
  if (hasUpstreamRecords.value) {
    e.preventDefault()
    e.returnValue = t('sora.beforeUnloadWarning')
    return e.returnValue
  REDACTED
REDACTED

// ==================== 轮询 ====================

function getPollingIntervalByRuntime(createdAt: string): number {
  const createdAtMs = new Date(createdAt).getTime()
  if (Number.isNaN(createdAtMs)) return 3000
  const elapsedMs = Date.now() - createdAtMs
  if (elapsedMs < 2 * 60 * 1000) return 3000
  if (elapsedMs < 10 * 60 * 1000) return 10000
  return 30000
REDACTED

function schedulePolling(id: number) {
  const current = activeGenerations.value.find(g => g.id === id)
  const interval = current ? getPollingIntervalByRuntime(current.created_at) : 3000
  if (pollTimers[id]) clearTimeout(pollTimers[id])
  pollTimers[id] = setTimeout(() => { void pollGeneration(id) REDACTED, interval)
REDACTED

async function pollGeneration(id: number) {
  try {
    const gen = await soraAPI.getGeneration(id)
    const idx = activeGenerations.value.findIndex(g => g.id === id)
    if (idx >= 0) {
      checkStatusTransition(activeGenerations.value[idx], gen)
      activeGenerations.value[idx] = gen
    REDACTED
    if (gen.status === 'pending' || gen.status === 'generating') {
      schedulePolling(id)
    REDACTED else {
      delete pollTimers[id]
    REDACTED
  REDACTED catch {
    delete pollTimers[id]
  REDACTED
REDACTED

async function loadActiveGenerations() {
  try {
    const res = await soraAPI.listGenerations({
      status: 'pending,generating,completed,failed,cancelled',
      page_size: 50
    REDACTED)
    const generations = Array.isArray(res.data) ? res.data : []
    activeGenerations.value = generations
    for (const gen of generations) {
      if ((gen.status === 'pending' || gen.status === 'generating') && !pollTimers[gen.id]) {
        schedulePolling(gen.id)
      REDACTED
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load generations:', e)
  REDACTED
REDACTED

// ==================== 操作 ====================

async function handleGenerate(req: GenerateRequest) {
  generating.value = true
  try {
    const res = await soraAPI.generate(req)
    const gen = await soraAPI.getGeneration(res.generation_id)
    activeGenerations.value.unshift(gen)
    schedulePolling(gen.id)
  REDACTED catch (e: any) {
    console.error('Generate failed:', e)
    alert(e?.response?.data?.message || e?.message || 'Generation failed')
  REDACTED finally {
    generating.value = false
  REDACTED
REDACTED

async function handleCancel(id: number) {
  try {
    await soraAPI.cancelGeneration(id)
    const idx = activeGenerations.value.findIndex(g => g.id === id)
    if (idx >= 0) activeGenerations.value[idx].status = 'cancelled'
  REDACTED catch (e) {
    console.error('Cancel failed:', e)
  REDACTED
REDACTED

async function handleDelete(id: number) {
  try {
    await soraAPI.deleteGeneration(id)
    activeGenerations.value = activeGenerations.value.filter(g => g.id !== id)
  REDACTED catch (e) {
    console.error('Delete failed:', e)
  REDACTED
REDACTED

async function handleSave(id: number) {
  try {
    await soraAPI.saveToStorage(id)
    const gen = await soraAPI.getGeneration(id)
    const idx = activeGenerations.value.findIndex(g => g.id === id)
    if (idx >= 0) activeGenerations.value[idx] = gen
  REDACTED catch (e) {
    console.error('Save failed:', e)
  REDACTED
REDACTED

function handleRetry(gen: SoraGeneration) {
  handleGenerate({ model: gen.model, prompt: gen.prompt, media_type: gen.media_type REDACTED)
REDACTED

function fillPrompt(text: string) {
  promptBarRef.value?.fillPrompt(text)
REDACTED

// ==================== 检查存储状态 ====================

async function checkStorageStatus() {
  try {
    const status = await soraAPI.getStorageStatus()
    if (!status.s3_enabled || !status.s3_healthy) {
      showNoStorageToast.value = true
      setTimeout(() => { showNoStorageToast.value = false REDACTED, 8000)
    REDACTED
  REDACTED catch {
    // 忽略
  REDACTED
REDACTED

onMounted(() => {
  loadActiveGenerations()
  requestNotificationPermission()
  checkStorageStatus()
  window.addEventListener('beforeunload', beforeUnloadHandler)
REDACTED)

onUnmounted(() => {
  Object.values(pollTimers).forEach(clearTimeout)
  pollTimers = {REDACTED
  stopTitleBlink()
  window.removeEventListener('beforeunload', beforeUnloadHandler)
REDACTED)
</script>

<style scoped>
.sora-generate-page {
  padding-bottom: 200px;
  min-height: calc(100vh - 56px);
  display: flex;
  flex-direction: column;
REDACTED

/* 任务区域 */
.sora-task-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
  gap: 24px;
  max-width: 900px;
  margin: 0 auto;
  width: 100%;
REDACTED

/* 欢迎区域 */
.sora-welcome-section {
  text-align: center;
  padding: 60px 0 40px;
REDACTED

.sora-welcome-title {
  font-size: 36px;
  font-weight: 700;
  letter-spacing: -0.03em;
  margin-bottom: 12px;
  background: linear-gradient(135deg, var(--sora-text-primary) 0%, var(--sora-text-secondary) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
REDACTED

.sora-welcome-subtitle {
  font-size: 16px;
  color: var(--sora-text-secondary, #A0A0A0);
  max-width: 480px;
  margin: 0 auto;
  line-height: 1.6;
REDACTED

/* 示例提示词 */
.sora-example-prompts {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  width: 100%;
  max-width: 640px;
REDACTED

.sora-example-prompt {
  padding: 16px 20px;
  background: var(--sora-bg-secondary, #1A1A1A);
  border: 1px solid var(--sora-border-color, #2A2A2A);
  border-radius: var(--sora-radius-md, 12px);
  font-size: 13px;
  color: var(--sora-text-secondary, #A0A0A0);
  cursor: pointer;
  transition: all 150ms ease;
  text-align: left;
  line-height: 1.5;
  font-family: inherit;
REDACTED

.sora-example-prompt:hover {
  background: var(--sora-bg-tertiary, #242424);
  border-color: var(--sora-bg-hover, #333);
  color: var(--sora-text-primary, #FFF);
  transform: translateY(-1px);
REDACTED

/* 任务卡片列表 */
.sora-task-cards {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
REDACTED

/* 无存储 Toast */
.sora-no-storage-toast {
  position: fixed;
  top: 80px;
  right: 24px;
  background: var(--sora-bg-elevated, #2A2A2A);
  border: 1px solid var(--sora-warning, #F59E0B);
  border-radius: var(--sora-radius-md, 12px);
  padding: 14px 20px;
  font-size: 13px;
  color: var(--sora-warning, #F59E0B);
  z-index: 50;
  box-shadow: var(--sora-shadow-lg, 0 8px 32px rgba(0,0,0,0.5));
  animation: sora-slide-in-right 0.3s ease;
  max-width: 340px;
  display: flex;
  align-items: center;
  gap: 10px;
REDACTED

@keyframes sora-slide-in-right {
  from { transform: translateX(100%); opacity: 0; REDACTED
  to { transform: translateX(0); opacity: 1; REDACTED
REDACTED

/* 响应式 */
@media (max-width: 900px) {
  .sora-example-prompts {
    grid-template-columns: 1fr;
  REDACTED
REDACTED

@media (max-width: 600px) {
  .sora-welcome-title {
    font-size: 28px;
  REDACTED

  .sora-task-area {
    padding: 24px 16px;
  REDACTED
REDACTED
</style>
