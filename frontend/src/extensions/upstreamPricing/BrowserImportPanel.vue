<template>
  <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
    <div class="flex flex-wrap items-center gap-3">
      <button type="button" class="btn btn-secondary" :disabled="busy || !config.base_url" @click="start">自动读取凭证</button>
      <a v-if="config.base_url" :href="installer" target="_blank" rel="noopener noreferrer" class="text-sm text-primary-600 underline">安装 / 更新 Firefox 连接脚本</a>
    </div>
    <p class="text-xs text-gray-500">Firefox 首次需安装 Tampermonkey 和当前站点连接脚本，并允许访问本机。以后点击“自动读取凭证”，会打开站点并自动导入已登录令牌，无需再次点击站点或复制。每个站点安装一次；更换本机地址后需更新脚本。站点仅在内存保存令牌时仍需手动导入。</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <template v-if="pending">
      <p class="text-sm">已发起短时连接。若新标签页被浏览器阻止，<a :href="automaticSite" target="_blank" rel="noopener noreferrer" class="text-primary-600 underline">打开站点自动读取</a>。</p>
      <details class="text-sm"><summary class="cursor-pointer text-gray-500">未安装脚本时的手动连接</summary>
      <ol class="mt-2 list-inside list-decimal space-y-2">
        <li>将此链接拖入书签栏：<a :href="bookmark" @click.prevent class="text-primary-600 underline">连接 Sub2API 定价</a> <button type="button" class="text-primary-600 underline" @click="copy">复制书签地址</button></li>
        <li>打开 <a :href="site" target="_blank" rel="noopener noreferrer" class="text-primary-600 underline">已配置站点</a>，点击该书签并确认连接。</li>
        <li>在本机连接页授权验证面板令牌，然后回到这里。书签 {{ new Date(pending.expires_at).toLocaleTimeString() }} 失效。</li>
      </ol></details>
      <p role="status" class="text-xs text-gray-500">{{ status }}</p>
      <p class="text-xs text-gray-500">若站点阻止书签或弹窗，可直接 <a :href="receiver" target="_blank" rel="noopener noreferrer" class="text-primary-600 underline">打开本机连接页</a>；也可继续使用上方手动输入框。</p>
    </template>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { upstreamPricingAPI } from './api'
import { stationConnectBookmark } from './browserConnect'
import type { BrowserImportStart, BrowserImportStatus, PricingConfig } from '.'
const props = defineProps<{ accountId: number; revision: number; config: PricingConfig }>()
const emit = defineEmits<{ imported: [value: BrowserImportStatus] }>()
const pending = ref<BrowserImportStart>(), busy = ref(false), error = ref(''), status = ref('等待连接…'), site = ref('')
const receiver = computed(() => pending.value ? `${window.location.origin}/pricing-browser-connect#${pending.value.ticket}` : '')
const installer = computed(() => `${window.location.origin}/api/v1/upstream-pricing/browser-connector.user.js?${new URLSearchParams({ site: props.config.base_url, origin: window.location.origin })}`)
const automaticSite = computed(() => {
  if (!pending.value) return ''
  const url = new URL(site.value)
  url.hash = `sub2api-connect=${pending.value.ticket}`
  return url.toString()
})
const bookmark = computed(() => pending.value ? stationConnectBookmark(site.value, receiver.value) : '')
const controller = new AbortController()
let timer: ReturnType<typeof setTimeout> | undefined
let generation = 0
function cancel() { generation++; clearTimeout(timer); pending.value = undefined }
watch(() => [props.accountId, props.revision, props.config.provider, props.config.base_url, props.config.user_id, props.config.token], cancel)
async function start() {
  // Reserve the tab during the user gesture, before awaiting the local API.
  const popup = window.open('about:blank', '_blank')
  if (popup) popup.opener = null
  cancel(); const current = generation
  busy.value = true; error.value = ''; status.value = '等待连接…'
  try {
    const value = await upstreamPricingAPI.startImport(props.accountId, props.revision, props.config, window.location.origin, controller.signal)
    if (current !== generation) { popup?.close(); return }
    site.value = props.config.base_url; pending.value = value; timer = setTimeout(() => poll(current), 2000)
    if (popup) popup.location.replace(automaticSite.value)
  } catch (e) { popup?.close(); if (!controller.signal.aborted) error.value = (e as Error).message || '连接失败' }
  finally { busy.value = false }
}
async function poll(current: number) {
  if (!pending.value || current !== generation) return
  if (Date.now() >= Date.parse(pending.value.expires_at)) { cancel(); error.value = '连接已过期，请重新发起'; return }
  try {
    const value = await upstreamPricingAPI.importStatus(props.accountId, pending.value.id, controller.signal)
    if (current !== generation) return
    if (value.status === 'ready') { cancel(); emit('imported', value); return }
    if (value.status === 'failed') { cancel(); error.value = value.error || '验证失败，请重新连接'; return }
    status.value = value.status === 'validating' ? '正在验证面板读取权限…' : '等待连接…'
    timer = setTimeout(() => poll(current), 2000)
  } catch (e) { if (current === generation && !controller.signal.aborted) { cancel(); error.value = (e as Error).message || '连接失败' } }
}
async function copy() {
  try { await navigator.clipboard.writeText(bookmark.value); status.value = '已复制短时书签地址，未包含面板凭证' }
  catch { error.value = '无法复制，请将链接拖入书签栏' }
}
onBeforeUnmount(() => { cancel(); controller.abort() })
</script>
