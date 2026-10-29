<template>
  <AppLayout>
    <div class="mx-auto max-w-[1500px] space-y-6 pb-8">
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ tx('对话记录', 'Conversation logs') }}</h1>
          <p class="mt-2 text-sm text-gray-500 dark:text-dark-300">{{ tx('按请求查看用户输入、上下文、模型回复和工具调用。', 'Inspect user prompts, context, model replies and tool calls for each request.') }}</p>
        </div>
        <button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ tx('刷新', 'Refresh') }}</button>
      </header>

      <div v-if="error" role="alert" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>
      <div v-if="notice" role="status" class="rounded-xl bg-emerald-50 p-4 text-sm text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-300">{{ notice }}</div>

      <section class="card p-5" aria-labelledby="capture-title">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h2 id="capture-title" class="text-base font-semibold">{{ tx('正文保存设置', 'Content capture') }}</h2>
          <span class="rounded-full px-3 py-1 text-xs font-medium" :class="runtime?.config.enabled ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'">{{ runtime?.config.enabled ? tx('保存已开启', 'Capture enabled') : tx('保存已关闭', 'Capture disabled') }}</span>
        </div>
        <p class="mt-2 text-sm leading-relaxed text-gray-500 dark:text-dark-300">{{ tx('默认关闭。开启后保存经过本服务的 Chat Completions、Responses 和 Messages HTTP 请求正文与回复。WebSocket 暂不采集，过去未记录的对话无法恢复。', 'Disabled by default. Captures Chat Completions, Responses and Messages HTTP traffic passing through this service. WebSockets are excluded; past conversations cannot be recovered.') }}</p>
        <form class="mt-4 flex flex-wrap items-end gap-5" @submit.prevent="saveConfig">
          <label class="flex min-h-10 items-center gap-2 text-sm"><input v-model="draft.enabled" data-test="capture-enabled" type="checkbox" :disabled="!runtime" class="h-4 w-4 rounded" />{{ tx('保存用户输入和模型回复', 'Save prompts and model replies') }}</label>
          <label class="flex min-h-10 items-center gap-2 text-sm"><input v-model="draft.redact_sensitive" data-test="capture-redaction" type="checkbox" :disabled="!runtime" class="h-4 w-4 rounded" />{{ tx('脱敏保存', 'Redact sensitive fields') }}</label>
          <label class="block text-sm"><span class="mb-1 block">{{ tx('保存天数', 'Retention (days)') }}</span><input v-model.number="draft.retention_days" class="input w-28" type="number" min="1" max="365" required :disabled="!runtime" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ tx('单侧正文上限', 'Limit per request / reply') }}</span><select v-model.number="draft.max_body_bytes" class="input w-36" :disabled="!runtime"><option :value="65536">64 KiB</option><option :value="262144">256 KiB</option><option :value="1048576">1 MiB</option><option :value="2097152">2 MiB</option></select></label>
          <button class="btn btn-primary" type="submit" :disabled="saving || !runtime">{{ saving ? tx('保存中…', 'Saving…') : tx('保存设置', 'Save settings') }}</button>
        </form>
        <p v-if="!draft.redact_sensitive" class="mt-3 text-sm text-amber-700 dark:text-amber-300">{{ tx('脱敏关闭：请求/回复正文中的密码、密钥等字段将按原值保存。仅对之后的请求生效，历史记录不变。', 'Redaction disabled: password and credential fields in future request/reply bodies are stored as supplied. Existing records are unchanged.') }}</p>
        <p class="mt-4 border-t border-gray-100 pt-3 text-xs leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-400">{{ tx('仅管理员可查看。Authorization 请求头始终不保存，图片与内嵌二进制仍省略。到期记录自动分批清理；超出上限时明确标记为截断。', 'Administrator access only. Authorization headers are always excluded and inline binary data is omitted. Expired records are cleaned in batches; size limits are marked explicitly.') }}</p>
        <div v-if="runtime" class="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
          <span>{{ tx('本进程已保存', 'Saved this process') }}: {{ runtime.saved_records }}</span>
          <span>{{ tx('写入队列', 'Queued') }}: {{ runtime.queue_depth }}</span>
          <span :class="runtime.dropped_records ? 'text-amber-700 dark:text-amber-300' : ''">{{ tx('容量不足跳过', 'Skipped at capacity') }}: {{ runtime.dropped_records }}</span>
          <span :class="runtime.failed_writes ? 'text-red-600 dark:text-red-300' : ''">{{ tx('写入失败', 'Write failures') }}: {{ runtime.failed_writes }}</span>
        </div>
      </section>

      <section class="card overflow-hidden">
        <form class="flex flex-wrap items-end gap-3 border-b border-gray-100 p-5 dark:border-dark-700" @submit.prevent="search">
          <label class="block flex-1 text-xs"><span class="mb-1 block">{{ tx('内容 / 请求 ID', 'Content / request ID') }}</span><input v-model="filters.q" class="input w-full min-w-48" maxlength="256" :placeholder="tx('搜索问题或回答', 'Search prompt or reply')" /></label>
          <label class="block text-xs"><span class="mb-1 block">{{ tx('用户 ID', 'User ID') }}</span><input v-model="filters.user_id" class="input w-24" type="number" min="1" /></label>
          <label class="block text-xs"><span class="mb-1 block">{{ tx('模型', 'Model') }}</span><input v-model="filters.model" class="input w-36" maxlength="256" /></label>
          <label class="block text-xs"><span class="mb-1 block">{{ tx('状态', 'Outcome') }}</span><select v-model="filters.outcome" class="input w-32"><option value="">{{ tx('全部', 'All') }}</option><option v-for="s in outcomes" :key="s" :value="s">{{ outcomeLabel(s) }}</option></select></label>
          <label class="block text-xs"><span class="mb-1 block">{{ tx('开始时间', 'From') }}</span><input v-model="filters.from" class="input" type="datetime-local" /></label>
          <label class="block text-xs"><span class="mb-1 block">{{ tx('结束时间', 'Until') }}</span><input v-model="filters.until" class="input" type="datetime-local" /></label>
          <button class="btn btn-primary" :disabled="loading">{{ tx('查询', 'Search') }}</button>
        </form>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-300"><tr><th class="px-5 py-3">{{ tx('时间 / 用户', 'Time / user') }}</th><th class="px-4 py-3">{{ tx('模型 / 状态', 'Model / outcome') }}</th><th class="min-w-56 px-4 py-3">{{ tx('用户问题', 'User prompt') }}</th><th class="min-w-56 px-4 py-3">{{ tx('模型回复', 'Model reply') }}</th><th class="px-5 py-3">{{ tx('操作', 'Actions') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in items" :key="row.id" class="align-top hover:bg-gray-50/70 dark:hover:bg-dark-800/40">
                <td class="whitespace-nowrap px-5 py-4"><div>{{ date(row.created_at) }}</div><div class="mt-1 text-xs text-gray-500">{{ row.user_label || `User #${row.user_id}` }}</div><div class="mt-1 text-xs text-gray-400">{{ row.key_name || `Key #${row.api_key_id}` }}</div></td>
                <td class="px-4 py-4"><div class="font-medium">{{ row.model }}</div><div class="mt-1 text-xs" :class="row.outcome === 'completed' ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300'">{{ outcomeLabel(row.outcome) }} · {{ row.status_code }}</div><div class="mt-1 text-xs text-gray-400">{{ row.stream ? 'SSE' : 'JSON' }} · {{ (row.duration_ms / 1000).toFixed(2) }}s</div><div v-if="row.request_truncated || row.response_truncated" class="mt-1 text-xs text-amber-600">{{ tx('正文截断', 'Content truncated') }}</div></td>
                <td class="max-w-sm px-4 py-4"><p class="line-clamp-3 whitespace-pre-wrap break-words">{{ row.prompt || '—' }}</p></td>
                <td class="max-w-sm px-4 py-4"><p class="line-clamp-3 whitespace-pre-wrap break-words">{{ row.answer || tx('无文本回复，查看工具调用或原始响应', 'No text. Inspect tool calls or raw response.') }}</p></td>
                <td class="px-5 py-4"><button class="btn btn-secondary btn-sm whitespace-nowrap" @click="openDetail(row.id)">{{ tx('查看', 'View') }}</button></td>
              </tr>
              <tr v-if="!items.length"><td colspan="5" class="py-16 text-center text-gray-500">{{ loading ? tx('加载中…', 'Loading…') : tx('暂无记录。开启保存后，通过测试 API 发送一条请求即可查看。', 'No records. Enable capture and send a request through the test API.') }}</td></tr>
            </tbody>
          </table>
        </div>
        <footer class="flex items-center justify-between border-t border-gray-100 px-5 py-4 text-sm dark:border-dark-700"><span>{{ tx('总记录', 'Total') }}: {{ total }} · {{ page }} / {{ pages }}</span><div class="flex gap-2"><button class="btn btn-secondary btn-sm" :disabled="page <= 1 || loading" @click="changePage(-1)">{{ tx('上一页', 'Previous') }}</button><button class="btn btn-secondary btn-sm" :disabled="page >= pages || loading" @click="changePage(1)">{{ tx('下一页', 'Next') }}</button></div></footer>
      </section>
    </div>

    <BaseDialog :show="showDetail" :title="tx('请求详情', 'Request details')" width="extra-wide" @close="closeDetail">
      <div v-if="detailLoading" class="py-12 text-center text-gray-500">{{ tx('加载中…', 'Loading…') }}</div>
      <div v-else-if="detail" class="space-y-5">
        <dl class="grid grid-cols-1 gap-3 rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800 sm:grid-cols-2"><div><dt class="text-xs text-gray-500">Request ID</dt><dd class="mt-1 break-all font-mono text-xs">{{ detail.request_id }}</dd></div><div><dt class="text-xs text-gray-500">{{ tx('用户 / Key', 'User / key') }}</dt><dd class="mt-1">{{ detail.user_label || detail.user_id }} · {{ detail.key_name || detail.api_key_id }}</dd></div><div><dt class="text-xs text-gray-500">{{ tx('接口', 'Endpoint') }}</dt><dd class="mt-1 font-mono text-xs">{{ detail.endpoint }}</dd></div><div><dt class="text-xs text-gray-500">{{ tx('结果', 'Result') }}</dt><dd class="mt-1">{{ outcomeLabel(detail.outcome) }} · {{ detail.model }} · {{ detail.duration_ms }} ms</dd></div></dl>
        <div v-if="detail.request_truncated || detail.response_truncated" role="alert" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">{{ tx('此记录达到正文大小上限，显示的内容并不完整。', 'This record reached the capture limit. Its content is not complete.') }}</div>
        <section><h3 class="mb-2 text-sm font-semibold">{{ tx('本次用户问题', 'Latest user prompt') }}</h3><pre class="content-block">{{ detail.prompt || '—' }}</pre></section>
        <section><h3 class="mb-2 text-sm font-semibold">{{ tx('模型回复', 'Model reply') }}</h3><pre class="content-block">{{ detail.answer || '—' }}</pre></section>
        <section v-if="detail.tool_calls?.length"><h3 class="mb-2 text-sm font-semibold">{{ tx('工具调用', 'Tool calls') }}</h3><pre class="content-block">{{ pretty(detail.tool_calls) }}</pre></section>
        <details class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><summary class="cursor-pointer text-sm font-medium">{{ detail.redacted ? tx('完整请求上下文（JSON，已脱敏）', 'Full request context (redacted JSON)') : tx('完整请求上下文（JSON，未脱敏）', 'Full request context (unredacted JSON)') }}</summary><pre class="content-block mt-3">{{ prettyJSON(detail.request_body) }}</pre></details>
        <details class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><summary class="cursor-pointer text-sm font-medium">{{ detail.redacted ? tx('原始响应（JSON / SSE，已脱敏）', 'Wire response (redacted JSON / SSE)') : tx('原始响应（JSON / SSE，未脱敏）', 'Wire response (unredacted JSON / SSE)') }}</summary><pre class="content-block mt-3">{{ detail.stream ? detail.response_body : prettyJSON(detail.response_body) }}</pre></details>
      </div>
      <template #footer><div class="flex w-full items-center justify-between"><button v-if="detail" class="btn btn-secondary text-red-600" @click="showDelete = true">{{ tx('删除记录', 'Delete record') }}</button><div class="ml-auto flex gap-2"><button v-if="detail" class="btn btn-secondary" @click="download">{{ tx('导出 JSON', 'Export JSON') }}</button><button class="btn btn-primary" @click="closeDetail">{{ tx('关闭', 'Close') }}</button></div></div></template>
    </BaseDialog>
    <BaseDialog :show="showDelete" :title="tx('删除这条记录？', 'Delete this record?')" width="narrow" @close="showDelete = false"><p class="text-sm text-gray-500">{{ tx('将删除本次请求保存的输入、回复和上下文，无法撤销。不会影响账单或上游对话。', 'Deletes the saved prompt, reply and context permanently. Billing records and upstream conversations are unchanged.') }}</p><template #footer><button class="btn btn-secondary" :disabled="deleting" @click="showDelete = false">{{ tx('取消', 'Cancel') }}</button><button class="btn btn-primary" :disabled="deleting" @click="removeDetail">{{ tx('确认删除', 'Delete') }}</button></template></BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { conversationAPI, type CaptureConfig, type CaptureRuntime, type ConversationRecord } from './api'

