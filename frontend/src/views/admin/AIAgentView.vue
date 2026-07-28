<template>
  <AppLayout>
    <div class="flex min-h-[calc(100vh-4rem)] flex-col rounded-lg border border-gray-200/80 bg-gray-50 shadow-sm dark:border-dark-700/80 dark:bg-dark-950 lg:h-[calc(100dvh-8rem)] lg:min-h-[560px] lg:overflow-hidden">
      <header class="rounded-t-lg border-b border-gray-100 bg-white/95 px-4 py-3 backdrop-blur-sm dark:border-dark-800 dark:bg-dark-900/95 sm:px-6 lg:rounded-none">
        <div class="mx-auto flex max-w-[1500px] flex-wrap items-center justify-between gap-3">
          <div class="flex min-w-0 items-center gap-3">
            <div :class="['agent-mark flex h-9 w-9 flex-none items-center justify-center rounded-lg bg-emerald-600 text-white shadow-sm', { 'agent-mark--active': busy }]">
              <Icon name="sparkles" size="md" />
            </div>
            <div class="min-w-0">
              <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.aiAgent.title') }}</h1>
              <p class="truncate text-xs text-gray-500 dark:text-dark-400">
                {{ config?.model || t('admin.aiAgent.noModel') }} · {{ config?.catalog_size || 0 }} {{ t('admin.aiAgent.tools') }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span v-if="config && !config.enabled" class="badge badge-warning">{{ t('admin.aiAgent.disabled') }}</span>
            <span v-if="config?.enabled && config.response_cache" class="badge badge-success">
              {{ t('admin.aiAgent.responseCache') }}
            </span>
            <span v-if="config?.enabled" :class="['badge', config.streaming ? 'badge-success' : 'badge-warning']">
              {{ config?.streaming ? t('admin.aiAgent.streamingMode') : t('admin.aiAgent.nonStreamingMode') }}
            </span>
            <span v-if="config?.enabled" :class="['badge', config.auto_approve ? 'badge-warning' : 'badge-success']">
              {{ config?.auto_approve ? t('admin.aiAgent.autoMode') : t('admin.aiAgent.supervisedMode') }}
            </span>
            <button v-if="config?.enabled" class="btn btn-secondary px-3" :title="t('admin.aiAgent.newConversation')" @click="createConversation">
              <Icon name="plus" size="sm" />
            </button>
            <button v-if="config?.enabled" class="btn btn-secondary px-3" :title="t('admin.aiAgent.history')" @click="showHistory = true">
              <Icon name="clock" size="sm" />
            </button>
            <button v-if="config?.enabled" class="btn btn-secondary px-3" :title="t('admin.aiAgent.deleteConversation')" :disabled="!conversationId" @click="deleteCurrentConversation">
              <Icon name="trash" size="sm" />
            </button>
            <button class="btn btn-secondary px-3" :title="t('admin.aiAgent.settings')" @click="showSettings = true">
              <Icon name="cog" size="sm" />
            </button>
          </div>
        </div>
      </header>

      <div v-if="config && !config.enabled" class="mx-auto flex w-full max-w-[1500px] flex-1 flex-col items-center justify-center px-6 py-16 text-center">
        <div class="flex h-14 w-14 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-500 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300">
          <Icon name="play" size="xl" />
        </div>
        <h2 class="mt-5 text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.aiAgent.disabledTitle') }}</h2>
        <p class="mt-2 max-w-md text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.disabledDescription') }}</p>
        <div class="mt-6 flex flex-wrap justify-center gap-2">
          <button class="btn btn-primary" :disabled="settingsSaving" @click="enableAgent"><Icon name="play" size="sm" />{{ t('admin.aiAgent.enable') }}</button>
          <button class="btn btn-secondary" @click="showSettings = true"><Icon name="cog" size="sm" />{{ t('admin.aiAgent.settings') }}</button>
        </div>
      </div>

      <div v-else-if="config?.enabled" class="mx-auto grid w-full max-w-[1500px] flex-1 grid-cols-1 lg:min-h-0 lg:grid-cols-[minmax(0,1fr)_320px] lg:overflow-hidden">
        <main class="flex min-h-[620px] min-w-0 flex-col border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900 lg:min-h-0 lg:border-r">
          <div ref="messagePane" class="min-h-0 flex-1 space-y-6 overflow-y-auto overscroll-contain px-4 py-6 pb-32 sm:px-8 lg:pb-6">
            <div v-if="!messages.length" class="mx-auto flex max-w-xl flex-col items-center justify-center py-20 text-center">
              <div class="mb-5 flex h-14 w-14 items-center justify-center rounded-lg border border-emerald-200/80 bg-emerald-50 text-emerald-700 shadow-sm dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300">
                <Icon name="brain" size="xl" />
              </div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.aiAgent.emptyTitle') }}</h2>
              <p class="mt-2 max-w-md text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.emptyDescription') }}</p>
              <div class="mt-6 flex flex-wrap justify-center gap-2">
                <button v-for="suggestion in suggestions" :key="suggestion" class="rounded-lg border border-gray-200/80 bg-white px-3 py-2 text-sm text-gray-700 shadow-sm transition-all hover:-translate-y-0.5 hover:border-emerald-300 hover:text-emerald-700 hover:shadow dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300" @click="prompt = suggestion">
                  {{ suggestion }}
                </button>
              </div>
            </div>

            <template v-for="item in messageFlow" :key="item.key">
              <article v-if="item.message" :class="['agent-message flex gap-3', item.message.role === 'user' ? 'justify-end' : 'justify-start']">
                <div v-if="item.message.role === 'assistant'" class="mt-1 flex h-7 w-7 flex-none items-center justify-center rounded-lg bg-emerald-600 text-white shadow-sm">
                  <Icon name="sparkles" size="sm" />
                </div>
                <div :class="['max-w-[min(760px,85%)] break-words rounded-lg px-4 py-3 text-sm leading-6', item.message.role === 'user' ? 'whitespace-pre-wrap bg-gray-900 text-white shadow-sm dark:bg-gray-100 dark:text-gray-900' : 'border border-gray-100 bg-gray-50/80 text-gray-800 dark:border-dark-700/70 dark:bg-dark-800/70 dark:text-dark-200']">
                  <template v-if="item.message.role === 'user'">{{ messageContent(item.message) }}</template>
                  <div v-else class="agent-markdown" v-html="renderAgentMarkdown(messageContent(item.message))"></div>
                  <span v-if="item.message.streaming" class="agent-stream-cursor" aria-hidden="true"></span>
                </div>
              </article>

              <details v-else :open="item.open" class="rounded-lg border border-gray-200/80 bg-gray-50/70 px-4 py-3 text-sm dark:border-dark-700 dark:bg-dark-800/60">
                <summary class="cursor-pointer font-medium text-gray-700 dark:text-dark-200">{{ t('admin.aiAgent.processTitle') }}</summary>
                <ol class="mt-3 space-y-3 border-l border-gray-200 pl-5 dark:border-dark-600">
                  <li v-for="event in item.processEvents" :key="event.id" class="relative min-w-0 text-xs text-gray-600 dark:text-dark-300">
                    <span :class="['absolute -left-[31px] top-0 flex h-5 w-5 items-center justify-center rounded-md ring-2 ring-gray-50 dark:ring-dark-800', processEventTone(event)]">
                      <Icon :name="processEventIcon(event)" size="xs" />
                    </span>
                    <div class="flex flex-wrap items-center justify-between gap-2">
                      <p class="font-medium text-gray-700 dark:text-dark-200">{{ processEventTitle(event) }}</p>
                      <time class="text-[11px] text-gray-400">{{ formatEventTime(event.created_at) }}</time>
                    </div>
                    <div v-if="processEventMetadata(event).length" class="mt-1.5 flex flex-wrap gap-1.5">
                      <span v-for="metadata in processEventMetadata(event)" :key="metadata" class="rounded-md bg-gray-200/70 px-2 py-0.5 font-mono text-[10px] text-gray-600 dark:bg-dark-700 dark:text-dark-300">{{ metadata }}</span>
                    </div>
                    <pre v-if="config?.process_display === 'full' && event.detail" class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-gray-900 p-2 text-[11px] leading-5 text-gray-100">{{ formatEventDetail(event.detail) }}</pre>
                  </li>
                </ol>
              </details>
            </template>

            <div v-if="busy && !hasStreamingMessage" class="agent-message flex items-center gap-3 text-sm text-gray-500 dark:text-dark-400" role="status" aria-live="polite">
              <div class="agent-thinking-avatar flex h-7 w-7 flex-none items-center justify-center rounded-lg bg-emerald-600 shadow-sm" aria-hidden="true">
                <span class="agent-thinking-bars"><span></span><span></span><span></span></span>
              </div>
              <span class="inline-flex items-center">
                {{ t('admin.aiAgent.working') }}
                <span class="agent-thinking-dots" aria-hidden="true"><span></span><span></span><span></span></span>
              </span>
            </div>
          </div>

          <section v-if="pending && confirmationReady" class="fixed inset-x-4 bottom-[104px] z-30 max-h-[48dvh] overflow-y-auto rounded-lg border border-amber-300 bg-amber-50 p-4 shadow-xl dark:border-amber-800 dark:bg-amber-950 lg:hidden">
            <p class="text-xs font-medium text-amber-700 dark:text-amber-300">{{ t('admin.aiAgent.pendingTitle') }}</p>
            <h2 class="mt-1 text-sm font-semibold text-amber-950 dark:text-amber-100">{{ pendingOperationLabel(pending) }}</h2>
            <p v-if="pending.target_label" class="mt-1 break-words text-xs text-amber-800 dark:text-amber-300">{{ t('admin.aiAgent.target') }}：{{ pending.target_label }}</p>
            <p v-if="pending.plan" class="mt-2 text-xs text-amber-800 dark:text-amber-300">{{ t('admin.aiAgent.planSummary', { count: pending.plan.nodes.length, policy: planFailurePolicyLabel(pending.plan.failure_policy) }) }}</p>
            <p v-if="pending.sensitive" class="mt-2 flex items-center gap-1.5 text-xs font-medium text-amber-800 dark:text-amber-200"><Icon name="shield" size="xs" />{{ t('admin.aiAgent.sensitiveConfirmation') }}</p>
            <button v-if="pending.plan" class="mt-3 w-full rounded-lg border border-amber-300 bg-white px-3 py-2 text-sm font-medium text-amber-900 shadow-sm dark:border-amber-800 dark:bg-dark-900 dark:text-amber-200" @click="showPlanConfirmation = true">{{ t('admin.aiAgent.reviewPlan') }}</button>
            <dl v-if="!pending.plan && (pending.preview?.length || pending.changes?.length)" class="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-amber-200 pt-3 text-xs dark:border-amber-800">
              <div v-for="change in (pending.preview || pending.changes)" :key="change.field" class="min-w-0">
                <dt class="font-medium text-amber-900 dark:text-amber-200">{{ pendingFieldLabel(change.field) }}</dt>
                <dd class="truncate text-amber-800 dark:text-amber-300"><template v-if="change.before !== null && change.before !== undefined">{{ formatValue(change.before) }} → </template>{{ formatValue(change.after) }}</dd>
              </div>
            </dl>
            <div class="mt-4 grid grid-cols-2 gap-2">
              <button class="btn btn-secondary justify-center" :disabled="actionBusy || busy" @click="cancelPending"><Icon name="x" size="sm" />{{ pending.plan ? t('admin.aiAgent.cancelPlan') : t('common.cancel') }}</button>
              <button class="btn justify-center bg-amber-600 text-white hover:bg-amber-700" :disabled="actionBusy" @click="pending.plan ? showPlanConfirmation = true : confirmPending()"><Icon name="check" size="sm" />{{ pending.plan ? t('admin.aiAgent.reviewPlan') : t('admin.aiAgent.confirm') }}</button>
            </div>
          </section>

          <div class="fixed inset-x-0 bottom-0 z-20 flex-none border-t border-gray-100 bg-white/95 p-4 backdrop-blur-sm dark:border-dark-800 dark:bg-dark-900/95 sm:px-8 lg:sticky lg:inset-x-auto">
            <form class="mx-auto max-w-4xl" @submit.prevent="sendMessage">
              <div class="flex items-end gap-2 rounded-lg border border-gray-200 bg-white p-2 shadow-sm transition-shadow focus-within:border-emerald-400 focus-within:shadow-md focus-within:ring-2 focus-within:ring-emerald-500/10 dark:border-dark-600 dark:bg-dark-800">
                <textarea v-model="prompt" rows="2" :placeholder="t('admin.aiAgent.placeholder')" :disabled="busy || !!pending" class="max-h-40 min-h-[48px] flex-1 resize-none border-0 bg-transparent px-2 py-2 text-sm text-gray-900 outline-none dark:text-white" @compositionstart="composerCompositionActive = true" @compositionend="composerCompositionActive = false" @keydown="handleComposerKeydown"></textarea>
                <button type="button" :class="['flex h-9 w-9 flex-none items-center justify-center rounded-lg text-white shadow-sm transition-all hover:-translate-y-0.5 hover:shadow disabled:cursor-not-allowed disabled:translate-y-0 disabled:opacity-40', busy ? 'bg-red-600 hover:bg-red-700' : 'bg-emerald-600 hover:bg-emerald-700']" :disabled="busy ? conversationStatus === 'stopping' : (!!pending || !prompt.trim())" :title="busy ? t('admin.aiAgent.stop') : t('admin.aiAgent.send')" @click="busy ? stopGeneration() : sendMessage()">
                  <Icon :name="busy ? 'stop' : 'arrowUp'" size="sm" />
                </button>
              </div>
            </form>
          </div>
        </main>

        <aside class="bg-gray-50 p-4 pb-32 dark:bg-dark-950 sm:p-5 sm:pb-32 lg:min-h-0 lg:overflow-y-auto lg:overscroll-contain lg:pb-5">
          <section v-if="pending && confirmationReady" class="rounded-lg border border-amber-200 bg-amber-50 p-4 shadow-sm dark:border-amber-900 dark:bg-amber-950/30">
            <div class="flex items-start gap-2">
              <Icon name="exclamationTriangle" size="md" class="mt-0.5 text-amber-600" />
              <div class="min-w-0 flex-1">
                <p class="text-xs font-medium text-amber-700 dark:text-amber-300">{{ t('admin.aiAgent.pendingTitle') }}</p>
                <h2 class="mt-1 text-sm font-semibold text-amber-950 dark:text-amber-100">{{ pendingOperationLabel(pending) }}</h2>
                <p v-if="pending.target_label" class="mt-1 break-words text-xs text-amber-800 dark:text-amber-300">{{ t('admin.aiAgent.target') }}：{{ pending.target_label }}</p>
                <p v-if="pending.plan" class="mt-2 text-xs text-amber-800 dark:text-amber-300">{{ t('admin.aiAgent.planSummary', { count: pending.plan.nodes.length, policy: planFailurePolicyLabel(pending.plan.failure_policy) }) }}</p>
                <details v-if="!pending.plan" class="mt-2 text-xs text-amber-700 dark:text-amber-400">
                  <summary class="cursor-pointer select-none">{{ t('admin.aiAgent.technicalDetails') }}</summary>
                  <p class="mt-1 break-all font-mono">{{ pending.method }} {{ pending.path }}</p>
                </details>
              </div>
            </div>
            <div v-if="pending.sensitive" class="mt-3 rounded-lg border border-amber-200 bg-white/70 p-3 text-xs text-amber-900 dark:border-amber-900 dark:bg-dark-900/50 dark:text-amber-200">
              <p class="flex items-center gap-2 font-medium"><Icon name="shield" size="sm" />{{ t('admin.aiAgent.sensitiveConfirmation') }}</p>
              <p class="mt-1 text-amber-700 dark:text-amber-300">{{ t('admin.aiAgent.sensitiveConfirmationHint') }}</p>
              <p v-if="pending.sensitive_fields?.length" class="mt-2 break-words">{{ pending.sensitive_fields.map(pendingFieldLabel).join('、') }}</p>
            </div>
            <button v-if="pending.plan" class="mt-4 w-full rounded-lg border border-amber-300 bg-white px-3 py-2 text-sm font-medium text-amber-900 shadow-sm transition-colors hover:bg-amber-100 dark:border-amber-800 dark:bg-dark-900 dark:text-amber-200 dark:hover:bg-amber-950" @click="showPlanConfirmation = true">{{ t('admin.aiAgent.reviewPlan') }}</button>
            <dl v-if="!pending.plan && (pending.preview?.length || pending.changes?.length)" class="mt-4 space-y-2 border-t border-amber-200 pt-3 text-xs dark:border-amber-900">
              <div v-for="change in (pending.preview || pending.changes)" :key="change.field">
                <dt class="font-medium text-amber-900 dark:text-amber-200">{{ pendingFieldLabel(change.field) }}</dt>
                <dd class="mt-0.5 break-words text-amber-800 dark:text-amber-300">
                  <template v-if="change.before !== null && change.before !== undefined">{{ formatValue(change.before) }} → </template>{{ formatValue(change.after) }}
                </dd>
              </div>
            </dl>
            <div class="mt-4 grid grid-cols-2 gap-2">
              <button class="btn btn-secondary justify-center" :disabled="actionBusy || busy" @click="cancelPending"><Icon name="x" size="sm" />{{ pending.plan ? t('admin.aiAgent.cancelPlan') : t('common.cancel') }}</button>
              <button class="btn justify-center bg-amber-600 text-white hover:bg-amber-700" :disabled="actionBusy" @click="pending.plan ? showPlanConfirmation = true : confirmPending()"><Icon name="check" size="sm" />{{ pending.plan ? t('admin.aiAgent.reviewPlan') : t('admin.aiAgent.confirm') }}</button>
            </div>
          </section>

          <section :class="pending ? 'mt-6' : ''">
            <div class="flex items-center justify-between">
              <h2 class="text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackTitle') }}</h2>
              <span class="text-xs text-gray-400">{{ rollbacks.length }}/20</span>
            </div>
            <div v-if="rollbacks.length" class="mt-3 space-y-2">
              <button v-for="rollback in rollbacks" :key="rollback.id" type="button" class="block w-full rounded-lg border border-gray-200/80 bg-white p-3 text-left shadow-sm transition-colors hover:border-gray-300 hover:bg-gray-50 disabled:cursor-default dark:border-dark-700 dark:bg-dark-900 dark:hover:border-dark-600 dark:hover:bg-dark-800" :disabled="actionBusy" @click="openRollback(rollback)">
                <div class="flex items-start justify-between gap-2">
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-gray-800 dark:text-dark-200">{{ rollbackTargetName(rollback) }}</p>
                    <p v-if="rollbackTargetMeta(rollback)" class="mt-1 truncate text-[11px] text-gray-400 dark:text-dark-500">{{ rollbackTargetMeta(rollback) }}</p>
                    <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ rollbackConsequence(rollback) }}</p>
                  </div>
                  <span :class="['flex-none rounded-md px-2 py-1 text-[11px] font-medium', rollbackStatusTone(rollback.status)]">{{ rollbackStatusLabel(rollback.status) }}</span>
                </div>
                <div v-if="rollback.changes?.length" class="mt-2 border-t border-gray-100 pt-2 text-xs text-gray-500 dark:border-dark-800 dark:text-dark-400">
                  <p v-for="change in rollback.changes.slice(0, 2)" :key="change.field" class="truncate">{{ pendingFieldLabel(change.field) }}：{{ formatValue(change.after) }} → {{ formatValue(change.before) }}</p>
                </div>
              </button>
            </div>
            <p v-else class="mt-4 text-sm text-gray-400 dark:text-dark-500">{{ t('admin.aiAgent.noRollbacks') }}</p>
          </section>

          <section class="mt-8 border-t border-gray-200 pt-5 dark:border-dark-700">
            <h2 class="text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.boundaryTitle') }}</h2>
            <div class="mt-3 space-y-3 text-xs leading-5 text-gray-500 dark:text-dark-400">
              <p class="flex gap-2"><Icon name="shield" size="sm" class="mt-0.5 flex-none text-emerald-600" />{{ t('admin.aiAgent.boundaryAuth') }}</p>
              <p class="flex gap-2"><Icon name="clipboard" size="sm" class="mt-0.5 flex-none text-emerald-600" />{{ t('admin.aiAgent.boundaryCatalog') }}</p>
              <p class="flex gap-2"><Icon name="eye" size="sm" class="mt-0.5 flex-none text-emerald-600" />{{ t('admin.aiAgent.boundaryConfirm') }}</p>
            </div>
          </section>
        </aside>
      </div>
      <div v-else class="flex flex-1 items-center justify-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</div>
    </div>

    <BaseDialog :show="showSettings" :title="t('admin.aiAgent.settings')" width="normal" @close="showSettings = false">
      <form class="space-y-5" @submit.prevent="saveSettings">
        <label class="flex items-start justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
          <span><span class="block text-sm font-medium text-gray-800 dark:text-dark-200">{{ t('admin.aiAgent.enabled') }}</span><span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.enabledHint') }}</span></span>
          <input v-model="settingsForm.enabled" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-emerald-600 focus:ring-emerald-500" />
        </label>
        <div>
          <label class="input-label">{{ t('admin.aiAgent.protocol') }}</label>
          <div class="grid grid-cols-3 gap-1 rounded-md bg-gray-100 p-1 dark:bg-dark-800" role="radiogroup" :aria-label="t('admin.aiAgent.protocol')">
            <button
              v-for="option in protocolOptions"
              :key="option.value"
              type="button"
              role="radio"
              :aria-checked="settingsForm.protocol === option.value"
              :class="['min-w-0 rounded-md px-2 py-2 text-xs font-medium transition-colors', settingsForm.protocol === option.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200']"
              @click="selectProtocol(option.value)"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.aiAgent.thinkingMode') }}</label>
          <input v-model="settingsForm.thinking_mode" class="input" type="text" :placeholder="thinkingPlaceholder" autocomplete="off" />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ thinkingHint }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.aiAgent.contextWindow') }}</label>
          <input v-model.trim="settingsForm.context_window" class="input" type="text" inputmode="text" autocomplete="off" :placeholder="t('admin.aiAgent.contextWindowPlaceholder')" />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.contextWindowHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.aiAgent.processDisplay') }}</label>
          <div class="grid grid-cols-3 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800" role="radiogroup" :aria-label="t('admin.aiAgent.processDisplay')">
            <button v-for="option in processDisplayOptions" :key="option.value" type="button" role="radio" :aria-checked="settingsForm.process_display === option.value" :class="['rounded-lg px-2 py-2 text-xs font-medium transition-colors', settingsForm.process_display === option.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-dark-400']" @click="settingsForm.process_display = option.value">
              {{ option.label }}
            </button>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.aiAgent.baseUrl') }}</label>
          <input v-model="settingsForm.base_url" class="input" type="url" :placeholder="t('admin.aiAgent.baseUrlPlaceholder')" />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.baseUrlHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.aiAgent.apiKey') }}</label>
          <input v-model="settingsForm.api_key" class="input" type="password" autocomplete="new-password" :placeholder="config?.api_key_set ? t('admin.aiAgent.apiKeyConfigured') : t('admin.aiAgent.apiKeyPlaceholder')" />
        </div>
        <div>
          <div class="mb-2 flex items-center justify-between">
            <label class="input-label mb-0">{{ t('admin.aiAgent.model') }}</label>
            <button type="button" class="text-xs font-medium text-emerald-700 hover:text-emerald-800 dark:text-emerald-400" :disabled="modelsLoading" @click="loadModels">{{ modelsLoading ? t('common.loading') : t('admin.aiAgent.fetchModels') }}</button>
          </div>
          <select v-if="models.length" v-model="settingsForm.model" class="input">
            <option value="">{{ t('admin.aiAgent.selectModel') }}</option>
            <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
          </select>
          <input v-else v-model="settingsForm.model" class="input" type="text" :placeholder="t('admin.aiAgent.modelPlaceholder')" />
        </div>
        <label class="flex items-start justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-700">
          <span><span class="block text-sm font-medium text-gray-800 dark:text-dark-200">{{ t('admin.aiAgent.autoApprove') }}</span><span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.autoApproveHint') }}</span></span>
          <input v-model="settingsForm.auto_approve" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-amber-600 focus:ring-amber-500" />
        </label>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="showSettings = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="settingsSaving" @click="saveSettings">{{ settingsSaving ? t('common.saving') : t('common.save') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="showPlanConfirmation && !!pending?.plan && confirmationReady" :title="t('admin.aiAgent.planConfirmTitle')" width="extra-wide" :close-on-click-outside="false" @close="showPlanConfirmation = false">
      <div v-if="pending?.plan" class="flex max-h-[70dvh] flex-col">
        <div class="grid flex-none gap-3 border-b border-gray-200 pb-4 text-sm dark:border-dark-700 sm:grid-cols-3">
          <div><p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.planName') }}</p><p class="mt-1 font-semibold text-gray-900 dark:text-white">{{ pending.plan.title }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.planOperations') }}</p><p class="mt-1 font-semibold text-gray-900 dark:text-white">{{ pending.plan.nodes.length }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.planFailurePolicy') }}</p><p class="mt-1 font-semibold text-gray-900 dark:text-white">{{ planFailurePolicyLabel(pending.plan.failure_policy) }}</p></div>
        </div>
        <div v-if="pending.sensitive" class="mt-4 flex flex-none items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-900 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
          <Icon name="shield" size="sm" class="mt-0.5 flex-none" />
          <div><p class="font-medium">{{ t('admin.aiAgent.planSensitiveTitle') }}</p><p class="mt-1 text-amber-700 dark:text-amber-300">{{ t('admin.aiAgent.planSensitiveHint') }}</p></div>
        </div>
        <ol class="mt-4 min-h-0 overflow-y-auto border-y border-gray-200 dark:border-dark-700">
          <li v-for="(node, index) in pending.plan.nodes" :key="node.id" class="grid gap-3 border-b border-gray-100 py-4 last:border-b-0 dark:border-dark-800 sm:grid-cols-[36px_minmax(0,1fr)_auto]">
            <span :class="['flex h-8 w-8 items-center justify-center rounded-lg text-xs font-semibold', planNodeTone(node.status)]">{{ index + 1 }}</span>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <p class="font-medium text-gray-900 dark:text-white">{{ planNodeLabel(node) }}</p>
                <span v-if="node.sensitive" class="rounded-md bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-950 dark:text-amber-300">{{ t('admin.aiAgent.planSensitiveStep') }}</span>
              </div>
              <p v-if="node.depends_on?.length" class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.planDependsOn', { nodes: node.depends_on.join(', ') }) }}</p>
              <dl v-if="node.preview?.length" class="mt-3 grid gap-x-5 gap-y-2 text-xs sm:grid-cols-2">
                <div v-for="change in node.preview" :key="change.field" class="min-w-0">
                  <dt class="font-medium text-gray-600 dark:text-dark-300">{{ pendingFieldLabel(change.field) }}</dt>
                  <dd class="mt-0.5 break-words text-gray-800 dark:text-dark-200">{{ formatValue(change.after) }}</dd>
                </div>
              </dl>
              <p v-if="node.error" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ node.error }}</p>
              <details class="mt-3 text-xs text-gray-500 dark:text-dark-400">
                <summary class="cursor-pointer select-none">{{ t('admin.aiAgent.technicalDetails') }}</summary>
                <p class="mt-1 break-all font-mono">{{ node.id }} · {{ node.endpoint_key }}</p>
              </details>
            </div>
            <span class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ planNodeStatusLabel(node.status) }}</span>
          </li>
        </ol>
      </div>
      <template #footer>
        <button class="btn btn-secondary" :disabled="actionBusy || busy" @click="cancelPending"><Icon name="x" size="sm" />{{ t('admin.aiAgent.cancelPlan') }}</button>
        <button class="btn bg-amber-600 text-white hover:bg-amber-700" :disabled="actionBusy || busy" @click="confirmPending"><Icon name="check" size="sm" />{{ actionBusy ? t('admin.aiAgent.executingPlan') : t('admin.aiAgent.confirmPlan') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="showRollbackConfirmation && !!selectedRollback" :title="rollbackDialogTitle()" width="wide" :close-on-click-outside="false" @close="closeRollback()">
      <div v-if="rollbackLoading" class="flex min-h-40 items-center justify-center text-sm text-gray-500 dark:text-dark-400">
        <Icon name="refresh" size="sm" class="mr-2 animate-spin" />{{ t('admin.aiAgent.rollbackChecking') }}
      </div>
      <div v-else-if="rollbackPreview" class="max-h-[70dvh] overflow-y-auto overscroll-contain">
        <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
          <div class="min-w-0">
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ rollbackConsequence(rollbackPreview.rollback) }}</p>
            <p v-if="rollbackTargetMeta(rollbackPreview.rollback)" class="mt-1 break-words text-xs font-medium text-gray-600 dark:text-dark-300">{{ rollbackTargetMeta(rollbackPreview.rollback) }}</p>
            <p class="mt-1 break-words text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackSourceOperation') }}：{{ rollbackPreview.rollback.operation }}</p>
          </div>
          <span :class="['flex-none rounded-md px-2 py-1 text-xs font-medium', rollbackStatusTone(rollbackPreview.status)]">{{ rollbackStatusLabel(rollbackPreview.status) }}</span>
        </div>

        <div v-if="rollbackPreview.action === 'delete_created'" class="border-b border-gray-200 py-5 dark:border-dark-700">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.aiAgent.rollbackDeleteImpact', { target: rollbackTargetName(rollbackPreview.rollback) }) }}</p>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackDeleteHint') }}</p>
        </div>
        <div v-else-if="rollbackPreview.action === 'rollback_plan'" class="border-b border-gray-200 py-5 dark:border-dark-700">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.aiAgent.rollbackPlanImpact', { count: rollbackPreview.rollback.child_count || 0 }) }}</p>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackPlanHint') }}</p>
        </div>

        <div v-if="rollbackDiffGroups.length" class="border-b border-gray-200 dark:border-dark-700">
          <div class="border-b border-gray-100 py-2 text-[11px] font-medium uppercase text-gray-400 dark:border-dark-800">{{ t('admin.aiAgent.rollbackDiffTitle') }}</div>
          <section v-for="group in rollbackDiffGroups" :key="group.key" class="border-b border-gray-200 py-4 last:border-b-0 dark:border-dark-700">
            <header class="border-l-2 border-emerald-500 pl-3">
              <div class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
                <span class="text-[11px] font-medium uppercase text-gray-400 dark:text-dark-500">{{ rollbackResourceLabel(group.resource) || t('admin.aiAgent.rollbackResource') }}</span>
                <strong class="break-words text-sm text-gray-900 dark:text-white">{{ rollbackGroupTargetName(group) }}</strong>
                <span v-if="group.targetID" class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackTargetID', { id: group.targetID }) }}</span>
              </div>
              <p class="mt-1 break-words text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackSourceOperation') }}：{{ group.operations.join('、') }}</p>
            </header>
            <div v-for="(field, fieldIndex) in group.fields" :key="`${group.key}-${field.operation || ''}-${field.field}-${fieldIndex}`" class="mt-4 border-t border-gray-100 pt-3 first:mt-3 first:border-t-0 first:pt-0 dark:border-dark-800">
              <div class="flex min-w-0 flex-wrap items-center gap-2 text-xs">
                <span class="font-semibold text-gray-800 dark:text-dark-200">{{ pendingFieldLabel(field.field) }}</span>
                <span v-if="group.operations.length > 1 && field.operation" class="text-[11px] text-gray-400 dark:text-dark-500">{{ field.operation }}</span>
                <span :class="['rounded-md px-1.5 py-0.5 text-[10px] font-medium', rollbackStatusTone(field.status === 'will_restore' ? 'safe' : field.status)]">{{ t(`admin.aiAgent.rollbackFieldStatuses.${field.status}`) }}</span>
              </div>
              <div v-if="rollbackDiffLines(field).length" class="mt-3 overflow-hidden rounded-md border border-gray-200 font-mono text-xs dark:border-dark-700">
                <div v-for="(line, index) in rollbackDiffLines(field)" :key="`${line.kind}-${line.path}-${index}`" :class="['grid grid-cols-[24px_minmax(0,1fr)] border-b border-black/5 py-1.5 last:border-b-0 dark:border-white/5', line.kind === 'remove' ? 'bg-red-50 text-red-800 dark:bg-red-950/35 dark:text-red-300' : 'bg-emerald-50 text-emerald-800 dark:bg-emerald-950/35 dark:text-emerald-300']">
                  <span class="select-none text-center font-semibold">{{ line.kind === 'remove' ? '−' : '+' }}</span>
                  <span v-if="line.path" class="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-1 pr-3"><span class="break-all font-semibold">{{ line.path }}:</span><span class="whitespace-nowrap">{{ line.value }}</span></span>
                  <span v-else class="min-w-0 break-all pr-3">{{ line.value }}</span>
                </div>
              </div>
              <p v-else class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackNoFieldChange') }}</p>
            </div>
          </section>
        </div>

        <div v-if="rollbackPreview.status === 'conflict' || rollbackPreview.status === 'unavailable'" class="flex items-start gap-2 border-b border-red-200 bg-red-50 px-3 py-3 text-xs text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
          <Icon name="shield" size="sm" class="mt-0.5 flex-none" />
          <p>{{ rollbackPreview.status === 'conflict' ? t('admin.aiAgent.rollbackConflictHint', { count: rollbackPreview.conflict_count || 0 }) : t('admin.aiAgent.rollbackUnavailableHint') }}</p>
        </div>
        <div v-if="rollbackPreview.requires_step_up" class="flex items-start gap-2 border-b border-amber-200 py-3 text-xs text-amber-800 dark:border-amber-900 dark:text-amber-300">
          <Icon name="shield" size="sm" class="mt-0.5 flex-none" /><p>{{ t('admin.aiAgent.rollbackStepUpHint') }}</p>
        </div>

        <div v-if="rollbackPreview.status !== 'completed' && rollbackPreview.status !== 'running'" class="pt-4">
          <label class="input-label">{{ t('admin.aiAgent.rollbackAgentInstruction') }}</label>
          <textarea v-model="rollbackInstruction" class="input min-h-20 resize-y" maxlength="2000" :placeholder="t('admin.aiAgent.rollbackAgentInstructionPlaceholder')"></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.rollbackAgentHint') }}</p>
        </div>
        <details class="mt-4 text-xs text-gray-500 dark:text-dark-400">
          <summary class="cursor-pointer select-none">{{ t('admin.aiAgent.technicalDetails') }}</summary>
          <p class="mt-1 break-all font-mono">{{ rollbackPreview.rollback.method }} {{ rollbackPreview.rollback.path }}</p>
          <p class="mt-1">{{ t('admin.aiAgent.rollbackCheckedAt') }}：{{ formatConversationTime(rollbackPreview.checked_at) }}</p>
        </details>
      </div>
      <template #footer>
        <button class="btn btn-secondary" :disabled="actionBusy" :title="t('common.cancel')" @click="closeRollback()"><Icon name="x" size="sm" /><span class="hidden sm:inline">{{ t('common.cancel') }}</span></button>
        <button v-if="rollbackPreview && rollbackPreview.status !== 'completed' && rollbackPreview.status !== 'running'" class="btn btn-secondary" :disabled="actionBusy || busy || !!pending" @click="assistRollback"><Icon name="sparkles" size="sm" />{{ rollbackPreview.status === 'conflict' ? t('admin.aiAgent.rollbackAgentResolve') : t('admin.aiAgent.rollbackAgentHandle') }}</button>
        <button v-if="rollbackPreview?.can_execute" class="btn bg-amber-600 text-white hover:bg-amber-700" :disabled="actionBusy" @click="runRollback"><Icon name="refresh" size="sm" />{{ rollbackConfirmLabel() }}</button>
      </template>
    </BaseDialog>

    <TotpStepUpDialog :controller="sensitiveStepUp" />

    <BaseDialog :show="showHistory" :title="t('admin.aiAgent.history')" width="normal" @close="showHistory = false">
      <div class="space-y-2">
        <button class="btn btn-primary w-full justify-center" @click="createConversation"><Icon name="plus" size="sm" />{{ t('admin.aiAgent.newConversation') }}</button>
        <div v-if="conversations.length" class="max-h-[55vh] space-y-2 overflow-y-auto pt-2">
          <div v-for="conversation in conversations" :key="conversation.id" :class="['flex items-center gap-2 rounded-lg border p-2', conversation.id === conversationId ? 'border-emerald-300 bg-emerald-50 dark:border-emerald-900 dark:bg-emerald-950/30' : 'border-gray-200 dark:border-dark-700']">
            <button class="min-w-0 flex-1 px-1 py-1 text-left" @click="selectConversation(conversation.id)">
              <p class="truncate text-sm font-medium text-gray-800 dark:text-dark-200">{{ conversation.title }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ formatConversationTime(conversation.updated_at) }} · {{ conversationStatusLabel(conversation.status) }}</p>
            </button>
            <button class="rounded-lg p-2 text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/30" :title="t('admin.aiAgent.deleteConversation')" @click="deleteConversation(conversation.id)"><Icon name="trash" size="sm" /></button>
          </div>
        </div>
        <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('admin.aiAgent.noHistory') }}</p>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import aiAgentAPI, { type AIAgentConfig, type AIAgentConversationStatus, type AIAgentConversationSummary, type AIAgentMessage, type AIAgentPendingAction, type AIAgentPlanNode, type AIAgentProcessEvent, type AIAgentProtocol, type AIAgentRollback, type AIAgentRollbackFieldPreview, type AIAgentRollbackPreview, type AIAgentSession } from '@/api/admin/aiAgent'
