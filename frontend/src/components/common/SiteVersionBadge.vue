<template>
  <div ref="root" class="site-version">
    <button v-if="auth.isAdmin" ref="trigger" type="button" class="version-trigger"
      :class="{ 'has-update': info?.has_update }" :aria-expanded="open" aria-controls="site-version-panel"
      aria-label="版本信息" @click="toggle">
      <span>{{ currentVersion ? `v${currentVersion}` : '版本检测中' }}</span>
      <span v-if="info?.has_update" class="version-dot" aria-label="有新版本" />
    </button>
    <span v-else-if="currentVersion" class="version-public">v{{ currentVersion }}</span>
    <Teleport to="body">
      <section v-if="open && auth.isAdmin" id="site-version-panel" ref="panel" class="version-panel"
        role="dialog" aria-label="版本信息" :style="position">
        <header class="version-panel-header">
          <span>当前版本</span>
          <button type="button" class="version-refresh" title="检查更新" aria-label="检查更新"
            :disabled="loading" @click="check(true)">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          </button>
        </header>
        <div class="version-panel-body">
          <strong class="version-number">{{ currentVersion ? `v${currentVersion}` : '--' }}</strong>
          <p class="version-latest">{{ info?.latest_version ? `最新版本：v${info.latest_version}` : '尚未获取最新版本' }}</p>
          <div v-if="info?.has_update" class="version-update">
            <Icon name="download" size="md" />
            <div><strong>有新版本可用！</strong><span>v{{ info.latest_version }}</span></div>
          </div>
          <p v-else-if="loading" class="version-status" role="status">正在检查更新…</p>
          <p v-else-if="!error && info" class="version-status">已是最新版本</p>
          <p v-if="error" class="version-error" role="status">{{ error }}</p>
          <p v-if="info?.build_type === 'custom'" class="version-status">BUAGA 定制版：升级和回滚须使用已验证的定制镜像。</p>
          <a v-if="releaseURL" class="version-release" :href="releaseURL" target="_blank" rel="noopener noreferrer">
            查看更新日志 <Icon name="externalLink" size="sm" />
          </a>
        </div>
      </section>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { checkUpdates, type VersionInfo } from '@/api/admin/system'
import Icon from '@/components/icons/Icon.vue'

const auth = useAuthStore()
const app = useAppStore()
const info = ref<VersionInfo | null>(null)
const loading = ref(false)
const error = ref('')
const open = ref(false)
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const position = ref<Record<string, string>>({})
const currentVersion = computed(() => (info.value?.current_version || app.siteVersion || '').replace(/^v/, ''))
const releaseURL = computed(() => {
  try {
    const url = new URL(info.value?.release_info?.html_url || '')
    return url.protocol === 'https:' && url.hostname === 'github.com' && url.pathname.startsWith('/Wei-Shaw/sub2api/releases/') ? url.href : ''
  } catch { return '' }
})
let checkedAt = 0
let timer: ReturnType<typeof setInterval> | undefined
let authGeneration = 0

async function check(force = false) {
  if (!auth.isAdmin || loading.value) return
  const generation = authGeneration
  loading.value = true
  try {
    const response = await checkUpdates(force)
    if (generation !== authGeneration || !auth.isAdmin) return
    info.value = response
    error.value = response.warning ? '暂时无法确认最新版本，可稍后重新检查。' : ''
    checkedAt = Date.now()
  } catch {
    if (generation === authGeneration) error.value = '检查更新失败，请稍后重试。'
  } finally { loading.value = false }
}
function reposition() {
  if (!open.value || !trigger.value) return
  const rect = trigger.value.getBoundingClientRect()
  const width = Math.min(300, window.innerWidth - 24)
  position.value = {
    left: `${Math.max(12, Math.min(rect.left, window.innerWidth - width - 12))}px`,
    top: `${rect.bottom + 8}px`, width: `${width}px`,
    maxHeight: `${Math.max(120, window.innerHeight - rect.bottom - 20)}px`
  }
}
async function toggle() {
  open.value = !open.value
  if (open.value) {
    await nextTick()
    reposition()
    if (Date.now() - checkedAt > 5 * 60_000) void check()
  }
}
function outside(event: MouseEvent) {
  if (!root.value?.contains(event.target as Node) && !panel.value?.contains(event.target as Node)) open.value = false
}
function escape(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) { open.value = false; trigger.value?.focus() }
}
function refreshVisible() {
  if (document.visibilityState === 'visible' && Date.now() - checkedAt >= 30 * 60_000) void check()
}
watch(() => auth.isAdmin, (admin) => {
  authGeneration++
  if (admin) void check()
  else { open.value = false; info.value = null; checkedAt = 0 }
}, { immediate: true })
onMounted(() => {
  timer = setInterval(refreshVisible, 30 * 60_000)
  document.addEventListener('click', outside)
  document.addEventListener('keydown', escape)
  document.addEventListener('visibilitychange', refreshVisible)
  window.addEventListener('resize', reposition)
})
onBeforeUnmount(() => {
  authGeneration++
  clearInterval(timer)
  document.removeEventListener('click', outside)
  document.removeEventListener('keydown', escape)
  document.removeEventListener('visibilitychange', refreshVisible)
  window.removeEventListener('resize', reposition)
})
</script>

<style scoped>
.site-version { line-height: 1; }
.version-trigger, .version-public { display: inline-flex; align-items: center; gap: 6px; font-size: 11px; font-weight: 500; line-height: 16px; color: #667085; }
.version-trigger { padding: 0 6px; border-radius: 4px; background: #f2f4f7; }
.version-trigger.has-update { color: #b45309; background: #fef3c7; }
.version-dot { width: 6px; height: 6px; background: #f59e0b; border-radius: 50%; }
.version-panel { position: fixed; z-index: 100001; overflow: auto; border: 1px solid #e4e7ec; border-radius: 8px; background: white; color: #344054; box-shadow: 0 8px 24px #10182824; font-size: 14px; letter-spacing: 0; }
.version-panel-header { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid #eaecf0; font-weight: 600; }
.version-refresh { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 4px; color: #667085; }
.version-refresh:hover { background: #f2f4f7; }
.version-panel-body { padding: 20px; text-align: center; }
.version-number { display: block; font-size: 26px; line-height: 34px; overflow-wrap: anywhere; color: #101828; }
.version-latest, .version-status { margin: 8px 0 16px; color: #667085; font-size: 13px; }
.version-update { display: flex; gap: 12px; align-items: center; padding: 14px; border: 1px solid #fde68a; border-radius: 6px; background: #fffbeb; color: #b45309; text-align: left; }
.version-update span { display: block; margin-top: 4px; font-size: 13px; overflow-wrap: anywhere; }
.version-release { display: flex; align-items: center; justify-content: center; gap: 6px; margin-top: 16px; padding: 9px; border-radius: 6px; background: #14b8a6; color: white; font-weight: 500; }
.version-error { margin-top: 12px; font-size: 12px; color: #b45309; }
:global(.dark) .version-panel { background: #1f242b; color: #e5e7eb; border-color: #374151; }
:global(.dark) .version-panel-header { border-color: #374151; }
:global(.dark) .version-number { color: #f9fafb; }
:global(.dark) .version-trigger { background: #292f38; color: #aab2c0; }
:global(.dark) .version-trigger.has-update, :global(.dark) .version-update { background: #493b19; color: #fcd34d; border-color: #705725; }
</style>
