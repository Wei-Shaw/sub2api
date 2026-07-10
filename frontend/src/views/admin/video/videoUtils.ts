import type { VideoProvider, VideoProviderAccount, VideoTask, VideoTaskCreatePayload, VideoTaskStatus, VideoTaskType } from '@/api/admin/video'
import { isVideoGatewayDemoMode, videoGatewayDisplayProvider } from '@/utils/productMode'

export const providerOptions: Array<{ value: VideoProvider; label: string }> = [
  { value: 'mock', label: isVideoGatewayDemoMode ? '试跑任务' : '演示通道' },
  { value: 'seedance', label: 'Seedance 2.0' },
  { value: 'kling', label: 'Kling' },
]

export const statusOptions: Array<{ value: VideoTaskStatus; label: string }> = [
  { value: 'queued', label: '排队中' },
  { value: 'submitted', label: isVideoGatewayDemoMode ? '处理中' : '已提交' },
  { value: 'running', label: isVideoGatewayDemoMode ? '处理中' : '生成中' },
  { value: 'succeeded', label: '已完成' },
  { value: 'failed', label: '失败，需要查看原因' },
  { value: 'cancelled', label: '已取消' },
]

export const taskTypeOptions: Array<{ value: VideoTaskType; label: string }> = [
  { value: 'text_to_video', label: '文生视频' },
  { value: 'image_to_video', label: '图生视频' },
  { value: 'reference_to_video', label: '参考视频生成' },
]

export interface PromptAssetCandidate {
  id: string
  name: string
  category: string
  task_type: VideoTaskType
  prompt: string
  negative_prompt: string
  reference_image_url?: string
  reference_video_url?: string
  aspect_ratio: string
  duration: number
  resolution: string
}

export const promptAssetCandidates: PromptAssetCandidate[] = [
  {
    id: 'urban-emotion-closeup',
    name: isVideoGatewayDemoMode ? '现代都市情绪爆发' : '短剧情绪镜头',
    category: '真人短剧',
    task_type: 'image_to_video',
    prompt: '真人短剧第 1 集情绪爆发镜头：女主在雨夜街边听到男主离开的消息，缓慢抬头，眼神从震惊转为克制的愤怒，镜头缓慢推进，电影感光影，9:16 竖屏。',
    negative_prompt: '脸部漂移、字幕遮挡、多余手指、低清晰度、过度夸张表演',
    aspect_ratio: '9:16',
    duration: 5,
    resolution: '1080p',
  },
  {
    id: 'cliffhanger-reveal',
    name: isVideoGatewayDemoMode ? '结尾钩子反转' : '悬念结尾镜头',
    category: 'AI短剧',
    task_type: 'text_to_video',
    prompt: 'AI 短剧结尾钩子：主角推开旧房门，发现桌上有第二封信和一张陌生合影，镜头从信封推进到主角震惊表情，最后停在门后黑影，悬疑反转。',
    negative_prompt: '字幕遮挡、画面抖动、无关人物、低清晰度',
    aspect_ratio: '9:16',
    duration: 5,
    resolution: '1080p',
  },
  {
    id: 'ancient-costume-conflict',
    name: isVideoGatewayDemoMode ? '古风身份反转' : '古风冲突镜头',
    category: '古风短剧',
    task_type: 'reference_to_video',
    prompt: '古风短剧情身份反转：女主在大殿中摘下面纱，众人震惊后退，男主从愤怒转为迟疑，镜头从环境建立切到过肩对话，再推进到女主坚定眼神。',
    negative_prompt: '现代服装、面部变形、背景穿帮、文字水印',
    aspect_ratio: '9:16',
    duration: 5,
    resolution: '1080p',
  },
  {
    id: 'anime-drama-dialogue',
    name: isVideoGatewayDemoMode ? '漫剧多角色对话' : '漫剧对话镜头',
    category: '漫剧',
    task_type: 'text_to_video',
    prompt: '漫剧多角色对话：两名角色在天台对峙，风吹动衣摆，镜头采用过肩对话和中景切换，情绪从压抑到爆发，最后留出反转悬念。',
    negative_prompt: '画风不一致、嘴型错位、肢体扭曲、背景闪烁',
    aspect_ratio: '9:16',
    duration: 5,
    resolution: '1080p',
  },
  {
    id: 'real-to-anime-transform',
    name: isVideoGatewayDemoMode ? '真人转漫剧' : '真人转漫镜头',
    category: '真人转漫剧',
    task_type: 'image_to_video',
    prompt: '真人转漫剧片段：现代都市男主在电梯门关闭前回头，现实光影逐渐转为精致漫剧质感，保留人物身份特征，镜头缓慢后拉形成反转钩子。',
    negative_prompt: '身份特征丢失、面部漂移、风格跳变、文字水印',
    aspect_ratio: '9:16',
    duration: 5,
    resolution: '1080p',
  },
]