import { useAppStore } from '@/stores'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { renderAgentMarkdown } from '@/utils/agentMarkdown'
import { shouldSendAgentComposer } from '@/utils/agentComposer'
import { announceAIAgentEnabled } from '@/utils/agentAvailability'

const { t } = useI18n()
const appStore = useAppStore()
const sensitiveStepUp = useStepUp()
const config = ref<AIAgentConfig | null>(null)
const messages = ref<AIAgentMessage[]>([])
const events = ref<AIAgentProcessEvent[]>([])
const pending = ref<AIAgentPendingAction | undefined>()
const rollbacks = ref<AIAgentRollback[]>([])
const conversations = ref<AIAgentConversationSummary[]>([])
const conversationId = ref('')
const conversationStatus = ref<AIAgentConversationStatus>('idle')
const prompt = ref('')
const composerCompositionActive = ref(false)
const busy = computed(() => conversationStatus.value === 'running' || conversationStatus.value === 'stopping')
const confirmationReady = computed(() => conversationStatus.value === 'idle')
const hasStreamingMessage = computed(() => messages.value.some(message => message.streaming))
type MessageFlowItem = { key: string; message?: AIAgentMessage; processEvents?: AIAgentProcessEvent[]; open?: boolean }
const messageFlow = computed<MessageFlowItem[]>(() => {
  const messageRunKeys = new Map<string, string>()
  let currentRunKey = ''
  for (const message of messages.value) {
    if (message.role === 'user') currentRunKey = message.run_id || `legacy-${message.id}`
    else if (message.run_id) currentRunKey = message.run_id
    if (currentRunKey) messageRunKeys.set(message.id, currentRunKey)
  }

  const eventsByRun = new Map<string, AIAgentProcessEvent[]>()
  for (const event of events.value) {
    let runKey = event.run_id || ''
    if (!runKey) {
      const eventTime = new Date(event.created_at).getTime()
      for (let index = messages.value.length - 1; index >= 0; index -= 1) {
        const message = messages.value[index]
        if (message.role === 'user' && new Date(message.created_at).getTime() <= eventTime) {
          runKey = messageRunKeys.get(message.id) || ''
          break
        }
      }
    }
    if (!runKey) continue
    const grouped = eventsByRun.get(runKey) || []
    grouped.push(event)
    eventsByRun.set(runKey, grouped)
  }

  const flow: MessageFlowItem[] = []
  const insertedRuns = new Set<string>()
  const latestRunKey = [...messages.value].reverse().find(message => message.role === 'user')
  const activeRunKey = latestRunKey ? messageRunKeys.get(latestRunKey.id) : undefined
  for (const message of messages.value) {
    const runKey = messageRunKeys.get(message.id)
    const processEvents = runKey ? eventsByRun.get(runKey) : undefined
    if (message.role === 'assistant' && runKey && processEvents?.length && !insertedRuns.has(runKey)) {
      flow.push({ key: `process-${runKey}`, processEvents, open: busy.value && runKey === activeRunKey })
      insertedRuns.add(runKey)
    }
    flow.push({ key: `message-${message.id}`, message })
    if (message.role === 'user' && runKey && processEvents?.length && !insertedRuns.has(runKey)) {
      flow.push({ key: `process-${runKey}`, processEvents, open: busy.value && runKey === activeRunKey })
      insertedRuns.add(runKey)
    }
  }
  return flow
})
const actionBusy = ref(false)
const showPlanConfirmation = ref(false)
const showRollbackConfirmation = ref(false)
const rollbackLoading = ref(false)
const rollbackPreview = ref<AIAgentRollbackPreview | null>(null)
const selectedRollback = ref<AIAgentRollback | null>(null)
const rollbackInstruction = ref('')
type RollbackDiffGroup = { key: string; resource?: string; targetLabel?: string; targetID?: string; operations: string[]; fields: AIAgentRollbackFieldPreview[] }
const rollbackDiffGroups = computed<RollbackDiffGroup[]>(() => buildRollbackDiffGroups(rollbackPreview.value))
const lastAutoOpenedPlanID = ref('')
const showSettings = ref(false)
const showHistory = ref(false)
const settingsSaving = ref(false)
const modelsLoading = ref(false)
const models = ref<string[]>([])
const messagePane = ref<HTMLElement | null>(null)
let pollTimer: ReturnType<typeof setTimeout> | undefined
const conversationStorageKey = 'ai_agent_active_conversation'
watch([() => pending.value?.plan?.id, () => conversationStatus.value], ([planID, status]) => {
  if (!planID) {
    showPlanConfirmation.value = false
    return
  }
  const agentFinished = status === 'idle'
  if (agentFinished && planID !== lastAutoOpenedPlanID.value && pending.value?.plan?.status === 'awaiting_confirmation') {
    lastAutoOpenedPlanID.value = planID
    showPlanConfirmation.value = true
  }
})

