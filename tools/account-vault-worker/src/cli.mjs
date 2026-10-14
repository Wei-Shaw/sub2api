import { randomUUID } from 'node:crypto';
import { BackendClient, JobSession } from './backend.mjs';
import { browserLaunchFailure, browserLaunchOptions } from './browser-launch.mjs';
import { launchPrivateEdge } from './edge-cdp.mjs';
import { WorkerError, delay, safeCode } from './errors.mjs';
import { runJob, pauseCode } from './machine.mjs';
import { runSessionJob } from './session-machine.mjs';
import { runPool } from './pool.mjs';
import { createProvider } from './provider.mjs';
import { askOrigin, readHiddenLine, waitForManual } from './terminal.mjs';
import { validateOrigin } from './security.mjs';

const LABELS = {
  login: '正在官方页面登录', prepared: '已核对账号及原 2FA', disable_intent: '正在关闭原 2FA',
  disabled: '原 2FA 已关闭', enroll_intent: '正在申请新 2FA', enrolled: '新 2FA 已加密保存',
  activate_intent: '正在激活新 2FA', verified: '正在确认新 2FA', completed: '首次换绑已完成',
};

function options(args) {
  const result = { once: false, concurrency: 2 };
  for (let index = 0; index < args.length; index++) {
    const arg = args[index];
    if (arg === '--help') return { help: true };
    if (arg === '--once') result.once = true;
    else if (arg === '--origin' && args[index + 1]) result.origin = args[++index];
    else if (arg === '--concurrency' && /^[1-4]$/.test(args[index + 1] ?? '')) result.concurrency = Number(args[++index]);
    else throw new WorkerError('ARGUMENT_INVALID');
  }
  // --once retains the original meaning: process at most one account.
  if (result.once) result.concurrency = 1;
  return result;
}

async function markWindow(page, accountId) {
  const suffix = ` · 账号 #${accountId}`;
  const mark = suffix => {
    const update = () => {
      if (document.title && !document.title.endsWith(suffix)) document.title += suffix;
    };
    update();
    new MutationObserver(update).observe(document, { subtree: true, childList: true, characterData: true });
  };
  await page.addInitScript(mark, suffix);
  await page.evaluate(mark, suffix).catch(() => {});
}

