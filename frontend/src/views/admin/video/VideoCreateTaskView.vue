<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '试跑任务' : '创建视频任务' }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ isVideoGatewayDemoMode ? '先用一条演示任务检查系统是否能正常接收、处理和记录任务，不会调用真实生成服务。' : '提交文生视频、图生视频或参考视频任务，创建成功后自动进入任务详情。' }}
          </p>
        </div>
        <RouterLink class="btn btn-outline" to="/admin/video/tasks">
          <Icon name="document" size="sm" />
          {{ isVideoGatewayDemoMode ? '任务记录' : '任务列表' }}
        </RouterLink>
      </div>

      <section v-if="false && isVideoGatewayDemoMode" class="rounded-lg border border-teal-200 bg-teal-50 p-5 dark:border-teal-500/20 dark:bg-teal-500/10">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <p class="text-sm font-medium text-teal-700 dark:text-teal-200">主路径</p>
            <h2 class="mt-2 text-xl font-semibold text-gray-950 dark:text-white">通过公司统一入口提交 AI 中剧 / 短剧任务</h2>
            <p class="mt-2 text-sm text-teal-800 dark:text-teal-100">内部页面、脚本、n8n 和自动化工具都通过调用凭证提交任务；中央主机统一选择生成通道、记录镜头决策和经验事件。</p>
          </div>
          <RouterLink class="btn btn-primary" to="/admin/video/tasks">
            <Icon name="document" size="sm" />
            查看任务记录
          </RouterLink>
        </div>
      </section>

      <section v-if="false && isVideoGatewayDemoMode" class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <div class="grid gap-3 md:grid-cols-6">
          <div
            v-for="(step, index) in gatewaySubmitSteps"
            :key="step.title"
            class="relative rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-700/40"
          >
            <div v-if="index > 0" class="absolute -left-2 top-1/2 hidden h-px w-4 bg-gray-300 dark:bg-dark-600 md:block" aria-hidden="true"></div>
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">0{{ index + 1 }}</div>
            <div class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ step.title }}</div>
          </div>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '业务提示词模板' : 'Prompt 模板候选' }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ isVideoGatewayDemoMode ? '选择 AI 中剧 / 短剧生产模板，快速填入试跑任务参数。' : '选择业务候选，系统会自动填入提示词、任务类型、画幅、时长和分辨率。' }}
            </p>
          </div>
        </div>
        <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          <button
            v-for="candidate in promptAssetCandidates"
            :key="candidate.id"
            class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-left transition hover:border-primary-300 hover:bg-primary-50 dark:border-dark-700 dark:bg-dark-700/30 dark:hover:border-primary-500/40 dark:hover:bg-primary-500/10"
            type="button"
            @click="applyTemplate(candidate)"
          >
            <span class="inline-flex rounded-md bg-white px-2 py-1 text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-300">{{ candidate.category }}</span>
            <span class="mt-3 block text-sm font-semibold text-gray-900 dark:text-white">{{ candidate.name }}</span>
            <span class="candidate-prompt mt-2 block text-xs leading-5 text-gray-500 dark:text-gray-400">{{ promptDisplayText(candidate.prompt) }}</span>
            <span class="mt-3 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ candidate.aspect_ratio }} · {{ candidate.duration }}s · {{ candidate.resolution }}</span>
          </button>
        </div>
      </section>

      <div class="grid gap-6 xl:grid-cols-[1fr_0.78fr]">
        <section class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
          <form class="space-y-4" @submit.prevent="submitTask">
            <div>
              <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">任务类型</label>
              <select v-model="form.task_type" class="input">
                <option v-for="item in taskTypeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
              </select>
            </div>

            <div>
              <div class="mb-1 flex items-center justify-between gap-3">
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">提示词</label>
                <div class="flex flex-wrap gap-3">
                  <button class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300" type="button" @click="applyMockSuccess">
                    {{ isVideoGatewayDemoMode ? '填入成功任务示例' : '填入成功演示' }}
                  </button>
                  <button class="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-300" type="button" @click="applyMockFailure">
                    {{ isVideoGatewayDemoMode ? '填入失败任务示例' : '填入失败演示' }}
                  </button>
                </div>
              </div>
              <textarea v-model="form.prompt" class="input min-h-36 resize-y" maxlength="8000" required />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ isVideoGatewayDemoMode ? '失败任务示例会提交预设失败场景，用于查看失败原因和重新发起流程。' : '失败演示会提交一个预设失败场景，用于查看失败原因和重新创建流程。' }}
              </p>
            </div>

            <details class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-700/30">
              <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">高级参数</summary>
              <div class="mt-4 space-y-4">
                <div>
                  <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">模型偏好（可选）</label>
                  <input v-model="form.model" class="input" maxlength="200" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">默认由网关按可用账号选择；普通员工可以保持不改。</p>
                </div>

                <div>
                  <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">负向提示词</label>
                  <textarea v-model="form.negative_prompt" class="input min-h-20 resize-y" maxlength="4000" />
                </div>

                <div>
                  <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">参考图 URL</label>
                  <input v-model="form.reference_image_url" class="input" maxlength="1000" placeholder="https://..." />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">仅图生视频或参考视频任务需要填写；请使用 http 或 https 链接。</p>
                </div>

                <div class="grid gap-4 sm:grid-cols-3">
                  <div>
                    <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">画幅比例</label>
                    <select v-model="form.aspect_ratio" class="input">
                      <option value="16:9">16:9</option>
                      <option value="9:16">9:16</option>
                      <option value="1:1">1:1</option>
                      <option value="4:3">4:3</option>
                    </select>
                  </div>
                  <div>
                    <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">时长（秒）</label>
                    <input v-model.number="form.duration" class="input" type="number" min="1" max="60" />
                  </div>
                  <div>
                    <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">分辨率</label>
                    <select v-model="form.resolution" class="input">
                      <option value="720p">720p</option>
                      <option value="1080p">1080p</option>
                      <option value="480p">480p</option>
                    </select>
                  </div>
                </div>
              </div>
            </details>

            <p v-if="validationMessage" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-300">
              {{ validationMessage }}
            </p>

            <button class="btn btn-primary" type="submit" :disabled="submitting || Boolean(validationMessage)">
              <Icon name="play" size="sm" />
              {{ isVideoGatewayDemoMode ? '试跑一条任务' : '创建任务' }}
            </button>
            <p v-if="isVideoGatewayDemoMode" class="text-xs text-gray-500 dark:text-gray-400">这次只检查系统接收、处理和记录能力，不会调用真实生成服务。</p>
          </form>
        </section>

        <div v-if="!isVideoGatewayDemoMode" class="space-y-6">
          <section v-if="isVideoGatewayDemoMode" class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">右侧只做说明，不需要员工手动调度</h2>
            <div class="mt-4 space-y-3">
              <div v-for="reason in gatewayReasons" :key="reason" class="flex items-start gap-3 text-sm text-gray-600 dark:text-gray-300">
                <Icon name="checkCircle" size="sm" class="mt-0.5 shrink-0 text-emerald-600 dark:text-emerald-300" />
                <span>{{ reason }}</span>
              </div>
            </div>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '生成通道状态（系统自动参考）' : '通道状态' }}</h2>
            <p v-if="isVideoGatewayDemoMode" class="mt-1 text-sm text-gray-500 dark:text-gray-400">这些状态帮助解释系统为什么能自动调度，普通员工不需要手动选择。</p>
            <div class="mt-4 space-y-3">
              <div v-for="provider in providers" :key="provider.id" class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
                <div class="min-w-0">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="providerBadgeClass(provider.provider)">
                    {{ providerDisplayName(provider) }}
                  </span>
                  <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ modelDisplayName(provider.provider, provider.default_model) }}</div>
                </div>
                <div class="text-right text-xs">
                  <div :class="provider.enabled ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-500 dark:text-gray-400'">
                    {{ providerEnabledLabel(provider.enabled) }}
                  </div>
                  <div class="mt-1 text-gray-500 dark:text-gray-400">{{ providerKeyLabel(provider.api_key_configured, provider.masked_key, provider.key_status, provider.provider) }}</div>
                  <div v-if="provider.route_skip_reason" class="mt-1 text-red-600 dark:text-red-300">{{ humanIssueLabel(provider.route_skip_reason) }}</div>
                </div>
              </div>
              <div v-if="!enabledProviders.length" class="rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
                {{ isVideoGatewayDemoMode ? '暂无可用生成通道，请先到生成通道页启用演示通道。' : '暂无可用通道，请先到模型通道页面启用演示通道。' }}
              </div>
            </div>
            <RouterLink v-if="authStore.isAdmin" class="btn btn-outline mt-5" to="/admin/video/providers">
              <Icon name="key" size="sm" />
              {{ isVideoGatewayDemoMode ? '配置生成通道' : '配置模型通道' }}
            </RouterLink>
          </section>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { videoTaskAPI, type VideoProviderAccount, type VideoTaskCreatePayload } from '@/api/admin/video'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isVideoGatewayDemoMode } from '@/utils/productMode'