const settingsForm = reactive<{ enabled: boolean; base_url: string; model: string; api_key: string; auto_approve: boolean; protocol: AIAgentProtocol; thinking_mode: string; context_window: string; process_display: 'off' | 'compact' | 'full' }>({
  enabled: true,
  base_url: '',
  model: '',
  api_key: '',
  auto_approve: false,
  protocol: 'chat_completions',
  thinking_mode: '',
  context_window: '150k',
  process_display: 'compact'
})

const protocolOptions = computed<Array<{ value: AIAgentProtocol; label: string }>>(() => [
  { value: 'chat_completions', label: t('admin.aiAgent.protocolChatCompletions') },
  { value: 'responses', label: t('admin.aiAgent.protocolResponses') },
  { value: 'messages', label: t('admin.aiAgent.protocolMessages') }
])
const processDisplayOptions = computed<Array<{ value: 'off' | 'compact' | 'full'; label: string }>>(() => [
  { value: 'off', label: t('admin.aiAgent.processOff') },
  { value: 'compact', label: t('admin.aiAgent.processCompact') },
  { value: 'full', label: t('admin.aiAgent.processFull') }
])
const thinkingPlaceholder = computed(() => settingsForm.protocol === 'messages' ? t('admin.aiAgent.thinkingMessagesPlaceholder') : t('admin.aiAgent.thinkingEffortPlaceholder'))
const thinkingHint = computed(() => settingsForm.protocol === 'messages' ? t('admin.aiAgent.thinkingMessagesHint') : t('admin.aiAgent.thinkingEffortHint'))