const { locale } = useI18n()
const tx = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const draft = ref<CaptureConfig>({ enabled: false, retention_days: 7, max_body_bytes: 1048576, redact_sensitive: true })
const runtime = ref<CaptureRuntime | null>(null)
const items = ref<ConversationRecord[]>([])
const total = ref(0), page = ref(1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / 20)))
const loading = ref(false), saving = ref(false), deleting = ref(false)
const error = ref(''), notice = ref('')
const filters = ref({ q: '', user_id: '', model: '', outcome: '', from: '', until: '' })
const outcomes = ['completed', 'failed', 'interrupted', 'incomplete', 'capture_truncated']
const showDetail = ref(false), detailLoading = ref(false), showDelete = ref(false)
const detail = ref<ConversationRecord | null>(null)
let loadGeneration = 0, detailGeneration = 0
const message = (e: unknown) => (e as { message?: string })?.message || tx('请求失败，请重试。', 'Request failed. Please retry.')
const date = (v: string) => new Date(v).toLocaleString(locale.value)
const pretty = (v: unknown) => JSON.stringify(v, null, 2)
const prettyJSON = (v?: string) => { try { return pretty(JSON.parse(v || '{}')) } catch { return v || '' } }
function outcomeLabel(v: string) {
  const labels: Record<string, string> = { completed: tx('完成', 'Completed'), failed: tx('失败', 'Failed'), interrupted: tx('中断', 'Interrupted'), incomplete: tx('未完整结束', 'Incomplete'), capture_truncated: tx('采集截断', 'Capture truncated') }
  return labels[v] || v
}
async function load() {
  const generation = ++loadGeneration
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: page.value, page_size: 20 }
    for (const [k, v] of Object.entries(filters.value)) if (v) params[k] = k === 'from' || k === 'until' ? new Date(v).toISOString() : v
    const result = await conversationAPI.list(params)
    if (generation !== loadGeneration) return
    items.value = result.items; total.value = result.total
  } catch (e) { if (generation === loadGeneration) error.value = message(e) }
  finally { if (generation === loadGeneration) loading.value = false }
}
async function refresh() {
  error.value = ''; notice.value = ''
  try { runtime.value = await conversationAPI.config(); draft.value = { ...runtime.value.config } }
  catch (e) { error.value = message(e) }
  await load()
}
async function saveConfig() {
  saving.value = true; error.value = ''; notice.value = ''
  try {
    await conversationAPI.save({ ...draft.value })
    runtime.value = await conversationAPI.config(); draft.value = { ...runtime.value.config }
    notice.value = tx('设置已保存，后续请求立即生效。', 'Settings saved. They apply to subsequent requests immediately.')
  } catch (e) { error.value = message(e) }
  finally { saving.value = false }
}
async function search() { page.value = 1; error.value = ''; await load() }
async function changePage(delta: number) { page.value += delta; await load() }
async function openDetail(id: number) {
  const generation = ++detailGeneration
  showDetail.value = true; detailLoading.value = true; detail.value = null
  try { const v = await conversationAPI.detail(id); if (generation === detailGeneration) detail.value = v }
  catch (e) { if (generation === detailGeneration) { error.value = message(e); showDetail.value = false } }
  finally { if (generation === detailGeneration) detailLoading.value = false }
}
function closeDetail() { detailGeneration++; showDetail.value = false; detail.value = null }
async function removeDetail() {
  if (!detail.value) return
  deleting.value = true
  try { await conversationAPI.remove(detail.value.id); showDelete.value = false; closeDetail(); await load() }
  catch (e) { error.value = message(e) }
  finally { deleting.value = false }
}
function download() {
  if (!detail.value) return
  const url = URL.createObjectURL(new Blob([pretty(detail.value)], { type: 'application/json' }))
  const a = document.createElement('a'); a.href = url; a.download = `conversation-${detail.value.id}.json`; a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
onMounted(refresh)
</script>

<style scoped>
.content-block { max-height: 28rem; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; border-radius: .75rem; background: rgb(249 250 251); padding: 1rem; font-size: .8125rem; line-height: 1.75; }
:global(.dark) .content-block { background: rgb(17 24 39); color: rgb(209 213 219); }
</style>