export function candidateToTaskPayload(candidate: PromptAssetCandidate): VideoTaskCreatePayload {
  return {
    provider_account_id: 0,
    task_type: candidate.task_type,
    prompt: candidate.prompt,
    negative_prompt: candidate.negative_prompt,
    reference_image_url: candidate.reference_image_url || '',
    reference_video_url: candidate.reference_video_url || '',
    aspect_ratio: candidate.aspect_ratio,
    duration: candidate.duration,
    resolution: candidate.resolution,
  }
}

export function providerLabel(provider: string): string {
  if (isVideoGatewayDemoMode && provider === 'mock') return '试跑任务'
  return videoGatewayDisplayProvider(provider, providerOptions.find((item) => item.value === provider)?.label)
}

export function providerDisplayName(provider: { provider: string; display_name?: string }): string {
  if (isVideoGatewayDemoMode && provider.provider === 'mock') return provider.display_name?.trim() || '试跑任务'
  return videoGatewayDisplayProvider(provider.provider, provider.display_name)
}

export function modelDisplayName(provider: string, model?: string): string {
  if (isVideoGatewayDemoMode && provider === 'mock') return '本机试跑引擎'
  if (isVideoGatewayDemoMode && provider === 'seedance') return 'seedance-2-0-pro'
  if (isVideoGatewayDemoMode && provider === 'kling') return 'kling-v1'
  return model?.trim() || '-'
}

export function promptDisplayText(prompt?: string): string {
  const value = prompt?.trim() || ''
  if (!value) return '-'
  if (!isVideoGatewayDemoMode) return value
  const normalized = value.toLowerCase()
  if (normalized.includes('mock') || normalized.includes('p0.5') || normalized.includes('local forced')) {
    return '无界互娱试跑任务'
  }
  return value.replace(/\[fail\]\s*/gi, '').trim() || '无界互娱试跑任务'
}

export function taskTypeLabel(taskType: string): string {
  return taskTypeOptions.find((item) => item.value === taskType)?.label || taskType
}

export function statusLabel(status: string): string {
  if (isVideoGatewayDemoMode) {
    if (status === 'queued') return '排队中'
    if (status === 'submitted') return '已提交'
    if (status === 'running') return '生成中'
    if (status === 'succeeded') return '已完成'
    if (status === 'failed') return '失败，需要查看原因'
    if (status === 'cancelled') return '已取消'
  }
  return statusOptions.find((item) => item.value === status)?.label || status
}

export function providerDescription(provider: string): string {
  switch (provider) {
    case 'mock':
      return isVideoGatewayDemoMode
        ? '试跑任务：用于验证排队、调度、状态回收和失败记录，不连接真实生成通道。'
        : '演示通道用于本地闭环验证，不调用外部模型。'
    case 'seedance':
      return isVideoGatewayDemoMode
        ? 'Seedance 2.0 预留生成通道，待授权验证后启用。'
        : 'Seedance 2.0 预留通道，待配置调用凭证后启用真实调用。'
    case 'kling':
      return isVideoGatewayDemoMode
        ? 'Kling 预留生成通道，待授权验证后启用。'
        : 'Kling 预留通道，待配置调用凭证后启用真实调用。'
    default:
      return isVideoGatewayDemoMode ? '视频生成通道' : '视频模型通道'
  }
}

