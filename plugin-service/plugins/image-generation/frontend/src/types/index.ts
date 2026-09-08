export type HistoryStatus = 'pending' | 'succeeded' | 'failed' | 'canceled'
export type MessageStatus = 'pending' | 'failed' | 'canceled'

export interface ModelsListConfig {
  enabled?: boolean
  models?: string[]
}

export interface ImageKeyGroup {
  allow_image_generation?: boolean
  models_list_config?: ModelsListConfig
}

export interface ImageApiKey {
  id: number
  key: string
  name: string
  status: string
  group?: ImageKeyGroup
}

export interface EnumCapability {
  values: string[]
  default: string
}

export interface IntegerCapability {
  min: number
  max: number
  default: number
}

export interface ImageModelCapability {
  max_reference_images: number
  max_output_images: number
  sizes?: EnumCapability
  aspect_ratios?: EnumCapability
  resolutions?: EnumCapability
  quality?: EnumCapability
  output_formats?: EnumCapability
  output_compression?: IntegerCapability
  background?: EnumCapability
  input_fidelity?: EnumCapability
}

export interface PluginConfig {
  image_model_capabilities: Record<string, ImageModelCapability>
}

export interface ReferenceImageRequest {
  name?: string
  mime_type?: string
  data_url?: string
  remote_url?: string
  storage_key?: string
  preview_url?: string
  preview_storage_key?: string
}

export interface UploadedReference extends ReferenceImageRequest {
  storage_key: string
  preview_storage_key: string
  original_url: string
  preview_url: string
}

export interface GenerateRequest {
  prompt: string
  api_key_id: number
  model: string
  size: string
  response_format: string
  output_count?: number
  quality?: string
  output_format?: string
  output_compression?: number
  background?: string
  input_fidelity?: string
  aspect_ratio?: string
  resolution?: string
  reference_images?: ReferenceImageRequest[]
  variants?: GenerateVariant[]
  inputs?: Record<string, unknown>
}

export interface GenerateVariant {
  label: string
  prompt: string
}

export interface ImagePresetSelection {
  styles: string[]
  scenes: string[]
  effects: string[]
  angles: string[]
  separateAngleImages: boolean
  keepAngleConsistency: boolean
}

export interface OptimizePromptRequest {
  prompt: string
  api_key_id: number
  model: string
}

export interface OptimizePromptResponse {
  prompt: string
  model: string
}

export interface PromptModelsResponse {
  models: string[]
}

export interface GeneratedImagePayload {
  url?: string
  preview_url?: string
  revised_prompt?: string
  variant_label?: string
}

export interface GenerationResult {
  created?: number
  images?: GeneratedImagePayload[]
  revised_prompt?: string
  failed_variants?: string[]
  [key: string]: unknown
}

export interface GenerateResponse {
  job_id: string
  status: HistoryStatus
  result?: GenerationResult
  error_message?: string
}

export interface HistoryRecord {
  id: string
  conversation_id?: string
  user_id: number
  user_email?: string
  plugin_key?: string
  prompt: string
  status: HistoryStatus
  request: Record<string, unknown>
  result?: GenerationResult
  error_message?: string
  created_at: string
  updated_at: string
}

export interface ConversationSummary {
  id: string
  title: string
  preview: string
  status: HistoryStatus
  updated_at: string
}

export interface ConversationList {
  items: ConversationSummary[]
  next_cursor?: string
}

export interface ConversationMessages {
  items: HistoryRecord[]
  next_cursor?: string
}

export interface ImageReference {
  id: string
  dataUrl: string
  originalDataUrl?: string
  uploadDataUrl?: string
  storageKey?: string
  previewStorageKey?: string
  fileName: string
  mimeType: string
}

export interface GeneratedImage {
  id: string
  historyId?: string
  src: string
  originalSrc?: string
  revisedPrompt: string
  variantLabel?: string
  createdAt: string
	// Historical images defer network loading until near the message viewport.
	lazy?: boolean
}

export interface GenerationSlot {
  id: string
  label?: string
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  progress: number
  image?: GeneratedImage
  error?: string
}

export interface HistoryTask {
  id: string
  outputCount: number
}

export interface RequestSetting {
  modelLabel: string
  sizeLabel: string
  countLabel: string
  detailsLabel?: string
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  createdAt: string
  status?: MessageStatus
  images?: GeneratedImage[]
  generationSlots?: GenerationSlot[]
  referenceImages?: ImageReference[]
  requestSettings?: RequestSetting[]
  historyIds?: string[]
  historyTasks?: HistoryTask[]
}

export interface Conversation {
  id: string
  conversationId?: string
  title: string
  preview: string
  lastUsedAt: string
  messages: ChatMessage[]
  referenceImages: ImageReference[]
  historyIds: string[]
}
