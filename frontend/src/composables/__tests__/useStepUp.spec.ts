import { describe, it, expect, vi REDACTED from 'vitest'
import {
  useStepUp,
  isStepUpRequired,
  isStepUpBlocked,
  isStepUpCancelled,
  stepUpBlockReason,
  StepUpCancelledError
REDACTED from '../useStepUp'

describe('useStepUp error classification', () => {
  it('detects STEP_UP_REQUIRED from code field', () => {
    expect(isStepUpRequired({ status: 403, code: 'STEP_UP_REQUIRED' REDACTED)).toBe(true)
    expect(isStepUpRequired({ status: 403, reason: 'STEP_UP_REQUIRED' REDACTED)).toBe(true)
    expect(isStepUpRequired({ status: 500, code: 'INTERNAL' REDACTED)).toBe(false)
    expect(isStepUpRequired(null)).toBe(false)
  REDACTED)

  it('detects blocked (non-retryable) step-up errors', () => {
    expect(isStepUpBlocked({ code: 'STEP_UP_TOTP_NOT_ENABLED' REDACTED)).toBe(true)
    expect(isStepUpBlocked({ reason: 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' REDACTED)).toBe(true)
    expect(isStepUpBlocked({ code: 'STEP_UP_REQUIRED' REDACTED)).toBe(false)
  REDACTED)

  it('surfaces the block reason marker', () => {
    expect(stepUpBlockReason({ reason: 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' REDACTED)).toBe('STEP_UP_ADMIN_API_KEY_FORBIDDEN')
    expect(stepUpBlockReason({ code: 'OTHER' REDACTED)).toBe('')
  REDACTED)
REDACTED)

describe('useStepUp.run', () => {
  it('returns the action result directly on success', async () => {
    const stepUp = useStepUp()
    const result = await stepUp.run(async () => 42)
    expect(result).toBe(42)
    expect(stepUp.visible.value).toBe(false)
  REDACTED)

  it('re-throws non-step-up errors without prompting', async () => {
    const stepUp = useStepUp()
    const err = { status: 500, code: 'INTERNAL' REDACTED
    await expect(stepUp.run(async () => { throw err REDACTED)).rejects.toBe(err)
    expect(stepUp.visible.value).toBe(false)
  REDACTED)

  it('re-throws blocked errors without prompting', async () => {
    const stepUp = useStepUp()
    const err = { status: 403, code: 'STEP_UP_TOTP_NOT_ENABLED' REDACTED
    await expect(stepUp.run(async () => { throw err REDACTED)).rejects.toBe(err)
    expect(stepUp.visible.value).toBe(false)
  REDACTED)

  it('prompts on STEP_UP_REQUIRED and retries after verification', async () => {
    const stepUp = useStepUp()
    let calls = 0
    const action = async () => {
      calls++
      if (calls === 1) throw { status: 403, code: 'STEP_UP_REQUIRED' REDACTED
      return 'ok'
    REDACTED
    const promise = stepUp.run(action)
    // The dialog should now be open, awaiting verification (after the first rejection is handled).
    await vi.waitFor(() => expect(stepUp.visible.value).toBe(true))
    stepUp.onVerified()
    await expect(promise).resolves.toBe('ok')
    expect(calls).toBe(2)
    expect(stepUp.visible.value).toBe(false)
  REDACTED)

  it('throws a cancellation sentinel (not the original error) if the user cancels the prompt', async () => {
    const stepUp = useStepUp()
    const err = { status: 403, code: 'STEP_UP_REQUIRED' REDACTED
    const promise = stepUp.run(async () => { throw err REDACTED)
    await vi.waitFor(() => expect(stepUp.visible.value).toBe(true))
    stepUp.onCancel()
    await expect(promise).rejects.toBeInstanceOf(StepUpCancelledError)
    expect(stepUp.visible.value).toBe(false)
  REDACTED)

  it('classifies the cancellation sentinel distinctly from step-up errors', () => {
    const cancelled = new StepUpCancelledError()
    expect(isStepUpCancelled(cancelled)).toBe(true)
    expect(isStepUpRequired(cancelled)).toBe(false)
    expect(isStepUpBlocked(cancelled)).toBe(false)
    expect(isStepUpCancelled({ status: 403, code: 'STEP_UP_REQUIRED' REDACTED)).toBe(false)
  REDACTED)
REDACTED)
