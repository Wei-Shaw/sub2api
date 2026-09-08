import { computed, nextTick, ref, watch } from 'vue'
import { authenticatedMediaUrl, type PluginApi } from '../api/client'
import { imageParameterLabel } from '../parameterLabels'
import { projectConversationMessages } from './conversationMessages'
import type {
  ChatMessage,
  Conversation,
  GenerateResponse,
  GeneratedImage,
  GeneratedImagePayload,
  HistoryRecord,
  HistoryTask,
  ImageApiKey,
  ImagePresetSelection,
  EnumCapability,
  ImageModelCapability,
  ImageReference,
  GenerationSlot,
} from '../types'

type GenerationStatus = 'idle' | 'submitting' | 'polling' | 'cancelling'

interface UseImageGenerationOptions {
  api: PluginApi
  loadKeys: () => Promise<ImageApiKey[]>
  pollInterval?: number
  now?: () => Date
}

interface SubmitOptions {
  prompt?: string
  references?: ImageReference[]
  outputCount?: number
}

interface ActiveGenerationTask {
  conversationId: string
  pendingId: string
  slotIndexes: number[]
  prompt: string
  label?: string
  jobId: string
  state: 'submitting' | 'polling' | 'cancelling'
  timer?: ReturnType<typeof setTimeout>
  progressTimer?: ReturnType<typeof setInterval>
}

interface GenerationDescriptor {
  label?: string
  prompt: string
  slotIndexes: number[]
}

const defaultModels = ['gpt-image-2', 'gpt-image-1', 'gemini-2.5-flash-image']
const emptyPresetSelection = (): ImagePresetSelection => ({ styles: [], scenes: [], effects: [], angles: [], separateAngleImages: false, keepAngleConsistency: false })

const presetLabels: Record<string, string> = {
  cinematic: '电影感',
  photorealistic: '写实摄影',
  anime: '动漫',
  illustration: '插画',
  product: '产品摄影',
  studio: '摄影棚',
  outdoor: '户外',
  interior: '室内',
  night: '夜景',
  minimal: '极简背景',
  soft_light: '柔和光线',
  dramatic_light: '戏剧光',
  depth_of_field: '景深',
  volumetric_light: '体积光',
  motion: '动态效果',
  front: '正面',
  back: '背面',
  left: '左侧',
  right: '右侧',
  three_quarter: '45 度',
  top: '俯视',
}

function selectSupported(current: string, descriptor?: EnumCapability): string {
  if (!descriptor) return ''
  return descriptor.values.includes(current) ? current : descriptor.default
}

function supportsImageGeneration(key: ImageApiKey): boolean {
  if (key.status !== 'active' || !key.group?.allow_image_generation) return false
  const config = key.group.models_list_config
  if (!config?.enabled || !config.models?.length) return true
  return config.models.some(model => model.startsWith('gpt-image-') || (model.startsWith('gemini-') && model.includes('image')))
}

function sourceOf(image: GeneratedImagePayload): string {
	return image.url ? authenticatedMediaUrl(image.url) : ''
}

