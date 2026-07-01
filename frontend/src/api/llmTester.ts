import { buildApiUrl REDACTED from '@/api/client'

export interface LLMTesterProfile {
  id: string
  name: string
  provider: 'openrouter' | 'sub2api' | 'custom'
  baseUrl: string
  apiKey: string
  selectedModel: string
  lastFetchedAt?: string
REDACTED

export interface LLMTesterModel {
  id: string
  name: string
  ownedBy?: string
  contextLength?: number
  raw?: Record<string, unknown>
REDACTED

export type LLMTesterModelCapability = 'chat' | 'vision' | 'image_generation' | 'video_generation'

export interface LLMTesterAttachment {
  id: string
  name: string
  type: string
  size: number
  kind: 'image' | 'text' | 'media' | 'file'
  dataUrl?: string
  text?: string
REDACTED

export interface LLMTesterMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  attachments?: LLMTesterAttachment[]
REDACTED

export interface ChatCompletionOptions {
  baseUrl: string
  apiKey: string
  model: string
  messages: LLMTesterMessage[]
  systemInstruction?: string
  temperature?: number
  maxTokens?: number
  signal?: AbortSignal
REDACTED

export interface ImageGenerationOptions {
  baseUrl: string
  apiKey: string
  model: string
  messages: LLMTesterMessage[]
  systemInstruction?: string
  signal?: AbortSignal
REDACTED

export interface ImageGenerationResult {
  text: string
  attachments: LLMTesterAttachment[]
  raw: unknown
REDACTED

export type MediaGenerationResult = ImageGenerationResult

interface OpenAIContentTextPart {
  type: 'text'
  text: string
REDACTED

interface OpenAIContentImagePart {
  type: 'image_url'
  image_url: {
    url: string
  REDACTED
REDACTED

type OpenAIMessageContent = string | Array<OpenAIContentTextPart | OpenAIContentImagePart>

interface OpenAIChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: OpenAIMessageContent
REDACTED

export const OPENROUTER_BASE_URL = 'https://openrouter.ai/api/v1'

export function defaultSub2APIBaseUrl(): string {
  return '/v1'
REDACTED

export function normalizeBaseUrl(input: string): string {
  const trimmed = input.trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  if (/^https?:\/\//i.test(trimmed) || trimmed.startsWith('/')) return trimmed
  return `https://${trimmedREDACTED`
REDACTED

export type LLMTesterProxyPath = 'models' | 'chat/completions' | 'images/generations' | 'videos/generations' | 'responses'

export function buildOpenAIEndpoint(baseUrl: string, path: LLMTesterProxyPath): string {
  const normalized = normalizeBaseUrl(baseUrl)
  if (!normalized) return ''
  const resource = path.replace(/^v\d+\//, '')
  if (/\/v\d+$/i.test(normalized)) return `${normalizedREDACTED/${resourceREDACTED`
  return `${normalizedREDACTED/v1/${resourceREDACTED`
REDACTED

function getHeaderSafeSiteTitle(): string {
  if (typeof document === 'undefined') return 'Sub2API LLM Tester'
  return document.title || 'Sub2API LLM Tester'
REDACTED

function buildHeaders(apiKey: string): HeadersInit {
  return {
    Authorization: `Bearer ${apiKeyREDACTED`,
    'Content-Type': 'application/json',
    'X-Title': getHeaderSafeSiteTitle(),
  REDACTED
REDACTED

function buildJsonHeaders(): HeadersInit {
  return {
    'Content-Type': 'application/json',
  REDACTED
REDACTED

function getObject(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' ? value as Record<string, unknown> : undefined
REDACTED

function getString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined
REDACTED

function getNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
REDACTED

function getStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value
    .map((item) => typeof item === 'string' ? item.trim().toLowerCase() : '')
    .filter(Boolean)
REDACTED

export function isLikelyChatCompletionModelId(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  if (!id) return false
  if (/(^|[/:-])(?:text-)?embedding/.test(id) || id.includes('embedding')) return false
  if (/(^|[/:-])(?:gpt-)?image(?:-|$)/.test(id) || id.includes('/image-')) return false
  if (isLikelyImageGenerationModelId(id) || isLikelyVideoGenerationModelId(id)) return false
  if (id.includes('dall-e') || id.includes('whisper') || id.includes('tts')) return false
  if (id.includes('moderation') || id.includes('omni-moderation')) return false
  if (id.includes('transcribe') || id.includes('realtime')) return false
  return true
REDACTED

const GROK_IMAGE_MODEL_IDS = new Set([
  'grok-imagine',
  'grok-imagine-image',
  'grok-imagine-image-quality',
  'grok-imagine-edit',
])

const GROK_VIDEO_MODEL_IDS = new Set([
  'grok-imagine-video',
  'grok-imagine-video-1.5',
])

export function isLikelyImageGenerationModelId(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  if (!id) return false
  return (
    GROK_IMAGE_MODEL_IDS.has(id) ||
    /(^|[/:-])(?:gpt-)?image(?:-|$)/.test(id) ||
    id.includes('/image-') ||
    id.includes('dall-e') ||
    id.includes('imagen')
  )
REDACTED

export function isLikelyVideoGenerationModelId(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  if (!id) return false
  return GROK_VIDEO_MODEL_IDS.has(id) || id.includes('video-generation') || /(^|[/:-])video(?:-|$)/.test(id)
REDACTED

function splitModalities(value: string): string[] {
  return value
    .split(/[+,]/)
    .map((part) => part.trim().toLowerCase())
    .filter(Boolean)
REDACTED

function getModelModalities(model: LLMTesterModel): { input: string[]; output: string[] REDACTED {
  const architecture = getObject(model.raw?.architecture)
  const input = new Set(getStringArray(architecture?.input_modalities))
  const output = new Set(getStringArray(architecture?.output_modalities))

  const modality = getString(architecture?.modality)?.toLowerCase()
  if (modality?.includes('->')) {
    const [inputSide, outputSide] = modality.split('->')
    splitModalities(inputSide || '').forEach((item) => input.add(item))
    splitModalities(outputSide || '').forEach((item) => output.add(item))
  REDACTED

  return {
    input: Array.from(input),
    output: Array.from(output),
  REDACTED
REDACTED

function isKnownUnsupportedModelId(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  return (
    /(^|[/:-])(?:text-)?embedding/.test(id) ||
    id.includes('embedding') ||
    id.includes('moderation') ||
    id.includes('omni-moderation') ||
    id.includes('whisper') ||
    id.includes('tts') ||
    id.includes('transcribe') ||
    id.includes('realtime')
  )
REDACTED

export function getLLMTesterModelCapabilities(model: LLMTesterModel): LLMTesterModelCapability[] {
  const capabilities = new Set<LLMTesterModelCapability>()
  const modalities = getModelModalities(model)
  const hasOutputMetadata = modalities.output.length > 0
  const outputsText = modalities.output.includes('text')
  const outputsImage = modalities.output.includes('image') || isLikelyImageGenerationModelId(model.id)
  const outputsVideo = modalities.output.includes('video') || isLikelyVideoGenerationModelId(model.id)
  const unsupportedByTester = isKnownUnsupportedModelId(model.id)

  if (outputsImage) {
    capabilities.add('image_generation')
  REDACTED

  if (outputsVideo) {
    capabilities.add('video_generation')
  REDACTED

  if (!unsupportedByTester && !outputsImage && !outputsVideo && (!hasOutputMetadata || outputsText)) {
    capabilities.add('chat')
  REDACTED

  if (capabilities.has('chat') && modalities.input.includes('image')) {
    capabilities.add('vision')
  REDACTED

  return Array.from(capabilities)
REDACTED

export function isChatCompletionModel(model: LLMTesterModel): boolean {
  return getLLMTesterModelCapabilities(model).includes('chat')
REDACTED

export function isImageGenerationModel(model: LLMTesterModel): boolean {
  return getLLMTesterModelCapabilities(model).includes('image_generation')
REDACTED

export function isVideoGenerationModel(model: LLMTesterModel): boolean {
  return getLLMTesterModelCapabilities(model).includes('video_generation')
REDACTED

export function isLLMTesterSupportedModel(model: LLMTesterModel): boolean {
  const capabilities = getLLMTesterModelCapabilities(model)
  return capabilities.includes('chat') || capabilities.includes('image_generation') || capabilities.includes('video_generation')
REDACTED

function extractErrorMessage(payload: unknown, fallback: string): string {
  const obj = getObject(payload)
  const errorObj = getObject(obj?.error)
  return (
    getString(errorObj?.message) ||
    getString(obj?.message) ||
    getString(obj?.detail) ||
    fallback
  )
REDACTED

async function parseResponsePayload(response: Response): Promise<unknown> {
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) return response.json()
  const text = await response.text()
  try {
    return JSON.parse(text)
  REDACTED catch {
    return text
  REDACTED
REDACTED

function unwrapApiEnvelope(payload: unknown): unknown {
  const obj = getObject(payload)
  if (!obj || !('code' in obj) || !('data' in obj)) return payload
  return obj.data
REDACTED

function shouldUseTesterProxy(baseUrl: string): boolean {
  const normalized = normalizeBaseUrl(baseUrl)
  if (!normalized || normalized.startsWith('/')) return false
  if (typeof window === 'undefined') return true
  try {
    return new URL(normalized).origin !== window.location.origin
  REDACTED catch {
    return true
  REDACTED
REDACTED

async function postTesterProxy(path: LLMTesterProxyPath, body: Record<string, unknown>, signal?: AbortSignal): Promise<unknown> {
  const response = await fetch(buildApiUrl(`/llm-tester/${pathREDACTED`), {
    method: 'POST',
    headers: buildJsonHeaders(),
    body: JSON.stringify(body),
    signal,
  REDACTED)
  const payload = await parseResponsePayload(response)
  if (!response.ok) {
    const fallback = path === 'models'
      ? `Failed to fetch models (${response.statusREDACTED)`
      : path === 'videos/generations'
        ? `Video generation failed (${response.statusREDACTED)`
        : path === 'images/generations' || path === 'responses'
          ? `Image generation failed (${response.statusREDACTED)`
          : `Chat request failed (${response.statusREDACTED)`
    throw new Error(extractErrorMessage(payload, fallback))
  REDACTED
  return unwrapApiEnvelope(payload)
REDACTED

export function parseModelList(payload: unknown): LLMTesterModel[] {
  const obj = getObject(payload)
  const data = Array.isArray(obj?.data) ? obj.data : Array.isArray(payload) ? payload : []

  return data
    .map((item): LLMTesterModel | null => {
      const raw = getObject(item)
      if (!raw) return null

      const id = getString(raw.id) || getString(raw.name)
      if (!id) return null

      const topProvider = getObject(raw.top_provider)
      return {
        id,
        name: getString(raw.name) || id,
        ownedBy: getString(raw.owned_by) || getString(raw.ownedBy),
        contextLength: getNumber(raw.context_length) || getNumber(raw.contextLength) || getNumber(topProvider?.context_length),
        raw,
      REDACTED
    REDACTED)
    .filter((model): model is LLMTesterModel => model !== null)
    .filter(isLLMTesterSupportedModel)
    .sort((a, b) => a.id.localeCompare(b.id))
REDACTED

export async function fetchLLMModels(baseUrl: string, apiKey: string, signal?: AbortSignal): Promise<LLMTesterModel[]> {
  const endpoint = buildOpenAIEndpoint(baseUrl, 'models')
  if (!endpoint) throw new Error('Base URL is required')

  if (shouldUseTesterProxy(baseUrl)) {
    const payload = await postTesterProxy('models', {
      base_url: normalizeBaseUrl(baseUrl),
      api_key: apiKey,
    REDACTED, signal)
    return parseModelList(payload)
  REDACTED

  const response = await fetch(endpoint, {
    method: 'GET',
    headers: buildHeaders(apiKey),
    signal,
  REDACTED)
  const payload = await parseResponsePayload(response)
  if (!response.ok) {
    throw new Error(extractErrorMessage(payload, `Failed to fetch models (${response.statusREDACTED)`))
  REDACTED

  return parseModelList(payload)
REDACTED

function inferLanguage(filename: string, type: string): string {
  const lower = filename.toLowerCase()
  const ext = lower.includes('.') ? lower.split('.').pop() || '' : ''
  const byExt: Record<string, string> = {
    js: 'javascript',
    jsx: 'jsx',
    ts: 'typescript',
    tsx: 'tsx',
    vue: 'vue',
    py: 'python',
    go: 'go',
    rs: 'rust',
    java: 'java',
    c: 'c',
    cpp: 'cpp',
    cs: 'csharp',
    html: 'html',
    css: 'css',
    json: 'json',
    md: 'markdown',
    sh: 'bash',
    sql: 'sql',
    yml: 'yaml',
    yaml: 'yaml',
    xml: 'xml',
    toml: 'toml',
    csv: 'csv',
  REDACTED
  if (byExt[ext]) return byExt[ext]
  if (type.includes('json')) return 'json'
  if (type.includes('markdown')) return 'markdown'
  if (type.includes('html')) return 'html'
  return ''
REDACTED

function formatTextAttachment(attachment: LLMTesterAttachment): string {
  const language = inferLanguage(attachment.name, attachment.type)
  return [
    `Attached file: ${attachment.nameREDACTED`,
    `\`\`\`${languageREDACTED`,
    attachment.text || '',
    '```',
  ].join('\n')
REDACTED

function buildImageGenerationPrompt(messages: LLMTesterMessage[], systemInstruction = ''): string {
  const latestUserMessage = [...messages].reverse().find((message) => message.role === 'user')
  const attachments = latestUserMessage?.attachments || []
  const textAttachments = attachments.filter((attachment) => attachment.kind === 'text' && attachment.text)
  const mediaAttachments = attachments.filter((attachment) => attachment.kind !== 'text')

  const sections = [
    systemInstruction.trim(),
    latestUserMessage?.content.trim() || '',
    ...textAttachments.map(formatTextAttachment),
    ...mediaAttachments.map((attachment) => `Attached reference file: ${attachment.nameREDACTED (${attachment.type || 'unknown type'REDACTED, ${attachment.sizeREDACTED bytes).`),
  ].filter(Boolean)

  return sections.join('\n\n')
REDACTED

function buildMediaGenerationPrompt(messages: LLMTesterMessage[], systemInstruction = ''): string {
  return buildImageGenerationPrompt(messages, systemInstruction)
REDACTED

function buildUserContent(message: LLMTesterMessage): OpenAIMessageContent {
  const attachments = message.attachments || []
  const imageAttachments = attachments.filter((attachment) => attachment.kind === 'image' && attachment.dataUrl)
  const textAttachments = attachments.filter((attachment) => attachment.kind === 'text' && attachment.text)
  const otherAttachments = attachments.filter((attachment) => attachment.kind !== 'image' && attachment.kind !== 'text')

  const textParts = [
    message.content.trim(),
    ...textAttachments.map(formatTextAttachment),
    ...otherAttachments.map((attachment) => `Attached media: ${attachment.nameREDACTED (${attachment.type || 'unknown type'REDACTED, ${attachment.sizeREDACTED bytes).`),
  ].filter(Boolean)

  if (imageAttachments.length === 0) return textParts.join('\n\n')

  const content: Array<OpenAIContentTextPart | OpenAIContentImagePart> = []
  content.push({
    type: 'text',
    text: textParts.join('\n\n') || 'Please analyze the attached image.',
  REDACTED)

  for (const attachment of imageAttachments) {
    if (!attachment.dataUrl) continue
    content.push({
      type: 'image_url',
      image_url: { url: attachment.dataUrl REDACTED,
    REDACTED)
  REDACTED

  return content
REDACTED

export function buildChatCompletionMessages(messages: LLMTesterMessage[], systemInstruction = ''): OpenAIChatMessage[] {
  const out: OpenAIChatMessage[] = []
  const system = systemInstruction.trim()
  if (system) {
    out.push({ role: 'system', content: system REDACTED)
  REDACTED

  for (const message of messages) {
    out.push({
      role: message.role,
      content: message.role === 'user' ? buildUserContent(message) : message.content,
    REDACTED)
  REDACTED

  return out
REDACTED

export function extractChatCompletionText(payload: unknown): string {
  const obj = getObject(payload)
  const choices = Array.isArray(obj?.choices) ? obj.choices : []
  const firstChoice = getObject(choices[0])
  const message = getObject(firstChoice?.message)
  const content = message?.content

  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .map((part) => {
        const partObj = getObject(part)
        return getString(partObj?.text) || getString(partObj?.content) || ''
      REDACTED)
      .filter(Boolean)
      .join('\n')
  REDACTED

  const text = getString(firstChoice?.text)
  if (text) return text

  return JSON.stringify(payload, null, 2)
REDACTED

export function extractImageGenerationResult(payload: unknown): ImageGenerationResult {
  const attachments: LLMTesterAttachment[] = []
  const lines: string[] = []

  const pushImageAttachment = (rawValue: unknown, index: number) => {
    const value = normalizeGeneratedImageValue(rawValue)
    if (!value) return
    attachments.push({
      id: `generated-image-${Date.now()REDACTED-${indexREDACTED`,
      name: `generated-image-${index + 1REDACTED.png`,
      type: 'image/png',
      size: 0,
      kind: 'image',
      dataUrl: value,
    REDACTED)
  REDACTED

  const explicitImageResult = (value: unknown): unknown => {
    const text = getString(value)
    if (!text) return value
    if (/^(?:data:image\/|https?:\/\/)/i.test(text)) return text
    return `data:image/png;base64,${textREDACTED`
  REDACTED

  const processOutputItem = (item: unknown) => {
    const outputItem = getObject(item)
    if (!outputItem) return
    const type = getString(outputItem.type)

    if (type === 'image_generation_call') {
      const b64 = getString(outputItem.b64_json)
      pushImageAttachment(b64 ? `data:image/png;base64,${b64REDACTED` : explicitImageResult(outputItem.result) || outputItem.image_url || outputItem.url, attachments.length)
      const revisedPrompt = getString(outputItem.revised_prompt)
      if (revisedPrompt) {
        lines.push(`Revised prompt: ${revisedPromptREDACTED`)
      REDACTED
    REDACTED

    const content = Array.isArray(outputItem.content) ? outputItem.content : []
    content.forEach((part) => {
      const partObj = getObject(part)
      if (!partObj) return
      const partType = getString(partObj.type)
      const text = getString(partObj.text)
      if (text && (partType === 'output_text' || partType === 'text')) {
        lines.push(text)
      REDACTED
      const b64 = getString(partObj.b64_json)
      pushImageAttachment(b64 ? `data:image/png;base64,${b64REDACTED` : explicitImageResult(partObj.result) || partObj.image_url || partObj.url, attachments.length)
    REDACTED)

    const outputText = getString(outputItem.text)
    if (outputText && type !== 'image_generation_call') {
      lines.push(outputText)
    REDACTED
  REDACTED

  const processPayload = (rawPayload: unknown) => {
    const obj = getObject(rawPayload)
    if (!obj) return

    if (obj.item) {
      processOutputItem(obj.item)
    REDACTED
    if (obj.response) {
      processPayload(obj.response)
    REDACTED

    const data = Array.isArray(obj.data) ? obj.data : []
    data.forEach((item, index) => {
      const image = getObject(item)
      if (!image) return

      const revisedPrompt = getString(image.revised_prompt)
      if (revisedPrompt) {
        lines.push(`Revised prompt: ${revisedPromptREDACTED`)
      REDACTED

      const b64 = getString(image.b64_json)
      const url = getString(image.url)
      pushImageAttachment(b64 ? `data:image/png;base64,${b64REDACTED` : url, index)
    REDACTED)

    const output = Array.isArray(obj.output) ? obj.output : []
    output.forEach(processOutputItem)
  REDACTED

  const payloads = typeof payload === 'string' ? parseEventStreamPayload(payload) : [payload]
  payloads.forEach(processPayload)

  if (attachments.length > 0) {
    lines.unshift(`Generated ${attachments.lengthREDACTED image${attachments.length === 1 ? '' : 's'REDACTED.`)
  REDACTED

  return {
    text: lines.join('\n\n') || JSON.stringify(payload, null, 2),
    attachments,
    raw: payload,
  REDACTED
REDACTED

function parseEventStreamPayload(payload: string): unknown[] {
  const events: unknown[] = []
  const dataLines: string[] = []

  const flush = () => {
    const data = dataLines.join('\n').trim()
    dataLines.length = 0
    if (!data || data === '[DONE]') return
    try {
      events.push(JSON.parse(data))
    REDACTED catch {
      events.push(data)
    REDACTED
  REDACTED

  for (const line of payload.split(/\r?\n/)) {
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
      continue
    REDACTED
    if (!line.trim()) {
      flush()
    REDACTED
  REDACTED
  flush()

  if (events.length > 0) return events
  try {
    return [JSON.parse(payload)]
  REDACTED catch {
    return []
  REDACTED
REDACTED

function normalizeGeneratedImageValue(value: unknown): string {
  if (typeof value === 'object' && value !== null) {
    const obj = getObject(value)
    return normalizeGeneratedImageValue(obj?.url || obj?.b64_json || obj?.result)
  REDACTED
  const text = getString(value)
  if (!text) return ''
  if (/^data:image\//i.test(text)) return text
  if (/^https?:\/\//i.test(text)) return text
  const compact = text.replace(/\s+/g, '')
  if (compact.length > 100 && /^[A-Za-z0-9+/=]+$/.test(compact)) {
    return `data:image/png;base64,${compactREDACTED`
  REDACTED
  return ''
REDACTED

function normalizeGeneratedMediaValue(value: unknown): string {
  if (typeof value === 'object' && value !== null) {
    const obj = getObject(value)
    return normalizeGeneratedMediaValue(
      obj?.url ||
      obj?.video_url ||
      obj?.download_url ||
      obj?.b64_json ||
      obj?.base64 ||
      obj?.result
    )
  REDACTED
  const text = getString(value)
  if (!text) return ''
  if (/^data:video\//i.test(text)) return text
  if (/^https?:\/\//i.test(text)) return text
  const compact = text.replace(/\s+/g, '')
  if (compact.length > 100 && /^[A-Za-z0-9+/=]+$/.test(compact)) {
    return `data:video/mp4;base64,${compactREDACTED`
  REDACTED
  return ''
REDACTED

export function extractVideoGenerationResult(payload: unknown): MediaGenerationResult {
  const attachments: LLMTesterAttachment[] = []
  const lines: string[] = []

  const pushVideoAttachment = (rawValue: unknown, index: number) => {
    const value = normalizeGeneratedMediaValue(rawValue)
    if (!value) return
    attachments.push({
      id: `generated-video-${Date.now()REDACTED-${indexREDACTED`,
      name: `generated-video-${index + 1REDACTED.mp4`,
      type: 'video/mp4',
      size: 0,
      kind: 'media',
      dataUrl: value,
    REDACTED)
  REDACTED

  const processObject = (value: unknown) => {
    const obj = getObject(value)
    if (!obj) return

    const status = getString(obj.status)
    if (status) lines.push(`Status: ${statusREDACTED`)
    const id = getString(obj.id) || getString(obj.request_id)
    if (id) lines.push(`Request ID: ${idREDACTED`)
    const revisedPrompt = getString(obj.revised_prompt)
    if (revisedPrompt) lines.push(`Revised prompt: ${revisedPromptREDACTED`)

    pushVideoAttachment(obj, attachments.length)

    const data = Array.isArray(obj.data) ? obj.data : []
    data.forEach((item) => {
      processObject(item)
    REDACTED)

    const output = Array.isArray(obj.output) ? obj.output : []
    output.forEach((item) => {
      processObject(item)
    REDACTED)

    const content = Array.isArray(obj.content) ? obj.content : []
    content.forEach((item) => {
      const itemObj = getObject(item)
      const text = getString(itemObj?.text)
      if (text) lines.push(text)
      processObject(item)
    REDACTED)
  REDACTED

  const payloads = typeof payload === 'string' ? parseEventStreamPayload(payload) : [payload]
  payloads.forEach(processObject)

  const uniqueLines = Array.from(new Set(lines))
  if (attachments.length > 0) {
    uniqueLines.unshift(`Generated ${attachments.lengthREDACTED video${attachments.length === 1 ? '' : 's'REDACTED.`)
  REDACTED

  return {
    text: uniqueLines.join('\n\n') || JSON.stringify(payload, null, 2),
    attachments,
    raw: payload,
  REDACTED
REDACTED

function imageToolModelId(model: string): string {
  const trimmed = model.trim()
  if (!trimmed) return 'gpt-image-2'
  const parts = trimmed.split('/').filter(Boolean)
  return parts[parts.length - 1] || trimmed
REDACTED

function imageResponsesDriverModel(model: string): string {
  return isLikelyImageGenerationModelId(model) ? 'gpt-5.4' : model
REDACTED

function buildResponsesImageGenerationBody(model: string, prompt: string): Record<string, unknown> {
  return {
    model: imageResponsesDriverModel(model),
    stream: true,
    tools: [
      {
        type: 'image_generation',
        model: imageToolModelId(model),
      REDACTED,
    ],
    input: [
      {
        role: 'user',
        content: [
          {
            type: 'input_text',
            text: prompt,
          REDACTED,
        ],
      REDACTED,
    ],
  REDACTED
REDACTED

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
REDACTED

async function postOpenAIResource(
  baseUrl: string,
  apiKey: string,
  path: LLMTesterProxyPath,
  body: Record<string, unknown>,
  signal?: AbortSignal
): Promise<unknown> {
  const endpoint = buildOpenAIEndpoint(baseUrl, path)
  if (!endpoint) throw new Error('Base URL is required')

  if (shouldUseTesterProxy(baseUrl)) {
    return postTesterProxy(path, {
      base_url: normalizeBaseUrl(baseUrl),
      api_key: apiKey,
      payload: body,
    REDACTED, signal)
  REDACTED

  const response = await fetch(endpoint, {
    method: 'POST',
    headers: buildHeaders(apiKey),
    body: JSON.stringify(body),
    signal,
  REDACTED)
  const payload = await parseResponsePayload(response)
  if (!response.ok) {
    const fallback = path === 'chat/completions'
      ? `Chat request failed (${response.statusREDACTED)`
      : path === 'videos/generations'
        ? `Video generation failed (${response.statusREDACTED)`
        : `Image generation failed (${response.statusREDACTED)`
    throw new Error(extractErrorMessage(payload, fallback))
  REDACTED

  return payload
REDACTED

export async function sendLLMChatCompletion(options: ChatCompletionOptions): Promise<{ text: string; raw: unknown REDACTED> {
  const endpoint = buildOpenAIEndpoint(options.baseUrl, 'chat/completions')
  if (!endpoint) throw new Error('Base URL is required')

  const body: Record<string, unknown> = {
    model: options.model,
    messages: buildChatCompletionMessages(options.messages, options.systemInstruction),
    stream: false,
  REDACTED

  if (typeof options.temperature === 'number' && Number.isFinite(options.temperature)) {
    body.temperature = options.temperature
  REDACTED
  if (typeof options.maxTokens === 'number' && Number.isFinite(options.maxTokens) && options.maxTokens > 0) {
    body.max_tokens = Math.floor(options.maxTokens)
  REDACTED

  if (shouldUseTesterProxy(options.baseUrl)) {
    const payload = await postTesterProxy('chat/completions', {
      base_url: normalizeBaseUrl(options.baseUrl),
      api_key: options.apiKey,
      payload: body,
    REDACTED, options.signal)
    return {
      text: extractChatCompletionText(payload),
      raw: payload,
    REDACTED
  REDACTED

  const response = await fetch(endpoint, {
    method: 'POST',
    headers: buildHeaders(options.apiKey),
    body: JSON.stringify(body),
    signal: options.signal,
  REDACTED)
  const payload = await parseResponsePayload(response)
  if (!response.ok) {
    throw new Error(extractErrorMessage(payload, `Chat request failed (${response.statusREDACTED)`))
  REDACTED

  return {
    text: extractChatCompletionText(payload),
    raw: payload,
  REDACTED
REDACTED

export async function sendLLMImageGeneration(options: ImageGenerationOptions): Promise<ImageGenerationResult> {
  const prompt = buildImageGenerationPrompt(options.messages, options.systemInstruction)
  if (!prompt) throw new Error('Prompt is required for image generation')

  const body: Record<string, unknown> = {
    model: options.model,
    prompt,
    n: 1,
  REDACTED
  if (/^gpt-image-/i.test(imageToolModelId(options.model))) {
    body.stream = true
  REDACTED

  try {
    const payload = await postOpenAIResource(options.baseUrl, options.apiKey, 'images/generations', body, options.signal)
    return extractImageGenerationResult(payload)
  REDACTED catch (primaryError) {
    if (isAbortError(primaryError)) throw primaryError

    try {
      const fallbackPayload = await postOpenAIResource(
        options.baseUrl,
        options.apiKey,
        'responses',
        buildResponsesImageGenerationBody(options.model, prompt),
        options.signal
      )
      const fallbackResult = extractImageGenerationResult(fallbackPayload)
      if (fallbackResult.attachments.length > 0) return fallbackResult
      throw new Error('Responses image tool returned no image output')
    REDACTED catch (fallbackError) {
      if (isAbortError(fallbackError)) throw fallbackError
      const primaryMessage = primaryError instanceof Error ? primaryError.message : 'Image endpoint failed'
      const fallbackMessage = fallbackError instanceof Error ? fallbackError.message : 'Responses fallback failed'
      throw new Error(`${primaryMessageREDACTED; responses fallback failed: ${fallbackMessageREDACTED`)
    REDACTED
  REDACTED
REDACTED

export async function sendLLMVideoGeneration(options: ImageGenerationOptions): Promise<MediaGenerationResult> {
  const prompt = buildMediaGenerationPrompt(options.messages, options.systemInstruction)
  if (!prompt) throw new Error('Prompt is required for video generation')

  const body: Record<string, unknown> = {
    model: options.model,
    prompt,
  REDACTED

  const payload = await postOpenAIResource(options.baseUrl, options.apiKey, 'videos/generations', body, options.signal)
  return extractVideoGenerationResult(payload)
REDACTED