export function providerKeyLabel(configured: boolean, maskedKey?: string, keyStatus?: string, provider?: string): string {
  if (isVideoGatewayDemoMode && provider === 'mock') return '试跑任务，不需要真实凭证'
  if (keyStatus === 'normal') return isVideoGatewayDemoMode ? '接入密钥已配置' : '已配置密钥'
  if (keyStatus === 'missing') return isVideoGatewayDemoMode ? '未配置真实凭证' : '未配置密钥'
  if (keyStatus === 'disabled') return isVideoGatewayDemoMode ? '当前未启用' : '停用'
  if (keyStatus === 'auth_failed') return '鉴权失败'
  if (keyStatus === 'rate_limited') return isVideoGatewayDemoMode ? '上游限流' : '触发限流'
  if (keyStatus === 'quota_exhausted') return '额度不足'
  if (!configured) return isVideoGatewayDemoMode ? '未配置真实凭证' : '未配置密钥'
  if (isVideoGatewayDemoMode) return '接入密钥已配置 / 已脱敏'
  return maskedKey ? `已脱敏：${maskedKey}` : '已配置密钥'
}

export function providerEnabledLabel(enabled: boolean): string {
  return enabled ? '当前已启用' : '当前未启用'
}

export function providerRuntimeStatus(provider: Pick<VideoProviderAccount, 'provider' | 'enabled' | 'api_key_configured'> & Partial<VideoProviderAccount>): string {
  if (!provider.enabled) return isVideoGatewayDemoMode ? '当前未启用' : '停用'
  if (provider.provider === 'mock' && isVideoGatewayDemoMode) return '试跑任务，不调用真实生成服务'
  if (provider.route_available === true) return '正常可用'
  if (provider.key_status === 'missing') return isVideoGatewayDemoMode ? '未配置真实凭证' : '未配置密钥'
  if (provider.key_status === 'auth_failed') return '鉴权失败'
  if (provider.key_status === 'rate_limited') return isVideoGatewayDemoMode ? '上游限流' : '触发限流'
  if (provider.key_status === 'quota_exhausted') return '额度不足'
  if (provider.route_skip_reason) return humanIssueLabel(provider.route_skip_reason)
  if (provider.diagnostic_type) return humanIssueLabel(provider.diagnostic_type)
  if (provider.provider === 'mock') return '可演示'
  if (!provider.api_key_configured) return isVideoGatewayDemoMode ? '未配置真实凭证' : '待配置'
  return isVideoGatewayDemoMode ? '真实生成通道待验证' : '已配置'
}

export function providerRuntimeStatusClass(status: string): string {
  switch (status) {
    case '可演示':
    case '演示可用':
    case '试跑任务，不调用真实生成服务':
    case '已配置':
    case '可调用':
    case '正常可用':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
    case '待配置':
    case '未配置密钥':
    case '未配置真实凭证':
    case '触发限流':
    case '上游限流':
    case '真实通道待验证':
    case '真实生成通道待验证':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
    case '停用':
    case '账号已停用':
    case '当前未启用':
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
    default:
      return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
  }
}

export function keyStatusClass(keyStatus?: string): string {
  switch (keyStatus) {
    case 'normal':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
    case 'missing':
    case 'rate_limited':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
    case 'disabled':
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
    default:
      return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
  }
}

export function routeAccountLabel(task: Pick<VideoTask, 'provider_account_id' | 'provider_account_name'>): string {
  if (isVideoGatewayDemoMode) return '本机试跑账号'
  return task.provider_account_name?.trim() || `账号 #${task.provider_account_id}`
}

export function createdByLabel(task: Partial<VideoTask>): string {
  return task.created_by_label?.trim()
    || task.created_by_name?.trim()
    || task.created_by_email?.trim()
    || (task.created_by ? `用户 #${task.created_by}` : '-')
}

