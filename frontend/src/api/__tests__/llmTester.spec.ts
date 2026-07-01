import { describe, expect, it REDACTED from 'vitest'
import {
  extractImageGenerationResult,
  extractVideoGenerationResult,
  getLLMTesterModelCapabilities,
  isLikelyChatCompletionModelId,
  parseModelList,
REDACTED from '@/api/llmTester'

describe('LLM tester model filtering', () => {
  it('keeps text chat and vision chat models from provider metadata', () => {
    const models = parseModelList({
      data: [
        {
          id: 'openai/gpt-4o',
          name: 'GPT-4o',
          architecture: {
            modality: 'text+image->text',
            input_modalities: ['text', 'image'],
            output_modalities: ['text'],
          REDACTED,
        REDACTED,
        {
          id: 'anthropic/claude-sonnet',
          architecture: {
            modality: 'text->text',
            output_modalities: ['text'],
          REDACTED,
        REDACTED,
      ],
    REDACTED)

    expect(models.map((model) => model.id)).toEqual([
      'anthropic/claude-sonnet',
      'openai/gpt-4o',
    ])
  REDACTED)

  it('keeps image-generation models while removing unsupported utility models', () => {
    const models = parseModelList({
      data: [
        {
          id: 'gpt-image-2',
          architecture: {
            modality: 'text+image->image',
            output_modalities: ['image'],
          REDACTED,
        REDACTED,
        {
          id: 'text-embedding-3-small',
          architecture: {
            modality: 'text->embedding',
          REDACTED,
        REDACTED,
        {
          id: 'grok',
          architecture: {
            modality: 'text->text',
            output_modalities: ['text'],
          REDACTED,
        REDACTED,
      ],
    REDACTED)

    expect(models.map((model) => model.id)).toEqual(['gpt-image-2', 'grok'])
    expect(getLLMTesterModelCapabilities(models[0])).toContain('image_generation')
  REDACTED)

  it('keeps Grok media models and classifies them by route capability', () => {
    const models = parseModelList({
      data: [
        { id: 'grok-imagine', owned_by: 'xai' REDACTED,
        { id: 'grok-imagine-image', owned_by: 'xai' REDACTED,
        { id: 'grok-imagine-image-quality', owned_by: 'xai' REDACTED,
        { id: 'grok-imagine-edit', owned_by: 'xai' REDACTED,
        { id: 'grok-imagine-video', owned_by: 'xai' REDACTED,
        { id: 'grok-imagine-video-1.5', owned_by: 'xai' REDACTED,
      ],
    REDACTED)

    expect(models.map((model) => model.id)).toEqual([
      'grok-imagine',
      'grok-imagine-edit',
      'grok-imagine-image',
      'grok-imagine-image-quality',
      'grok-imagine-video',
      'grok-imagine-video-1.5',
    ])
    expect(getLLMTesterModelCapabilities(models[0])).toEqual(['image_generation'])
    expect(getLLMTesterModelCapabilities(models[4])).toEqual(['video_generation'])
  REDACTED)

  it('uses id heuristics when simple OpenAI-compatible model rows omit metadata', () => {
    expect(isLikelyChatCompletionModelId('gpt-5.4')).toBe(true)
    expect(isLikelyChatCompletionModelId('gpt-image-2')).toBe(false)
    expect(isLikelyChatCompletionModelId('grok-imagine-video-1.5')).toBe(false)
    expect(isLikelyChatCompletionModelId('text-embedding-3-small')).toBe(false)
  REDACTED)

  it('converts image generation responses into assistant attachments', () => {
    const result = extractImageGenerationResult({
      data: [
        {
          b64_json: 'abc123',
          revised_prompt: 'A bright test image',
        REDACTED,
      ],
    REDACTED)

    expect(result.text).toContain('Generated 1 image')
    expect(result.text).toContain('A bright test image')
    expect(result.attachments).toHaveLength(1)
    expect(result.attachments[0].dataUrl).toBe('data:image/png;base64,abc123')
  REDACTED)

  it('converts Responses image_generation_call results into assistant attachments', () => {
    const result = extractImageGenerationResult({
      output: [
        {
          type: 'image_generation_call',
          result: 'a'.repeat(120),
        REDACTED,
      ],
    REDACTED)

    expect(result.text).toContain('Generated 1 image')
    expect(result.attachments).toHaveLength(1)
    expect(result.attachments[0].dataUrl).toBe(`data:image/png;base64,${'a'.repeat(120)REDACTED`)
  REDACTED)

  it('keeps generated image URLs from provider responses', () => {
    const result = extractImageGenerationResult({
      output: [
        {
          type: 'image_generation_call',
          image_url: 'https://example.com/generated.png',
        REDACTED,
      ],
    REDACTED)

    expect(result.attachments).toHaveLength(1)
    expect(result.attachments[0].dataUrl).toBe('https://example.com/generated.png')
  REDACTED)

  it('converts Responses SSE image output events into assistant attachments', () => {
    const result = extractImageGenerationResult([
      'data: {"type":"response.output_item.done","item":{"id":"ig_123","type":"image_generation_call","result":"aGVsbG8=","revised_prompt":"draw a cat","output_format":"png"REDACTEDREDACTED',
      '',
      'data: {"type":"response.completed","response":{"output":[]REDACTEDREDACTED',
      '',
      'data: [DONE]',
      '',
    ].join('\n'))

    expect(result.text).toContain('Generated 1 image')
    expect(result.text).toContain('draw a cat')
    expect(result.attachments).toHaveLength(1)
    expect(result.attachments[0].dataUrl).toBe('data:image/png;base64,aGVsbG8=')
  REDACTED)

  it('converts video generation responses into media attachments', () => {
    const result = extractVideoGenerationResult({
      id: 'video_req_123',
      status: 'completed',
      data: [
        {
          url: 'https://example.com/generated.mp4',
        REDACTED,
      ],
    REDACTED)

    expect(result.text).toContain('Generated 1 video')
    expect(result.text).toContain('Request ID: video_req_123')
    expect(result.attachments).toHaveLength(1)
    expect(result.attachments[0].kind).toBe('media')
    expect(result.attachments[0].dataUrl).toBe('https://example.com/generated.mp4')
  REDACTED)
REDACTED)
