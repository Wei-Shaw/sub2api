<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '生成通道' : '模型通道' }}</h1>
          <p class="mt-1 max-w-4xl text-sm text-gray-500 dark:text-gray-400">
            {{ isVideoGatewayDemoMode ? '管理员看 Seedance / Kling / 演示通道的能力边界、切换状态和真实验证缺口。' : '管理演示通道与未来真实模型通道的启用状态和调用凭证。' }}
          </p>
        </div>
        <button class="btn btn-outline" type="button" :disabled="loading" @click="loadProviders">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          刷新
        </button>
      </div>

      <section v-if="isVideoGatewayDemoMode" class="grid gap-3 md:grid-cols-3">
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">老板先看</div>
          <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">凭证状态是否正常</div>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">未配置真实凭证、鉴权失败、限流都会用人话展示。</p>
        </div>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">员工无需选择</div>
          <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">网关自动挑可用账号</div>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">生成通道只影响系统调度，不让普通用户手动选择账号。</p>
        </div>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">运维处理</div>
          <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">看建议动作而不是日志</div>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">先配置真实凭证、检查鉴权、降并发或启停账号。</p>
        </div>
      </section>

      <section v-if="isVideoGatewayDemoMode" class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">引擎能力矩阵</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">Seedance、Kling、官方接口、内部授权通道和演示通道分开管理；真实生成通道当前均需授权验证。</p>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-700/40 dark:text-gray-400">
              <tr>
                <th class="px-5 py-3 font-medium">引擎</th>
                <th class="px-5 py-3 font-medium">模式</th>
                <th class="px-5 py-3 font-medium">适合场景</th>
                <th class="px-5 py-3 font-medium">能力位</th>
                <th class="px-5 py-3 font-medium">验证状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="profile in engineMatrix" :key="profile.provider">
                <td class="px-5 py-3 font-medium text-gray-900 dark:text-white">{{ profile.provider }}</td>
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ profile.mode }}</td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{{ profile.bestFor }}</td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{{ profile.capabilities }}</td>
                <td class="px-5 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="profile.safe ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'">
                    {{ profile.status }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div class="grid gap-6 xl:grid-cols-[1.35fr_0.65fr]">
        <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
              <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-700/40 dark:text-gray-400">
                <tr>
                  <th class="px-5 py-3 font-medium">通道名称</th>
                  <th class="px-5 py-3 font-medium">凭证状态</th>
                  <th class="px-5 py-3 font-medium">是否启用</th>
                  <th class="px-5 py-3 font-medium">今日调用</th>
                  <th class="px-5 py-3 font-medium">当前处理中</th>
                  <th class="px-5 py-3 font-medium">最近异常 / 未配置原因</th>
                  <th class="px-5 py-3 font-medium">建议动作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr
                  v-for="provider in providers"
                  :key="provider.id"
                  class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/40"
                  :class="selected?.id === provider.id ? 'bg-gray-50 dark:bg-dark-700/40' : ''"
                  @click="selectProvider(provider)"
                >
                  <td class="px-5 py-3">
                    <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="providerBadgeClass(provider.provider)">
                      {{ providerDisplayName(provider) }}
                    </span>
                    <div class="mt-1 max-w-xs text-xs text-gray-500 dark:text-gray-400">{{ providerDescription(provider.provider) }}</div>
                  </td>
                  <td class="px-5 py-3">
                    <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="keyStatusClass(provider.key_status)">
                      {{ providerKeyLabel(provider.api_key_configured, provider.masked_key, provider.key_status, provider.provider) }}
                    </span>
                  </td>
                  <td class="px-5 py-3">
                    <span
                      class="inline-flex rounded-md px-2 py-1 text-xs font-medium"
                      :class="providerRuntimeStatusClass(providerRuntimeStatus(provider))"
                    >
                      {{ provider.enabled ? '当前已启用' : '当前未启用' }}
                    </span>
                  </td>
                  <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ provider.today_tasks }}</td>
                  <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ provider.current_inflight }}</td>
                  <td class="px-5 py-3">
                    <div class="max-w-xs text-xs text-gray-600 dark:text-gray-300">
                      {{ humanIssueLabel(provider.last_error || provider.diagnostic_type || provider.route_skip_reason || provider.key_status) }}
                    </div>
                  </td>
                  <td class="px-5 py-3 text-xs leading-5 text-amber-700 dark:text-amber-300">
                    {{ providerSuggestedAction(provider) }}
                  </td>
                </tr>
                <tr v-if="!loading && !providers.length">
                  <td colspan="7" class="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ isVideoGatewayDemoMode ? '暂无生成通道，请确认初始化数据已写入。' : '暂无通道，请确认数据库迁移已执行。' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '通道详情 / 技术配置' : '通道配置' }}</h2>
          <p v-if="isVideoGatewayDemoMode" class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            左侧表格给老板看，右侧保留技术配置。调用凭证（API Key）加密保存，前端只显示脱敏状态；留空表示保留当前凭证。
          </p>
          <form v-if="selected" class="mt-4 space-y-4" @submit.prevent="saveProvider">
            <div>
              <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">展示名称</label>
              <input v-model="form.display_name" class="input" maxlength="120" />
            </div>
            <div>
              <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ isVideoGatewayDemoMode ? '接口入口地址' : '上游地址' }}</label>
              <input v-model="form.base_url" class="input" maxlength="500" />
            </div>
            <div>
              <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ isVideoGatewayDemoMode ? '默认模型' : '默认模型' }}</label>
              <input v-model="form.default_model" class="input" maxlength="200" />
            </div>
            <div>
              <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">调用凭证（API Key）</label>
              <input v-model="form.api_key" class="input" type="password" autocomplete="off" placeholder="留空表示保留当前凭证" maxlength="4000" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前：{{ providerKeyLabel(selected.api_key_configured, selected.masked_key, selected.key_status, selected.provider) }}</p>
            </div>
            <div>
              <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">每分钟限额</label>
              <input v-model.number="form.rate_limit_per_minute" class="input" type="number" min="0" max="100000" />
            </div>
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="form.enabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              {{ isVideoGatewayDemoMode ? '允许员工通过该生成通道提交任务' : '启用该通道' }}
            </label>
            <div class="flex flex-wrap gap-2">
              <button class="btn btn-primary" type="submit" :disabled="saving">
                <Icon name="check" size="sm" />
                保存
              </button>
              <button class="btn btn-outline" type="button" :disabled="testingId === selected.id" @click="testProvider(selected.id)">
                <Icon name="beaker" size="sm" />
                测试
              </button>
            </div>
          </form>
          <div v-else class="mt-6 text-sm text-gray-500 dark:text-gray-400">{{ isVideoGatewayDemoMode ? '请选择一个生成通道进行编辑或测试。' : '请选择一个通道进行编辑或测试。' }}</div>

          <div v-if="testResult" class="mt-5 rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm dark:border-dark-700 dark:bg-dark-700/40">
            <div class="flex items-center justify-between gap-3">
              <span class="font-medium text-gray-900 dark:text-white">最近一次测试</span>
              <span class="rounded-md px-2 py-1 text-xs font-medium" :class="testResult.reachable ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300' : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'">
                {{ testResult.reachable ? '可用' : '已跳过' }}
              </span>
            </div>
            <p class="mt-2 text-gray-600 dark:text-gray-300">{{ providerTestMessage(testResult.message) }}</p>
            <details v-if="!isVideoGatewayDemoMode" class="mt-3">
              <summary class="cursor-pointer text-xs font-medium text-gray-500 dark:text-gray-400">技术数据（payload）预览</summary>
              <pre class="mt-2 max-h-64 overflow-auto rounded-md bg-white p-3 text-xs text-gray-700 dark:bg-dark-900 dark:text-gray-200">{{ JSON.stringify(testResult.payload_preview || {}, null, 2) }}</pre>
            </details>
          </div>
        </section>
      </div>

      <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">通道健康诊断</h2>
          <p v-if="isVideoGatewayDemoMode" class="mt-1 text-sm text-gray-500 dark:text-gray-400">这里不是报错堆栈，而是告诉老板哪个生成账号影响了任务，以及下一步该处理什么。</p>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-700/40 dark:text-gray-400">
              <tr>
                <th class="px-5 py-3 font-medium">生成通道</th>
                <th class="px-5 py-3 font-medium">系统调度账号</th>
                <th class="px-5 py-3 font-medium">凭证状态</th>
                <th class="px-5 py-3 font-medium">最近测试</th>
                <th class="px-5 py-3 font-medium">异常类型</th>
                <th class="px-5 py-3 font-medium">影响任务数</th>
                <th class="px-5 py-3 font-medium">最近错误</th>
                <th class="px-5 py-3 font-medium">建议动作</th>
                <th class="px-5 py-3 font-medium">状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in dashboard?.health_diagnostics || []" :key="`${item.provider}-${item.route_account}`">
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ providerLabel(item.provider) }}</td>
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ item.route_account }}</td>
                <td class="px-5 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="keyStatusClass(item.key_status)">
                    {{ providerKeyLabel(item.key_status === 'normal', '', item.key_status, item.provider) }}
                  </span>
                </td>
                <td class="px-5 py-3 text-gray-500 dark:text-gray-400">{{ formatDate(item.last_test_at) }}</td>
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ humanIssueLabel(item.exception_type || item.key_status) }}</td>
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ item.impact_tasks }}</td>
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">{{ humanIssueLabel(item.recent_error) }}</td>
                <td class="px-5 py-3 text-amber-700 dark:text-amber-300">{{ diagnosticSuggestedAction(item) }}</td>
                <td class="px-5 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="item.status === '正常' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'">
                    {{ item.status }}
                  </span>
                </td>
              </tr>
              <tr v-if="!(dashboard?.health_diagnostics || []).length">
                <td colspan="9" class="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400">暂无诊断数据。</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { VideoDashboard, VideoProviderAccount, VideoProviderTestResult } from '@/api/admin/video'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isVideoGatewayDemoMode } from '@/utils/productMode'
