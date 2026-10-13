import { WorkerError, safeCode, assertAlive, delay } from './errors.mjs';
import { assertIdentity, isNewFactorConfirmed } from './security.mjs';

function wireMfa(mfa) {
  return { enabled: mfa.enabled, enabled_v2: mfa.enabledV2, default_factor_id: mfa.defaultId, totp_factor_ids: mfa.ids };
}

function disabled(mfa) {
  return !mfa.enabled && !mfa.enabledV2 && mfa.ids.length === 0 && !mfa.defaultId;
}

function requirePermit(result, action, phase) {
  if (result?.permit !== action || result.job?.phase !== phase) throw new WorkerError('BACKEND_PERMIT_MISSING');
}

function requireCode(value) {
  if (!value || !/^\d{6}(?:\d{2})?$/.test(value.code) || !Number.isFinite(value.expires_at) ||
      !Number.isFinite(value.server_time) || value.expires_at <= value.server_time ||
      !Number.isInteger(value.period) || value.period < 1 || value.period > 300) throw new WorkerError('BACKEND_CODE_INVALID');
  return value;
}

export async function freshCode(read, signal, { now = () => performance.now(), wait = delay } = {}) {
  for (let attempt = 0; attempt < 3; attempt++) {
    const started = now();
    const result = requireCode(await read());
    const received = now();
    // Subtract the entire round trip, conservatively accounting for time that
    // passed after the server generated the code. Wall-clock skew is irrelevant.
    const left = result.expires_at - result.server_time - (received - started);
    if (left >= Math.min(5000, result.period * 500)) return { ...result, validUntilMono: received + left };
    if (attempt < 2) await wait(Math.max(0, left) + 150, signal);
  }
  throw new WorkerError('BACKEND_CODE_EXPIRED');
}

