<template>
  <div class="auth-shell">
    <aside class="auth-brand">
      <div class="brand-glow" aria-hidden="true"></div>
      <div class="brand-rule" aria-hidden="true"></div>

      <router-link to="/home" class="brand-mark" aria-label="返回 EasyHub 首页">
        <span class="brand-glyph" aria-hidden="true">EH</span>
        <span>
          <strong>EasyHub</strong>
          <small>AI API RELAY</small>
        </span>
      </router-link>

      <div class="brand-copy">
        <template v-if="variant === 'register'">
          <p class="status"><i></i> 开通新账户</p>
          <h1>开始接入。<br /><b>几分钟即可就绪。</b></h1>
          <p>创建 EasyHub 账户并获取 API Key，快速把模型能力接入你的产品与工作流。</p>
        </template>
        <template v-else>
          <p class="status"><i></i> Secure access</p>
          <h1>欢迎回来。<br /><b>继续你的工作。</b></h1>
          <p>统一管理模型调用、API Key 与团队用量，登录后继续进入 EasyHub 控制台。</p>
        </template>
      </div>

      <dl v-if="variant === 'register'" class="brand-index">
        <div><dt><span>01</span>创建账户</dt><dd>Email</dd></div>
        <div><dt><span>02</span>获取密钥</dt><dd>API Key</dd></div>
        <div><dt><span>03</span>开始接入</dt><dd>Ready</dd></div>
      </dl>
      <dl v-else class="brand-index">
        <div><dt><span>01</span>协议兼容</dt><dd>OpenAI</dd></div>
        <div><dt><span>02</span>统一接入</dt><dd>One API Key</dd></div>
        <div><dt><span>03</span>精确计量</dt><dd>Token billing</dd></div>
      </dl>
    </aside>

    <main class="auth-panel">
      <div class="panel-inner">
        <div class="mobile-brand">
          <span class="mobile-glyph" aria-hidden="true">EH</span>
          <span>EasyHub</span>
        </div>
        <slot />
        <div class="auth-footer"><slot name="footer" /></div>
        <div class="copyright">© {{ currentYear }} EasyHub. All rights reserved.</div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'

withDefaults(defineProps<{
  variant?: 'login' | 'register'
}>(), {
  variant: 'login'
})