export function routingStrategyLabel(strategy?: string): string {
  if (isVideoGatewayDemoMode) return '系统自动安排处理'
  if (strategy === 'least_inflight') return isVideoGatewayDemoMode ? '系统自动选择处理中最少的可用账号' : 'least_inflight（处理中最少）'
  if (strategy === 'explicit') return '指定账号'
  return strategy || '-'
}

export interface RoutingTraceSkippedAccount {
  id: number
  display_name: string
  provider: string
  reason: string
}

export interface RoutingTraceSummary {
  strategy: string
  reason: string
  selected_account_id: number
  selected_account_name: string
  provider: string
  skippedAccounts: RoutingTraceSkippedAccount[]
}

export function extractRoutingTrace(task: Pick<VideoTask, 'events' | 'routing_strategy' | 'routing_reason' | 'provider' | 'provider_account_id' | 'provider_account_name'>): RoutingTraceSummary | null {
  const routedEvent = task.events?.find((event) => event.event_type === 'routed')
  const payload = routedEvent?.payload_json || {}
  const skippedRaw = Array.isArray(payload.skipped_accounts) ? payload.skipped_accounts : []
  const skippedAccounts = skippedRaw
    .map(toRoutingTraceSkippedAccount)
    .filter((item): item is RoutingTraceSkippedAccount => Boolean(item))

  const strategy = safeTraceString(payload.strategy) || task.routing_strategy || ''
  const reason = safeTraceString(payload.reason) || task.routing_reason || ''
  const selectedAccountId = safeTraceNumber(payload.selected_account_id, task.provider_account_id)
  const selectedAccountName = safeTraceString(payload.selected_account_name) || task.provider_account_name || ''
  const provider = safeTraceString(payload.provider) || task.provider || ''

  if (!strategy && !reason && !selectedAccountId && !selectedAccountName && !skippedAccounts.length) {
    return null
  }

  return {
    strategy,
    reason,
    selected_account_id: selectedAccountId,
    selected_account_name: selectedAccountName,
    provider,
    skippedAccounts,
  }
}

function toRoutingTraceSkippedAccount(value: unknown): RoutingTraceSkippedAccount | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  return {
    id: safeTraceNumber(record.id, 0),
    display_name: safeTraceString(record.display_name),
    provider: safeTraceString(record.provider),
    reason: safeTraceString(record.reason),
  }
}

function safeTraceString(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return ''
}

function safeTraceNumber(value: unknown, fallback: number): number {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return fallback
}

export function humanIssueLabel(value?: string | null): string {
  const normalized = (value || '').trim().toLowerCase()
  if (!normalized) return '-'
  if (normalized.includes('missing') || normalized.includes('not configured') || normalized.includes('未配置') || normalized.includes('待配置')) return isVideoGatewayDemoMode ? '未配置真实凭证' : '需要配置密钥'
  if (normalized.includes('disabled') || normalized.includes('停用')) return isVideoGatewayDemoMode ? '当前未启用' : '账号已停用'
  if (normalized.includes('rate') || normalized.includes('limit') || normalized.includes('限流')) return '上游限流'
  if (normalized.includes('auth') || normalized.includes('unauthorized') || normalized.includes('鉴权')) return '鉴权失败'
  if (normalized.includes('quota') || normalized.includes('额度')) return '额度不足'
  if (normalized === '正常') return '正常'
  return value || '-'
}

export function providerSuggestedAction(provider: Partial<VideoProviderAccount>): string {
  if (!provider.enabled) return isVideoGatewayDemoMode ? '确认还需要这个账号后再启用。' : '按业务需要启用通道。'
  if (provider.key_status === 'missing' || !provider.api_key_configured) return '请配置授权真实凭证，再启用真实调用。'
  if (provider.key_status === 'auth_failed') return '请重新粘贴有效凭证，保存后再测试。'
  if (provider.key_status === 'rate_limited') return '请降低并发，或临时切换到其他可用账号。'
  if (provider.key_status === 'quota_exhausted') return '请补充上游额度，或切换到可用账号。'
  if (provider.suggested_action) return actionMessageLabel(provider.suggested_action, provider.key_status)
  if (provider.route_skip_reason) return actionMessageLabel(provider.route_skip_reason, provider.key_status)
  return provider.route_available ? '保持启用，继续观察调用成功率。' : '请查看配置详情并执行一次通道测试。'
}

