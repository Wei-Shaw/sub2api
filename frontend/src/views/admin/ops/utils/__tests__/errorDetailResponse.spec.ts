import { describe, expect, it REDACTED from 'vitest'
import type { OpsErrorDetail REDACTED from '@/api/admin/ops'
import { resolvePrimaryResponseBody, resolveUpstreamPayload REDACTED from '../errorDetailResponse'

function makeDetail(overrides: Partial<OpsErrorDetail>): OpsErrorDetail {
  return {
    id: 1,
    created_at: '2026-01-01T00:00:00Z',
    phase: 'request',
    type: 'api_error',
    error_owner: 'platform',
    error_source: 'gateway',
    severity: 'P2',
    status_code: 502,
    platform: 'openai',
    model: 'gpt-4o-mini',
    resolved: false,
    client_request_id: 'crid-1',
    request_id: 'rid-1',
    message: 'Upstream request failed',
    user_email: 'user@example.com',
    account_name: 'acc',
    group_name: 'group',
    error_body: '',
    user_agent: '',
    is_business_limited: false,
    ...overrides
  REDACTED
REDACTED

describe('errorDetailResponse', () => {
  it('prefers upstream payload for request modal when error_body is generic gateway wrapper', () => {
    const detail = makeDetail({
      error_body: JSON.stringify({
        type: 'error',
        error: {
          type: 'upstream_error',
          message: 'Upstream request failed'
        REDACTED
      REDACTED),
      upstream_error_detail: '{"provider_message":"real upstream detail"REDACTED'
    REDACTED)

    expect(resolvePrimaryResponseBody(detail, 'request')).toBe('{"provider_message":"real upstream detail"REDACTED')
  REDACTED)

  it('keeps error_body for request modal when body is not generic wrapper', () => {
    const detail = makeDetail({
      error_body: JSON.stringify({
        type: 'error',
        error: {
          type: 'upstream_error',
          message: 'Upstream authentication failed, please contact administrator'
        REDACTED
      REDACTED),
      upstream_error_detail: '{"provider_message":"real upstream detail"REDACTED'
    REDACTED)

    expect(resolvePrimaryResponseBody(detail, 'request')).toBe(detail.error_body)
  REDACTED)

  it('uses upstream payload first in upstream modal', () => {
    const detail = makeDetail({
      phase: 'upstream',
      upstream_error_message: 'provider 503 overloaded',
      error_body: '{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"REDACTEDREDACTED'
    REDACTED)

    expect(resolvePrimaryResponseBody(detail, 'upstream')).toBe('provider 503 overloaded')
  REDACTED)

  it('falls back to upstream payload when request error_body is empty', () => {
    const detail = makeDetail({
      error_body: '',
      upstream_error_message: 'dial tcp timeout'
    REDACTED)

    expect(resolvePrimaryResponseBody(detail, 'request')).toBe('dial tcp timeout')
  REDACTED)

  it('resolves upstream payload by detail -> events -> message priority', () => {
    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: 'detail payload',
      upstream_errors: '[{"message":"event payload"REDACTED]',
      upstream_error_message: 'message payload'
    REDACTED))).toBe('detail payload')

    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: '',
      upstream_errors: '[{"message":"event payload"REDACTED]',
      upstream_error_message: 'message payload'
    REDACTED))).toBe('[{"message":"event payload"REDACTED]')

    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: '',
      upstream_errors: '',
      upstream_error_message: 'message payload'
    REDACTED))).toBe('message payload')
  REDACTED)

  it('treats empty JSON placeholders in upstream payload as empty', () => {
    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: '',
      upstream_errors: '[]',
      upstream_error_message: ''
    REDACTED))).toBe('')

    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: '',
      upstream_errors: '{REDACTED',
      upstream_error_message: ''
    REDACTED))).toBe('')

    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: '',
      upstream_errors: 'null',
      upstream_error_message: ''
    REDACTED))).toBe('')
  REDACTED)

  it('skips placeholder candidates and falls back to the next upstream field', () => {
    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: '',
      upstream_errors: '[]',
      upstream_error_message: 'fallback message'
    REDACTED))).toBe('fallback message')

    expect(resolveUpstreamPayload(makeDetail({
      upstream_error_detail: 'null',
      upstream_errors: '',
      upstream_error_message: 'fallback message'
    REDACTED))).toBe('fallback message')
  REDACTED)
REDACTED)