import {
  diagnosticSuggestedAction,
  formatDate,
  humanIssueLabel,
  keyStatusClass,
  providerBadgeClass,
  providerDescription,
  providerDisplayName,
  providerKeyLabel,
  providerLabel,
  providerRuntimeStatus,
  providerRuntimeStatusClass,
  providerSuggestedAction,
  providerTestMessage,
} from './videoUtils'

const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const testingId = ref<number | null>(null)
const providers = ref<VideoProviderAccount[]>([])
const selected = ref<VideoProviderAccount | null>(null)
const testResult = ref<VideoProviderTestResult | null>(null)
const dashboard = ref<VideoDashboard | null>(null)
const testResultsByProvider = ref<Record<number, VideoProviderTestResult>>({})

const engineMatrix = [
  {
    provider: 'Seedance 2.0',
    mode: '文生 / 图生 / 参考生成',
    bestFor: 'AI 中剧、AI 短剧、全局参考、多模态 reference、多镜头潜力',
    capabilities: '全局参考、图片 / 视频 / 音频参考、运镜 / 表演 / 光影控制',
    status: '待授权验证',
    safe: false,
  },
  {
    provider: 'Kling',
    mode: '文生 / 图生 / 动作控制 / 口型同步',
    bestFor: '真人短剧、漫剧、真人转漫剧、情绪爆发、动作冲突、多角色对话',
    capabilities: '首尾帧、动作控制、口型同步、真人、漫剧、多角色、对话 / 声音模式',
    status: '待授权验证',
    safe: false,
  },
  {
    provider: '官方接口通道',
    mode: '按通道配置',
    bestFor: '后续授权真实闭环验证',
    capabilities: '按官方接口合同拆分模型版本与生成模式',
    status: '未开启',
    safe: false,
  },
  {
    provider: '内部授权通道',
    mode: '接口适配准备',
    bestFor: '公司主机受控试运行',
    capabilities: '只允许后端安全配置，前端不展示凭证',
    status: '未开启',
    safe: false,
  },
  {
    provider: '演示通道',
    mode: '试跑任务',
    bestFor: '内部演示、调度记录、经验沉淀、错误处理验证',
    capabilities: '本地演示闭环，不代表真实生成通道',
    status: '仅内部演示',
    safe: true,
  },
]