export function diagnosticSuggestedAction(item: { key_status?: string; suggested_action?: string; recent_error?: string; status?: string }): string {
  if (item.status === '正常') return '无需处理，继续观察。'
  if (item.key_status === 'missing') return '请先配置授权真实凭证。'
  if (item.key_status === 'disabled') return '确认业务需要后再启用该账号。'
  if (item.key_status === 'auth_failed') return '请检查凭证是否过期或填错，更新后重新测试。'
  if (item.key_status === 'rate_limited') return '请降低并发，或切换到其他可用账号。'
  if (item.key_status === 'quota_exhausted') return '请补充上游额度，或暂停使用该账号。'
  return actionMessageLabel(item.suggested_action || item.recent_error || '', item.key_status)
}

function actionMessageLabel(message?: string | null, keyStatus?: string): string {
  const issue = humanIssueLabel(keyStatus || message)
  if (issue === '需要配置密钥' || issue === '未配置真实凭证') return '请配置授权真实凭证，再启用真实调用。'
  if (issue === '账号已停用' || issue === '当前未启用') return '确认业务需要后再启用该账号。'
  if (issue === '上游限流') return '请降低并发，或切换到其他可用账号。'
  if (issue === '鉴权失败') return '请检查凭证是否过期或填错，更新后重新测试。'
  if (issue === '额度不足') return '请补充上游额度，或切换到可用账号。'
  return message || '请查看通道配置并执行一次测试。'
}

export function eventTypeLabel(eventType: string): string {
  if (isVideoGatewayDemoMode) {
    switch (eventType) {
      case 'queued':
        return '任务进入网关队列'
      case 'routed':
        return '系统自动调度账号'
      case 'submitted':
        return '已分发至生成通道'
      case 'running':
        return '生成中'
      case 'succeeded':
        return '已完成'
      case 'failed':
        return '失败，需要查看原因'
      case 'cancelled':
        return '调用已取消'
      case 'timeout':
        return '状态回收超时'
      default:
        return eventType
    }
  }
  switch (eventType) {
    case 'queued':
      return '任务已入队'
    case 'routed':
      return '已选择通道账号'
    case 'submitted':
      return '已提交到通道'
    case 'running':
      return '通道处理中'
    case 'succeeded':
      return '生成成功'
    case 'failed':
      return '生成失败'
    case 'cancelled':
      return '任务已取消'
    case 'timeout':
      return '任务超时'
    default:
      return eventType
  }
}

export function eventMessageLabel(message: string, eventType: string): string {
  const normalized = message.trim().toLowerCase()
  if (!normalized) return eventTypeLabel(eventType)
  if (isVideoGatewayDemoMode) {
    if (normalized === 'video task queued') return '任务进入网关队列'
    if (normalized === 'video task routed') return '已自动选择生成账号'
    if (normalized === 'video task submitted to provider') return '已分发至生成通道'
    if (normalized === 'video task status updated') return '上游状态已回收'
    if (normalized === 'video task succeeded') return '结果已入库'
    if (normalized === 'video task failed') return '失败，需要查看原因'
    if (normalized === 'video task cancelled') return '调用已取消'
    if (normalized === 'video task timed out') return '状态回收超时'
  } else {
    if (normalized === 'video task queued') return '任务已进入视频队列'
    if (normalized === 'video task routed') return '已选择模型通道账号'
    if (normalized === 'video task submitted to provider') return '任务已提交到模型通道'
    if (normalized === 'video task status updated') return '任务状态已更新'
    if (normalized === 'video task succeeded') return '视频生成成功'
    if (normalized === 'video task failed') return '视频生成失败'
    if (normalized === 'video task cancelled') return '任务已取消'
    if (normalized === 'video task timed out') return '任务处理超时'
  }
  return message
}