const suggestions = computed(() => [
  t('admin.aiAgent.suggestionUsers'),
  t('admin.aiAgent.suggestionAccounts'),
  t('admin.aiAgent.suggestionUsage')
])

function syncSettingsForm() {
  settingsForm.enabled = config.value?.enabled ?? true
  settingsForm.base_url = config.value?.base_url || ''
  settingsForm.model = config.value?.model || ''
  settingsForm.api_key = ''
  settingsForm.auto_approve = config.value?.auto_approve || false
  settingsForm.protocol = config.value?.protocol || 'chat_completions'
  settingsForm.thinking_mode = config.value?.thinking_mode || ''
  settingsForm.context_window = config.value?.context_window || '150k'
  settingsForm.process_display = config.value?.process_display || 'compact'
}

function selectProtocol(protocol: AIAgentProtocol) {
  if (settingsForm.protocol !== protocol) {
    settingsForm.protocol = protocol
    models.value = []
  }
}

function applySession(session: AIAgentSession) {
  const previousConversationID = conversationId.value
  const previousStatus = conversationStatus.value
  const notifyNewFailure = previousConversationID === session.conversation.id && (previousStatus === 'running' || previousStatus === 'stopping') && session.conversation.status === 'error'
  conversationId.value = session.conversation.id
  localStorage.setItem(conversationStorageKey, session.conversation.id)
  conversationStatus.value = session.conversation.status
  messages.value = session.messages || []
  events.value = session.events || []
  pending.value = session.pending
  rollbacks.value = session.rollbacks || []
  if (notifyNewFailure && session.error) appStore.showError(agentErrorMessage(session.error, t('admin.aiAgent.chatFailed')))
}

