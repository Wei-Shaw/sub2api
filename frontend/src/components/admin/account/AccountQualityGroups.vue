<template>
  <section class="quality-workspace">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <div class="flex items-center gap-2">
          <span class="h-2 w-2 rounded-full bg-emerald-500"></span
          ><span
            class="text-xs font-medium uppercase tracking-widest text-gray-500"
            >{{ t('accountQuality.policyModes') }}</span
          >
        </div>
        <h2
          class="mt-2 text-2xl font-semibold tracking-tight text-gray-900 dark:text-white"
        >
          {{ t('accountQuality.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500">
          {{ t('accountQuality.subtitle') }}
        </p>
      </div>
      <div class="flex gap-2">
        <button class="btn btn-secondary" :disabled="loading" @click="refresh">
          {{ t('accountQuality.refresh') }}</button
        ><button class="btn btn-primary" @click="emit('create')">
          + {{ t('accountQuality.create') }}
        </button>
      </div>
    </div>
    <div class="mt-5 grid grid-cols-3 gap-3">
      <div
        v-for="stat in overview"
        :key="stat.label"
        class="rounded-xl border border-gray-200 bg-white px-5 py-4 dark:border-dark-700 dark:bg-dark-800"
      >
        <p class="text-xs text-gray-500">{{ stat.label }}</p>
        <p
          class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white"
        >
          {{ stat.value }}
        </p>
      </div>
    </div>
    <div class="mt-4 flex flex-wrap items-center gap-3">
      <select
        v-model="benchmark"
        class="input w-auto"
        :aria-label="t('accountQuality.benchmark')"
      >
        <option value="candy">{{ t('accountQuality.benchmarkCandy') }}</option>
        <option value="modeltrace">
          {{ t('accountQuality.benchmarkTrace') }}
        </option>
        <option value="context">
          {{ t('accountQuality.benchmarkContext') }}
        </option>
      </select>
      <input
        v-model="search"
        class="input min-w-[220px] flex-1"
        :placeholder="t('accountQuality.search')"
      /><select v-model="platform" class="input w-auto">
        <option value="">{{ t('accountQuality.all') }}</option>
        <option v-for="p in platforms" :key="p" :value="p">
          {{ p }}
        </option></select
      ><select
        v-model="status"
        class="input w-auto"
        :aria-label="t('accountQuality.statusFilter')"
      >
        <option value="">{{ t('accountQuality.allStatuses') }}</option>
        <option value="available">{{ t('accountQuality.active') }}</option>
        <option value="unavailable">{{ t('accountQuality.disabled') }}</option>
        <option value="error">{{ t('accountQuality.errorStatus') }}</option>
      </select>
      <select v-model="sort" class="input w-auto">
        <option value="score">{{ t('accountQuality.sortScore') }}</option>
        <option value="ttft">{{ t('accountQuality.sortTTFT') }}</option>
        <option value="cache">{{ t('accountQuality.sortCache') }}</option>
        <option value="name">{{ t('accountQuality.sortName') }}</option>
      </select>
    </div>
    <p class="my-3 text-xs leading-5 text-gray-500">
      {{ t('accountQuality.observeHint') }}
    </p>
    <p
      v-if="error"
      role="alert"
      class="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700"
    >
      {{ error }}
    </p>
    <p
      v-if="notice"
      role="status"
      class="mb-4 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-700"
    >
      {{ notice }}
    </p>
    <div v-if="loading" class="py-10 text-center text-gray-500">
      {{ t('accountQuality.loading') }}
    </div>
    <div
      v-else-if="!visibleGroups.length"
      class="rounded-xl border border-dashed p-12 text-center text-gray-500"
    >
      {{ t('accountQuality.empty') }}
    </div>
    <div v-else class="space-y-5">
      <article
        v-for="g in visibleGroups"
        :key="g.id"
        class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <header
          class="flex flex-wrap items-center gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700"
        >
          <input
            type="checkbox"
            :checked="isGroupPageSelected(g.id)"
            :disabled="!pages[g.id]?.items.length"
            :aria-label="
              t('accountQuality.selectGroupPage', { name: groupName(g) })
            "
            @change="toggleGroupSelection(g.id)"
          /><button
            class="flex min-w-0 items-center gap-3 text-left"
            :aria-expanded="!collapsed[g.id]"
            @click="toggle(g)"
          >
            <span
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-sm font-semibold text-primary-700 dark:bg-primary-900/30"
              >{{ g.id || '—' }}</span
            ><span
              ><span
                class="block font-semibold text-gray-900 dark:text-white"
                >{{ groupName(g) }}</span
              ><span class="text-xs text-gray-500"
                >{{ g.platform }} · {{ g.total }} · {{ g.available }}
                {{ t('accountQuality.available') }}</span
              ></span
            >
          </button>
          <div class="ml-auto flex flex-wrap items-center gap-2">
            <span class="mr-2 text-xs text-gray-400">{{
              g.policy.enabled
                ? g.policy.interval_minutes +
                  ' ' +
                  t('accountQuality.minutes') +
                  ' · ' +
                  date(g.next_run_at)
                : t('accountQuality.notScheduled')
            }}</span
            ><template v-if="g.id"
              ><button
                class="btn btn-secondary text-xs"
                @click="openSettings(g)"
              >
                {{ t('accountQuality.settings') }}</button
              ><button
                class="btn btn-secondary text-xs"
                :disabled="!!activeRun"
                @click="requestRun(g)"
              >
                {{ t('accountQuality.inspect') }}
              </button></template
            ><button
              v-if="g.id"
              class="btn btn-secondary text-xs"
              @click="openRouting(g)"
            >
              {{ t('accountQuality.routingDiagnostics') }}</button
            ><button class="px-2 text-xs text-gray-500" @click="toggle(g)">
              {{
                t(
                  collapsed[g.id]
                    ? 'accountQuality.expand'
                    : 'accountQuality.collapse'
                )
              }}</button
            ><button
              v-if="g.id"
              class="btn btn-secondary text-xs"
              @click="openHistory(g, 0)"
            >
              {{ t('accountQuality.history') }}
            </button>
          </div>
        </header>
        <div v-if="!collapsed[g.id]">
          <div
            class="flex flex-wrap justify-between gap-2 bg-gray-50/70 px-5 py-2 text-xs text-gray-500 dark:bg-dark-900/30"
          >
            <span
              >{{ t('accountQuality.window') }} ·
              {{ g.policy.model || t('accountQuality.allModels')
              }}<span v-if="g.policy.model">
                / {{ g.policy.effort }} /
                {{
                  g.policy.service_tier || t('accountQuality.standard')
                }}</span
              ></span
            ><span
              >{{ t('accountQuality.mode.' + g.policy.mode)
              }}<span v-if="pages[g.id]?.summary" class="ml-3">
                ·
                {{
                  t('accountQuality.groupSummary', {
                    scored: pages[g.id].summary.scored,
                    pending: pages[g.id].summary.pending,
                    review: pages[g.id].summary.review
                  })
                }}</span
              ></span
            >
          </div>
          <p
            v-if="rowErrors[g.id]"
            role="alert"
            class="p-5 text-sm text-red-600"
          >
            {{ rowErrors[g.id] }}
            <button class="underline" @click="load(g)">
              {{ t('accountQuality.refresh') }}
            </button>
          </p>
          <div
            v-else-if="busy[g.id] && !pages[g.id]"
            class="p-8 text-center text-sm text-gray-500"
          >
            {{ t('accountQuality.loading') }}
          </div>
          <div v-else class="overflow-x-auto">
            <table class="w-full whitespace-nowrap text-left text-sm">
              <thead class="text-[11px] uppercase tracking-wide text-gray-400">
                <tr>
                  <th class="px-5 py-3">{{ t('accountQuality.account') }}</th>
                  <th class="px-3 py-3">{{ t('admin.accounts.columns.capacity') }}</th>
                  <th class="px-3 py-3" :title="t('accountQuality.schedulingHint')">{{ t('admin.accounts.columns.schedulable') }}</th>
                  <th class="px-3 py-3">{{ t('accountQuality.candy') }}</th>
                  <th class="px-3 py-3">{{ t('accountQuality.ttft') }}</th>
                  <th class="px-3 py-3">{{ t('accountQuality.cache') }}</th>
                  <th class="px-3 py-3">{{ t('accountQuality.stability') }}</th>
                  <th class="px-3 py-3">{{ t('accountQuality.score') }}</th>
                  <th class="px-3 py-3">{{ t('accountQuality.weight') }}</th>
                  <th class="px-5 py-3 text-right">
                    {{ t('accountQuality.actions') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr
                  v-for="a in pages[g.id]?.items || []"
                  :key="a.id"
                  class="hover:bg-gray-50/80 dark:hover:bg-dark-700/30"
                >
                  <td class="px-5 py-4">
                    <div class="flex items-center gap-2">
                      <input
                        type="checkbox"
                        :checked="props.selectedIds.includes(a.id)"
                        :aria-label="
                          t('accountQuality.selectAccount', { name: a.name })
                        "
                        @change="toggleSelection(a)"
                      />
                      <span
                        :class="
                          a.status === 'active' && a.schedulable
                            ? 'bg-emerald-500'
                            : 'bg-gray-300'
                        "
                        class="h-1.5 w-1.5 rounded-full"
                      ></span
                      ><span
                        class="max-w-[260px] truncate font-medium text-gray-900 dark:text-white"
                        :title="a.name"
                        >{{ a.name }}</span
                      >
                    </div>
                    <div class="mt-1 text-[11px] text-gray-400">
                      #{{ a.id }} · {{ a.type }} ·
                      {{
                        t(
                          a.status === 'active' && a.schedulable
                            ? 'accountQuality.active'
                            : 'accountQuality.disabled'
                        )
                      }}
                    </div>
                  </td>
                  <td class="px-3 py-4" :title="t('accountQuality.capacityHint')">
                    <AccountCapacityCell v-if="a.capacity" :account="a.capacity" />
                    <span v-else class="font-mono text-xs text-gray-400">— / {{ a.concurrency }}</span>
                  </td>
                  <td class="px-3 py-4">
                    <button
                      role="switch"
                      :aria-checked="a.schedulable"
                      :aria-label="t('accountQuality.toggleScheduling', { name: a.name })"
                      :aria-busy="!!scheduling[a.id]"
                      :disabled="!!scheduling[a.id]"
                      :title="t('accountQuality.schedulingHint')"
                      class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-wait disabled:opacity-50 dark:focus:ring-offset-dark-800"
                      :class="a.schedulable ? 'bg-primary-500 hover:bg-primary-600' : 'bg-gray-200 hover:bg-gray-300 dark:bg-dark-600 dark:hover:bg-dark-500'"
                      @click="toggleScheduling(a)"
                    >
                      <span class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform" :class="a.schedulable ? 'translate-x-4' : 'translate-x-0'" />
                    </button>
                    <p class="mt-1 text-[10px] text-gray-500">{{ t(a.schedulable ? 'admin.accounts.schedulableEnabled' : 'admin.accounts.schedulableDisabled') }}</p>
                    <p v-if="schedulingErrors[a.id]" role="alert" class="mt-1 max-w-40 whitespace-normal text-xs text-red-600">{{ schedulingErrors[a.id] }}</p>
                  </td>
                  <td class="px-3 py-4">
                    <button
                      class="rounded-full px-2 py-1 text-xs"
                      :class="verdictClass(a.candy_status)"
                      @click="openHistory(g, a.id)"
                    >
                      {{ verdict(a.candy_status) }}
                    </button>
                    <div
                      v-if="a.trace_status && a.trace_status !== 'untested'"
                      class="mt-1 text-[10px] text-gray-500"
                    >
                      {{ t('accountQuality.trace') }}:
                      {{ verdict(a.trace_status)
                      }}<span v-if="a.trace_candidate">
                        · {{ a.trace_candidate }}</span
                      >
                    </div>
                    <div
                      v-if="a.quality_samples"
                      class="mt-1 text-[10px] text-gray-400"
                    >
                      {{ a.quality_passed }}/{{ a.quality_samples }} ·
                      {{ date(a.last_test_at) }}
                    </div>
                  </td>
                  <td class="px-3 py-4">
                    <div class="font-mono text-xs">
                      {{ latency(a.p50_ms) }} / {{ latency(a.p95_ms) }}
                    </div>
                    <div
                      class="mt-1 text-[10px] text-gray-400"
                      :title="t('accountQuality.sampleHint')"
                    >
                      {{ a.requests }} / {{ a.ttft_samples }}
                      {{ t('accountQuality.samples') }}
                    </div>
                  </td>
                  <td class="px-3 py-4 font-mono text-xs">
                    {{ percent(a.cache_rate) }}
                  </td>
                  <td class="px-3 py-4">
                    <div class="font-mono text-xs">
                      {{ percent(a.success_rate) }}
                    </div>
                    <div class="mt-1 text-[10px] text-gray-400">
                      {{ a.requests }} / {{ a.failures
                      }}<span v-if="!a.reliability_known">
                        · {{ t('accountQuality.coverageUnknown') }}</span
                      >
                    </div>
                  </td>
                  <td class="px-3 py-4">
                    <span
                      v-if="a.score !== null"
                      class="cursor-pointer font-mono font-semibold text-primary-600 underline decoration-dotted"
                      @click="explanation = a"
                      >{{ a.score.toFixed(1) }}</span
                    ><span v-else class="text-xs text-gray-400">{{
                      t('accountQuality.noData')
                    }}</span>
                  </td>
                  <td class="px-3 py-4 font-mono text-xs text-gray-500">
                    {{ percent(a.suggested_weight) }}
                    <div
                      v-if="g.policy.mode === 'auto'"
                      class="mt-1 text-[10px] text-primary-600"
                    >
                      {{
                        a.effective_multiplier == null
                          ? t('accountQuality.notApplied')
                          : a.effective_multiplier.toFixed(2) + '×'
                      }}
                    </div>
                  </td>
                  <td class="px-5 py-4">
                    <div class="flex justify-end gap-3 text-xs">
                      <button
                        class="text-primary-600 hover:underline"
                        @click="emit('edit', a.id)"
                      >
                        {{ t('accountQuality.edit') }}</button
                      ><button
                        class="text-gray-500 hover:underline"
                        @click="emit('test', a.id)"
                      >
                        {{ t('accountQuality.test') }}</button
                      ><button
                        v-if="g.id && a.platform === 'openai'"
                        :disabled="
                          !!activeRun || a.status !== 'active' || !a.schedulable
                        "
                        class="text-primary-600 hover:underline disabled:opacity-40"
                        @click="requestRun(g, a.id)"
                      >
                        {{
                          t(
                            benchmark === 'modeltrace'
                              ? 'accountQuality.benchmarkTrace'
                              : benchmark === 'context'
                                ? 'accountQuality.benchmarkContext'
                                : 'accountQuality.candy'
                          )
                        }}</button
                      ><button
                        class="text-gray-500 hover:underline"
                        @click="openHistory(g, a.id)"
                      >
                        {{ t('accountQuality.history') }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
            <p
              v-if="!pages[g.id]?.items.length && !busy[g.id]"
              class="p-6 text-center text-sm text-gray-400"
            >
              {{ t('accountQuality.noRows') }}
            </p>
          </div>
          <footer
            class="flex items-center justify-between border-t border-gray-100 px-5 py-3 text-xs text-gray-400 dark:border-dark-700"
          >
            <span
              >{{ pages[g.id]?.total || 0 }} ·
              {{ t('accountQuality.account') }}</span
            >
            <div class="flex items-center gap-3">
              <button
                :disabled="busy[g.id] || (pages[g.id]?.page || 1) <= 1"
                class="disabled:opacity-30"
                @click="load(g, (pages[g.id]?.page || 1) - 1)"
              >
                {{ t('accountQuality.previous') }}</button
              ><span
                >{{ pages[g.id]?.page || 1 }} /
                {{
                  Math.max(1, Math.ceil((pages[g.id]?.total || 0) / 8))
                }}</span
              ><button
                :disabled="
                  busy[g.id] ||
                  (pages[g.id]?.page || 1) * 8 >= (pages[g.id]?.total || 0)
                "
                class="disabled:opacity-30"
                @click="load(g, (pages[g.id]?.page || 1) + 1)"
              >
                {{ t('accountQuality.next') }}
              </button>
            </div>
          </footer>
        </div>
      </article>
    </div>
    <p class="mt-4 text-xs leading-5 text-gray-400">
      {{ t('accountQuality.metricHint') }}
    </p>
    <div
      v-if="activeRun"
      class="sticky bottom-4 mt-4 flex items-center justify-between rounded-xl border border-primary-200 bg-primary-50 px-5 py-3 text-sm text-primary-800 shadow-lg dark:bg-dark-700 dark:text-white"
    >
      <span
        >{{ verdict(activeRun.status) }} ·
        {{ t('accountQuality.runProgress') }} {{ activeRun.completed }} / ≤{{
          activeRun.planned
        }}</span
      ><button class="underline" @click="cancelRound">
        {{ t('accountQuality.cancelRun') }}
      </button>
    </div>
    <Teleport to="body"
      ><div
        v-if="settingsGroup || historyGroup || pendingRun"
        class="fixed inset-0 z-[70] flex items-center justify-center bg-black/40 p-4"
        @click.self="closeModal"
      >
        <section
          role="dialog"
          aria-modal="true"
          :aria-label="
            settingsGroup
              ? t('accountQuality.settings')
              : pendingRun
                ? t('accountQuality.confirmRun')
                : t('accountQuality.historyTitle')
          "
          class="max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-2xl bg-white p-6 shadow-2xl dark:bg-dark-800"
        >
          <template v-if="settingsGroup"
            ><h3 class="text-lg font-semibold">
              {{ t('accountQuality.settings') }} ·
              {{ groupName(settingsGroup) }}
            </h3>
            <p class="mt-2 text-xs leading-5 text-gray-500">
              {{ t('accountQuality.settingsHint') }}
            </p>
            <form class="mt-5 space-y-4" @submit.prevent="save">
              <label class="block text-sm"
                >{{ t('accountQuality.routingMode')
                }}<select v-model="draft.mode" class="input mt-1 w-full">
                  <option value="manual">
                    {{ t('accountQuality.mode.manual') }}
                  </option>
                  <option value="observe">
                    {{ t('accountQuality.mode.observe') }}
                  </option>
                  <option value="auto">
                    {{ t('accountQuality.mode.auto') }}
                  </option></select
                ><span class="mt-1 block text-xs leading-5 text-gray-500">{{
                  t('accountQuality.autoHint')
                }}</span></label
              ><label class="block text-sm"
                >{{ t('accountQuality.endpoint')
                }}<select v-model="draft.endpoint" class="input mt-1 w-full">
                  <option value="responses">OpenAI Responses</option>
                  <option value="chat_completions">
                    OpenAI Chat Completions
                  </option>
                  <option value="messages">Anthropic Messages</option>
                  <option value="generate_content">
                    Gemini generateContent
                  </option>
                </select></label
              >
              <fieldset
                class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
              >
                <legend class="px-2 text-sm">
                  {{ t('accountQuality.componentWeights') }}
                </legend>
                <div class="grid grid-cols-2 gap-3">
                  <label
                    v-for="key in [
                      'quality',
                      'ttft',
                      'cache',
                      'stability'
                    ] as const"
                    :key="key"
                    class="text-xs"
                    >{{ t('accountQuality.component.' + key)
                    }}<input
                      v-model.number="draft.score_weights[key]"
                      class="input mt-1 w-full"
                      type="number"
                      min="0"
                      max="100"
                      required
                  /></label>
                </div>
              </fieldset>
              <label class="flex items-center gap-2 text-sm"
                ><input v-model="draft.enabled" type="checkbox" />{{
                  t('accountQuality.enabled')
                }}</label
              ><label class="block text-sm"
                >{{ t('accountQuality.model')
                }}<input
                  v-model.trim="draft.model"
                  class="input mt-1 w-full"
                  maxlength="200"
                /><span class="mt-1 block text-xs text-gray-400">{{
                  t('accountQuality.modelHint')
                }}</span></label
              >
              <div class="grid grid-cols-2 gap-4">
                <label class="text-sm"
                  >{{ t('accountQuality.effort')
                  }}<select v-model="draft.effort" class="input mt-1 w-full">
                    <option
                      v-for="e in [
                        'unspecified',
                        'none',
                        'low',
                        'medium',
                        'high',
                        'xhigh',
                        'max'
                      ]"
                      :key="e"
                    >
                      {{ e }}
                    </option>
                  </select></label
                ><label class="text-sm"
                  >{{ t('accountQuality.tier')
                  }}<select
                    v-model="draft.service_tier"
                    class="input mt-1 w-full"
                  >
                    <option value="">{{ t('accountQuality.standard') }}</option>
                    <option value="priority">priority</option>
                    <option value="flex">flex</option>
                  </select></label
                ><label class="text-sm"
                  >{{ t('accountQuality.intervalLabel')
                  }}<input
                    v-model.number="draft.interval_minutes"
                    class="input mt-1 w-full"
                    type="number"
                    min="5"
                    max="1440"
                    required /></label
                ><label class="text-sm"
                  >{{ t('accountQuality.budget')
                  }}<input
                    v-model.number="draft.daily_budget"
                    class="input mt-1 w-full"
                    type="number"
                    min="1"
                    max="100000"
                    required /></label
                ><label class="text-sm"
                  >{{ t('accountQuality.roundLimit')
                  }}<input
                    v-model.number="draft.max_accounts"
                    class="input mt-1 w-full"
                    type="number"
                    min="1"
                    max="200"
                    required
                /></label>
              </div>
              <div
                class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
              >
                <label class="block text-sm"
                  >{{ t('accountQuality.referenceModel')
                  }}<select
                    v-model="draft.reference_model"
                    class="input mt-1 w-full"
                  >
                    <option value="">
                      {{ t('accountQuality.chooseReference') }}
                    </option>
                    <option v-for="m in referenceModels" :key="m" :value="m">
                      {{ m }}
                    </option>
                  </select></label
                ><label class="flex items-center gap-2 text-sm"
                  ><input v-model="draft.trace_on_anomaly" type="checkbox" />{{
                    t('accountQuality.traceOnAnomaly')
                  }}</label
                ><label class="flex items-center gap-2 text-sm"
                  ><input v-model="draft.context_check" type="checkbox" />{{
                    t('accountQuality.contextCheck')
                  }}</label
                ><label v-if="draft.context_check" class="block text-sm"
                  >{{ t('accountQuality.contextLength')
                  }}<input
                    v-model.number="draft.context_chars"
                    class="input mt-1 w-full"
                    type="number"
                    min="4000"
                    max="128000"
                    required
                /></label>
                <p class="text-xs leading-5 text-gray-500">
                  {{ t('accountQuality.fingerprintNotice') }}
                </p>
              </div>
              <p
                class="rounded-lg bg-amber-50 p-3 text-xs leading-5 text-amber-800"
              >
                {{ t('accountQuality.runHint')
                }}<span class="mt-2 block">{{
                  t('accountQuality.budgetForecast', {
                    requests:
                      Math.ceil(1440 / Math.max(5, draft.interval_minutes)) *
                      Math.min(
                        settingsGroup?.available || 0,
                        draft.max_accounts
                      ) *
                      (3 +
                        (draft.trace_on_anomaly ? 3 : 0) +
                        (draft.context_check ? 3 : 0))
                  })
                }}</span>
              </p>
              <p v-if="modalError" role="alert" class="text-sm text-red-600">
                {{ modalError }}
              </p>
              <div class="flex justify-end gap-3">
                <button
                  type="button"
                  class="btn btn-secondary"
                  @click="closeModal"
                >
                  {{ t('accountQuality.cancel') }}</button
                ><button class="btn btn-primary" :disabled="saving">
                  {{
                    t(saving ? 'accountQuality.saving' : 'accountQuality.save')
                  }}
                </button>
              </div>
            </form></template
          >
          <template v-else-if="pendingRun"
            ><h3 class="text-lg font-semibold">
              {{ t('accountQuality.confirmRun') }}
            </h3>
            <p class="mt-3 text-sm leading-6 text-gray-500">
              {{ t('accountQuality.confirmHint') }}
            </p>
            <p class="mt-3 rounded-lg bg-amber-50 p-3 text-xs text-amber-800">
              {{ t('accountQuality.runHint') }} · {{ pendingRun?.benchmark }}
            </p>
            <p v-if="modalError" role="alert" class="mt-3 text-sm text-red-600">
              {{ modalError }}
            </p>
            <div class="mt-6 flex justify-end gap-3">
              <button class="btn btn-secondary" @click="closeModal">
                {{ t('accountQuality.cancel') }}</button
              ><button
                class="btn btn-primary"
                :disabled="saving"
                @click="startRun"
              >
                {{ t('accountQuality.start') }}
              </button>
            </div></template
          >
          <template v-else
            ><div class="flex justify-between">
              <h3 class="text-lg font-semibold">
                {{ t('accountQuality.historyTitle') }}
              </h3>
              <button @click="closeModal">
                {{ t('accountQuality.close') }}
              </button>
            </div>
            <p class="my-3 text-xs text-gray-500">
              {{ t('accountQuality.timingHint') }}
            </p>
            <details
              v-if="runHistory.length"
              class="my-4 rounded-xl border border-gray-200 p-3 dark:border-dark-600"
            >
              <summary class="cursor-pointer text-sm">
                {{ t('accountQuality.runHistory') }}
              </summary>
              <div
                v-for="run in runHistory"
                :key="run.id"
                class="mt-3 flex justify-between gap-3 border-t border-gray-100 pt-2 text-xs dark:border-dark-600"
              >
                <span
                  >#{{ run.id }} · {{ run.benchmark }} ·
                  {{ date(run.created_at) }}</span
                ><span
                  >{{ verdict(run.status) }} · {{ run.completed }} /
                  {{ run.planned }}</span
                >
              </div>
            </details>
            <p v-if="modalError" role="alert" class="my-3 text-sm text-red-600">
              {{ modalError }}
            </p>
            <p v-if="historyLoading" class="py-8 text-center text-gray-400">
              {{ t('accountQuality.loading') }}
            </p>
            <p
              v-else-if="!history.length && !modalError"
              class="py-8 text-center text-gray-400"
            >
              {{ t('accountQuality.noHistory') }}
            </p>
            <div
              v-for="r in history"
              :key="r.id"
              class="mt-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600"
            >
              <div class="flex justify-between gap-2">
                <span class="text-xs"
                  >#{{ r.account_id }} · {{ r.model }} / {{ r.effort }}</span
                ><span
                  class="rounded-full px-2 py-1 text-xs"
                  :class="verdictClass(r.verdict)"
                  >{{ verdict(r.verdict) }}</span
                >
              </div>
              <p class="mt-2 text-[11px] text-gray-400">
                {{ r.benchmark }} · {{ date(r.created_at) }} ·
                {{ (r.duration_ms / 1000).toFixed(1) }}s
              </p>
              <div
                v-if="r.evidence?.attribution"
                class="mt-3 rounded-lg bg-gray-50 p-3 text-xs leading-6 dark:bg-dark-900"
              >
                <p>
                  {{ t('accountQuality.referenceModel') }}:
                  {{ r.evidence.expected_model }}
                </p>
                <p>
                  {{ t('accountQuality.candidate') }}:
                  {{ r.evidence.attribution.prediction || '—' }}
                </p>
                <p>
                  {{ t('accountQuality.relativeSupport') }}:
                  {{ percent(100 * r.evidence.attribution.probability) }}
                </p>
                <p class="break-all">
                  {{ t('accountQuality.bankRevision') }}:
                  {{ r.evidence.bank_revision }}
                </p>
                <p class="text-gray-500">
                  {{ t('accountQuality.fingerprintNotice') }}
                </p>
              </div>
              <p v-if="r.detail" class="mt-2 text-xs text-red-600">
                {{ detailText(r.detail) }}
              </p>
              <details v-if="r.answer" class="mt-3 text-xs">
                <summary class="cursor-pointer text-primary-600">
                  {{ t('accountQuality.answer') }}
                </summary>
                <pre
                  class="mt-2 max-h-60 overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-3 dark:bg-dark-900"
                  >{{ r.answer }}</pre
                >
              </details>
            </div></template
          >
        </section>
      </div></Teleport
    >
    <Teleport to="body"
      ><div
        v-if="explanation"
        class="fixed inset-0 z-[75] flex items-center justify-center bg-black/40 p-4"
        @click.self="explanation = null"
      >
        <section
          role="dialog"
          aria-modal="true"
          :aria-label="t('accountQuality.explanation')"
          class="w-full max-w-xl rounded-2xl bg-white p-6 dark:bg-dark-800"
        >
          <div class="flex justify-between gap-4">
            <h3 class="font-semibold">
              {{ explanation.name }} · {{ t('accountQuality.explanation') }}
            </h3>
            <button @click="explanation = null">
              {{ t('accountQuality.close') }}
            </button>
          </div>
          <div
            v-for="c in explanation.components || []"
            :key="c.key"
            class="mt-4 flex justify-between border-b border-gray-100 pb-3 text-sm dark:border-dark-700"
          >
            <div>
              <p>{{ t('accountQuality.component.' + c.key) }}</p>
              <p class="mt-1 text-xs text-gray-400">
                {{ c.samples }} {{ t('accountQuality.samples') }} ·
                {{ t('accountQuality.baseAndEffective') }} {{ c.weight }}% /
                {{ c.effective_weight.toFixed(1) }}%<span class="mt-1 block">{{
                  t('accountQuality.reason.' + c.key)
                }}</span>
              </p>
            </div>
            <span class="font-mono">{{
              c.score === null ? t('accountQuality.noData') : c.score.toFixed(1)
            }}</span>
          </div>
          <p class="mt-4 text-xs leading-5 text-gray-500">
            {{ t('accountQuality.metricHint') }}
          </p>
        </section>
      </div></Teleport
    >
    <Teleport to="body"
      ><div
        v-if="routingGroup"
        class="fixed inset-0 z-[76] flex items-center justify-center bg-black/40 p-4"
        @click.self="routingGroup = null"
      >
        <section
          role="dialog"
          aria-modal="true"
          :aria-label="t('accountQuality.routingDiagnostics')"
          class="max-h-[85vh] w-full max-w-3xl overflow-auto rounded-2xl bg-white p-6 dark:bg-dark-800"
        >
          <div class="flex justify-between gap-4">
            <h3 class="font-semibold">
              {{ routingGroup.name }} ·
              {{ t('accountQuality.routingDiagnostics') }}
            </h3>
            <button @click="routingGroup = null">
              {{ t('accountQuality.close') }}
            </button>
          </div>
          <p v-if="routingError" role="alert" class="mt-4 text-sm text-red-600">
            {{ routingError }}
          </p>
          <p v-if="routingLoading" class="py-8 text-center text-gray-500">
            {{ t('accountQuality.loading') }}
          </p>
          <template v-else-if="routingReport"
            ><p class="mt-4 rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-900">
              {{ t('accountQuality.mode.' + routingReport.configured_mode) }} ·
              {{ t('accountQuality.routeReason.' + routingReport.reason) }}
            </p>
            <p class="mt-2 text-xs text-gray-500">
              {{ routingGroup.policy.model || '—' }} /
              {{ routingGroup.policy.effort }} /
              {{
                routingGroup.policy.service_tier || t('accountQuality.standard')
              }}
              / {{ routingGroup.policy.endpoint }}
            </p>
            <p class="mt-2 text-xs leading-5 text-gray-500">
              {{ t('accountQuality.routingHint') }}
            </p>
            <div
              v-if="routingReport.applied"
              class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3"
            >
              <div
                v-for="(w, id) in routingReport.weights"
                :key="id"
                class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600"
              >
                <span>#{{ id }}</span
                ><span class="float-right font-mono text-primary-600"
                  >{{ w.multiplier.toFixed(2) }}×</span
                >
                <p v-if="w.confirmed_bad" class="mt-2 text-amber-700">
                  {{ t('accountQuality.confirmedCapabilityFailure') }}
                </p>
              </div>
            </div>
            <h4 class="mt-6 text-sm font-medium">
              {{ t('accountQuality.recentRoutes') }}
            </h4>
            <p class="mt-2 text-xs text-gray-500">
              {{ t('accountQuality.eventRetention') }}
            </p>
            <p
              v-if="!routingEvents.length"
              class="py-6 text-center text-sm text-gray-400"
            >
              {{ t('accountQuality.noRoutes') }}
            </p>
            <div
              v-for="(e, i) in routingEvents"
              :key="i"
              class="mt-3 rounded-lg border border-gray-100 p-3 text-xs dark:border-dark-600"
            >
              <div class="flex justify-between">
                <span>#{{ e.account_id }} · {{ e.scope.model }}</span
                ><span class="font-mono">{{ e.multiplier.toFixed(2) }}×</span>
              </div>
              <p class="mt-1 text-gray-500">
                {{ e.reason }} · {{ date(e.created_at) }}
              </p>
              <p class="mt-1 break-all font-mono text-gray-400">
                {{ e.request_id || '—' }}
              </p>
            </div></template
          >
        </section>
      </div></Teleport
    >
  </section>
</template>
<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AccountCapacityCell from '@/components/account/AccountCapacityCell.vue'
import { setSchedulable } from '@/api/admin/accounts'
import {
  listQualityGroups,
  qualityCatalog,
  qualityRoutingPreview,
  qualityRoutingEvents,
  type QualityRoutingPreview,
  type QualityRoutingEvent,
  type QualityBenchmark,
  type QualityAccount,
  qualityPage,
  saveQualityPolicy,
  runQuality,
  qualityRuns,
  qualityResults,
  cancelQuality,
  type QualityPolicy,
  type QualityGroup,
  type QualityPage,
  type QualityRun,
  type QualityResult
} from '@/api/admin/accountQuality'
const { t, locale } = useI18n()
const props = withDefaults(
  defineProps<{ selectedIds?: number[]; refreshToken?: number }>(),
  {
    selectedIds: () => []
  }
)
const selectionMetadata = new Map<number, QualityAccount>()
const emit = defineEmits<{
  selection: [rows: QualityAccount[]]
  create: []
  edit: [id: number]
  test: [id: number]
  schedulingChange: [id: number, schedulable: boolean]
}>()
const groups = ref<QualityGroup[]>([]),
  pages = reactive<Record<number, QualityPage>>({}),
  busy = reactive<Record<number, boolean>>({}),
  rowErrors = reactive<Record<number, string>>({}),
  collapsed = reactive<Record<number, boolean>>({})
const scheduling = reactive<Record<number, boolean>>({}),
  schedulingErrors = reactive<Record<number, string>>({})
const loading = ref(false),
  error = ref(''),
  notice = ref(''),
  search = ref(''),
  platform = ref(''),
  status = ref(''),
  sort = ref('score'),
  benchmark = ref<QualityBenchmark>('candy'),
  referenceModels = ref<string[]>([]),
  explanation = ref<QualityAccount | null>(null),
  saving = ref(false),
  modalError = ref('')
const routingGroup = ref<QualityGroup | null>(null),
  routingReport = ref<QualityRoutingPreview | null>(null),
  routingEvents = ref<QualityRoutingEvent[]>([]),
  routingLoading = ref(false),
  routingError = ref('')
const settingsGroup = ref<QualityGroup | null>(null),
  historyGroup = ref<QualityGroup | null>(null),
  runHistory = ref<QualityRun[]>([]),
  history = ref<QualityResult[]>([]),
  historyLoading = ref(false),
  pendingRun = ref<{
    group: QualityGroup
    id: number
    benchmark: QualityBenchmark
  } | null>(null),
  activeRun = ref<QualityRun | null>(null)
let historyRequestVersion = 0
const defaults: QualityPolicy = {
  enabled: false,
  interval_minutes: 15,
  model: '',
  effort: 'low',
  service_tier: '',
  daily_budget: 200,
  max_accounts: 50,
  mode: 'observe',
  endpoint: 'responses',
  trace_on_anomaly: false,
  reference_model: '',
  context_check: false,
  context_chars: 32000,
  score_weights: { quality: 40, ttft: 25, cache: 15, stability: 20 }
}
const draft = reactive<QualityPolicy>({ ...defaults }),
  controllers = new Map<number, AbortController>()
let timer: ReturnType<typeof setInterval> | undefined,
  debounce: ReturnType<typeof setTimeout> | undefined,
  disposed = false
const platforms = computed(() => [
  ...new Set(groups.value.map((g) => g.platform))
])
const visibleGroups = computed(() =>
  groups.value.filter(
    (g) =>
      (!platform.value || g.platform === platform.value) &&
      (!search.value ||
        groupName(g).toLowerCase().includes(search.value.toLowerCase()) ||
        (pages[g.id]?.total || 0) > 0)
  )
)
const overview = computed(() => [
  { label: t('accountQuality.groups'), value: groups.value.length },
  {
    label: t('accountQuality.memberships'),
    value: groups.value.reduce((n, g) => n + g.available, 0)
  },
  {
    label: t('accountQuality.interval'),
    value: '15 ' + t('accountQuality.minutes')
  }
])
const groupName = (g: QualityGroup) =>
  g.id ? g.name : t('accountQuality.ungrouped')
const date = (v: string | null) =>
  v
    ? new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      }).format(new Date(v))
    : '—'
const latency = (v: number | null) =>
  v === null ? '—' : (v / 1000).toFixed(2) + 's'
const percent = (v: number | null) =>
  v == null || !Number.isFinite(v) ? '—' : v.toFixed(1) + '%'
const verdict = (v: string) => t('accountQuality.' + v)
const verdictClass = (v: string) =>
  v === 'passed'
    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    : v === 'incorrect'
      ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
      : v === 'error' || v === 'unparseable' || v === 'inconsistent'
        ? 'bg-amber-50 text-amber-700'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700'
function detailText(value: string) {
  if (value.startsWith('reused_result:'))
    return t('accountQuality.reusedEvidence', { id: value.split(':')[1] })
  const keys: Record<string, string> = {
    authentication_failed: 'auth',
    upstream_balance_exhausted: 'balance',
    access_denied: 'access',
    rate_limited: 'rate',
    timeout: 'timeout',
    cancelled: 'cancelled',
    request_rejected: 'request',
    upstream_server_error: 'server',
    upstream_or_transport_error: 'transport',
    incomplete_response: 'incomplete',
    insufficient_valid_fingerprint_outputs: 'fingerprint',
    account_not_schedulable: 'unavailable',
    unsupported_account: 'unsupported',
    use_unspecified_effort_for_native_gemini: 'unsupported',
    response_too_large: 'large'
  }
  return keys[value] ? t('accountQuality.failure.' + keys[value]) : value
}
function message(e: unknown) {
  return e instanceof Error ? e.message : t('accountQuality.failed')
}
async function load(g: QualityGroup, page = 1) {
  controllers.get(g.id)?.abort()
  const c = new AbortController()
  controllers.set(g.id, c)
  busy[g.id] = true
  rowErrors[g.id] = ''
  try {
    const q = groupName(g).toLowerCase().includes(search.value.toLowerCase())
      ? ''
      : search.value
    const data = await qualityPage(
      g.id,
      { page, page_size: 8, search: q, status: status.value, sort: sort.value },
      c.signal
    )
    if (!c.signal.aborted) {
      pages[g.id] = data
      for (const row of data.items) selectionMetadata.set(row.id, row)
    }
  } catch (e) {
    if (!c.signal.aborted) rowErrors[g.id] = message(e)
  } finally {
    if (controllers.get(g.id) === c) {
      busy[g.id] = false
      controllers.delete(g.id)
    }
  }
}
async function refresh() {
  loading.value = true
  error.value = ''
  try {
    groups.value = (await listQualityGroups()).sort(
      (a, b) => b.total - a.total || a.id - b.id
    )
    if (disposed) return
    for (let i = 0; i < groups.value.length; i++) {
      const g = groups.value[i]
      if (!(g.id in collapsed)) collapsed[g.id] = i >= 3
    }
    await Promise.all(
      groups.value
        .filter((g) => !collapsed[g.id])
        .map((g) => load(g, pages[g.id]?.page || 1))
    )
    await poll()
  } catch (e) {
    error.value = message(e)
  } finally {
    loading.value = false
  }
}
async function toggleScheduling(account: QualityAccount) {
  if (scheduling[account.id]) return
  scheduling[account.id] = true
  schedulingErrors[account.id] = ''
  let saved = false
  try {
    const updated = await setSchedulable(account.id, !account.schedulable)
    saved = true
    const affectedGroups = groups.value.filter(g => pages[g.id]?.items.some(row => row.id === account.id))
    for (const page of Object.values(pages)) {
      for (const row of page.items) {
        if (row.id === account.id) row.schedulable = updated.schedulable
      }
    }
    const selected = selectionMetadata.get(account.id)
    if (selected) selected.schedulable = updated.schedulable
    emit('schedulingChange', account.id, updated.schedulable)
    await Promise.all([
      listQualityGroups().then(data => { groups.value = data.sort((a,b) => b.total-a.total || a.id-b.id) }),
      ...affectedGroups.filter(g => !collapsed[g.id]).map(g => load(g, pages[g.id]?.page || 1))
    ])
  } catch (e) {
    if (saved) error.value = message(e)
    else schedulingErrors[account.id] = t('admin.accounts.failedToToggleSchedulable')
  } finally {
    scheduling[account.id] = false
  }
}
function emitSelection(ids: Set<number>) {
  emit(
    'selection',
    [...ids]
      .map((id) => selectionMetadata.get(id))
      .filter((row): row is QualityAccount => !!row)
  )
}
function toggleSelection(row: QualityAccount) {
  selectionMetadata.set(row.id, row)
  const ids = new Set(props.selectedIds)
  if (ids.has(row.id)) ids.delete(row.id)
  else ids.add(row.id)
  emitSelection(ids)
}
function isGroupPageSelected(id: number) {
  const rows = pages[id]?.items || []
  return (
    rows.length > 0 && rows.every((row) => props.selectedIds.includes(row.id))
  )
}
function toggleGroupSelection(id: number) {
  const rows = pages[id]?.items || []
  const remove = isGroupPageSelected(id)
  const ids = new Set(props.selectedIds)
  for (const row of rows) {
    selectionMetadata.set(row.id, row)
    if (remove) ids.delete(row.id)
    else ids.add(row.id)
  }
  emitSelection(ids)
}
function toggle(g: QualityGroup) {
  collapsed[g.id] = !collapsed[g.id]
  if (!collapsed[g.id] && !pages[g.id]) void load(g)
}
function openSettings(g: QualityGroup) {
  settingsGroup.value = g
  Object.assign(draft, JSON.parse(JSON.stringify({ ...defaults, ...g.policy })))
  modalError.value = ''
}
function closeModal() {
  if (saving.value) return
  historyRequestVersion++
  settingsGroup.value = null
  historyGroup.value = null
  pendingRun.value = null
  modalError.value = ''
}
async function save() {
  if (!settingsGroup.value) return
  saving.value = true
  modalError.value = ''
  try {
    if (!draft.reference_model && referenceModels.value.includes(draft.model)) {
      draft.reference_model = draft.model
    }
    await saveQualityPolicy(settingsGroup.value.id, { ...draft })
    saving.value = false
    closeModal()
    notice.value = t('accountQuality.saved')
    await refresh()
  } catch (e) {
    modalError.value = message(e)
  } finally {
    saving.value = false
  }
}
function requestRun(g: QualityGroup, id = 0) {
  notice.value = ''
  if (!g.policy.model) {
    openSettings(g)
    modalError.value = t('accountQuality.configure')
    return
  }
  if (benchmark.value === 'modeltrace' && !g.policy.reference_model) {
    openSettings(g)
    modalError.value = t('accountQuality.referenceRequired')
    return
  }
  pendingRun.value = { group: g, id, benchmark: benchmark.value }
  modalError.value = ''
}
async function startRun() {
  if (!pendingRun.value) return
  saving.value = true
  modalError.value = ''
  try {
    activeRun.value = await runQuality(
      pendingRun.value.group.id,
      pendingRun.value.id,
      pendingRun.value.benchmark
    )
    saving.value = false
    closeModal()
    notice.value = t('accountQuality.runQueued')
  } catch (e) {
    modalError.value = message(e)
  } finally {
    saving.value = false
  }
}
async function poll() {
  const runs = await qualityRuns(0)
  const run = runs.find((r) => r.status === 'running' || r.status === 'queued')
  if (run) {
    activeRun.value = run
    return
  }
  if (activeRun.value) {
    activeRun.value = null
    for (const g of groups.value) {
      if (!collapsed[g.id]) void load(g, pages[g.id]?.page || 1)
    }
  }
}
async function cancelRound() {
  if (!activeRun.value) return
  try {
    await cancelQuality(activeRun.value.id)
    await poll()
  } catch (e) {
    error.value = message(e)
  }
}
async function openRouting(g: QualityGroup) {
  routingGroup.value = g
  routingReport.value = null
  routingEvents.value = []
  routingError.value = ''
  routingLoading.value = true
  try {
    const [report, events] = await Promise.all([
      qualityRoutingPreview(g),
      qualityRoutingEvents(g.id)
    ])
    if (routingGroup.value?.id === g.id) {
      routingReport.value = report
      routingEvents.value = events
    }
  } catch (e) {
    routingError.value = message(e)
  } finally {
    routingLoading.value = false
  }
}
async function openHistory(g: QualityGroup, id: number) {
  const requestVersion = ++historyRequestVersion
  historyGroup.value = g
  modalError.value = ''
  history.value = []
  runHistory.value = []
  historyLoading.value = true
  try {
    const [results, runs] = await Promise.all([
      qualityResults(g.id, id),
      qualityRuns(g.id)
    ])
    if (requestVersion !== historyRequestVersion) return
    history.value = results
    runHistory.value = runs
  } catch (e) {
    modalError.value = message(e)
  } finally {
    if (requestVersion === historyRequestVersion) historyLoading.value = false
  }
}
watch(
  () => props.refreshToken,
  () => {
    void refresh()
  }
)
watch([search, sort, status], () => {
  if (debounce) clearTimeout(debounce)
  debounce = setTimeout(() => {
    for (const g of groups.value)
      if (search.value || !collapsed[g.id]) void load(g)
  }, 300)
})
onMounted(async () => {
  try {
    referenceModels.value = (await qualityCatalog()).reference_models
  } catch {
    error.value = t('accountQuality.failed')
  }
  await refresh()
  timer = setInterval(() => {
    void poll().catch((e) => {
      error.value = message(e)
    })
  }, 5000)
})
onUnmounted(() => {
  disposed = true
  historyRequestVersion++
  if (timer) clearInterval(timer)
  if (debounce) clearTimeout(debounce)
  controllers.forEach((c) => c.abort())
})
</script>
