<template>
  <BaseDialog
    :show="show"
    :title="t('admin.scheduledTests.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- Add Plan Button -->
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.title') }}
        </p>
        <button
          @click="showAddForm = !showAddForm"
          class="btn btn-primary flex items-center gap-1.5 text-sm"
        >
          <Icon name="plus" size="sm" :stroke-width="2" />
          {{ t('admin.scheduledTests.addPlan') }}
        </button>
      </div>

      <!-- Add Plan Form -->
      <div
        v-if="showAddForm"
        class="rounded-xl border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-800 dark:bg-primary-900/20"
      >
        <div class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.scheduledTests.addPlan') }}
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="sm:col-span-2">
            <div class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.triggerMode') }}
              <HelpTooltip trigger="click">
                <template #trigger>
                  <button
                    type="button"
                    :aria-label="t('admin.scheduledTests.errorRecoveryHelpAriaLabel')"
                    class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400"
                  >
                    ?
                  </button>
                </template>
                <div class="max-w-xs space-y-1.5">
                  <p>{{ t('admin.scheduledTests.errorRecoveryTooltipFailedModels') }}</p>
                  <p>{{ t('admin.scheduledTests.errorRecoveryTooltipBoundaries') }}</p>
                </div>
              </HelpTooltip>
            </div>
            <div class="inline-flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-500">
              <button
                v-for="mode in triggerModes"
                :key="mode.value"
                type="button"
                :aria-pressed="newPlan.trigger_mode === mode.value"
                :class="[
                  'rounded-md px-3 py-1.5 text-sm transition-colors',
                  newPlan.trigger_mode === mode.value
                    ? 'bg-primary-500 text-white'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-600'
                ]"
                @click="setTriggerMode(newPlan, mode.value)"
              >
                {{ mode.label }}
              </button>
            </div>
          </div>
          <div>
            <label
              :for="newPlan.trigger_mode === 'error_recovery' ? 'new-plan-error-models' : undefined"
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {{ t(newPlan.trigger_mode === 'error_recovery' ? 'admin.scheduledTests.probeModel' : 'admin.scheduledTests.model') }}
            </label>
            <ModelMultiSelect
              v-if="newPlan.trigger_mode === 'error_recovery'"
              id="new-plan-error-models"
              v-model="newPlan.model_ids"
              :options="modelOptions"
              :aria-label="t('admin.scheduledTests.probeModel')"
              :placeholder="t('admin.scheduledTests.selectModels')"
              :all-label="t('admin.scheduledTests.allFailedModels')"
              :clear-label="t('admin.scheduledTests.clearSelection')"
            />
            <Select
              v-else
              v-model="newPlan.model_id"
              :options="modelOptions"
              :placeholder="t('admin.scheduledTests.model')"
              :searchable="modelOptions.length > 5"
            />
          </div>
          <div v-if="newPlan.trigger_mode === 'scheduled'">
            <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.cronExpression') }}
              <HelpTooltip>
                <template #trigger>
                  <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                    ?
                  </span>
                </template>
                <div class="space-y-1.5">
                  <p class="font-medium">{{ t('admin.scheduledTests.cronTooltipTitle') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipMeaning') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleEvery30Min') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleHourly') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleDaily') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleWeekly') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipRange') }}</p>
                </div>
              </HelpTooltip>
            </label>
            <Input
              v-model="newPlan.cron_expression"
              :placeholder="'*/30 * * * *'"
              :hint="t('admin.scheduledTests.cronHelp')"
            />
          </div>
          <div v-else>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ newPlan.advanced_cron ? t('admin.scheduledTests.cronExpression') : t('admin.scheduledTests.retryInterval') }}
            </label>
            <Input
              v-if="!newPlan.advanced_cron"
              v-model="newPlan.retry_interval_minutes"
              type="number"
              min="1"
              max="1440"
            />
            <Input
              v-else
              v-model="newPlan.retry_cron_expression"
              :placeholder="'*/5 * * * *'"
            />
            <label class="mt-2 flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <Toggle v-model="newPlan.advanced_cron" />
              {{ t('admin.scheduledTests.advancedCron') }}
            </label>
          </div>
          <div>
            <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.maxResults') }}
              <HelpTooltip>
                <template #trigger>
                  <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                    ?
                  </span>
                </template>
                <div class="space-y-1.5">
                  <p class="font-medium">{{ t('admin.scheduledTests.maxResultsTooltipTitle') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipMeaning') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipBody') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipExample') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipRange') }}</p>
                </div>
              </HelpTooltip>
            </label>
            <Input
              v-model="newPlan.max_results"
              type="number"
              placeholder="100"
            />
          </div>
          <div v-if="newPlan.trigger_mode === 'scheduled'" class="flex items-end">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <Toggle v-model="newPlan.enabled" />
              {{ t('admin.scheduledTests.enabled') }}
            </label>
          </div>
          <div class="flex items-end">
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle v-model="newPlan.auto_recover" />
                {{ t('admin.scheduledTests.autoRecover') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                {{ t('admin.scheduledTests.autoRecoverHelp') }}
              </p>
            </div>
          </div>
        </div>
        <div class="mt-3 flex justify-end gap-2">
          <button
            @click="showAddForm = false; resetNewPlan()"
            class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            @click="handleCreate"
            :disabled="!isPlanFormValid(newPlan) || creating"
            class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Icon v-if="creating" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
            {{ t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="md" class="animate-spin text-gray-400" :stroke-width="2" />
        <span class="ml-2 text-sm text-gray-500">{{ t('common.loading') }}...</span>
      </div>

      <!-- Empty State -->
      <div
        v-else-if="plans.length === 0"
        class="rounded-xl border border-dashed border-gray-300 py-10 text-center dark:border-dark-600"
      >
        <Icon name="calendar" size="lg" class="mx-auto mb-2 text-gray-400" :stroke-width="1.5" />
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.noPlans') }}
        </p>
      </div>

      <!-- Plans List -->
      <div v-else class="space-y-3">
        <div
          v-for="plan in plans"
          :key="plan.id"
          class="rounded-xl border border-gray-200 bg-white transition-all dark:border-dark-600 dark:bg-dark-800"
        >
          <!-- Plan Header -->
          <div
            class="flex cursor-pointer items-center justify-between px-4 py-3"
            @click="toggleExpand(plan.id)"
          >
            <div class="flex flex-1 items-center gap-4">
              <!-- Model -->
              <div class="min-w-0">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-gray-100" :title="formatPlanModels(plan)">
                  {{ formatPlanModels(plan) }}
                </div>
                <div class="mt-0.5 font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ formatPlanSchedule(plan) }}
                </div>
              </div>

              <!-- Enabled Toggle -->
              <div class="flex items-center gap-1.5" @click.stop>
                <Toggle
                  :model-value="plan.enabled"
                  @update:model-value="(val: boolean) => handleToggleEnabled(plan, val)"
                />
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ plan.enabled ? t('admin.scheduledTests.enabled') : '' }}
                </span>
              </div>

              <!-- Auto Recover Badge -->
              <span
                v-if="plan.trigger_mode === 'error_recovery' || plan.auto_recover"
                class="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-400"
              >
                {{ t(plan.trigger_mode === 'error_recovery' ? 'admin.scheduledTests.errorRecoveryMode' : 'admin.scheduledTests.autoRecover') }}
              </span>
            </div>

            <div class="flex items-center gap-3">
              <!-- Last Run -->
              <div v-if="plan.last_run_at" class="hidden text-right text-xs text-gray-500 dark:text-gray-400 sm:block">
                <div>{{ t('admin.scheduledTests.lastRun') }}</div>
                <div>{{ formatDateTime(plan.last_run_at) }}</div>
              </div>

              <!-- Next Run -->
              <div v-if="plan.next_run_at" class="hidden text-right text-xs text-gray-500 dark:text-gray-400 sm:block">
                <div>{{ t('admin.scheduledTests.nextRun') }}</div>
                <div>{{ formatDateTime(plan.next_run_at) }}</div>
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-1" @click.stop>
                <button
                  @click="startEdit(plan)"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-500 dark:hover:bg-blue-900/20"
                  :title="t('admin.scheduledTests.editPlan')"
                >
                  <Icon name="edit" size="sm" :stroke-width="2" />
                </button>
                <button
                  @click="confirmDeletePlan(plan)"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                  :title="t('admin.scheduledTests.deletePlan')"
                >
                  <Icon name="trash" size="sm" :stroke-width="2" />
                </button>
              </div>

              <!-- Expand indicator -->
              <Icon
                name="chevronDown"
                size="sm"
                :class="[
                  'text-gray-400 transition-transform duration-200',
                  expandedPlanId === plan.id ? 'rotate-180' : ''
                ]"
              />
            </div>
          </div>

          <!-- Edit Form -->
          <div
            v-if="editingPlanId === plan.id"
            class="border-t border-blue-100 bg-blue-50/50 px-4 py-3 dark:border-blue-900 dark:bg-blue-900/10"
            @click.stop
          >
            <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.editPlan') }}
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div class="sm:col-span-2">
                <div class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.triggerMode') }}
                  <HelpTooltip trigger="click">
                    <template #trigger>
                      <button
                        type="button"
                        :aria-label="t('admin.scheduledTests.errorRecoveryHelpAriaLabel')"
                        class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400"
                      >
                        ?
                      </button>
                    </template>
                    <div class="max-w-xs space-y-1.5">
                      <p>{{ t('admin.scheduledTests.errorRecoveryTooltipFailedModels') }}</p>
                      <p>{{ t('admin.scheduledTests.errorRecoveryTooltipBoundaries') }}</p>
                    </div>
                  </HelpTooltip>
                </div>
                <div class="inline-flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-500">
                  <button
                    v-for="mode in triggerModes"
                    :key="mode.value"
                    type="button"
                    :aria-pressed="editForm.trigger_mode === mode.value"
                    :class="[
                      'rounded-md px-3 py-1.5 text-sm transition-colors',
                      editForm.trigger_mode === mode.value
                        ? 'bg-primary-500 text-white'
                        : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-600'
                    ]"
                    @click="setTriggerMode(editForm, mode.value)"
                  >
                    {{ mode.label }}
                  </button>
                </div>
              </div>
              <div>
                <label
                  :for="editForm.trigger_mode === 'error_recovery' ? 'edit-plan-error-models' : undefined"
                  class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                >
                  {{ t(editForm.trigger_mode === 'error_recovery' ? 'admin.scheduledTests.probeModel' : 'admin.scheduledTests.model') }}
                </label>
                <ModelMultiSelect
                  v-if="editForm.trigger_mode === 'error_recovery'"
                  id="edit-plan-error-models"
                  v-model="editForm.model_ids"
                  :options="modelOptions"
                  :aria-label="t('admin.scheduledTests.probeModel')"
                  :placeholder="t('admin.scheduledTests.selectModels')"
                  :all-label="t('admin.scheduledTests.allFailedModels')"
                  :clear-label="t('admin.scheduledTests.clearSelection')"
                />
                <Select
                  v-else
                  v-model="editForm.model_id"
                  :options="modelOptions"
                  :placeholder="t('admin.scheduledTests.model')"
                  :searchable="modelOptions.length > 5"
                />
              </div>
              <div v-if="editForm.trigger_mode === 'scheduled'">
                <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.cronExpression') }}
                  <HelpTooltip>
                    <template #trigger>
                      <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                        ?
                      </span>
                    </template>
                    <div class="space-y-1.5">
                      <p class="font-medium">{{ t('admin.scheduledTests.cronTooltipTitle') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipMeaning') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleEvery30Min') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleHourly') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleDaily') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleWeekly') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipRange') }}</p>
                    </div>
                  </HelpTooltip>
                </label>
                <Input
                  v-model="editForm.cron_expression"
                  :placeholder="'*/30 * * * *'"
                  :hint="t('admin.scheduledTests.cronHelp')"
                />
              </div>
              <div v-else>
                <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ editForm.advanced_cron ? t('admin.scheduledTests.cronExpression') : t('admin.scheduledTests.retryInterval') }}
                </label>
                <Input
                  v-if="!editForm.advanced_cron"
                  v-model="editForm.retry_interval_minutes"
                  type="number"
                  min="1"
                  max="1440"
                />
                <Input
                  v-else
                  v-model="editForm.retry_cron_expression"
                  :placeholder="'*/5 * * * *'"
                />
                <label class="mt-2 flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <Toggle v-model="editForm.advanced_cron" />
                  {{ t('admin.scheduledTests.advancedCron') }}
                </label>
              </div>
              <div>
                <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.maxResults') }}
                  <HelpTooltip>
                    <template #trigger>
                      <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                        ?
                      </span>
                    </template>
                    <div class="space-y-1.5">
                      <p class="font-medium">{{ t('admin.scheduledTests.maxResultsTooltipTitle') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipMeaning') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipBody') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipExample') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipRange') }}</p>
                    </div>
                  </HelpTooltip>
                </label>
                <Input
                  v-model="editForm.max_results"
                  type="number"
                  placeholder="100"
                />
              </div>
              <div v-if="editForm.trigger_mode === 'scheduled'" class="flex items-end">
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <Toggle v-model="editForm.enabled" />
                  {{ t('admin.scheduledTests.enabled') }}
                </label>
              </div>
              <div class="flex items-end">
                <div>
                  <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <Toggle v-model="editForm.auto_recover" />
                    {{ t('admin.scheduledTests.autoRecover') }}
                  </label>
                  <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('admin.scheduledTests.autoRecoverHelp') }}
                  </p>
                </div>
              </div>
            </div>
            <div class="mt-3 flex justify-end gap-2">
              <button
                @click="cancelEdit"
                class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
              >
                {{ t('common.cancel') }}
              </button>
              <button
                @click="handleEdit"
                :disabled="!isPlanFormValid(editForm) || updating"
                class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon v-if="updating" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
                {{ t('common.save') }}
              </button>
            </div>
          </div>

          <!-- Expanded Results Section -->
          <div
            v-if="expandedPlanId === plan.id"
            class="border-t border-gray-100 px-4 py-3 dark:border-dark-700"
          >
            <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.results') }}
            </div>

            <!-- Results Loading -->
            <div v-if="loadingResults" class="flex items-center justify-center py-4">
              <Icon name="refresh" size="sm" class="animate-spin text-gray-400" :stroke-width="2" />
              <span class="ml-2 text-xs text-gray-500">{{ t('common.loading') }}...</span>
            </div>

            <!-- No Results -->
            <div
              v-else-if="results.length === 0"
              class="py-4 text-center text-xs text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.scheduledTests.noResults') }}
            </div>

            <!-- Results List -->
            <div v-else class="max-h-64 space-y-2 overflow-y-auto">
              <div
                v-for="result in results"
                :key="result.id"
                class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-xs text-gray-600 dark:text-gray-300">
                      {{ result.model_id }}
                    </span>
                    <!-- Status Badge -->
                    <span
                      :class="[
                        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                        displayResultStatus(result) === 'success'
                          ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
                          : displayResultStatus(result) === 'running'
                            ? 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400'
                            : displayResultStatus(result) === 'interrupted'
                              ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400'
                              : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'
                      ]"
                    >
                      {{
                        displayResultStatus(result) === 'success'
                          ? t('admin.scheduledTests.success')
                          : displayResultStatus(result) === 'running'
                            ? t('admin.scheduledTests.running')
                            : displayResultStatus(result) === 'interrupted'
                              ? t('admin.scheduledTests.interrupted')
                              : t('admin.scheduledTests.failed')
                      }}
                    </span>

                    <!-- Latency -->
                    <span v-if="result.latency_ms > 0" class="text-xs text-gray-500 dark:text-gray-400">
                      {{ result.latency_ms }}ms
                    </span>
                  </div>

                  <!-- Started At -->
                  <span class="text-xs text-gray-400">
                    {{ formatDateTime(result.started_at) }}
                  </span>
                </div>

                <!-- Response / Error (collapsible) -->
                <div v-if="result.error_message" class="mt-2">
                  <div
                    class="cursor-pointer text-xs font-medium text-red-600 dark:text-red-400"
                    @click="toggleResultDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.errorMessage') }}
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedResultIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedResultIds.has(result.id)"
                    class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300"
                  >{{ result.error_message }}</pre>
                </div>
                <div v-else-if="result.response_text" class="mt-2">
                  <div
                    class="cursor-pointer text-xs font-medium text-gray-600 dark:text-gray-400"
                    @click="toggleResultDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.responseText') }}
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedResultIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedResultIds.has(result.id)"
                    class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-gray-100 p-2 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300"
                  >{{ result.response_text }}</pre>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.scheduledTests.deletePlan')"
      :message="t('admin.scheduledTests.confirmDelete')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import ModelMultiSelect from './ModelMultiSelect.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import { Icon } from '@/components/icons'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import type { CreateScheduledTestPlanRequest, ScheduledTestPlan, ScheduledTestResult, UpdateScheduledTestPlanRequest } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  show: boolean
  accountId: number | null
  modelOptions: SelectOption[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

// State
const loading = ref(false)
const creating = ref(false)
const loadingResults = ref(false)
const plans = ref<ScheduledTestPlan[]>([])
const results = ref<ScheduledTestResult[]>([])
const expandedPlanId = ref<number | null>(null)
const expandedResultIds = reactive(new Set<number>())
const resultsStatusNow = ref(Date.now())
const resultsRefreshIntervalMs = 5000
const runningResultTimeoutMs = 5 * 60 * 1000
let resultsRefreshTimer: ReturnType<typeof setTimeout> | null = null
let resultsRequestGeneration = 0
const showAddForm = ref(false)
const showDeleteConfirm = ref(false)
const deletingPlan = ref<ScheduledTestPlan | null>(null)
const editingPlanId = ref<number | null>(null)
const updating = ref(false)
type TriggerMode = 'scheduled' | 'error_recovery'
type PlanForm = {
  model_id: string
  model_ids: string[]
  cron_expression: string
  trigger_mode: TriggerMode
  retry_interval_minutes: string
  retry_cron_expression: string
  advanced_cron: boolean
  max_results: string
  enabled: boolean
  auto_recover: boolean
}

const triggerModes = computed(() => [
  { value: 'scheduled' as const, label: t('admin.scheduledTests.scheduledMode') },
  { value: 'error_recovery' as const, label: t('admin.scheduledTests.errorRecoveryMode') }
])

const editForm = reactive({
  model_id: '' as string,
  model_ids: [] as string[],
  cron_expression: '' as string,
  trigger_mode: 'scheduled' as TriggerMode,
  retry_interval_minutes: '5' as string,
  retry_cron_expression: '' as string,
  advanced_cron: false,
  max_results: '100' as string,
  enabled: true,
  auto_recover: true
})

const newPlan = reactive({
  model_id: '' as string,
  model_ids: props.modelOptions.map(option => String(option.value)),
  cron_expression: '*/30 * * * *' as string,
  trigger_mode: 'error_recovery' as TriggerMode,
  retry_interval_minutes: '5' as string,
  retry_cron_expression: '' as string,
  advanced_cron: false,
  max_results: '100' as string,
  enabled: true,
  auto_recover: true
})

const modelOptionIDs = computed(() => props.modelOptions.map(option => String(option.value)))

const allModelsSelected = (modelIDs: string[]) => {
  const available = modelOptionIDs.value
  return available.length > 0 && modelIDs.length === available.length && available.every(modelID => modelIDs.includes(modelID))
}

const canonicalModelIDs = (modelIDs: string[]) => allModelsSelected(modelIDs) ? [] : [...modelIDs]

const setTriggerMode = (form: PlanForm, mode: TriggerMode) => {
  form.trigger_mode = mode
  if (mode === 'error_recovery' && form.model_ids.length === 0) {
    form.model_ids = [...modelOptionIDs.value]
    form.model_id = form.model_ids[0] || form.model_id
  }
}

const resetNewPlan = () => {
  newPlan.model_ids = [...modelOptionIDs.value]
  newPlan.model_id = newPlan.model_ids[0] || ''
  newPlan.cron_expression = '*/30 * * * *'
  newPlan.trigger_mode = 'error_recovery'
  newPlan.retry_interval_minutes = '5'
  newPlan.retry_cron_expression = ''
  newPlan.advanced_cron = false
  newPlan.max_results = '100'
  newPlan.enabled = true
  newPlan.auto_recover = true
}

const isPlanFormValid = (form: PlanForm) => {
  if (form.trigger_mode === 'error_recovery' && form.model_ids.length === 0) return false
  if (form.trigger_mode === 'scheduled' && !form.model_id) return false
  if (form.trigger_mode === 'scheduled') return Boolean(form.cron_expression.trim())
  if (form.advanced_cron) return Boolean(form.retry_cron_expression.trim())
  const interval = Number(form.retry_interval_minutes)
  return Number.isInteger(interval) && interval >= 1 && interval <= 1440
}

const recoveryFields = (form: PlanForm) => {
  if (form.trigger_mode === 'scheduled') {
    return {
      trigger_mode: 'scheduled' as const,
      cron_expression: form.cron_expression,
      retry_interval_minutes: null,
      retry_cron_expression: null,
      auto_recover: form.auto_recover,
      model_ids: []
    }
  }
  return {
    trigger_mode: 'error_recovery' as const,
    retry_interval_minutes: form.advanced_cron ? null : Number(form.retry_interval_minutes),
    retry_cron_expression: form.advanced_cron ? form.retry_cron_expression.trim() : null,
    auto_recover: form.auto_recover,
    model_ids: canonicalModelIDs(form.model_ids)
  }
}

const formatPlanSchedule = (plan: ScheduledTestPlan) => {
  if (plan.trigger_mode !== 'error_recovery') return plan.cron_expression
  if (plan.retry_cron_expression) return plan.retry_cron_expression
  return t('admin.scheduledTests.everyMinutes', { minutes: plan.retry_interval_minutes ?? 5 })
}

const formatPlanModels = (plan: ScheduledTestPlan) => {
  if (plan.trigger_mode !== 'error_recovery') return plan.model_id
  if (!plan.model_ids?.length) return t('admin.scheduledTests.allFailedModels')
  return plan.model_ids.join(', ')
}

const isStaleRunningResult = (result: ScheduledTestResult) => {
  if (result.status !== 'running') return false
  const startedAt = Date.parse(result.started_at)
  return Number.isFinite(startedAt) && resultsStatusNow.value - startedAt > runningResultTimeoutMs
}

const displayResultStatus = (result: ScheduledTestResult) => {
  return isStaleRunningResult(result) ? 'interrupted' : result.status
}

const hasActiveRunningResult = (items: ScheduledTestResult[]) => {
  return items.some((result) => result.status === 'running' && !isStaleRunningResult(result))
}

const stopResultsPolling = () => {
  if (resultsRefreshTimer !== null) {
    clearTimeout(resultsRefreshTimer)
    resultsRefreshTimer = null
  }
}

const invalidateResultsRequests = () => {
  resultsRequestGeneration += 1
}

const startResultsPolling = () => {
  if (resultsRefreshTimer !== null) return
  resultsRefreshTimer = setTimeout(async () => {
    resultsRefreshTimer = null
    resultsStatusNow.value = Date.now()
    const planId = expandedPlanId.value
    if (!planId || !hasActiveRunningResult(results.value)) return
    await loadResults(planId, false)
  }, resultsRefreshIntervalMs)
}

watch(
  () => props.modelOptions,
  () => {
    if (newPlan.trigger_mode === 'error_recovery' && newPlan.model_ids.length === 0) {
      resetNewPlan()
    }
  },
  { immediate: true, deep: true }
)

// Load plans when dialog opens
watch(
  () => props.show,
  async (visible) => {
    if (visible && props.accountId) {
      await loadPlans()
    } else {
      invalidateResultsRequests()
      stopResultsPolling()
      plans.value = []
      results.value = []
      expandedPlanId.value = null
      expandedResultIds.clear()
      showAddForm.value = false
      showDeleteConfirm.value = false
    }
  }
)

const loadPlans = async () => {
  if (!props.accountId) return
  loading.value = true
  try {
    plans.value = await adminAPI.scheduledTests.listByAccount(props.accountId)
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to load plans')
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  if (!props.accountId || !isPlanFormValid(newPlan)) return
  creating.value = true
  try {
    const maxResults = Number(newPlan.max_results) || 100
    const request: CreateScheduledTestPlanRequest = {
      account_id: props.accountId,
      model_id: newPlan.trigger_mode === 'error_recovery' ? newPlan.model_ids[0] : newPlan.model_id,
      enabled: newPlan.enabled,
      max_results: maxResults,
      ...recoveryFields(newPlan)
    }
    await adminAPI.scheduledTests.create(request)
    appStore.showSuccess(t('admin.scheduledTests.createSuccess'))
    showAddForm.value = false
    resetNewPlan()
    await loadPlans()
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to create plan')
  } finally {
    creating.value = false
  }
}

const handleToggleEnabled = async (plan: ScheduledTestPlan, enabled: boolean) => {
  try {
    const updated = await adminAPI.scheduledTests.update(plan.id, { enabled })
    const index = plans.value.findIndex((p) => p.id === plan.id)
    if (index !== -1) {
      plans.value[index] = updated
    }
    appStore.showSuccess(t('admin.scheduledTests.updateSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to update plan')
  }
}

const startEdit = (plan: ScheduledTestPlan) => {
  editingPlanId.value = plan.id
  editForm.model_id = plan.model_id
  editForm.model_ids = plan.trigger_mode === 'error_recovery'
    ? (plan.model_ids?.length ? [...plan.model_ids] : [...modelOptionIDs.value])
    : []
  if (plan.trigger_mode === 'error_recovery' && editForm.model_ids.length > 0) {
    editForm.model_id = editForm.model_ids[0]
  }
  editForm.cron_expression = plan.cron_expression
  editForm.trigger_mode = plan.trigger_mode || 'scheduled'
  editForm.retry_interval_minutes = String(plan.retry_interval_minutes ?? 5)
  editForm.retry_cron_expression = plan.retry_cron_expression || ''
  editForm.advanced_cron = Boolean(plan.retry_cron_expression)
  editForm.max_results = String(plan.max_results)
  editForm.enabled = plan.enabled
  editForm.auto_recover = plan.auto_recover
}

const cancelEdit = () => {
  editingPlanId.value = null
}

const handleEdit = async () => {
  if (!editingPlanId.value || !isPlanFormValid(editForm)) return
  updating.value = true
  try {
    const request: UpdateScheduledTestPlanRequest = {
      model_id: editForm.trigger_mode === 'error_recovery' ? editForm.model_ids[0] : editForm.model_id,
      max_results: Number(editForm.max_results) || 100,
      enabled: editForm.enabled,
      ...recoveryFields(editForm)
    }
    const updated = await adminAPI.scheduledTests.update(editingPlanId.value, request)
    const index = plans.value.findIndex((p) => p.id === editingPlanId.value)
    if (index !== -1) {
      plans.value[index] = updated
    }
    appStore.showSuccess(t('admin.scheduledTests.updateSuccess'))
    editingPlanId.value = null
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to update plan')
  } finally {
    updating.value = false
  }
}

const confirmDeletePlan = (plan: ScheduledTestPlan) => {
  deletingPlan.value = plan
  showDeleteConfirm.value = true
}

const handleDelete = async () => {
  if (!deletingPlan.value) return
  try {
    await adminAPI.scheduledTests.delete(deletingPlan.value.id)
    appStore.showSuccess(t('admin.scheduledTests.deleteSuccess'))
    plans.value = plans.value.filter((p) => p.id !== deletingPlan.value!.id)
    if (expandedPlanId.value === deletingPlan.value.id) {
      invalidateResultsRequests()
      stopResultsPolling()
      expandedPlanId.value = null
      results.value = []
    }
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to delete plan')
  } finally {
    showDeleteConfirm.value = false
    deletingPlan.value = null
  }
}

const loadResults = async (planId: number, showLoading = true) => {
  const requestGeneration = ++resultsRequestGeneration
  if (showLoading) loadingResults.value = true
  try {
    const loaded = await adminAPI.scheduledTests.listResults(planId, 20)
    if (requestGeneration === resultsRequestGeneration && expandedPlanId.value === planId) {
      resultsStatusNow.value = Date.now()
      results.value = loaded
      if (hasActiveRunningResult(loaded)) startResultsPolling()
      else stopResultsPolling()
    }
  } catch (error: any) {
    if (requestGeneration !== resultsRequestGeneration || expandedPlanId.value !== planId) return
    results.value = []
    stopResultsPolling()
    appStore.showError(error?.message || 'Failed to load results')
  } finally {
    if (showLoading && requestGeneration === resultsRequestGeneration) loadingResults.value = false
  }
}

const toggleExpand = async (planId: number) => {
  if (expandedPlanId.value === planId) {
    invalidateResultsRequests()
    stopResultsPolling()
    expandedPlanId.value = null
    results.value = []
    expandedResultIds.clear()
    return
  }

  invalidateResultsRequests()
  stopResultsPolling()
  expandedPlanId.value = planId
  expandedResultIds.clear()
  results.value = []
  await loadResults(planId)
}

const toggleResultDetail = (resultId: number) => {
  if (expandedResultIds.has(resultId)) {
    expandedResultIds.delete(resultId)
  } else {
    expandedResultIds.add(resultId)
  }
}

onUnmounted(() => {
  invalidateResultsRequests()
  stopResultsPolling()
})
</script>