export function providerTestMessage(message: string): string {
  const normalized = message.trim().toLowerCase()
  if (normalized === 'mock provider is local and ready') {
    return isVideoGatewayDemoMode
      ? '演示通道可用：任务流转验证已就绪，不连接真实生成通道。'
      : '演示通道可用：本地任务流转验证已就绪，不调用外部模型。'
  }
  if (normalized.includes('api key is not configured')) return '真实通道未配置调用凭证：待授权配置后再启用。'
  if (normalized.includes('real network test is disabled')) return '真实通道已映射：当前演示不发起外部网络调用。'
  return message || '暂无测试结果'
}

export function errorMessageLabel(message?: string | null): string {
  if (!message) return ''
  const normalized = message.trim().toLowerCase()
  if (normalized === 'mock provider forced failure for p0 validation') {
    return isVideoGatewayDemoMode
      ? '试跑任务失败：系统按提示词返回失败，失败原因已记录。可复制参数重新发起。'
      : '生成失败：演示通道按提示词返回失败。可复制参数重新创建，或更换模型通道。'
  }
  if (normalized === 'video task timed out') return isVideoGatewayDemoMode ? '调用状态回收超时' : '视频任务处理超时'
  if (normalized.startsWith('video provider submit failed:')) {
    return `${isVideoGatewayDemoMode ? '分发至生成通道失败' : '提交到模型通道失败'}：${message.slice('video provider submit failed:'.length).trim()}`
  }
  if (normalized.startsWith('video provider poll failed:')) {
    return `${isVideoGatewayDemoMode ? '状态回收失败' : '查询模型通道状态失败'}：${message.slice('video provider poll failed:'.length).trim()}`
  }
  return message
}

export function statusBadgeClass(status: string): string {
  switch (status) {
    case 'queued':
      return 'bg-slate-100 text-slate-700 dark:bg-slate-500/10 dark:text-slate-300'
    case 'submitted':
      return 'bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300'
    case 'running':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300'
    case 'succeeded':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
    case 'cancelled':
      return 'bg-slate-100 text-slate-700 dark:bg-slate-500/10 dark:text-slate-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  }
}

export function providerBadgeClass(provider: string): string {
  switch (provider) {
    case 'mock':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
    case 'seedance':
      return 'bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300'
    case 'kling':
      return 'bg-violet-50 text-violet-700 dark:bg-violet-500/10 dark:text-violet-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  }
}

export function formatDate(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

export function shortText(value: string, max = 96): string {
  if (!value) return '-'
  return value.length > max ? `${value.slice(0, max)}...` : value
}

export function isTerminalStatus(status: string): boolean {
  return status === 'succeeded' || status === 'failed' || status === 'cancelled'
}

export const VIDEO_TASK_DRAFT_KEY = 'video_gateway_task_draft'

export function saveTaskDraft(task: VideoTask): void {
  const draft: VideoTaskCreatePayload = {
    provider_account_id: 0,
    task_type: task.task_type,
    model: task.model,
    prompt: task.prompt,
    negative_prompt: task.negative_prompt,
    reference_image_url: task.reference_image_url,
    reference_video_url: task.reference_video_url,
    aspect_ratio: task.aspect_ratio,
    duration: task.duration,
    resolution: task.resolution,
  }
  sessionStorage.setItem(VIDEO_TASK_DRAFT_KEY, JSON.stringify(draft))
}

export function loadTaskDraft(): VideoTaskCreatePayload | null {
  const raw = sessionStorage.getItem(VIDEO_TASK_DRAFT_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as VideoTaskCreatePayload
  } catch {
    return null
  } finally {
    sessionStorage.removeItem(VIDEO_TASK_DRAFT_KEY)
  }
}