// Deliberately finite transitions. A persisted intent from another attempt is
// never permission to resend a provider write; only a fresh backend permit is.
export async function runJob({ session, provider, onManual = async () => { throw new WorkerError('LOGIN_MANUAL_REQUIRED'); }, onProgress = () => {} }) {
  let phase = session.job.phase;
  const email = session.account.email;
  let oldId = session.checkpointData.old_factor_id ?? '';
  let newId = session.checkpointData.new_factor_id ?? '';
  let expectedSubject = session.checkpointData.upstream_id;
  let lastManualProgress;
  const check = () => { assertAlive(session.signal); session.checkLease(); };
  const checkIdentity = async () => {
    check();
    const identity = assertIdentity(await provider.identity(email), email);
    if (expectedSubject && identity.id !== expectedSubject) throw new WorkerError('ACCOUNT_IDENTITY_MISMATCH');
    check();
    return identity;
  };
  const observe = async () => {
    const identity = await checkIdentity();
    const mfa = await provider.mfa();
    return { identity, mfa };
  };
  const advance = async (action, data) => {
    check();
    const result = await session.checkpoint(action, data);
    phase = session.job.phase;
    onProgress(phase);
    return result;
  };
  const confirm = async observation => {
    if (!isNewFactorConfirmed(observation.mfa, oldId, newId)) throw new WorkerError('ACTIVATION_OUTCOME_UNKNOWN', { unknown: true });
    await advance('verify', { identity: observation.identity, mfa: wireMfa(observation.mfa) });
    if (session.job.status !== 'completed' || session.job.phase !== 'completed') throw new WorkerError('COMPLETION_NOT_CONFIRMED', { unknown: true });
  };

  try {
    check();
    if (phase === 'completed' && session.job.status === 'completed') return { status: 'completed' };
    if (phase === 'enroll_intent') throw new WorkerError('ENROLLMENT_OUTCOME_UNKNOWN', { unknown: true });
    if (!['login', 'prepared', 'disable_intent', 'disabled', 'enrolled', 'activate_intent', 'verified'].includes(phase)) throw new WorkerError('JOB_PHASE_UNSUPPORTED');
    const identity = await provider.login({
      email, password: session.account.password,
      // The backend selects the correct saved old/pending seed for this phase.
      // activation-code is reserved for a newly permitted activation attempt.
      getOldCode: () => freshCode(() => session.loginCode(), session.signal),
      checkLease: check,
      onManual: async reason => {
        check();
        const progress = reason === 'LOGIN_CLOUDFLARE_CHALLENGE' ? 'awaiting_cloudflare' : reason === 'LOGIN_NAVIGATION_REQUIRED' ? 'awaiting_navigation' : reason === 'LOGIN_PROVIDER_REDIRECT' ? 'awaiting_provider' : 'awaiting_user';
        if (progress !== lastManualProgress) {
          await session.heartbeat(progress);
          lastManualProgress = progress;
        }
        const manuallyConfirmed = await onManual(reason, session.signal);
        check();
        if (manuallyConfirmed !== false) {
          await session.heartbeat('working');
          lastManualProgress = 'working';
        }
      },
    });
    assertIdentity(identity, email);
    if (expectedSubject && identity.id !== expectedSubject) throw new WorkerError('ACCOUNT_IDENTITY_MISMATCH');
    expectedSubject ||= identity.id;

    if (phase === 'login') {
      const current = await observe();
      if (!current.mfa.enabled || !current.mfa.enabledV2 || current.mfa.ids.length !== 1 || current.mfa.defaultId !== current.mfa.ids[0]) throw new WorkerError('INITIAL_MFA_UNSUPPORTED');
      oldId = current.mfa.ids[0];
      await advance('prepared', { identity: current.identity, mfa: wireMfa(current.mfa) });
      session.remember({ old_factor_id: oldId, upstream_id: current.identity.id });
    }
    if (phase === 'prepared') {
      const current = await observe();
      if (!oldId || !current.mfa.enabled || !current.mfa.enabledV2 || current.mfa.ids.length !== 1 || current.mfa.ids[0] !== oldId || current.mfa.defaultId !== oldId) throw new WorkerError('INITIAL_MFA_CHANGED');
      const permit = await advance('disable');
      requirePermit(permit, 'disable', 'disable_intent');
      if (permit.factor_id !== oldId) throw new WorkerError('BACKEND_FACTOR_MISMATCH');
      await checkIdentity();
      await provider.disable(oldId);
    }
    if (phase === 'disable_intent') {
      const current = await observe();
      if (!disabled(current.mfa)) throw new WorkerError('DISABLE_OUTCOME_UNKNOWN', { unknown: true });
      await advance('disabled', { identity: current.identity, mfa: wireMfa(current.mfa) });
    }
    if (phase === 'disabled') {
      const current = await observe();
      if (!disabled(current.mfa)) throw new WorkerError('INITIAL_MFA_CHANGED');
      const permit = await advance('enroll');
      requirePermit(permit, 'enroll', 'enroll_intent');
      await checkIdentity();
      let enrollment;
      try {
        enrollment = await provider.enroll(email, oldId);
        newId = enrollment.factorId;
        await advance('enrolled', { enrollment: { secret: enrollment.secret, session_id: enrollment.sessionId, factor_id: enrollment.factorId, factor_type: 'totp' } });
        if (phase !== 'enrolled') throw new WorkerError('ENROLLMENT_NOT_SAVED', { unknown: true });
        session.remember({ new_factor_id: newId, session_id: enrollment.sessionId });
      } finally {
        if (enrollment) { enrollment.secret = ''; enrollment.sessionId = ''; }
      }
    }
    if (phase === 'enrolled') {
      const current = await observe();
      if (isNewFactorConfirmed(current.mfa, oldId, newId)) {
        await confirm(current);
      } else {
        if (!disabled(current.mfa)) throw new WorkerError('ACTIVATION_OUTCOME_UNKNOWN', { unknown: true });
        const permit = await advance('activate');
        requirePermit(permit, 'activate', 'activate_intent');
        // Identity checks may require several network calls. Complete them
        // before obtaining the short-lived OTP, then do only synchronous lease
        // and freshness checks before the one permitted activation write.
        await checkIdentity();
        let candidate;
        for (let attempt = 0; attempt < 2; attempt++) {
          candidate = await freshCode(() => session.activationCode(), session.signal);
          check();
          if (candidate.validUntilMono - performance.now() > 250) break;
          candidate.code = ''; candidate.session_id = '';
        }
        try {
          if (!candidate?.code || candidate.validUntilMono - performance.now() <= 250) throw new WorkerError('BACKEND_CODE_EXPIRED');
          if (!newId || candidate.factor_id !== newId || typeof candidate.session_id !== 'string' || !candidate.session_id) throw new WorkerError('BACKEND_FACTOR_MISMATCH');
          check();
          await provider.activate({ code: candidate.code, sessionId: candidate.session_id });
        } finally {
          if (candidate) { candidate.code = ''; candidate.session_id = ''; }
        }
        await confirm(await observe());
      }
    } else if (phase === 'activate_intent' || phase === 'verified') {
      // This intent predates the current run. Observe only; do not activate a
      // second time even if the page currently says MFA is disabled.
      await confirm(await observe());
    }
    if (session.job.status !== 'completed') throw new WorkerError('JOB_NOT_COMPLETED');
    // The upstream may show its introduction only after login() has already
    // returned. Dismiss that known prompt once more before the owned browser
    // closes; it never affects the durable completion result.
    try { await provider.dismissKnownOnboarding?.(); } catch { /* Optional UI cleanup. */ }
    return { status: 'completed' };
  } catch (error) {
    const code = session.stopReason || safeCode(error);
    // Raw-terminal Ctrl+C rejects the prompt instead of emitting SIGINT.
    // Abort the owned context immediately, before the best-effort pause HTTP.
    if (code === 'WORKER_STOPPED') session.stop?.('WORKER_STOPPED');
    if (error instanceof WorkerError && error.unknown && code.startsWith('BACKEND_') && !session.signal.aborted) {
      // The same lease may read back a newer revision after a checkpoint
      // response was lost. This observation never creates a provider permit.
      try {
        await session.heartbeat();
        phase = session.job.phase;
        if (session.job.status === 'completed' && phase === 'completed') return { status: 'completed' };
      } catch { /* Pause using the last known state; the lease will expire. */ }
    }
    // Record the machine code before browser cleanup; never print provider
    // errors, URLs, credentials, or page content.
    if (Number.isSafeInteger(session.job.account_id) && session.job.account_id > 0 && /^[A-Z][A-Z0-9_]{2,63}$/.test(code)) {
      console.error(`账号 #${session.job.account_id}：已暂停（${code}）`);
    }
    await session.pause(pauseCode(code, phase));
    return { status: 'paused', code };
  } finally {
    await provider.close();
  }
}

