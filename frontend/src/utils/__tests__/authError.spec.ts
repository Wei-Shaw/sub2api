import { describe, expect, it REDACTED from 'vitest'
import { buildAuthErrorMessage REDACTED from '@/utils/authError'

describe('buildAuthErrorMessage', () => {
  it('prefers response detail message when available', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            detail: 'detailed message',
            message: 'plain message'
          REDACTED
        REDACTED,
      REDACTED,
      { fallback: 'fallback' REDACTED
    )
    expect(message).toBe('detailed message')
  REDACTED)

  it('falls back to response message when detail is unavailable', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            message: 'plain message'
          REDACTED
        REDACTED,
      REDACTED,
      { fallback: 'fallback' REDACTED
    )
    expect(message).toBe('plain message')
  REDACTED)

  it('falls back to error.message when response payload is unavailable', () => {
    const message = buildAuthErrorMessage(
      {
        message: 'error message'
      REDACTED,
      { fallback: 'fallback' REDACTED
    )
    expect(message).toBe('error message')
  REDACTED)

  it('uses fallback when no message can be extracted', () => {
    expect(buildAuthErrorMessage({REDACTED, { fallback: 'fallback' REDACTED)).toBe('fallback')
  REDACTED)
REDACTED)