import {
  candidateToTaskPayload,
  humanIssueLabel,
  loadTaskDraft,
  modelDisplayName,
  promptAssetCandidates,
  promptDisplayText,
  providerBadgeClass,
  providerDisplayName,
  providerEnabledLabel,
  providerKeyLabel,
  taskTypeOptions,
  type PromptAssetCandidate,
} from './videoUtils'

const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const providers = ref<VideoProviderAccount[]>([])
const submitting = ref(false)

const gatewaySubmitSteps = [
  { title: '选择剧种模板' },
  { title: '填写镜头目标' },
  { title: '统一入口入队' },
  { title: '系统推荐引擎' },
  { title: '记录经验事件' },
  { title: '回收结果反馈' },
]

const gatewayReasons = [
  '员工不接触底层凭证或真实生成账号。',
  '内部页面、脚本和自动化工具都通过接入密钥提交任务。',
  '系统记录剧种、场景、镜头目标、提示词结构、引擎选择和结果反馈。',
  '后续接 Seedance/Kling 授权通道时，员工入口不需要变化。',
]

const form = reactive<VideoTaskCreatePayload>({
  provider_account_id: 0,
  task_type: 'text_to_video',
  model: '',
  prompt: isVideoGatewayDemoMode
    ? '真人短剧第 1 集情绪爆发镜头：女主在雨夜街边听到男主离开的消息，缓慢抬头，眼神从震惊转为克制的愤怒，镜头缓慢推进。'
    : '生成一段企业 API 管理后台的安全产品演示短片，画面清晰、节奏简洁。',
  negative_prompt: '',
  reference_image_url: '',
  aspect_ratio: '16:9',
  duration: 5,
  resolution: '720p',
})