async function refreshConversationList() {
  const result = await aiAgentAPI.listConversations()
  conversations.value = result.conversations || []
  return result
}

function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  const hasRunningConversation = conversations.value.some(item => item.status === 'running' || item.status === 'stopping')
  if (!busy.value && !hasRunningConversation) return
  pollTimer = setTimeout(pollAgentState, busy.value ? 250 : 900)
}

async function pollAgentState() {
  try {
    const list = await refreshConversationList()
    const current = list.conversations.find(item => item.id === conversationId.value)
    if (current && (current.status === 'running' || current.status === 'stopping' || busy.value)) {
      applySession(await aiAgentAPI.getSession(conversationId.value))
      await scrollToBottom()
    }
  } catch {
    // A transient polling failure should not discard the running conversation.
  } finally {
    schedulePoll()
  }
}

function publishAIAgentAvailability() {
  if (config.value) announceAIAgentEnabled(config.value.enabled)
}

async function loadAgentWorkspace() {
  const list = await refreshConversationList()
  const savedID = localStorage.getItem(conversationStorageKey)
  const targetID = (savedID && list.conversations.some(item => item.id === savedID) ? savedID : '') || list.active_id || list.conversations[0]?.id
  const session = targetID ? await aiAgentAPI.getSession(targetID) : await aiAgentAPI.createConversation()
  applySession(session)
  await refreshConversationList()
  await scrollToBottom()
  schedulePoll()
}