const form = reactive({
  display_name: '',
  enabled: false,
  api_key: '',
  base_url: '',
  default_model: '',
  rate_limit_per_minute: 60,
})

function selectProvider(provider: VideoProviderAccount) {
  selected.value = provider
  form.display_name = providerDisplayName(provider)
  form.enabled = provider.enabled
  form.api_key = ''
  form.base_url = provider.base_url
  form.default_model = provider.default_model
  form.rate_limit_per_minute = provider.rate_limit_per_minute
  testResult.value = testResultsByProvider.value[provider.id] || null
}

async function loadProviders() {
  loading.value = true
  try {
    const [providerRes, dashboardRes] = await Promise.all([
      adminAPI.video.listProviders(),
      adminAPI.video.dashboard().catch(() => null),
    ])
    providers.value = providerRes.items || []
    dashboard.value = dashboardRes
    const current = selected.value ? providers.value.find((item) => item.id === selected.value?.id) : providers.value[0]
    if (current) selectProvider(current)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '加载生成通道失败' : '加载模型通道失败'))
  } finally {
    loading.value = false
  }
}

async function saveProvider() {
  if (!selected.value) return
  saving.value = true
  try {
    await adminAPI.video.updateProvider(selected.value.id, {
      display_name: form.display_name,
      enabled: form.enabled,
      base_url: form.base_url,
      default_model: form.default_model,
      rate_limit_per_minute: form.rate_limit_per_minute,
      ...(form.api_key.trim() ? { api_key: form.api_key.trim() } : {}),
    })
    form.api_key = ''
    appStore.showSuccess(isVideoGatewayDemoMode ? '生成通道配置已保存' : '通道配置已保存')
    await loadProviders()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '保存生成通道配置失败' : '保存通道配置失败'))
  } finally {
    saving.value = false
  }
}

async function testProvider(id: number) {
  testingId.value = id
  try {
    const result = await adminAPI.video.testProvider(id)
    testResultsByProvider.value = { ...testResultsByProvider.value, [id]: result }
    testResult.value = result
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '生成通道测试失败' : '通道测试失败'))
  } finally {
    testingId.value = null
  }
}

onMounted(loadProviders)
</script>