export function useImageGeneration(options: UseImageGenerationOptions) {
  const now = options.now ?? (() => new Date())
  const pollInterval = options.pollInterval ?? 1500
  const keys = ref<ImageApiKey[]>([])
  const selectedKeyId = ref<number | null>(null)
  const model = ref(defaultModels[0])
  const size = ref('1024x1024')
  const outputCount = ref(1)
  const quality = ref('')
  const outputFormat = ref('')
  const outputCompression = ref<number | null>(null)
  const background = ref('')
  const inputFidelity = ref('')
  const aspectRatio = ref('')
  const resolution = ref('')
  const prompt = ref('')
  const presetSelection = ref<ImagePresetSelection>(emptyPresetSelection())
  const appliedPresetText = ref('')
  const promptOptimizerModel = ref('')
  const promptOptimizerModels = ref<string[]>([])
  const optimizingPrompt = ref(false)
  const conversations = ref<Conversation[]>([])
  const activeConversationId = ref('')
  const generationStatus = ref<GenerationStatus>('idle')
  const activeJobId = ref('')
  const errorMessage = ref('')
  const modelCapabilities = ref<Record<string, ImageModelCapability>>({})
  const conversationNextCursor = ref<Record<string, string>>({})
  const loadingConversation = ref(false)
  let conversationRequestSequence = 0
  let promptModelRequestSequence = 0
  let promptOptimizationController: AbortController | null = null
  const activeTasks = new Map<string, ActiveGenerationTask>()
  const canceledSubmittingTasks = new Set<string>()
  let initializedKeySelection = false

  const selectedKey = computed(() => keys.value.find(key => key.id === selectedKeyId.value) ?? null)
  const activeConversation = computed(() => conversations.value.find(item => item.id === activeConversationId.value) ?? null)
  const hasOlderMessages = computed(() => Boolean(conversationNextCursor.value[activeConversationId.value]))
  const availableModels = computed(() => {
    const config = selectedKey.value?.group?.models_list_config
    if (!config?.enabled || !config.models?.length) return defaultModels
    return config.models.filter(value => defaultModels.includes(value) || value.startsWith('gpt-image-') || value.includes('image'))
  })
  const maxReferenceImages = computed(() => modelCapabilities.value[model.value]?.max_reference_images ?? 1)
  const maxOutputImages = computed(() => modelCapabilities.value[model.value]?.max_output_images ?? 1)
  const activeCapability = computed(() => modelCapabilities.value[model.value])
  const availableSizes = computed(() => activeCapability.value?.sizes?.values ?? [])
  const availableAspectRatios = computed(() => activeCapability.value?.aspect_ratios?.values ?? [])
  const availableResolutions = computed(() => activeCapability.value?.resolutions?.values ?? [])
  const availableQualities = computed(() => activeCapability.value?.quality?.values ?? [])
  const availableOutputFormats = computed(() => activeCapability.value?.output_formats?.values ?? [])
  const availableBackgrounds = computed(() => activeCapability.value?.background?.values ?? [])
  const availableInputFidelities = computed(() => activeCapability.value?.input_fidelity?.values ?? [])
  const compressionCapability = computed(() => activeCapability.value?.output_compression)
  const supportsOutputCompression = computed(() => Boolean(compressionCapability.value && ['jpeg', 'webp'].includes(outputFormat.value)))
  const referenceLimitExceeded = computed(() => (activeConversation.value?.referenceImages.length ?? 0) > maxReferenceImages.value)

  watch(maxOutputImages, limit => {
    outputCount.value = Math.min(Math.max(outputCount.value, 1), limit)
  })

  watch(activeCapability, (capability) => {
    size.value = capability ? selectSupported(size.value, capability.sizes) : '1024x1024'
    aspectRatio.value = selectSupported(aspectRatio.value, capability?.aspect_ratios)
    resolution.value = selectSupported(resolution.value, capability?.resolutions)
    quality.value = selectSupported(quality.value, capability?.quality)
    outputFormat.value = selectSupported(outputFormat.value, capability?.output_formats)
    background.value = selectSupported(background.value, capability?.background)
    inputFidelity.value = selectSupported(inputFidelity.value, capability?.input_fidelity)
    outputCompression.value = capability?.output_compression?.default ?? null
  }, { immediate: true })

  watch(supportsOutputCompression, supported => {
    if (supported && outputCompression.value == null) outputCompression.value = compressionCapability.value?.default ?? null
    if (!supported) outputCompression.value = null
  })

  watch(selectedKeyId, async (next, previous) => {
    if (!initializedKeySelection || next === previous) return
    try {
      await options.api.saveImageGenerationPreference(next)
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : String(error)
    }
    await loadPromptOptimizerModels(next)
  })

  watch(promptOptimizerModels, models => {
    promptOptimizerModel.value = models.includes(promptOptimizerModel.value) ? promptOptimizerModel.value : models[0] ?? ''
  }, { immediate: true })

  function timestamp(): string {
    return now().toLocaleString()
  }

  function syncGenerationState(): void {
    const tasks = [...activeTasks.values()]
    activeJobId.value = tasks.find(task => task.jobId)?.jobId ?? ''
    if (tasks.some(task => task.state === 'cancelling')) generationStatus.value = 'cancelling'
    else if (tasks.some(task => task.state === 'polling')) generationStatus.value = 'polling'
    else if (tasks.length) generationStatus.value = 'submitting'
    else generationStatus.value = 'idle'
  }

  function finishTask(pendingId: string): void {
    const task = activeTasks.get(pendingId)
    if (task?.timer) clearTimeout(task.timer)
    if (task?.progressTimer) clearInterval(task.progressTimer)
    activeTasks.delete(pendingId)
    syncGenerationState()
  }

  async function cancelLateJob(taskId: string, jobId: string): Promise<boolean> {
    if (!canceledSubmittingTasks.delete(taskId)) return false
    try {
      await options.api.cancel(jobId)
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : String(error)
    }
    return true
  }

  function createConversation(): Conversation {
    const id = `conversation-live-${now().getTime()}`
    const conversation: Conversation = {
      id,
      title: '新会话',
      preview: '',
      lastUsedAt: timestamp(),
      messages: [],
      referenceImages: [],
      historyIds: [],
    }
    conversations.value.unshift(conversation)
    activeConversationId.value = id
    prompt.value = ''
    appliedPresetText.value = ''
    return conversation
  }

  function updateConversation(id: string, update: (conversation: Conversation) => Conversation): void {
    conversations.value = conversations.value.map(conversation => conversation.id === id ? update(conversation) : conversation)
  }

  function promoteConversation(id: string): void {
    const conversation = conversations.value.find(item => item.id === id)
    if (!conversation || conversations.value[0]?.id === id) return
    conversations.value = [conversation, ...conversations.value.filter(item => item.id !== id)]
  }

  async function loadPromptOptimizerModels(apiKeyID: number | null): Promise<void> {
    const sequence = ++promptModelRequestSequence
    if (!apiKeyID) {
      promptOptimizerModels.value = []
      promptOptimizerModel.value = ''
      return
    }
    try {
      const response = await options.api.listPromptModels(apiKeyID)
      if (sequence !== promptModelRequestSequence) return
      promptOptimizerModels.value = response.models
      promptOptimizerModel.value = response.models.includes(promptOptimizerModel.value) ? promptOptimizerModel.value : response.models[0] ?? ''
    } catch (error) {
      if (sequence !== promptModelRequestSequence) return
      promptOptimizerModels.value = []
      promptOptimizerModel.value = ''
      errorMessage.value = error instanceof Error ? error.message : String(error)
    }
  }

  async function initialize(): Promise<void> {
    if (!activeConversation.value) createConversation()
    const configPromise = options.api.getConfig().then((config) => {
      modelCapabilities.value = config.image_model_capabilities ?? {}
    })
    const keysPromise = options.loadKeys()
    const preferencePromise = options.api.getImageGenerationPreference()
    const summaries = await options.api.listConversations()
    if (summaries.items.length > 0) {
      conversations.value = summaries.items.map(item => ({
        id: item.id, conversationId: item.id, title: item.title, preview: item.preview,
        lastUsedAt: new Date(item.updated_at).toLocaleString(), messages: [], referenceImages: [], historyIds: [],
      }))
      await selectConversation(summaries.items[0].id)
    }
    const [loadedKeys, preference] = await Promise.all([keysPromise, preferencePromise])
    keys.value = loadedKeys.filter(supportsImageGeneration)
    const savedKeyID = preference.last_api_key_id
    const nextKeyID = keys.value.some(key => key.id === savedKeyID) ? savedKeyID : keys.value[0]?.id ?? null
    selectedKeyId.value = nextKeyID
    if (nextKeyID !== savedKeyID) {
      try {
        await options.api.saveImageGenerationPreference(nextKeyID)
      } catch (error) {
        errorMessage.value = error instanceof Error ? error.message : String(error)
      }
    }
    await loadPromptOptimizerModels(nextKeyID)
    await nextTick()
    initializedKeySelection = true
    await configPromise
  }

  async function loadConversation(id: string, before = ''): Promise<void> {
    const sequence = ++conversationRequestSequence
    loadingConversation.value = true
    try {
      const page = await options.api.listConversationMessages(id, before)
      if (sequence !== conversationRequestSequence) return
      const messages = projectConversationMessages(page.items)
      updateConversation(id, conversation => ({
        ...conversation,
        messages: before ? [...messages, ...conversation.messages] : messages,
        referenceImages: before
          ? conversation.referenceImages
          : [...messages].reverse().find(message => message.role === 'user' && message.referenceImages?.length)?.referenceImages ?? conversation.referenceImages,
        historyIds: before ? [...page.items.map(item => item.id), ...conversation.historyIds] : page.items.map(item => item.id),
      }))
      conversationNextCursor.value = { ...conversationNextCursor.value, [id]: page.next_cursor || '' }
      for (const pending of page.items.filter(record => record.status === 'pending')) {
        const assistant = messages.find(message => message.role === 'assistant' && message.historyTasks?.some(task => task.id === pending.id))
        const historyTasks = assistant?.historyTasks ?? [{ id: pending.id, outputCount: Number(pending.request.output_count) || 1 }]
        const taskIndex = historyTasks.findIndex(task => task.id === pending.id)
        const slotOffset = historyTasks.slice(0, Math.max(taskIndex, 0)).reduce((total, task) => total + task.outputCount, 0)
        const outputCount = historyTasks[Math.max(taskIndex, 0)]?.outputCount ?? 1
        const taskId = `${pending.id}-assistant`
        activeTasks.set(taskId, {
          conversationId: id,
          pendingId: assistant?.id ?? taskId,
          slotIndexes: Array.from({ length: outputCount }, (_, index) => slotOffset + index),
          prompt: pending.prompt,
          jobId: pending.id,
          state: 'polling',
        })
        syncGenerationState()
        schedulePoll(taskId)
      }
    } finally {
      if (sequence === conversationRequestSequence) loadingConversation.value = false
    }
  }

  async function selectConversation(id: string): Promise<void> {
    activeConversationId.value = id
    const conversation = conversations.value.find(item => item.id === id)
    if (conversation && conversation.messages.length === 0) await loadConversation(id)
  }

  async function loadOlderMessages(): Promise<void> {
    const id = activeConversationId.value
    const cursor = conversationNextCursor.value[id]
    if (id && cursor && !loadingConversation.value) await loadConversation(id, cursor)
  }

  function referencesToRequest(references: ImageReference[]) {
    return references.filter(reference => reference.dataUrl).map(reference => ({
      name: reference.fileName,
      mime_type: reference.mimeType,
      data_url: reference.storageKey ? undefined : reference.uploadDataUrl || reference.originalDataUrl || reference.dataUrl,
      storage_key: reference.storageKey,
      preview_storage_key: reference.previewStorageKey,
      preview_url: reference.dataUrl,
    }))
  }

  async function uploadReference(files: File[]): Promise<void> {
    errorMessage.value = ''
    const remainingCapacity = Math.max(0, maxReferenceImages.value - (activeConversation.value?.referenceImages.length ?? 0))
    const acceptedFiles = files.slice(0, remainingCapacity)
    const errors: string[] = []
    if (acceptedFiles.length < files.length) {
      errors.push(`当前模型最多支持 ${maxReferenceImages.value} 张参考图`)
    }
    for (const file of acceptedFiles) {
      try {
        const uploaded = await options.api.uploadReference(file)
        setReference({
          id: uploaded.storage_key,
          dataUrl: authenticatedMediaUrl(uploaded.preview_url),
          originalDataUrl: authenticatedMediaUrl(uploaded.original_url),
          storageKey: uploaded.storage_key,
          previewStorageKey: uploaded.preview_storage_key,
          fileName: uploaded.name || file.name,
          mimeType: uploaded.mime_type || file.type || 'image/png',
        })
      } catch (error) {
        errors.push(`${file.name}: ${error instanceof Error ? error.message : String(error)}`)
      }
    }
    errorMessage.value = errors.join('\n')
  }

  async function optimizePrompt(selectedModel = promptOptimizerModel.value): Promise<void> {
    const key = selectedKey.value
    const userPrompt = prompt.value.trim()
    const modelName = selectedModel.trim()
    if (!key || !userPrompt || !modelName || optimizingPrompt.value) return
    const controller = new AbortController()
    promptOptimizationController = controller
    optimizingPrompt.value = true
    errorMessage.value = ''
    try {
      const response = await options.api.optimizePrompt({
        prompt: userPrompt,
        api_key_id: key.id,
        model: modelName,
      }, controller.signal)
      const optimized = response.prompt.trim()
      if (optimized) prompt.value = optimized
    } catch (error) {
      if (controller.signal.aborted) return
      errorMessage.value = error instanceof Error ? error.message : String(error)
    } finally {
      if (promptOptimizationController === controller) {
        promptOptimizationController = null
        optimizingPrompt.value = false
      }
    }
  }

  function cancelPromptOptimization(): void {
    promptOptimizationController?.abort()
    promptOptimizationController = null
    optimizingPrompt.value = false
  }

  function requestPrompt(userPrompt: string, references: ImageReference[]): string {
    const lines = ['Follow the user request with highest priority.']
    if (references.length > 0) {
      lines.push('Use the uploaded reference image as the primary subject and preserve its identity unless the user asks to change it.')
    }
    lines.push(`User request: ${userPrompt}`)
    return lines.join('\n')
  }

  function editablePrompt(value: string): string {
    const userRequest = value.match(/(?:^|\n)User request:\s*([\s\S]*)$/)
    const prompt = (userRequest?.[1] ?? value)
      .replace(/。?只生成这个角度的一张独立图片；[^\n]*/g, '')
      .trim()
    return prompt
  }

  function presetDescription(selected: ImagePresetSelection): string {
    const details = [
      selected.styles.length ? `风格：${selected.styles.map(item => presetLabels[item] ?? item).join('、')}` : '',
      selected.scenes.length ? `场景：${selected.scenes.map(item => presetLabels[item] ?? item).join('、')}` : '',
      selected.effects.length ? `特效：${selected.effects.map(item => presetLabels[item] ?? item).join('、')}` : '',
      selected.angles.length ? `角度：${selected.angles.map(item => presetLabels[item] ?? item).join('、')}` : '',
    ].filter(Boolean)
    return details.join('\n')
  }

  function presetPrompt(userPrompt: string, includeAngles = true): string {
    const fullDescription = presetDescription(presetSelection.value)
    const basePrompt = fullDescription && userPrompt.includes(fullDescription)
      ? userPrompt.replace(fullDescription, '').trimEnd()
      : userPrompt
    const selection = includeAngles
      ? presetSelection.value
      : { ...presetSelection.value, angles: [] }
    const description = presetDescription(selection)
    if (!description || basePrompt.includes(description)) return basePrompt
    return `${basePrompt}\n${description}`
  }

  function applyPresetSelection(selection: ImagePresetSelection): void {
    const previous = appliedPresetText.value
    let basePrompt = prompt.value
    if (previous) {
      basePrompt = basePrompt.replace(`\n${previous}`, '').replace(previous, '').trimEnd()
    }
    presetSelection.value = selection
    const next = presetDescription(selection)
    appliedPresetText.value = next
    prompt.value = next ? `${basePrompt.trimEnd()}\n${next}`.trimStart() : basePrompt
  }

  function angleVariants(basePrompt: string) {
    const angles = presetSelection.value.angles.slice(0, maxOutputImages.value)
    if (angles.length < 2) return undefined
    return angles.map(angle => {
      const label = presetLabels[angle] ?? angle
      const consistency = presetSelection.value.keepAngleConsistency
        ? '；保持主体身份、服装、比例、材质、场景和光线与其他角度一致'
        : ''
      return {
        label,
        prompt: `${basePrompt}\n角度：${label}。只生成这个角度的一张独立图片${consistency}。`,
      }
    })
  }

  function collagePrompt(basePrompt: string): string {
    const labels = presetSelection.value.angles.slice(0, maxOutputImages.value).map(angle => presetLabels[angle] ?? angle)
    if (labels.length < 2) return basePrompt
    return `${basePrompt}\n角度：${labels.join('、')}。生成一张多视角拼图，按上述顺序排列；每个分区只展示对应角度。`
  }

  function imagesFromResult(response: GenerateResponse, fallbackPrompt = '', fallbackLabel?: string): GeneratedImage[] {
    const resultPrompt = response.result?.revised_prompt || fallbackPrompt
    return (response.result?.images ?? []).map((image, index) => ({
      id: `${response.job_id}-image-${index}`,
      historyId: response.job_id,
      src: image.preview_url ? authenticatedMediaUrl(image.preview_url) : sourceOf(image),
      originalSrc: sourceOf(image),
      revisedPrompt: image.revised_prompt || resultPrompt,
      variantLabel: image.variant_label || fallbackLabel,
      createdAt: response.result?.created
        ? new Date(response.result.created * 1000).toLocaleString()
        : timestamp(),
    })).filter(image => image.src)
  }

  function replacePending(conversationId: string, pendingId: string, message: ChatMessage): void {
    updateConversation(conversationId, conversation => ({
      ...conversation,
      preview: message.content,
      lastUsedAt: message.createdAt,
      messages: conversation.messages.map(item => item.id === pendingId ? message : item),
    }))
  }

  function updateGenerationSlots(conversationId: string, pendingId: string, update: (slots: GenerationSlot[]) => GenerationSlot[]): void {
    updateConversation(conversationId, conversation => ({
      ...conversation,
      messages: conversation.messages.map(message => message.id === pendingId
        ? (() => {
            const generationSlots = update(message.generationSlots ?? [])
            return { ...message, generationSlots, images: generationSlots.flatMap(slot => slot.image ? [slot.image] : []) }
          })()
        : message),
    }))
  }

  function addMessageHistoryID(conversationId: string, messageId: string, historyID: string, outputCount: number): void {
    updateConversation(conversationId, conversation => ({
      ...conversation,
      historyIds: conversation.historyIds.includes(historyID) ? conversation.historyIds : [...conversation.historyIds, historyID],
      messages: conversation.messages.map(message => message.id === messageId
        ? {
            ...message,
            historyIds: message.historyIds?.includes(historyID) ? message.historyIds : [...(message.historyIds ?? []), historyID],
            historyTasks: message.historyTasks?.some(task => task.id === historyID)
              ? message.historyTasks
              : [...(message.historyTasks ?? []), { id: historyID, outputCount }],
          }
        : message),
    }))
  }

  function finalizeGenerationMessage(conversationId: string, pendingId: string): void {
    const hasPending = [...activeTasks.values()].some(task => task.conversationId === conversationId && task.pendingId === pendingId)
    if (hasPending) return
    updateConversation(conversationId, conversation => ({
      ...conversation,
      messages: conversation.messages.map(message => message.id === pendingId
        ? (() => {
            const slots = message.generationSlots ?? []
            const hasImages = slots.some(slot => slot.image)
            const failedSlots = slots.filter(slot => slot.status === 'failed' || slot.status === 'canceled')
            return {
              ...message,
              status: hasImages ? undefined : failedSlots.every(slot => slot.status === 'canceled') ? 'canceled' : 'failed',
              content: hasImages
                ? `生成结果${failedSlots.length ? `\n生成失败：${failedSlots.map(slot => slot.label || '图片').join('、')}` : ''}`
                : failedSlots[0]?.error || '图片生成未返回可显示的图片',
            }
          })()
        : message),
    }))
  }

  function activeParameterSummary(hasReferences: boolean): string {
    const details = [
      aspectRatio.value ? `比例: ${aspectRatio.value}` : '',
      resolution.value ? `分辨率: ${resolution.value}` : '',
      quality.value ? `画质: ${imageParameterLabel('quality', quality.value)}` : '',
      outputFormat.value ? `格式: ${outputFormat.value}` : '',
      supportsOutputCompression.value && outputCompression.value != null ? `压缩: ${outputCompression.value}%` : '',
      background.value ? `背景: ${imageParameterLabel('background', background.value)}` : '',
      hasReferences && inputFidelity.value ? `保真度: ${imageParameterLabel('input_fidelity', inputFidelity.value)}` : '',
    ]
    return details.filter(Boolean).join(' | ')
  }

  async function submit(submitOptions: SubmitOptions = {}): Promise<void> {
    if (generationStatus.value !== 'idle') return
    const conversation = activeConversation.value
    const key = selectedKey.value
    const userPrompt = (submitOptions.prompt ?? prompt.value).trim()
    const references = submitOptions.references ?? conversation?.referenceImages ?? []
    const hasMultipleAngles = presetSelection.value.angles.length > 1
    const useSeparateAngleImages = hasMultipleAngles && presetSelection.value.separateAngleImages
    const basePrompt = presetPrompt(userPrompt, !hasMultipleAngles)
    const composedPrompt = hasMultipleAngles ? collagePrompt(basePrompt) : basePrompt
    const variants = useSeparateAngleImages ? angleVariants(basePrompt) : undefined
    const requestedOutputCount = useSeparateAngleImages
      ? variants!.length
      : hasMultipleAngles
        ? 1
        : Math.min(Math.max(submitOptions.outputCount ?? outputCount.value, 1), maxOutputImages.value)
    if (!conversation || !key || !userPrompt || references.length > maxReferenceImages.value) return

    const createdAt = timestamp()
    const userId = `user-${now().getTime()}`
    const generationGroupID = `generation-${now().getTime()}`
    const independentGPTTasks = !useSeparateAngleImages && !hasMultipleAngles && model.value.startsWith('gpt-image-') && requestedOutputCount > 1
    const descriptors: GenerationDescriptor[] = independentGPTTasks
      ? (variants ?? Array.from({ length: requestedOutputCount }, (_, index) => ({
          label: `图片 ${index + 1}`,
          prompt: composedPrompt,
        }))).map((descriptor, index) => ({ ...descriptor, slotIndexes: [index] }))
      : [{ prompt: composedPrompt, slotIndexes: Array.from({ length: requestedOutputCount }, (_, index) => index) }]
    const pendingId = `assistant-pending-${now().getTime()}`
    const generationSlots: GenerationSlot[] = Array.from({ length: requestedOutputCount }, (_, index) => ({
      id: `${pendingId}-slot-${index}`,
      label: variants?.[index]?.label ?? descriptors[index]?.label,
      status: 'pending',
      progress: 1,
    }))
    const pendingMessage: ChatMessage = {
      id: pendingId,
      role: 'assistant',
      content: '正在生成图片，请稍候...',
      createdAt,
      status: 'pending',
      generationSlots,
    }
    const userMessage: ChatMessage = {
      id: userId,
      role: 'user',
      content: userPrompt,
      createdAt,
      referenceImages: references.map(reference => ({ ...reference })),
      requestSettings: [{
        modelLabel: model.value,
        sizeLabel: size.value || aspectRatio.value,
        countLabel: `数量: ${requestedOutputCount}`,
        detailsLabel: activeParameterSummary(references.length > 0),
      }],
    }
    updateConversation(conversation.id, current => ({
      ...current,
      title: current.messages.length === 0 ? userPrompt.slice(0, 24) : current.title,
      preview: pendingMessage.content,
      lastUsedAt: createdAt,
      messages: [...current.messages, userMessage, pendingMessage],
    }))
    promoteConversation(conversation.id)
    prompt.value = ''
    errorMessage.value = ''

    const commonRequest = {
        api_key_id: key.id,
        model: model.value,
        size: size.value,
        response_format: 'b64_json',
        ...(quality.value ? { quality: quality.value } : {}),
        ...(outputFormat.value ? { output_format: outputFormat.value } : {}),
        ...(supportsOutputCompression.value && outputCompression.value != null ? { output_compression: outputCompression.value } : {}),
        ...(background.value ? { background: background.value } : {}),
        ...(references.length > 0 && inputFidelity.value ? { input_fidelity: inputFidelity.value } : {}),
        ...(aspectRatio.value ? { aspect_ratio: aspectRatio.value } : {}),
        ...(resolution.value ? { resolution: resolution.value } : {}),
        reference_images: referencesToRequest(references),
        ...(variants ? {
          variants: variants.map(variant => ({ ...variant, prompt: requestPrompt(variant.prompt, references) })),
        } : {}),
        inputs: {
          display_prompt: userPrompt,
          conversation_id: conversation.conversationId || conversation.id,
          generation_group_id: generationGroupID,
        },
    }

    const results = await Promise.all(descriptors.map(async (descriptor, index) => {
      const taskId = `${pendingId}-task-${index}`
      const task: ActiveGenerationTask = {
        conversationId: conversation.id,
        pendingId,
        slotIndexes: descriptor.slotIndexes,
        prompt: userPrompt,
        label: descriptor.label,
        jobId: '',
        state: 'submitting',
      }
      activeTasks.set(taskId, task)
      task.progressTimer = setInterval(() => {
        if (!activeTasks.has(taskId) || task.state === 'cancelling') return
        updateGenerationSlots(conversation.id, pendingId, slots => slots.map((slot, slotIndex) => descriptor.slotIndexes.includes(slotIndex) && slot.status === 'pending'
          ? { ...slot, progress: Math.min(99, slot.progress + 2) }
          : slot))
      }, 700)
      syncGenerationState()
      try {
        const response = await options.api.generate({
          ...commonRequest,
          prompt: requestPrompt(descriptor.prompt, references),
          output_count: independentGPTTasks ? 1 : requestedOutputCount,
        })
        if (await cancelLateJob(taskId, response.job_id)) return null
        if (!activeTasks.has(taskId)) return false
        task.jobId = response.job_id
        addMessageHistoryID(conversation.id, pendingId, response.job_id, descriptor.slotIndexes.length)
        if (response.status === 'pending') {
          task.state = 'polling'
          syncGenerationState()
          schedulePoll(taskId)
          return true
        }
        const images = imagesFromResult(response, userPrompt, descriptor.label)
        const failedVariants = new Set(response.result?.failed_variants ?? [])
        updateGenerationSlots(conversation.id, pendingId, slots => slots.map((slot, slotIndex) => {
          const imageIndex = descriptor.slotIndexes.indexOf(slotIndex)
          if (imageIndex < 0) return slot
          const image = images[imageIndex]
          if (slot.label && failedVariants.has(slot.label)) return { ...slot, status: 'failed', progress: 100, error: '图片生成失败' }
          return image
            ? { ...slot, status: 'succeeded', progress: 100, image: { ...image, variantLabel: image.variantLabel || slot.label } }
            : { ...slot, status: 'failed', progress: 100, error: response.error_message || '图片生成未返回可显示的图片' }
        }))
        finishTask(taskId)
        finalizeGenerationMessage(conversation.id, pendingId)
        return images.length > 0
      } catch (error) {
        canceledSubmittingTasks.delete(taskId)
        if (!activeTasks.has(taskId)) return false
        const message = error instanceof Error ? error.message : String(error)
        updateGenerationSlots(conversation.id, pendingId, slots => slots.map((slot, slotIndex) => descriptor.slotIndexes.includes(slotIndex)
          ? { ...slot, status: 'failed', error: message }
          : slot))
        finishTask(taskId)
        finalizeGenerationMessage(conversation.id, pendingId)
        return false
      }
    }))
    if (results.every(result => result === false)) errorMessage.value = '所有图片生成任务均失败'
  }

  function terminalMessage(record: GenerateResponse, fallbackPrompt = '', fallbackLabel?: string): ChatMessage {
    if (record.status === 'failed' || record.status === 'canceled') {
      return {
        id: `${record.job_id}-assistant`,
        role: 'assistant',
        content: record.error_message || (record.status === 'canceled' ? '生成已取消' : '图片生成失败'),
        createdAt: timestamp(),
        status: record.status,
      }
    }
    const images = imagesFromResult(record, fallbackPrompt, fallbackLabel)
    const failedVariants = record.result?.failed_variants ?? []
    return {
      id: `${record.job_id}-assistant`,
      role: 'assistant',
      content: images.length
        ? `生成结果${failedVariants.length ? `\n生成失败的角度：${failedVariants.join('、')}` : ''}`
        : '图片生成未返回可显示的图片',
      createdAt: timestamp(),
      status: images.length ? undefined : 'failed',
      images: images.length ? images : undefined,
    }
  }

  function schedulePoll(pendingId: string): void {
    const task = activeTasks.get(pendingId)
    if (!task?.jobId) return
    if (task.timer) clearTimeout(task.timer)
    task.timer = setTimeout(async () => {
      const current = activeTasks.get(pendingId)
      if (!current?.jobId) return
      try {
        const record = await options.api.getStatus(current.jobId)
        if (record.status === 'pending') {
          const images = imagesFromResult(record, current.prompt, current.label)
          if (images.length) {
            updateGenerationSlots(current.conversationId, current.pendingId, slots => slots.map((slot, slotIndex) => {
              if (!current.slotIndexes.includes(slotIndex) || slot.image) return slot
              const imageIndex = current.slotIndexes.indexOf(slotIndex)
              const image = slot.label
                ? images.find(candidate => candidate.variantLabel === slot.label)
                : images[imageIndex]
              return image ? { ...slot, status: 'succeeded', progress: 100, image } : slot
            }))
          }
          schedulePoll(pendingId)
          return
        }
        const images = imagesFromResult(record, current.prompt, current.label)
        updateGenerationSlots(current.conversationId, current.pendingId, slots => slots.map((slot, slotIndex) => current.slotIndexes.includes(slotIndex)
          ? (images[current.slotIndexes.indexOf(slotIndex)]
              ? { ...slot, status: 'succeeded', progress: 100, image: images[current.slotIndexes.indexOf(slotIndex)] }
              : { ...slot, status: 'failed', progress: 100, error: record.error_message || '图片生成未返回可显示的图片' })
          : slot))
        finishTask(pendingId)
        finalizeGenerationMessage(current.conversationId, current.pendingId)
      } catch (error) {
        errorMessage.value = error instanceof Error ? error.message : String(error)
        schedulePoll(pendingId)
      }
    }, pollInterval)
  }

  async function cancelGeneration(): Promise<void> {
    const tasks = [...activeTasks.values()]
    if (!tasks.length) return
    for (const task of tasks) {
      task.state = 'cancelling'
      if (task.timer) clearTimeout(task.timer)
    }
    syncGenerationState()
    await Promise.all(tasks.map(async task => {
      if (task.jobId) {
        try {
          const response = await options.api.cancel(task.jobId)
          updateGenerationSlots(task.conversationId, task.pendingId, slots => slots.map((slot, slotIndex) => task.slotIndexes.includes(slotIndex)
            ? { ...slot, status: 'canceled', progress: 100, error: response.error_message || '生成已取消' }
            : slot))
        } catch (error) {
          const message = error instanceof Error ? error.message : '生成已取消'
          updateGenerationSlots(task.conversationId, task.pendingId, slots => slots.map((slot, slotIndex) => task.slotIndexes.includes(slotIndex)
            ? { ...slot, status: 'canceled', progress: 100, error: message }
            : slot))
        }
      } else {
        const taskID = [...activeTasks.entries()].find(([, current]) => current === task)?.[0]
        if (taskID) canceledSubmittingTasks.add(taskID)
        updateGenerationSlots(task.conversationId, task.pendingId, slots => slots.map((slot, slotIndex) => task.slotIndexes.includes(slotIndex)
          ? { ...slot, status: 'canceled', progress: 100, error: '生成已取消' }
          : slot))
      }
      const taskID = [...activeTasks.entries()].find(([, current]) => current === task)?.[0]
      if (taskID) finishTask(taskID)
      finalizeGenerationMessage(task.conversationId, task.pendingId)
    }))
  }

  async function repeatFromImage(image: GeneratedImage, repeatPrompt: string): Promise<void> {
    const reference: ImageReference = {
      id: `${image.id}-repeat-reference`,
      dataUrl: image.src,
      originalDataUrl: image.originalSrc,
      fileName: `${image.id}.png`,
      mimeType: 'image/png',
    }
    const conversation = activeConversation.value
    const assistantIndex = conversation?.messages.findIndex(message => message.images?.some(item => item.id === image.id)) ?? -1
    const sourceMessage = assistantIndex > 0 ? conversation?.messages.slice(0, assistantIndex).reverse().find(message => message.role === 'user') : undefined
    if (image.historyId && sourceMessage) {
      const sourceTask = conversation?.messages[assistantIndex].historyTasks?.find(task => task.id === image.historyId)
      await retryStoredHistory(sourceMessage, [sourceTask ?? { id: image.historyId, outputCount: 1 }])
      return
    }
    await submit({
      prompt: editablePrompt(repeatPrompt) || sourceMessage?.content || editablePrompt(image.revisedPrompt),
      references: [reference],
      outputCount: messageOutputCount(sourceMessage),
    })
  }

  function refineFromImage(image: GeneratedImage): void {
    prompt.value = editablePrompt(image.revisedPrompt) || prompt.value
  }

  async function retryMessage(messageId: string): Promise<void> {
    if (generationStatus.value !== 'idle') return
    const conversation = activeConversation.value
    if (!conversation) return
    const failedIndex = conversation.messages.findIndex(message => message.id === messageId)
    if (failedIndex < 0) return
    const failedMessage = conversation.messages[failedIndex]
    const userMessage = conversation.messages.slice(0, failedIndex).reverse().find(message => message.role === 'user')
    if (!userMessage) return
    const historyTasks = failedMessage.historyTasks?.length
      ? failedMessage.historyTasks
      : userMessage.historyTasks?.length
        ? userMessage.historyTasks
        : (failedMessage.historyIds?.length ? failedMessage.historyIds : userMessage.historyIds ?? []).map(id => ({ id, outputCount: 1 }))
    if (historyTasks.length) {
      updateConversation(conversation.id, current => ({
        ...current,
        messages: current.messages.filter(message => message.id !== messageId && message.id !== userMessage.id),
      }))
      await retryStoredHistory(userMessage, historyTasks)
      return
    }
    updateConversation(conversation.id, current => ({
      ...current,
      messages: current.messages.filter(message => message.id !== messageId && message.id !== userMessage.id),
    }))
    await submit({ prompt: userMessage.content, references: userMessage.referenceImages ?? [], outputCount: messageOutputCount(userMessage) })
  }

  async function retryStoredHistory(userMessage: ChatMessage, historyTasks: HistoryTask[]): Promise<void> {
    if (generationStatus.value !== 'idle' || historyTasks.length === 0) return
    const conversation = activeConversation.value
    if (!conversation) return
    const createdAt = timestamp()
    const pendingId = `assistant-retry-${now().getTime()}`
    const generationGroupID = `generation-${now().getTime()}`
    const requestedOutputCount = historyTasks.reduce((total, task) => total + task.outputCount, 0)
    const generationSlots: GenerationSlot[] = Array.from({ length: requestedOutputCount }, (_, index) => ({
      id: `${pendingId}-slot-${index}`,
      status: 'pending',
      progress: 1,
    }))
    const retryUserMessage: ChatMessage = {
      ...userMessage,
      id: `user-retry-${now().getTime()}`,
      createdAt,
      historyIds: undefined,
      historyTasks: undefined,
      referenceImages: userMessage.referenceImages?.map(reference => ({ ...reference })),
    }
    const pendingMessage: ChatMessage = {
      id: pendingId,
      role: 'assistant',
      content: '正在生成图片，请稍候...',
      createdAt,
      status: 'pending',
      generationSlots,
      historyIds: [],
    }
    updateConversation(conversation.id, current => ({
      ...current,
      preview: pendingMessage.content,
      lastUsedAt: createdAt,
      messages: [...current.messages, retryUserMessage, pendingMessage],
    }))
    promoteConversation(conversation.id)
    errorMessage.value = ''

    let slotOffset = 0
    const retries = historyTasks.map(task => {
      const slotIndexes = Array.from({ length: task.outputCount }, (_, index) => slotOffset + index)
      slotOffset += task.outputCount
      return { ...task, slotIndexes }
    })
    await Promise.all(retries.map(async ({ id: historyID, outputCount, slotIndexes }, index) => {
      const taskID = `${pendingId}-task-${index}`
      const task: ActiveGenerationTask = {
        conversationId: conversation.id,
        pendingId,
        slotIndexes,
        prompt: userMessage.content,
        jobId: '',
        state: 'submitting',
      }
      activeTasks.set(taskID, task)
      task.progressTimer = setInterval(() => {
        if (!activeTasks.has(taskID) || task.state === 'cancelling') return
        updateGenerationSlots(conversation.id, pendingId, slots => slots.map((slot, slotIndex) => slotIndexes.includes(slotIndex) && slot.status === 'pending'
          ? { ...slot, progress: Math.min(99, slot.progress + 2) }
          : slot))
      }, 700)
      syncGenerationState()
      try {
        const response = await options.api.retryHistory(historyID, { generation_group_id: generationGroupID })
        if (await cancelLateJob(taskID, response.job_id)) return
        if (!activeTasks.has(taskID)) return
        task.jobId = response.job_id
        addMessageHistoryID(conversation.id, pendingId, response.job_id, outputCount)
        if (response.status === 'pending') {
          task.state = 'polling'
          syncGenerationState()
          schedulePoll(taskID)
          return
        }
        const images = imagesFromResult(response, userMessage.content)
        updateGenerationSlots(conversation.id, pendingId, slots => slots.map((slot, slotIndex) => {
          const imageIndex = slotIndexes.indexOf(slotIndex)
          if (imageIndex < 0) return slot
          return images[imageIndex]
            ? { ...slot, status: 'succeeded', progress: 100, image: images[imageIndex] }
            : { ...slot, status: 'failed', progress: 100, error: response.error_message || '图片生成未返回可显示的图片' }
        }))
        finishTask(taskID)
        finalizeGenerationMessage(conversation.id, pendingId)
      } catch (error) {
        canceledSubmittingTasks.delete(taskID)
        const message = error instanceof Error ? error.message : String(error)
        updateGenerationSlots(conversation.id, pendingId, slots => slots.map((slot, slotIndex) => slotIndexes.includes(slotIndex)
          ? { ...slot, status: 'failed', progress: 100, error: message }
          : slot))
        finishTask(taskID)
        finalizeGenerationMessage(conversation.id, pendingId)
      }
    }))
  }

  function messageOutputCount(message?: ChatMessage): number {
    const label = message?.requestSettings?.[0]?.countLabel ?? ''
    const parsed = Number(label.match(/\d+/)?.[0])
    return Number.isInteger(parsed) && parsed > 0 ? parsed : 1
  }

  function setReference(reference?: ImageReference): void {
    const conversation = activeConversation.value
    if (!conversation) return
    updateConversation(conversation.id, current => {
      if (!reference) return { ...current, referenceImages: [] }
      const identity = reference.storageKey || reference.id
      const exists = current.referenceImages.some(item => (item.storageKey || item.id) === identity)
      return exists ? current : { ...current, referenceImages: [...current.referenceImages, reference] }
    })
  }

  function removeReference(id: string): void {
    const conversation = activeConversation.value
    if (!conversation) return
    updateConversation(conversation.id, current => ({
      ...current,
      referenceImages: current.referenceImages.filter(reference => reference.id !== id),
    }))
  }

  function clearReferences(): void {
    setReference()
  }

  async function deleteConversation(conversation: Conversation): Promise<void> {
    if (conversation.conversationId) await options.api.deleteConversation(conversation.conversationId)
    conversations.value = conversations.value.filter(item => item.id !== conversation.id)
    if (activeConversationId.value === conversation.id) {
      activeConversationId.value = conversations.value[0]?.id ?? ''
      if (!activeConversationId.value) createConversation()
    }
  }

  function dispose(): void {
    for (const task of activeTasks.values()) {
      if (task.timer) clearTimeout(task.timer)
    }
    activeTasks.clear()
    canceledSubmittingTasks.clear()
    syncGenerationState()
    cancelPromptOptimization()
  }

  return {
    keys,
    selectedKeyId,
    selectedKey,
    availableModels,
    maxReferenceImages,
    referenceLimitExceeded,
    model,
    size,
    quality,
    outputFormat,
    outputCompression,
    background,
    inputFidelity,
    aspectRatio,
    resolution,
    availableSizes,
    availableAspectRatios,
    availableResolutions,
    availableQualities,
    availableOutputFormats,
    availableBackgrounds,
    availableInputFidelities,
    compressionCapability,
    supportsOutputCompression,
    outputCount,
    prompt,
    presetSelection,
    applyPresetSelection,
    promptOptimizerModel,
    promptOptimizerModels,
    optimizingPrompt,
    conversations,
    activeConversationId,
    activeConversation,
    hasOlderMessages,
    generationStatus,
    activeJobId,
    errorMessage,
    maxOutputImages,
    loadingConversation,
    initialize,
    createConversation,
    selectConversation,
    loadOlderMessages,
    optimizePrompt,
    cancelPromptOptimization,
    submit,
    cancelGeneration,
    repeatFromImage,
    refineFromImage,
    retryMessage,
    setReference,
    removeReference,
    clearReferences,
    uploadReference,
    deleteConversation,
    dispose,
  }
}