async function loadInitial() {
  try {
    config.value = await aiAgentAPI.getConfig()
    syncSettingsForm()
    publishAIAgentAvailability()
    if (config.value.enabled) await loadAgentWorkspace()
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.loadFailed')))
  }
}

async function enableAgent() {
  settingsSaving.value = true
  try {
    config.value = await aiAgentAPI.updateConfig({ enabled: true })
    syncSettingsForm()
    publishAIAgentAvailability()
    await loadAgentWorkspace()
    appStore.showSuccess(t('admin.aiAgent.enabledSuccess'))
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.settingsFailed')))
  } finally {
    settingsSaving.value = false
  }
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (!shouldSendAgentComposer(event, composerCompositionActive.value)) return
  event.preventDefault()
  void sendMessage()
}

async function sendMessage() {
  const message = prompt.value.trim()
  if (!message || busy.value || pending.value || !conversationId.value) return
  prompt.value = ''
  try {
    applySession(await aiAgentAPI.chat(conversationId.value, message))
    await refreshConversationList()
    await scrollToBottom()
    schedulePoll()
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.chatFailed')))
  }
}

async function stopGeneration() {
  if (!conversationId.value || !busy.value) return
  conversationStatus.value = 'stopping'
  try {
    await aiAgentAPI.stop(conversationId.value)
    schedulePoll()
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.stopFailed')))
  }
}

async function confirmPending() {
  if (!pending.value) return
  actionBusy.value = true
  try {
    const action = pending.value
    await sensitiveStepUp.run(() => aiAgentAPI.confirm(conversationId.value, action.id, !!action.requires_step_up))
    showPlanConfirmation.value = false
    if (action.plan) {
      conversationStatus.value = 'running'
      appStore.showSuccess(t('admin.aiAgent.planAccepted'))
      schedulePoll()
    } else {
      applySession(await aiAgentAPI.getSession(conversationId.value))
      appStore.showSuccess(t('admin.aiAgent.confirmed'))
    }
  } catch (error: any) {
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
      return
    }
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.actionFailed')))
  } finally {
    actionBusy.value = false
  }
}

async function cancelPending() {
  if (!pending.value) return
  actionBusy.value = true
  try {
    await aiAgentAPI.cancel(conversationId.value, pending.value.id)
    applySession(await aiAgentAPI.getSession(conversationId.value))
    showPlanConfirmation.value = false
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.actionFailed')))
  } finally {
    actionBusy.value = false
  }
}

async function openRollback(rollback: AIAgentRollback) {
  selectedRollback.value = rollback
  rollbackPreview.value = null
  rollbackInstruction.value = ''
  showRollbackConfirmation.value = true
  rollbackLoading.value = true
  try {
    rollbackPreview.value = await aiAgentAPI.previewRollback(conversationId.value, rollback.id)
    selectedRollback.value = { ...rollback, ...rollbackPreview.value.rollback }
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.rollbackPreviewFailed')))
  } finally {
    rollbackLoading.value = false
  }
}

function closeRollback(force = false) {
  if (actionBusy.value && !force) return
  showRollbackConfirmation.value = false
  selectedRollback.value = null
  rollbackPreview.value = null
  rollbackInstruction.value = ''
}

async function runRollback() {
  const preview = rollbackPreview.value
  if (!preview?.can_execute) return
  actionBusy.value = true
  try {
    await sensitiveStepUp.run(() => aiAgentAPI.rollback(conversationId.value, preview.rollback.id, !!preview.requires_step_up))
    applySession(await aiAgentAPI.getSession(conversationId.value))
    closeRollback(true)
    appStore.showSuccess(t('admin.aiAgent.rolledBack'))
  } catch (error: any) {
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
      return
    }
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.rollbackFailed')))
    if (selectedRollback.value) await openRollback(selectedRollback.value)
  } finally {
    actionBusy.value = false
  }
}

async function assistRollback() {
  const rollback = selectedRollback.value
  if (!rollback || busy.value || pending.value) return
  actionBusy.value = true
  try {
    applySession(await aiAgentAPI.assistRollback(conversationId.value, rollback.id, rollbackInstruction.value.trim()))
    closeRollback(true)
    await refreshConversationList()
    await scrollToBottom()
    schedulePoll()
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.rollbackAssistFailed')))
  } finally {
    actionBusy.value = false
  }
}

async function createConversation() {
  try {
    applySession(await aiAgentAPI.createConversation())
    await refreshConversationList()
    showHistory.value = false
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.historyFailed')))
  }
}

async function selectConversation(id: string) {
  try {
    applySession(await aiAgentAPI.getSession(id))
    showHistory.value = false
    await scrollToBottom()
    schedulePoll()
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.historyFailed')))
  }
}

async function deleteConversation(id: string) {
  if (!window.confirm(t('admin.aiAgent.deleteConfirm'))) return
  try {
    await aiAgentAPI.deleteConversation(id)
    const list = await refreshConversationList()
    if (id === conversationId.value) {
      const nextID = list.active_id || list.conversations[0]?.id
      applySession(nextID ? await aiAgentAPI.getSession(nextID) : await aiAgentAPI.createConversation())
      await refreshConversationList()
    }
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.clearFailed')))
  }
}

async function deleteCurrentConversation() {
  if (conversationId.value) await deleteConversation(conversationId.value)
}

async function saveSettings() {
  settingsSaving.value = true
  const wasEnabled = config.value?.enabled ?? true
  try {
    config.value = await aiAgentAPI.updateConfig({
      enabled: settingsForm.enabled,
      base_url: settingsForm.base_url,
      model: settingsForm.model,
      auto_approve: settingsForm.auto_approve,
      protocol: settingsForm.protocol,
      thinking_mode: settingsForm.thinking_mode,
      context_window: settingsForm.context_window,
      process_display: settingsForm.process_display,
      ...(settingsForm.api_key ? { api_key: settingsForm.api_key } : {})
    })
    syncSettingsForm()
    publishAIAgentAvailability()
    if (!config.value.enabled) {
      if (pollTimer) clearTimeout(pollTimer)
      showPlanConfirmation.value = false
      showRollbackConfirmation.value = false
    } else if (!wasEnabled || !conversationId.value) {
      await loadAgentWorkspace()
    }
    showSettings.value = false
    appStore.showSuccess(t('admin.aiAgent.settingsSaved'))
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.settingsFailed')))
  } finally {
    settingsSaving.value = false
  }
}

async function loadModels() {
  modelsLoading.value = true
  try {
    if (settingsForm.api_key || settingsForm.base_url !== (config.value?.base_url || '') || settingsForm.protocol !== config.value?.protocol) {
      config.value = await aiAgentAPI.updateConfig({
        base_url: settingsForm.base_url,
        protocol: settingsForm.protocol,
        ...(settingsForm.api_key ? { api_key: settingsForm.api_key } : {})
      })
      settingsForm.api_key = ''
    }
    models.value = await aiAgentAPI.listModels()
  } catch (error: any) {
    appStore.showError(agentErrorMessage(error, t('admin.aiAgent.modelsFailed')))
  } finally {
    modelsLoading.value = false
  }
}

function messageContent(message: AIAgentMessage): string {
  if (message.event === 'operation_confirmed') {
    return t('admin.aiAgent.operationConfirmed', {
      method: String(message.metadata?.method || ''),
      path: String(message.metadata?.path || '')
    })
  }
  if (message.event === 'plan_confirmed') {
    return t('admin.aiAgent.planConfirmed', { title: String(message.metadata?.title || '') })
  }
  const legacyConfirmation = message.content.match(/^Confirmed operation completed:\s+(\S+)\s+(.+)$/)
  if (legacyConfirmation) {
    return t('admin.aiAgent.operationConfirmed', { method: legacyConfirmation[1], path: legacyConfirmation[2] })
  }
  return message.content
}

function agentErrorMessage(error: unknown, fallback: string): string {
  const raw = typeof error === 'string' ? error : (error as any)?.message || ''
  const normalized = raw.toLowerCase()
  if (normalized.includes('model api key is not configured') || normalized.includes('stored model api key cannot be decrypted')) {
    return t('admin.aiAgent.errors.modelKey')
  }
  if (normalized.includes('tls handshake timeout') || normalized.includes('context deadline exceeded') || normalized.includes('request timeout')) {
    return t('admin.aiAgent.errors.networkTimeout')
  }
  if (normalized.includes('agent context window') || normalized.includes('context setting') || normalized.includes('context length') || normalized.includes('too many tokens') || normalized.includes('token limit')) {
    return t('admin.aiAgent.errors.contextWindow')
  }
  if (normalized.includes('call agent model') || normalized.includes('fetch models')) {
    return t('admin.aiAgent.errors.modelRequest')
  }
  return raw || fallback
}

function processEventIcon(event: AIAgentProcessEvent): 'play' | 'brain' | 'sparkles' | 'search' | 'terminal' | 'checkCircle' | 'xCircle' | 'stop' {
  if (event.kind === 'started') return 'play'
  if (event.kind === 'model') return 'brain'
  if (event.kind === 'model_result') return 'sparkles'
  if (event.kind === 'tool') return event.metadata?.tool === 'search_admin_operations' ? 'search' : 'terminal'
  if (event.kind === 'error') return 'xCircle'
  if (event.kind === 'stopped') return 'stop'
  return 'checkCircle'
}

