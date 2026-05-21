<template>
  <div class="min-h-screen bg-gradient-to-br from-gray-50 via-primary-50/20 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950">
    <!-- ==================== Header ==================== -->
    <header class="border-b border-gray-200/50 bg-white/40 backdrop-blur-md dark:border-dark-800/50 dark:bg-dark-950/40">
      <div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <router-link to="/home" class="flex items-center gap-2.5">
          <div class="h-8 w-8 overflow-hidden rounded-lg">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-lg font-bold text-gray-900 dark:text-white">{{ siteName }}</span>
        </router-link>
        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <router-link
            to="/login"
            class="inline-flex items-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
          >
            {{ t('guide.login') }}
          </router-link>
        </div>
      </div>
    </header>

    <!-- ==================== Three-Column Layout ==================== -->
    <div class="mx-auto max-w-6xl px-6 py-8">
      <div class="flex gap-8">

        <!-- ========== Left Sidebar: Doc Navigation ========== -->
        <aside class="hidden w-56 shrink-0 lg:block">
          <nav class="sticky top-8 space-y-6">
            <!-- 站点介绍 -->
            <div>
              <p class="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400">{{ t('guide.nav.intro') }}</p>
              <ul class="space-y-1">
                <li><a href="#overview" :class="['block rounded-md px-3 py-1.5 text-sm transition-colors', activeSection === 'overview' ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-400' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white']">{{ t('guide.nav.overview') }}</a></li>
                <li><a href="#faq" :class="['block rounded-md px-3 py-1.5 text-sm transition-colors', activeSection === 'faq' ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-400' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white']">{{ t('guide.faq.title') }}</a></li>
              </ul>
            </div>
            <!-- 快速接入 -->
            <div>
              <p class="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400">{{ t('guide.nav.quickStart') }}</p>
              <ul class="space-y-1">
                <li v-for="tool in tools" :key="tool.key">
                  <a :href="'#' + tool.key" :class="['block rounded-md px-3 py-1.5 text-sm transition-colors', activeSection === tool.key ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-400' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white']">
                    <span v-if="tool.badge" class="mr-1.5 inline-block rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-bold leading-none text-primary-700 dark:bg-primary-900/30 dark:text-primary-400">{{ tool.badge }}</span>
                    {{ tool.title }}
                  </a>
                </li>
              </ul>
            </div>
            <!-- 账户与规则 -->
            <div>
              <p class="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400">{{ t('guide.nav.account') }}</p>
              <ul class="space-y-1">
                <li><a href="#errors" :class="['block rounded-md px-3 py-1.5 text-sm transition-colors', activeSection === 'errors' ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-400' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white']">{{ t('guide.nav.errors') }}</a></li>
              </ul>
            </div>
          </nav>
        </aside>

        <!-- ========== Center: Main Content ========== -->
        <main class="min-w-0 flex-1">
          <article class="mx-auto max-w-2xl">

            <!-- ===== Overview ===== -->
            <section id="overview" class="mb-12">
              <span class="mb-2 inline-block rounded bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-400">{{ t('guide.badge') }}</span>
              <h1 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">{{ t('guide.title') }}</h1>
              <p class="mb-6 text-gray-500 dark:text-dark-300">{{ t('guide.subtitle') }}</p>

              <!-- Quick Config Panel -->
              <div class="config-panel rounded-xl p-5">
                <dl class="space-y-3">
                  <div class="flex items-center justify-between">
                    <dt class="text-xs uppercase tracking-wider text-gray-400">Base URL (OpenAI)</dt>
                    <dd class="flex items-center gap-2">
                      <code class="font-mono text-sm text-gray-200">https://api.mcorgai.com/v1</code>
                      <button @click="copy('https://api.mcorgai.com/v1')" class="copy-btn text-xs text-primary-400 hover:text-primary-300">{{ copiedField === 'base-url-oai' ? '✓' : t('guide.copy') }}</button>
                    </dd>
                  </div>
                  <div class="flex items-center justify-between">
                    <dt class="text-xs uppercase tracking-wider text-gray-400">Base URL (Anthropic)</dt>
                    <dd class="flex items-center gap-2">
                      <code class="font-mono text-sm text-gray-200">https://api.mcorgai.com</code>
                      <button @click="copy('https://api.mcorgai.com')" class="copy-btn text-xs text-primary-400 hover:text-primary-300">{{ copiedField === 'base-url-ant' ? '✓' : t('guide.copy') }}</button>
                    </dd>
                  </div>
                  <div class="flex items-center justify-between">
                    <dt class="text-xs uppercase tracking-wider text-gray-400">API Key</dt>
                    <dd>
                      <a href="https://api.mcorgai.com/keys" target="_blank" class="text-xs text-primary-400 hover:text-primary-300">{{ t('guide.createKey') }}</a>
                    </dd>
                  </div>
                </dl>
              </div>
            </section>

            <!-- ===== Codex CLI ===== -->
            <section id="codex" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.codex.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.codex.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step1') }}</h3>
              <div class="code-block mb-6">
                <button @click="copy(codexConfig)" class="code-copy-btn">{{ copiedField === 'codex-env' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ codexConfig }}</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step2') }}</h3>
              <div class="code-block mb-6">
                <button @click="copy(codexVerify)" class="code-copy-btn">{{ copiedField === 'codex-verify' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ codexVerify }}</code></pre>
              </div>

              <!-- Troubleshooting Table -->
              <h3 class="mb-3 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.troubleshoot') }}</h3>
              <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                <table class="w-full text-sm">
                  <thead class="bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">Error</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.solution') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">INSUFFICIENT_BALANCE</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.codex.err1') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">401 Unauthorized</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.codex.err2') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">429 Rate Limited</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.codex.err3') }}</td></tr>
                  </tbody>
                </table>
              </div>
            </section>

            <!-- ===== Claude Code ===== -->
            <section id="claude" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.claude.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.claude.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step1') }}</h3>
              <div class="code-block mb-6">
                <button @click="copy(claudeConfig)" class="code-copy-btn">{{ copiedField === 'claude-env' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ claudeConfig }}</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step2') }}</h3>
              <div class="code-block mb-6">
                <button @click="copy(claudeVerify)" class="code-copy-btn">{{ copiedField === 'claude-verify' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ claudeVerify }}</code></pre>
              </div>

              <h3 class="mb-3 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.troubleshoot') }}</h3>
              <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                <table class="w-full text-sm">
                  <thead class="bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">Error</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.solution') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">session_id required</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.claude.err1') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">model not found</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.claude.err2') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">Base URL /v1</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.claude.err3') }}</td></tr>
                  </tbody>
                </table>
              </div>
            </section>

            <!-- ===== Cursor ===== -->
            <section id="cursor" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.cursor.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.cursor.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step1') }}</h3>
              <div class="mb-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
                <ol class="list-decimal list-inside space-y-2 text-sm text-gray-700 dark:text-dark-300">
                  <li>{{ t('guide.cursor.setting1') }}</li>
                  <li>{{ t('guide.cursor.setting2') }}</li>
                  <li>{{ t('guide.cursor.setting3') }}</li>
                </ol>
              </div>
              <div class="code-block mb-6">
                <button @click="copy('https://api.mcorgai.com/v1')" class="code-copy-btn">{{ copiedField === 'cursor-url' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>https://api.mcorgai.com/v1</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step2') }}</h3>
              <p class="text-sm text-gray-500 dark:text-dark-300">{{ t('guide.cursor.test') }}</p>
            </section>

            <!-- ===== OpenAI SDK ===== -->
            <section id="sdk" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.sdk.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.sdk.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">Python</h3>
              <div class="code-block mb-6">
                <button @click="copy(pythonSdk)" class="code-copy-btn">{{ copiedField === 'python-sdk' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ pythonSdk }}</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">Node.js</h3>
              <div class="code-block mb-6">
                <button @click="copy(nodeSdk)" class="code-copy-btn">{{ copiedField === 'node-sdk' ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ nodeSdk }}</code></pre>
              </div>
            </section>

            <!-- ===== Hermes Agent ===== -->
            <section id="hermes" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.hermes.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.hermes.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step1') }}</h3>
              <p class="mb-3 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.hermes.step1desc') }}</p>
              <div class="code-block mb-6">
                <button @click="copy(hermesConfig)" class="code-copy-btn">{{ copiedField === hermesConfig ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ hermesConfig }}</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step2') }}</h3>
              <p class="mb-3 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.hermes.step2desc') }}</p>
              <div class="code-block mb-6">
                <button @click="copy(hermesModel)" class="code-copy-btn">{{ copiedField === hermesModel ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ hermesModel }}</code></pre>
              </div>

              <h3 class="mb-3 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.troubleshoot') }}</h3>
              <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                <table class="w-full text-sm">
                  <thead class="bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">Error</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.solution') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">provider not found</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.hermes.err1') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">model not found</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.hermes.err2') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">connection refused</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.hermes.err3') }}</td></tr>
                  </tbody>
                </table>
              </div>
            </section>

            <!-- ===== OpenClaw ===== -->
            <section id="openclaw" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.openclaw.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.openclaw.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step1') }}</h3>
              <p class="mb-3 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.openclaw.step1desc') }}</p>
              <div class="code-block mb-6">
                <button @click="copy(openclawConfig)" class="code-copy-btn">{{ copiedField === openclawConfig ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ openclawConfig }}</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step2') }}</h3>
              <p class="mb-3 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.openclaw.step2desc') }}</p>
              <div class="code-block mb-6">
                <button @click="copy(openclawModel)" class="code-copy-btn">{{ copiedField === openclawModel ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ openclawModel }}</code></pre>
              </div>

              <h3 class="mb-3 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.troubleshoot') }}</h3>
              <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                <table class="w-full text-sm">
                  <thead class="bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">Error</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.solution') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">provider not found</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.openclaw.err1') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">invalid config</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.openclaw.err2') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">model not listed</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.openclaw.err3') }}</td></tr>
                  </tbody>
                </table>
              </div>
            </section>

            <!-- ===== OpenCode ===== -->
            <section id="opencode" class="mb-12">
              <h2 class="mb-2 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.opencode.title') }}</h2>
              <p class="mb-6 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.opencode.intro') }}</p>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step1') }}</h3>
              <p class="mb-3 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.opencode.step1desc') }}</p>
              <div class="code-block mb-6">
                <button @click="copy(opencodeConfig)" class="code-copy-btn">{{ copiedField === opencodeConfig ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ opencodeConfig }}</code></pre>
              </div>

              <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.step2') }}</h3>
              <p class="mb-3 text-sm text-gray-500 dark:text-dark-300">{{ t('guide.opencode.step2desc') }}</p>
              <div class="code-block mb-6">
                <button @click="copy(opencodeModel)" class="code-copy-btn">{{ copiedField === opencodeModel ? '✓' : t('guide.copy') }}</button>
                <pre class="text-sm leading-relaxed text-gray-300"><code>{{ opencodeModel }}</code></pre>
              </div>
              <p class="mb-6 text-xs text-gray-400 dark:text-dark-400">配置文件路径：<code class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-dark-800">~/.config/opencode/opencode.jsonc</code>（或 opencode.json）</p>

              <h3 class="mb-3 text-sm font-semibold text-gray-800 dark:text-dark-200">{{ t('guide.troubleshoot') }}</h3>
              <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                <table class="w-full text-sm">
                  <thead class="bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">Error</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.solution') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">provider not found</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.opencode.err1') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">invalid config</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.opencode.err2') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs text-red-500">model not listed</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.opencode.err3') }}</td></tr>
                  </tbody>
                </table>
              </div>
            </section>

            <!-- ===== FAQ ===== -->
            <section id="faq" class="mb-12">
              <h2 class="mb-6 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.faq.title') }}</h2>
              <div class="space-y-3">
                <details
                  v-for="(item, idx) in faqItems"
                  :key="idx"
                  class="group rounded-xl border border-gray-200/50 bg-white/60 backdrop-blur-sm transition-all hover:shadow-md dark:border-dark-700/50 dark:bg-dark-800/60 [&[open]]:border-primary-200"
                >
                  <summary class="flex cursor-pointer select-none items-center justify-between px-5 py-4 text-sm font-medium text-gray-800 dark:text-dark-200">
                    {{ item.question }}
                    <svg class="h-4 w-4 shrink-0 text-gray-400 transition-transform group-open:rotate-180" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                    </svg>
                  </summary>
                  <div class="border-t border-gray-100 px-5 py-4 text-sm leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-300">
                    {{ item.answer }}
                  </div>
                </details>
              </div>
            </section>

            <!-- ===== Error Codes ===== -->
            <section id="errors" class="mb-12">
              <h2 class="mb-4 text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.nav.errors') }}</h2>
              <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                <table class="w-full text-sm">
                  <thead class="bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">Status</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.errorMeaning') }}</th>
                      <th class="px-4 py-2 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('guide.solution') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr><td class="px-4 py-2 font-mono text-xs">401</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err401') }}</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err401sol') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs">403</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err403') }}</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err403sol') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs">429</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err429') }}</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err429sol') }}</td></tr>
                    <tr><td class="px-4 py-2 font-mono text-xs">502</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err502') }}</td><td class="px-4 py-2 text-gray-600 dark:text-dark-300">{{ t('guide.err502sol') }}</td></tr>
                  </tbody>
                </table>
              </div>
            </section>

          </article>
        </main>

        <!-- ========== Right Sidebar: TOC ========== -->
        <aside class="hidden w-44 shrink-0 xl:block">
          <nav class="sticky top-8">
            <p class="mb-3 text-xs font-semibold uppercase tracking-wider text-gray-400">{{ t('guide.toc') }}</p>
            <ul class="space-y-2">
              <li v-for="section in tocItems" :key="section.id">
                <a :href="'#' + section.id" :class="['block text-sm transition-colors', activeSection === section.id ? 'font-medium text-primary-600 dark:text-primary-400' : 'text-gray-400 hover:text-gray-600 dark:text-dark-300 dark:hover:text-dark-300']">{{ section.label }}</a>
              </li>
            </ul>
          </nav>
        </aside>

      </div>
    </div>

    <!-- ==================== Footer ==================== -->
    <footer class="border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div class="mx-auto max-w-6xl text-center text-sm text-gray-400 dark:text-dark-300">
        &copy; {{ currentYear }} {{ siteName }} · <router-link to="/home" class="hover:text-gray-600 dark:hover:text-white">{{ t('guide.backToHome') }}</router-link>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const { t } = useI18n()