async function main() {
  const config = options(process.argv.slice(2));
  if (config.help) {
    console.log('用法：npm start -- --origin https://panel.example.com [--concurrency 1..4] [--once]\n连接码只通过隐藏的 stdin 输入。后台决定实际并发数。');
    return;
  }
  if (Number(process.versions.node.split('.')[0]) !== 24) throw new WorkerError('NODE_24_REQUIRED');
  delete process.env.DEBUG;
  delete process.env.PWDEBUG;
  const stop = new AbortController();
  const active = new Set();
  let browser, client;
  const interrupt = () => {
    stop.abort();
    for (const task of active) {
      task.session.stop('WORKER_STOPPED');
      void task.provider?.close();
    }
  };
  process.once('SIGINT', interrupt);
  process.once('SIGTERM', interrupt);
  try {
    const origin = validateOrigin(config.origin ?? await askOrigin({ signal: stop.signal }));
    let ticket = await readHiddenLine({ signal: stop.signal });
    client = new BackendClient({ origin, ticket });
    ticket = '';
    const { chromium } = await import('playwright');
    const privateEdge = process.platform === 'win32' && !process.env.ACCOUNT_VAULT_BROWSER_CHANNEL;
    if (!privateEdge) {
      const launchOptions = browserLaunchOptions();
      try { browser = await chromium.launch(launchOptions); }
      catch (error) { throw new WorkerError(browserLaunchFailure(error, launchOptions)); }
    }
    const workerIds = Array.from({ length: config.concurrency }, () => randomUUID());
    console.log(`助手已连接，最多 ${config.concurrency} 个独立窗口，后台控制实际并发。窗口标题标明账号编号；额外验证请在对应窗口完成。`);
    await runPool({
      concurrency: config.concurrency, once: config.once, signal: stop.signal,
      claim: index => client.claim(workerIds[index], stop.signal),
      run: async claim => {
        const session = new JobSession(client, claim);
        const task = { session, provider: undefined, host: undefined };
        active.add(task);
        // Browser startup is covered by the lease heartbeat too.
        session.startHeartbeat();
        const closeOnAbort = () => { void task.provider?.close(); };
        session.signal.addEventListener('abort', closeOnAbort, { once: true });
        let result;
        let setupStage = 'BROWSER_START';
        try {
          const needsBrowser = !(claim.job.kind === 'session' && claim.job.phase === 'prepared');
          if (needsBrowser) {
            if (privateEdge) task.host = await launchPrivateEdge(chromium, session.signal);
            setupStage = 'PROVIDER_CREATE';
            task.provider = await createProvider(task.host?.browser ?? browser, session.signal,
              () => session.stop('BROWSER_CLOSED'), task.host
                ? { context: task.host.context, page: task.host.page, externalNavigation: true, manualLogin: false }
                : { manualLogin: false });
            setupStage = 'MARK_WINDOW';
            if (task.host) await markWindow(task.host.page, claim.job.account_id);
          }
          setupStage = 'RUN_JOB';
          console.log(`账号 #${claim.job.account_id}：${LABELS[claim.job.phase] ?? '正在恢复状态'}`);
          result = await (claim.job.kind === 'session' ? runSessionJob : runJob)({
            session, provider: task.provider,
            // Multiple terminal readers would compete for the same input.
            // Parallel jobs independently recheck their visible browser.
            onManual: async () => {
              if (config.concurrency === 1) return waitForManual(session.signal);
              await delay(1500, session.signal);
              return false;
            },
            onProgress: phase => console.log(`账号 #${claim.job.account_id}：${phase === 'authorization' ? '正在自动确认 OAuth 授权' : phase === 'exchange' ? '正在交换 OAuth 凭据' : phase === 'import' ? '正在导入或更新系统账号' : LABELS[phase] ?? '状态已更新'}`),
          });
        } catch (error) {
          const rawCode = session.stopReason || safeCode(error);
          const kind = /^[A-Za-z]+$/.test(error?.name ?? '') ? error.name.toUpperCase() : 'ERROR';
          const candidate = rawCode === 'WORKER_FAILED' ? `WORKER_FAILED_${setupStage}_${kind}` : rawCode;
          const code = /^[A-Z][A-Z0-9_]{2,63}$/.test(candidate) ? candidate : 'WORKER_FAILED';
          // Only a numeric account ID and symbolic code reach the server log.
          // Emit this before browser cleanup, which can take time after a fault.
          if (Number.isSafeInteger(claim.job.account_id) && claim.job.account_id > 0) {
            console.error(`账号 #${claim.job.account_id}：已暂停（${code}）`);
          }
          result = { status: 'paused', code };
          await session.pause(pauseCode(result.code, session.job.phase));
        } finally {
          session.signal.removeEventListener('abort', closeOnAbort);
          // Only this task's page/context and its own Edge process are closed.
          await task.provider?.close().catch(() => {});
          await task.host?.close().catch(() => {});
          session.close();
          active.delete(task);
        }
        const code = result.code ?? '';
        // Each browser closure pauses its own job. An uncertain provider write
        // stops new claims; other active accounts still reconcile normally.
        const canContinue = result.status === 'completed' || (result.status === 'paused'
          && ['login', 'prepared'].includes(session.job.phase)
          && !['WORKER_STOPPED', 'WORKER_UNAUTHORIZED'].includes(code));
        console.log(result.status === 'completed'
          ? `账号 #${claim.job.account_id}：${claim.job.kind === 'session' ? 'OAuth 授权已导入或更新系统账号' : '已确认新 2FA 生效、保存'}并关闭窗口。`
          : `账号 #${claim.job.account_id}：已暂停（${code}），请查看后台进度。`);
        if (!canContinue) console.log('停止领取新任务；已运行的账号继续核对结果。');
        if (code === 'WORKER_UNAUTHORIZED' || code === 'WORKER_STOPPED') interrupt();
        return canContinue;
      },
    });
  } finally {
    interrupt();
    await browser?.close().catch(() => {});
    client?.close();
    process.removeListener('SIGINT', interrupt);
    process.removeListener('SIGTERM', interrupt);
  }
}

main().catch(error => {
  const code = safeCode(error);
  if (code === 'WORKER_STOPPED') { console.log('助手已停止，未完成任务可在后台查看恢复状态。'); process.exitCode = 130; }
  else { console.error(`助手停止：${code}。请查看 README 中对应的准备和恢复说明。`); process.exitCode = 1; }
});