function processEventTone(event: AIAgentProcessEvent): string {
  if (event.kind === 'model' || event.kind === 'model_result') return 'bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-300'
  if (event.kind === 'tool') return 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300'
  if (event.kind === 'error' || event.kind.includes('failed')) return 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300'
  if (event.kind === 'stopped') return 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
}

function processEventTitle(event: AIAgentProcessEvent): string {
  const metadata = event.metadata || {}
  switch (event.kind) {
    case 'started': return t('admin.aiAgent.process.started')
    case 'model': return t('admin.aiAgent.process.model', { round: Number(metadata.round || 1) })
    case 'model_result': return Number(metadata.tool_calls || 0) > 0
      ? t('admin.aiAgent.process.modelTools', { count: Number(metadata.tool_calls) })
      : t('admin.aiAgent.process.modelAnswer')
    case 'tool': return metadata.tool === 'search_admin_operations'
      ? t('admin.aiAgent.process.catalog')
      : t('admin.aiAgent.process.tool', { name: event.summary || String(metadata.tool || '') })
    case 'tool_result': return t('admin.aiAgent.process.toolResult', { name: event.summary || String(metadata.tool || '') })
    case 'plan_node_started': return t('admin.aiAgent.process.planNodeStarted', { name: event.summary })
    case 'plan_node_completed': return t('admin.aiAgent.process.planNodeCompleted', { name: event.summary })
    case 'plan_node_failed': return t('admin.aiAgent.process.planNodeFailed', { name: event.summary })
    case 'plan_node_rolled_back': return t('admin.aiAgent.process.planNodeRolledBack', { name: event.summary })
    case 'plan_node_rollback_failed': return t('admin.aiAgent.process.planNodeRollbackFailed', { name: event.summary })
    case 'context_compressed': return t('admin.aiAgent.process.contextCompressed')
    case 'capability_corrected': return t('admin.aiAgent.process.capabilityCorrected')
    case 'completed': return t('admin.aiAgent.process.completed')
    case 'stopped': return t('admin.aiAgent.process.stopped')
    case 'error': return t('admin.aiAgent.process.error')
    default: return event.summary || event.kind
  }
}

function formatDuration(value: unknown): string {
  const milliseconds = Number(value)
  if (!Number.isFinite(milliseconds)) return ''
  return milliseconds < 1000 ? `${milliseconds} ms` : `${(milliseconds / 1000).toFixed(1)} s`
}

function processEventMetadata(event: AIAgentProcessEvent): string[] {
  const metadata = event.metadata || {}
  const items: string[] = []
  if (metadata.protocol) items.push(String(metadata.protocol))
  if (metadata.model) items.push(String(metadata.model))
  if (metadata.method) items.push(String(metadata.method))
  if (metadata.path) items.push(String(metadata.path))
  if (metadata.status) items.push(String(metadata.status))
  if (metadata.context_before !== undefined && metadata.context_after !== undefined) items.push(`${metadata.context_before} → ${metadata.context_after} tokens`)
  if (metadata.retry_attempt) items.push(`retry ${metadata.retry_attempt}`)
  if (metadata.context_window) items.push(String(metadata.context_window))
  if (metadata.cache_enabled) items.push(metadata.cache_hit ? t('admin.aiAgent.cacheHit', { tokens: Number(metadata.cached_units || 0) }) : t('admin.aiAgent.cacheMiss'))
  if (metadata.duration_ms !== undefined) items.push(formatDuration(metadata.duration_ms))
  return items.filter(Boolean)
}

function formatEventTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function formatEventDetail(detail: string): string {
  try { return JSON.stringify(JSON.parse(detail), null, 2) } catch { return detail }
}

function formatConversationTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function conversationStatusLabel(status: AIAgentConversationStatus): string {
  return t(`admin.aiAgent.status.${status}`)
}

const pendingResourceTranslationKeys: Record<string, string> = {
  users: 'users',
  groups: 'groups',
  accounts: 'accounts',
  proxies: 'proxies',
  channels: 'channels',
  subscriptions: 'subscriptions',
  announcements: 'announcements',
  plan: 'plan'
}

function planFailurePolicyLabel(policy: 'stop_on_failure' | 'continue_independent' | 'rollback_on_failure'): string {
  return t(`admin.aiAgent.planPolicies.${policy}`)
}

function planNodeStatusLabel(status: AIAgentPlanNode['status']): string {
  return t(`admin.aiAgent.planStatuses.${status}`)
}

function planNodeTone(status: AIAgentPlanNode['status']): string {
  if (status === 'failed' || status === 'rollback_failed') return 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300'
  if (status === 'succeeded') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
  if (status === 'running') return 'bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-300'
  if (status === 'rolled_back' || status === 'blocked') return 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
  return 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300'
}

function planNodeLabel(node: AIAgentPlanNode): string {
  const resourceKey = node.resource ? pendingResourceTranslationKeys[node.resource] : undefined
  const resource = resourceKey ? t(`admin.aiAgent.resources.${resourceKey}`) : node.resource || ''
  const action = node.action ? t(`admin.aiAgent.actions.${node.action}`) : ''
  return action || resource ? t('admin.aiAgent.operationLabel', { action, resource }) : node.operation
}

function pendingOperationLabel(action: AIAgentPendingAction): string {
  if (action.plan) return action.plan.title
  const resourceKey = action.resource ? pendingResourceTranslationKeys[action.resource] : undefined
  if (!action.action || !resourceKey) return action.operation
  return t('admin.aiAgent.operationLabel', {
    action: t(`admin.aiAgent.actions.${action.action}`),
    resource: t(`admin.aiAgent.resources.${resourceKey}`)
  })
}

type RollbackDiffLine = { kind: 'remove' | 'add'; path: string; value: string }