const enabledProviders = computed(() => providers.value.filter((provider) => provider.route_available))
const validationMessage = computed(() => {
  if (!enabledProviders.value.length) return isVideoGatewayDemoMode ? '没有可用生成通道：请先前往生成通道页启用演示通道。' : '没有可用通道：请先前往模型通道页启用演示通道。'
  if (!form.prompt.trim()) return '提示词不能为空：请描述要生成的视频内容。'
  if ((form.duration || 0) < 1 || (form.duration || 0) > 60) return '时长必须在 1 到 60 秒之间。'
  if (form.reference_image_url && !isValidOptionalUrl(form.reference_image_url)) return '参考图 URL 格式不正确：请使用 http 或 https 链接，或留空。'
  return ''
})

async function loadProviders() {
  try {
    const res = await videoTaskAPI.listProviders()
    providers.value = res.items || []
    const first = preferredProvider()
    if (first && !form.model) {
      form.model = defaultModelValue(first)
    }
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '加载生成通道失败' : '加载模型通道失败'))
  }
}

function applyTemplate(template: PromptAssetCandidate) {
  selectPreferredProvider()
  Object.assign(form, candidateToTaskPayload(template))
  syncProviderModel()
}

function syncProviderModel() {
  const provider = form.provider_account_id
    ? providers.value.find((item) => item.id === form.provider_account_id)
    : preferredProvider()
  if (provider?.default_model) {
    form.model = defaultModelValue(provider)
  }
}

function preferredProvider() {
  return enabledProviders.value.find((provider) => provider.provider === 'mock') || enabledProviders.value[0]
}

function selectPreferredProvider() {
  const first = preferredProvider()
  if (!first) return
  form.provider_account_id = 0
  form.model = defaultModelValue(first)
}

function selectMockProvider() {
  selectPreferredProvider()
}

function applyMockSuccess() {
  selectMockProvider()
  form.task_type = 'text_to_video'
  form.prompt = isVideoGatewayDemoMode
    ? 'AI 短剧结尾钩子：主角推开旧房门，发现桌上有第二封信和一张陌生合影，镜头从信封推进到主角震惊表情。'
    : '生成一段企业 API 管理后台的安全产品演示短片，画面清晰、节奏简洁。'
  form.negative_prompt = ''
}

function applyMockFailure() {
  selectMockProvider()
  form.task_type = 'text_to_video'
  form.prompt = isVideoGatewayDemoMode
    ? '生成一段用于演示 AI 短剧任务失败审计的镜头：提示词会触发预设失败场景，方便查看失败原因、调度记录和经验事件。[fail]'
    : '生成一段用于演示失败处理的视频任务：提示词会触发预设失败场景，方便查看失败原因和重新创建流程。[fail]'
}

async function submitTask() {
  if (validationMessage.value) {
    appStore.showWarning(validationMessage.value)
    return
  }
  submitting.value = true
  try {
    const payload: VideoTaskCreatePayload = {
      ...form,
      prompt: form.prompt.trim(),
      negative_prompt: form.negative_prompt?.trim(),
      reference_image_url: form.reference_image_url?.trim(),
      model: form.model?.trim(),
    }
    if (!payload.provider_account_id) {
      delete payload.provider_account_id
    }
    if (!payload.model) {
      delete payload.model
    }
    const task = await videoTaskAPI.create(payload)
    appStore.showSuccess(isVideoGatewayDemoMode ? '试跑任务已提交，系统会检查接收、处理和记录流程。' : '任务已进入队列，可在详情查看处理进度。')
    router.push(`/admin/video/tasks/${task.id}`)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '试跑任务提交失败' : '创建视频任务失败'))
  } finally {
    submitting.value = false
  }
}

function isValidOptionalUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    const url = new URL(trimmed)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function defaultModelValue(provider: VideoProviderAccount): string {
  return provider.default_model?.trim() || ''
}

function applyDraftIfPresent() {
  const draft = loadTaskDraft()
  if (!draft) return
  Object.assign(form, draft)
  if (!enabledProviders.value.some((provider) => provider.id === form.provider_account_id)) {
    selectPreferredProvider()
  } else {
    syncProviderModel()
  }
  appStore.showInfo(isVideoGatewayDemoMode ? '已复制参数，可调整后重新发起调用。' : '已复制参数，可调整后重新创建任务。')
}

onMounted(async () => {
  await loadProviders()
  applyDraftIfPresent()
})
</script>

<style scoped>
.candidate-prompt {
  display: -webkit-box;
  min-height: 3.75rem;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}
</style>