const appStore = useAppStore()
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '起源AI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const currentYear = computed(() => new Date().getFullYear())

// Active section tracking
const activeSection = ref('overview')
const tocItems = computed(() => [
  { id: 'overview', label: t('guide.nav.overview') },
  { id: 'codex', label: t('guide.codex.title') },
  { id: 'claude', label: t('guide.claude.title') },
  { id: 'cursor', label: t('guide.cursor.title') },
  { id: 'sdk', label: t('guide.sdk.title') },
  { id: 'hermes', label: t('guide.hermes.title') },
  { id: 'openclaw', label: t('guide.openclaw.title') },
  { id: 'opencode', label: t('guide.opencode.title') },
  { id: 'faq', label: t('guide.faq.title') },
  { id: 'errors', label: t('guide.nav.errors') },
])

const tools = computed(() => [
  { key: 'codex', title: t('guide.codex.title') },
  { key: 'claude', title: t('guide.claude.title') },
  { key: 'cursor', title: t('guide.cursor.title') },
  { key: 'sdk', title: t('guide.sdk.title') },
  { key: 'hermes', title: t('guide.hermes.title'), badge: 'Agent' },
  { key: 'openclaw', title: t('guide.openclaw.title'), badge: 'Agent' },
  { key: 'opencode', title: t('guide.opencode.title') },
])

