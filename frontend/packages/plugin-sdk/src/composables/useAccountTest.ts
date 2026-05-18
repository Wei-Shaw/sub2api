import { ref, nextTick, type Ref } from 'vue'

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

export interface TestOutputLine {
  text: string
  class: string
}

export interface TestImage {
  url: string
  mimeType?: string
}

export interface TestStreamOptions {
  modelId: string
  prompt?: string
  mode?: string
  extra?: Record<string, unknown>
}

export interface AccountTestStream {
  status: Ref<'idle' | 'connecting' | 'success' | 'error'>
  outputLines: Ref<TestOutputLine[]>
  streamingContent: Ref<string>
  errorMessage: Ref<string>
  images: Ref<TestImage[]>
  startTest(options: TestStreamOptions): Promise<void>
  abort(): void
  reset(): void
  addLine(text: string, className?: string): void
}

// ---------------------------------------------------------------------------
// SSE event shape
// ---------------------------------------------------------------------------

interface SseEvent {
  type: string
  text?: string
  model?: string
  success?: boolean
  error?: string
  image_url?: string
  mime_type?: string
}

// ---------------------------------------------------------------------------
// Implementation
// ---------------------------------------------------------------------------

/**
 * Composable that manages an SSE test stream for a single account.
 *
 * Plugins call `useAccountTest(accountId)` and get reactive state plus
 * `startTest` / `abort` / `reset` helpers. The composable owns the
 * AbortController lifecycle and SSE line-protocol parsing so plugin code
 * never touches fetch / ReadableStream directly.
 */
export function useAccountTest(accountId: Ref<number>): AccountTestStream {
  const status = ref<'idle' | 'connecting' | 'success' | 'error'>('idle')
  const outputLines = ref<TestOutputLine[]>([])
  const streamingContent = ref('')
  const errorMessage = ref('')
  const images = ref<TestImage[]>([])

  let abortController: AbortController | null = null

  // -- helpers --------------------------------------------------------------

  const addLine = (text: string, className: string = 'text-gray-300') => {
    outputLines.value.push({ text, class: className })
  }

  const reset = () => {
    status.value = 'idle'
    outputLines.value = []
    streamingContent.value = ''
    errorMessage.value = ''
    images.value = []
  }

  const abort = () => {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
  }

  // -- SSE event dispatcher -------------------------------------------------

  const handleEvent = (event: SseEvent) => {
    switch (event.type) {
      case 'test_start':
        if (event.model) {
          addLine(event.model, 'text-cyan-400')
        }
        break

      case 'content':
        if (event.text) {
          streamingContent.value += event.text
          void nextTick() // allow scroll watchers to react
        }
        break

      case 'image':
        if (event.image_url) {
          images.value.push({
            url: event.image_url,
            mimeType: event.mime_type,
          })
        }
        break

      case 'test_complete':
        if (streamingContent.value) {
          addLine(streamingContent.value, 'text-green-300')
          streamingContent.value = ''
        }
        status.value = event.success ? 'success' : 'error'
        if (!event.success) {
          errorMessage.value = event.error || 'Test failed'
        }
        break

      case 'error':
        if (streamingContent.value) {
          addLine(streamingContent.value, 'text-green-300')
          streamingContent.value = ''
        }
        status.value = 'error'
        errorMessage.value = event.error || 'Unknown error'
        break
    }
  }

  // -- main entry point -----------------------------------------------------

  const startTest = async (options: TestStreamOptions): Promise<void> => {
    reset()
    status.value = 'connecting'

    abort() // cancel any in-flight request
    abortController = new AbortController()

    const url = `/api/v1/admin/accounts/${accountId.value}/test`

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          model_id: options.modelId,
          prompt: options.prompt || '',
          mode: options.mode || 'default',
          ...options.extra,
        }),
        signal: abortController.signal,
      })

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`)
      }

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('No response body')
      }

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const jsonStr = line.slice(6).trim()
          if (!jsonStr) continue
          try {
            handleEvent(JSON.parse(jsonStr) as SseEvent)
          } catch {
            // skip malformed JSON
          }
        }
      }
    } catch (error: unknown) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        status.value = 'idle'
        return
      }
      status.value = 'error'
      const msg = error instanceof Error ? error.message : 'Unknown error'
      errorMessage.value = msg
      addLine(`Error: ${msg}`, 'text-red-400')
    }
  }

  return {
    status,
    outputLines,
    streamingContent,
    errorMessage,
    images,
    startTest,
    abort,
    reset,
    addLine,
  }
}