function rollbackDiffValue(value: unknown): string {
  if (value === undefined) return t('admin.aiAgent.notSet')
  if (typeof value === 'string') return value
  if (value === null) return 'null'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function rollbackDiffLines(field: AIAgentRollbackFieldPreview): RollbackDiffLine[] {
  if (field.sensitive) {
    const protectedValue = t('admin.aiAgent.rollbackProtectedValue')
    return [
      { kind: 'remove', path: '', value: protectedValue },
      { kind: 'add', path: '', value: protectedValue }
    ]
  }
  const lines: RollbackDiffLine[] = []
  collectRollbackDiff(lines, field.current, field.result, '', true, true)
  return lines
}

function collectRollbackDiff(lines: RollbackDiffLine[], current: unknown, result: unknown, path: string, currentExists: boolean, resultExists: boolean) {
  const currentObject = current !== null && !Array.isArray(current) && typeof current === 'object' ? current as Record<string, unknown> : null
  const resultObject = result !== null && !Array.isArray(result) && typeof result === 'object' ? result as Record<string, unknown> : null
  if (currentObject && resultObject) {
    const keys = [...new Set([...Object.keys(currentObject), ...Object.keys(resultObject)])].sort()
    for (const key of keys) {
      collectRollbackDiff(lines, currentObject[key], resultObject[key], path ? `${path}.${key}` : key, Object.prototype.hasOwnProperty.call(currentObject, key), Object.prototype.hasOwnProperty.call(resultObject, key))
    }
    return
  }
  if (currentExists === resultExists && JSON.stringify(current) === JSON.stringify(result)) return
  if (currentExists) lines.push({ kind: 'remove', path, value: rollbackDiffValue(current) })
  if (resultExists) lines.push({ kind: 'add', path, value: rollbackDiffValue(result) })
}

function rollbackStatusLabel(status: AIAgentRollback['status'] | AIAgentRollbackPreview['status']): string {
  return t(`admin.aiAgent.rollbackStatuses.${status}`)
}

function rollbackStatusTone(status: AIAgentRollback['status'] | AIAgentRollbackPreview['status']): string {
  if (status === 'completed' || status === 'safe' || status === 'already_restored') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
  if (status === 'failed' || status === 'partial_failure' || status === 'conflict' || status === 'unavailable') return 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300'
  if (status === 'running') return 'bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-300'
  return 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300'
}

function buildRollbackDiffGroups(preview: AIAgentRollbackPreview | null): RollbackDiffGroup[] {
  if (!preview?.fields?.length) return []
  const groups = new Map<string, RollbackDiffGroup>()
  for (const field of preview.fields) {
    const resource = field.resource || preview.rollback.resource
    const targetLabel = field.target_label || preview.rollback.target_label
    const targetID = field.target_id || preview.rollback.target_id
    const operation = field.operation || preview.rollback.operation
    const identity = targetID || targetLabel || operation
    const key = `${resource || 'resource'}:${identity}`
    let group = groups.get(key)
    if (!group) {
      group = { key, resource, targetLabel, targetID, operations: [], fields: [] }
      groups.set(key, group)
    }
    if (operation && !group.operations.includes(operation)) group.operations.push(operation)
    group.fields.push(field)
  }
  return [...groups.values()]
}

function rollbackResourceLabel(resource?: string): string {
  if (!resource) return ''
  const key = pendingResourceTranslationKeys[resource]
  return key ? t(`admin.aiAgent.resources.${key}`) : resource
}

function rollbackTargetName(rollback: AIAgentRollback): string {
  if (rollback.target_label) return rollback.target_label
  const resource = rollbackResourceLabel(rollback.resource)
  if (resource && rollback.target_id) return t('admin.aiAgent.rollbackTargetFallback', { resource, id: rollback.target_id })
  return rollback.operation
}

function rollbackTargetMeta(rollback: AIAgentRollback): string {
  const parts = [rollbackResourceLabel(rollback.resource)]
  if (rollback.target_id) parts.push(t('admin.aiAgent.rollbackTargetID', { id: rollback.target_id }))
  return parts.filter(Boolean).join(' · ')
}

function rollbackGroupTargetName(group: RollbackDiffGroup): string {
  if (group.targetLabel) return group.targetLabel
  if (group.targetID) return t('admin.aiAgent.rollbackTargetFallback', { resource: rollbackResourceLabel(group.resource) || t('admin.aiAgent.rollbackResource'), id: group.targetID })
  return t('admin.aiAgent.rollbackUnknownTarget')
}

function rollbackConsequence(rollback: AIAgentRollback): string {
  if (rollback.strategy === 'delete_created') return t('admin.aiAgent.rollbackDeleteSummary', { target: rollbackTargetName(rollback) })
  if (rollback.strategy === 'rollback_plan') return t('admin.aiAgent.rollbackPlanSummary', { count: rollback.child_count || 0 })
  return t('admin.aiAgent.rollbackFieldSummary', { count: rollback.changes?.length || 0 })
}

function rollbackDialogTitle(): string {
  const rollback = selectedRollback.value
  if (!rollback) return t('admin.aiAgent.rollbackReviewTitle')
  return t('admin.aiAgent.rollbackTargetTitle', { target: rollbackTargetName(rollback) })
}

function rollbackConfirmLabel(): string {
  const preview = rollbackPreview.value
  if (!preview) return t('admin.aiAgent.rollback')
  if (preview.action === 'delete_created') return t('admin.aiAgent.rollbackDeleteAction', { target: rollbackTargetName(preview.rollback) })
  if (preview.action === 'rollback_plan') return t('admin.aiAgent.rollbackPlanAction')
  return t('admin.aiAgent.rollbackFieldsAction', { count: preview.change_count || 0 })
}

const pendingFieldTranslationKeys: Record<string, string> = {
  name: 'name',
  title: 'title',
  email: 'email',
  platform: 'platform',
  type: 'type',
  status: 'status',
  subscription_type: 'subscriptionType',
  is_exclusive: 'isExclusive',
  validity_days: 'validityDays',
  rate_multiplier: 'rateMultiplier',
  concurrency: 'concurrency',
  priority: 'priority',
  group_ids: 'groups',
  proxy_id: 'proxy',
  expires_at: 'expiresAt',
  enabled: 'enabled',
  api_key: 'apiKey',
  access_token: 'accessToken',
  refresh_token: 'refreshToken',
  password: 'password',
  credentials: 'credentials'
}

function pendingFieldLabel(field: string): string {
  const leaf = field.split('.').pop() || field
  const key = pendingFieldTranslationKeys[leaf]
  return key ? t(`admin.aiAgent.fields.${key}`) : field
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return t('admin.aiAgent.notSet')
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

async function scrollToBottom() {
  await nextTick()
  if (messagePane.value) messagePane.value.scrollTop = messagePane.value.scrollHeight
}

onMounted(loadInitial)
onUnmounted(() => {
  if (pollTimer) clearTimeout(pollTimer)
})
</script>

<style scoped>
.agent-message {
  animation: agent-message-in 220ms ease-out both;
}

.agent-mark--active,
.agent-thinking-avatar {
  animation: agent-breathe 1.8s ease-in-out infinite;
}

.agent-thinking-bars {
  display: inline-flex;
  width: 14px;
  height: 12px;
  align-items: center;
  justify-content: center;
  gap: 2px;
}

.agent-thinking-bars > span {
  width: 2px;
  height: 4px;
  border-radius: 999px;
  background: white;
  transform-origin: center;
  animation: agent-wave 900ms ease-in-out infinite;
}

.agent-thinking-bars > span:nth-child(2) {
  animation-delay: 120ms;
}

.agent-thinking-bars > span:nth-child(3) {
  animation-delay: 240ms;
}

.agent-markdown :deep(> :first-child) { margin-top: 0; }
.agent-markdown :deep(> :last-child) { margin-bottom: 0; }
.agent-markdown :deep(p) { margin: 0 0 0.65rem; }

.agent-markdown :deep(h1),
.agent-markdown :deep(h2),
.agent-markdown :deep(h3),
.agent-markdown :deep(h4),
.agent-markdown :deep(h5),
.agent-markdown :deep(h6) {
  margin: 1rem 0 0.45rem;
  color: rgb(17 24 39);
  font-weight: 650;
  line-height: 1.35;
}

.dark .agent-markdown :deep(h1),
.dark .agent-markdown :deep(h2),
.dark .agent-markdown :deep(h3),
.dark .agent-markdown :deep(h4),
.dark .agent-markdown :deep(h5),
.dark .agent-markdown :deep(h6) { color: rgb(243 244 246); }
.agent-markdown :deep(h1) { font-size: 1.2rem; }
.agent-markdown :deep(h2) { font-size: 1.1rem; }
.agent-markdown :deep(h3) { font-size: 1rem; }
.agent-markdown :deep(h4),
.agent-markdown :deep(h5),
.agent-markdown :deep(h6) { font-size: 0.925rem; }

.agent-markdown :deep(ul),
.agent-markdown :deep(ol) {
  margin: 0.5rem 0 0.75rem;
  padding-left: 1.35rem;
}

.agent-markdown :deep(ul) { list-style: disc; }
.agent-markdown :deep(ol) { list-style: decimal; }
.agent-markdown :deep(li) { margin: 0.2rem 0; }
.agent-markdown :deep(li > ul),
.agent-markdown :deep(li > ol) { margin: 0.2rem 0; }

.agent-markdown :deep(a) {
  color: rgb(5 150 105);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.agent-markdown :deep(blockquote) {
  margin: 0.75rem 0;
  border-left: 3px solid rgb(16 185 129);
  padding-left: 0.8rem;
  color: rgb(75 85 99);
}
.dark .agent-markdown :deep(blockquote) { color: rgb(209 213 219); }

.agent-markdown :deep(code) {
  border-radius: 4px;
  background: rgb(229 231 235 / 0.8);
  padding: 0.12rem 0.35rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.82em;
}
.dark .agent-markdown :deep(code) { background: rgb(55 65 81 / 0.9); }

.agent-markdown :deep(pre) {
  max-width: 100%;
  overflow-x: auto;
  margin: 0.75rem 0;
  border-radius: 6px;
  background: rgb(17 24 39);
  padding: 0.8rem;
  color: rgb(243 244 246);
  line-height: 1.55;
}

.agent-markdown :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
  font-size: 0.78rem;
}

.agent-markdown :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  margin: 0.75rem 0;
  border-collapse: collapse;
}

.agent-markdown :deep(th),
.agent-markdown :deep(td) {
  border: 1px solid rgb(209 213 219);
  padding: 0.4rem 0.55rem;
  text-align: left;
  white-space: nowrap;
}
.dark .agent-markdown :deep(th),
.dark .agent-markdown :deep(td) { border-color: rgb(75 85 99); }
.agent-markdown :deep(th) { background: rgb(229 231 235 / 0.65); font-weight: 600; }
.dark .agent-markdown :deep(th) { background: rgb(55 65 81 / 0.75); }
.agent-markdown :deep(hr) { margin: 0.9rem 0; border-color: rgb(209 213 219); }

.agent-stream-cursor {
  display: inline-block;
  width: 2px;
  height: 1em;
  margin-left: 2px;
  vertical-align: -0.12em;
  background: currentColor;
  animation: agent-cursor 800ms steps(1, end) infinite;
}

.agent-thinking-dots {
  display: inline-flex;
  width: 22px;
  height: 14px;
  align-items: flex-end;
  justify-content: center;
  gap: 3px;
}

.agent-thinking-dots > span {
  width: 3px;
  height: 3px;
  border-radius: 999px;
  background: currentColor;
  opacity: 0.35;
  animation: agent-dot 1.1s ease-in-out infinite;
}

.agent-thinking-dots > span:nth-child(2) {
  animation-delay: 140ms;
}

.agent-thinking-dots > span:nth-child(3) {
  animation-delay: 280ms;
}

@keyframes agent-message-in {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes agent-breathe {
  0%, 100% {
    box-shadow: 0 0 0 0 rgb(16 185 129 / 0), 0 1px 2px rgb(15 23 42 / 0.08);
  }
  50% {
    box-shadow: 0 0 0 5px rgb(16 185 129 / 0.12), 0 3px 8px rgb(15 23 42 / 0.12);
  }
}

@keyframes agent-wave {
  0%, 100% {
    height: 4px;
    opacity: 0.65;
  }
  50% {
    height: 11px;
    opacity: 1;
  }
}

@keyframes agent-cursor {
  0%, 45% { opacity: 1; }
  46%, 100% { opacity: 0; }
}

@keyframes agent-dot {
  0%, 60%, 100% {
    transform: translateY(0);
    opacity: 0.3;
  }
  30% {
    transform: translateY(-4px);
    opacity: 0.85;
  }
}

@media (prefers-reduced-motion: reduce) {
  .agent-message,
  .agent-mark--active,
  .agent-thinking-avatar,
  .agent-thinking-bars > span,
  .agent-thinking-dots > span,
  .agent-stream-cursor {
    animation: none;
  }
}
</style>