// Copy
const copiedField = ref('')
async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedField.value = text
    setTimeout(() => { copiedField.value = '' }, 1500)
  } catch { /* fallback */ }
}

// Code content
const codexConfig = `export OPENAI_BASE_URL="https://api.mcorgai.com/v1"
export OPENAI_API_KEY="sk-your-key"
export OPENAI_MODEL="gpt-5.5"

codex --model gpt-5.5`

const codexVerify = `curl https://api.mcorgai.com/v1/models \\
  -H "Authorization: Bearer sk-your-key"`

const claudeConfig = `export ANTHROPIC_BASE_URL="https://api.mcorgai.com"
export ANTHROPIC_API_KEY="sk-your-key"

claude`

const claudeVerify = `curl https://api.mcorgai.com/v1/messages \\
  -H "x-api-key: sk-your-key" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "content-type: application/json" \\
  -d '{"model":"claude-sonnet-4-6","max_tokens":10,"messages":[{"role":"user","content":"Hi"}]}'`

const pythonSdk = `from openai import OpenAI

client = OpenAI(
    api_key="sk-your-key",
    base_url="https://api.mcorgai.com/v1"
)

response = client.chat.completions.create(
    model="gpt-5.4",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)`

const nodeSdk = `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-your-key",
  baseURL: "https://api.mcorgai.com/v1",
});

const response = await client.chat.completions.create({
  model: "gpt-5.4",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(response.choices[0].message.content);`