const appStore = useAppStore()
const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-shell{--bg:#0b0b0c;--panel:#121214;--line:rgba(255,255,255,.09);--line-strong:rgba(255,255,255,.2);--text:#f2f0ec;--muted:#a5a29c;--faint:#6e6b66;--gold:#c8a96a;--live:#7fbf9a;min-height:100vh;display:grid;grid-template-columns:minmax(360px,42%) 1fr;background:var(--bg);color:var(--text);font-family:"Space Grotesk","PingFang SC",system-ui,sans-serif}.auth-brand{position:sticky;top:0;height:100vh;display:flex;flex-direction:column;justify-content:space-between;padding:clamp(28px,4vw,56px);border-right:1px solid var(--line);overflow:hidden}.brand-glow{position:absolute;inset:auto -30% -35% -20%;height:70%;background:radial-gradient(ellipse at 30% 100%,rgba(200,169,106,.14),transparent 62%);pointer-events:none}.brand-rule{position:absolute;inset:0 auto 0 24px;width:1px;background:linear-gradient(180deg,transparent,var(--line) 20%,var(--line) 80%,transparent)}.auth-brand>*:not(.brand-glow):not(.brand-rule){position:relative}.brand-mark{display:flex;align-items:center;gap:14px;width:max-content}.brand-logo{width:40px;height:40px;object-fit:contain;border:1px solid var(--line-strong)}.brand-mark strong{display:block;font-size:15px;letter-spacing:.02em}.brand-mark small{display:block;margin-top:2px;color:var(--faint);font-family:ui-monospace,monospace;font-size:10px;letter-spacing:.2em}.brand-copy{max-width:460px}.status{display:flex;align-items:center;gap:10px;color:var(--muted);font-family:ui-monospace,monospace;font-size:11px;letter-spacing:.16em;text-transform:uppercase}.status i{width:6px;height:6px;border-radius:50%;background:var(--live);box-shadow:0 0 0 4px rgba(127,191,154,.14)}.brand-copy h1{margin:34px 0;font-size:clamp(38px,4.3vw,58px);font-weight:500;line-height:1.02;letter-spacing:-.05em}.brand-copy h1 b{color:var(--gold);font-weight:500}.brand-copy>p:last-child{max-width:36ch;color:var(--muted);font-size:15px;line-height:1.8}.brand-index{margin:0}.brand-index div{display:flex;justify-content:space-between;gap:24px;padding:9px 0;border-top:1px solid var(--line);font-size:13px}.brand-index dt{color:var(--faint)}.brand-index dt span{margin-right:14px;color:var(--gold);font-family:ui-monospace,monospace;font-size:10px}.brand-index dd{color:var(--muted);font-family:ui-monospace,monospace;font-size:11px}.auth-panel{min-width:0;display:flex;align-items:center;justify-content:center;padding:clamp(40px,7vw,80px) clamp(24px,6vw,80px);background:var(--panel)}.panel-inner{width:min(100%,470px)}.mobile-brand{display:none}.auth-footer{margin-top:24px;text-align:center;font-size:13px}.copyright{margin-top:40px;padding-top:18px;border-top:1px solid var(--line);color:var(--faint);font-family:ui-monospace,monospace;font-size:10px;text-align:center}
@media(max-width:999px){.auth-shell{display:block}.auth-brand{position:relative;height:auto;min-height:300px;border-right:0;border-bottom:1px solid var(--line)}.brand-copy h1{font-size:42px}.brand-index{display:none}.auth-panel{min-height:calc(100vh - 300px)}.mobile-brand{display:none}}
@media(max-width:640px){.auth-brand{display:none}.auth-panel{min-height:100vh;padding:34px 24px}.mobile-brand{display:flex;align-items:center;gap:12px;margin-bottom:48px;font-size:16px;font-weight:600}.mobile-brand img{width:34px;height:34px;object-fit:contain;border:1px solid var(--line-strong)}}

.panel-inner { opacity: 0; transform: translateY(18px); animation: auth-rise .75s cubic-bezier(.16,1,.3,1) .12s forwards; }
.brand-mark { transition: transform .3s cubic-bezier(.16,1,.3,1), color .25s; }
.brand-mark:hover { transform: translateX(6px); color: var(--gold); }
.brand-copy { opacity: 0; transform: translateY(16px); animation: auth-rise .8s cubic-bezier(.16,1,.3,1) .04s forwards; }
.brand-index div { transition: padding .3s cubic-bezier(.16,1,.3,1), color .25s, border-color .25s; }
.brand-index div:hover { padding-left: 8px; padding-right: 8px; border-color: var(--line-strong); }
.status i { animation: auth-beat 2.6s ease-in-out infinite; }

@keyframes auth-rise { to { opacity: 1; transform: none; } }
@keyframes auth-beat { 0%,100% { opacity: 1; } 50% { opacity: .35; } }

@media(prefers-reduced-motion:reduce) {
  .panel-inner,
  .brand-copy { opacity: 1; transform: none; animation: none; }
  .status i { animation: none; }
}

/* 字号严格沿用参考稿的比例 */
.brand-glyph,
.mobile-glyph {
  display: grid;
  place-items: center;
  border: 1px solid var(--line-strong);
  color: var(--gold);
  font-family: "IBM Plex Mono", ui-monospace, monospace;
  font-weight: 500;
}
.brand-glyph { width: 40px; height: 40px; font-size: 12px; }
.mobile-glyph { width: 34px; height: 34px; font-size: 10px; }
.brand-mark small,
.status { font-family: "IBM Plex Mono", ui-monospace, monospace; }
.status { font-size: .68rem; }
.brand-copy { max-width: 420px; }
.brand-copy h1 { margin: 36px 0; font-size: clamp(2.1rem, 4vw, 3.2rem); letter-spacing: -.04em; }
.brand-copy > p:last-child { max-width: 34ch; font-size: .98rem; }

@media(max-width:999px) {
  .brand-copy h1 { font-size: 1.85rem; }
}

@media(min-width:1000px) and (min-height:1000px) {
  .auth-brand { justify-content: flex-start; }
  .auth-brand > .brand-mark.brand-mark {
    position: absolute;
    top: 42%;
    left: clamp(28px, 4vw, 56px);
  }
  .auth-brand > .brand-copy.brand-copy {
    position: absolute;
    top: 49%;
    right: clamp(28px, 4vw, 56px);
    left: clamp(28px, 4vw, 56px);
  }
  .auth-brand > .brand-index.brand-index {
    position: absolute;
    right: clamp(28px, 4vw, 56px);
    bottom: 5%;
    left: clamp(28px, 4vw, 56px);
  }
}
</style>