export function pauseCode(code, phase) {
  if (code === 'WORKER_STOPPED' || code === 'BROWSER_CLOSED' || code === 'LEASE_LOST') return 'worker_stopped';
  if (code.startsWith('ACCOUNT_IDENTITY_RESPONSE_')) return code.toLowerCase().replace('account_', '');
  if (code.includes('IDENTITY')) return 'identity_mismatch';
  if (code.startsWith('BACKEND_') || code === 'ENROLLMENT_NOT_SAVED' || code === 'COMPLETION_NOT_CONFIRMED') return 'storage_failed';
  if (code.startsWith('INITIAL_MFA') || code === 'MFA_RESPONSE_UNSUPPORTED') return 'mfa_state_mismatch';
  if (phase === 'enroll_intent' || code.startsWith('ENROLLMENT_')) return 'enrollment_uncertain';
  if (phase === 'activate_intent' || code.startsWith('ACTIVATION_')) return 'activation_uncertain';
  if (code === 'UPSTREAM_WRITE_UNCONFIRMED') return 'upstream_rejected';
  if (code === 'UPSTREAM_RESPONSE_UNKNOWN' || code === 'DISABLE_OUTCOME_UNKNOWN') return 'upstream_timeout';
  if (code === 'UPSTREAM_AUTH_REQUIRED' || code === 'UPSTREAM_ORIGIN_REQUIRED') return 'unsupported_login';
  if (code === 'LOGIN_CLOUDFLARE_CHALLENGE') return 'cloudflare_challenge';
  if (code === 'LOGIN_PAGE_UNAVAILABLE' || code === 'UPSTREAM_MFA_UNAVAILABLE') return 'network_error';
  if (code === 'LOGIN_MANUAL_REQUIRED') return 'manual_required';
  return 'manual_required';
}