const hermesConfig = `model:
  provider: mcorgai
  base_url: https://api.mcorgai.com/v1`

const hermesModel = `hermes model --provider mcorgai --model gpt-5.4`

const openclawConfig = `{
  "models": {
    "providers": {
      "mcorgai": {
        "baseUrl": "https://api.mcorgai.com/v1",
        "apiKey": "sk-your-key",
        "api": "openai-completions",
        "models": [
          {
            "id": "gpt-5.4",
            "name": "GPT-5.4 (起源AI)",
            "contextWindow": 131072,
            "maxTokens": 16384
          },
          {
            "id": "gpt-5.4-mini",
            "name": "GPT-5.4 Mini (起源AI)",
            "contextWindow": 131072,
            "maxTokens": 16384
          }
        ]
      }
    }
  }
}`

const openclawModel = `openclaw model --provider mcorgai --model gpt-5.4`

const opencodeConfig = `{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "mcorgai": {
      "name": "起源AI",
      "options": {
        "baseURL": "https://api.mcorgai.com/v1"
      },
      "models": {
        "gpt-5.5": {
          "name": "GPT-5.5"
        },
        "gpt-5.4": {
          "name": "GPT-5.4"
        },
        "gpt-5.4-mini": {
          "name": "GPT-5.4 Mini"
        }
      }
    }
  }
}`

const opencodeModel = `opencode auth login
# 1. Select provider → Other
# 2. Enter provider id → mcorgai
# 3. Enter your API key → sk-xxx`

const faqItems = computed(() => [
  { question: t('guide.faq.q1'), answer: t('guide.faq.a1') },
  { question: t('guide.faq.q2'), answer: t('guide.faq.a2') },
  { question: t('guide.faq.q3'), answer: t('guide.faq.a3') },
  { question: t('guide.faq.q4'), answer: t('guide.faq.a4') },
  { question: t('guide.faq.q5'), answer: t('guide.faq.a5') },
  { question: t('guide.faq.q6'), answer: t('guide.faq.a6') }
])

// Intersection observer for active section
let observer: IntersectionObserver | null = null
onMounted(() => {
  const sections = document.querySelectorAll('section[id]')
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          activeSection.value = entry.target.id
        }
      }
    },
    { rootMargin: '-20% 0px -70% 0px' }
  )
  sections.forEach((s) => observer?.observe(s))
})
onUnmounted(() => { observer?.disconnect() })
</script>

<style scoped>
.config-panel {
  background: linear-gradient(145deg, #1a1a2e 0%, #16213e 100%);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 10px 30px -5px rgba(0, 0, 0, 0.3);
}

.code-block {
  position: relative;
}

.code-block pre {
  overflow-x: auto;
  border-radius: 0.5rem;
  background: #1a1a1a;
  padding: 1rem 1.25rem;
}

.code-copy-btn {
  position: absolute;
  right: 0.75rem;
  top: 0.75rem;
  display: flex;
  align-items: center;
  gap: 0.375rem;
  border-radius: 0.375rem;
  background: rgba(255, 255, 255, 0.08);
  padding: 0.375rem 0.625rem;
  font-size: 0.75rem;
  font-weight: 500;
  color: #9ca3af;
  opacity: 0;
  backdrop-filter: blur(0.25rem);
  transition: all 0.2s;
}

.code-block:hover .code-copy-btn {
  opacity: 1;
}

.copy-btn {
  cursor: pointer;
  background: none;
  border: none;
}
</style>
