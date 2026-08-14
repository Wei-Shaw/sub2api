<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <SearchInput
              v-model="filterSearch"
              :placeholder="t('keys.searchPlaceholder')"
              class="w-full sm:w-64"
              @search="onFilterChange"
            />
            <Select
              :model-value="filterGroupId"
              class="w-40"
              :options="groupFilterOptions"
              :aria-label="t('keys.group')"
              @update:model-value="onGroupFilterChange"
            />
            <Select
              :model-value="filterStatus"
              class="w-40"
              :options="statusFilterOptions"
              :aria-label="t('common.status')"
              @update:model-value="onStatusFilterChange"
            />
          </div>
          <EndpointPopover
            v-if="publicSettings?.api_base_url || (publicSettings?.custom_endpoints?.length ?? 0) > 0"
            :api-base-url="publicSettings?.api_base_url || ''"
            :custom-endpoints="publicSettings?.custom_endpoints || []"
          />
        </div>
      </template>

      <template #actions>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <!--
            Icon-only, so the name lives in `aria-label`/`title`. `loading` keeps
            the box and overlays a spinner instead of spinning the glyph, and it
            already implies `disabled` + `aria-busy`.
          -->
          <Button
            variant="outline"
            size="md"
            :loading="loading"
            :title="t('common.refresh')"
            :aria-label="t('common.refresh')"
            data-testid="keys-refresh"
            @click="loadApiKeys"
          >
            <template #icon>
              <Icon name="refresh" size="sm" />
            </template>
          </Button>

          <div ref="columnDropdownRef" class="relative">
            <Button
              variant="outline"
              size="md"
              :title="t('keys.columnSettings')"
              :aria-label="t('keys.columnSettings')"
              :aria-expanded="showColumnDropdown"
              aria-haspopup="true"
              data-testid="keys-column-settings"
              @click="showColumnDropdown = !showColumnDropdown"
            >
              <template #icon>
                <svg
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                  aria-hidden="true"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z"
                  />
                </svg>
              </template>
              <span class="hidden md:inline">{{ t('keys.columnSettings') }}</span>
            </Button>

            <div
              v-if="showColumnDropdown"
              class="absolute right-0 top-full z-50 mt-1 max-h-80 w-52 overflow-y-auto rounded border border-line bg-surface-raised py-1 shadow-popover"
              data-testid="keys-column-menu"
            >
              <button
                v-for="col in toggleableColumns"
                :key="col.key"
                type="button"
                role="menuitemcheckbox"
                :aria-checked="isColumnVisible(col.key)"
                class="flex w-full items-center justify-between gap-3 px-3 py-1.5 text-left text-xs text-ink-secondary transition-colors duration-fast hover:bg-surface-hover hover:text-ink"
                @click="toggleColumn(col.key)"
              >
                <span class="min-w-0 truncate">{{ col.label }}</span>
                <!-- Accent = selection. It never signals status anywhere in this view. -->
                <Icon
                  v-if="isColumnVisible(col.key)"
                  name="check"
                  size="xs"
                  class="shrink-0 text-accent"
                  :stroke-width="2"
                />
              </button>
            </div>
          </div>

          <Button
            tone="accent"
            variant="solid"
            size="md"
            data-tour="keys-create-btn"
            data-testid="keys-create"
            @click="showCreateModal = true"
          >
            <template #icon>
              <Icon name="plus" size="xs" :stroke-width="2" />
            </template>
            {{ t('keys.createKey') }}
          </Button>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="apiKeys"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <!-- An id is an identifier, not a measurement: mono, but never grouped. -->
          <template #cell-id="{ value }">
            <span class="font-mono text-xs tabular-nums text-ink-tertiary">#{{ value }}</span>
          </template>

          <template #cell-key="{ value, row }">
            <div class="flex items-center gap-1.5">
              <code class="code">{{ maskApiKey(value) }}</code>
              <button
                type="button"
                class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors duration-fast hover:bg-surface-hover"
                :class="copiedKeyId === row.id ? 'text-success' : 'text-ink-tertiary hover:text-ink'"
                :title="copiedKeyId === row.id ? t('keys.copied') : t('keys.copyToClipboard')"
                :aria-label="copiedKeyId === row.id ? t('keys.copied') : t('keys.copyToClipboard')"
                @click="copyToClipboard(value, row.id)"
              >
                <Icon v-if="copiedKeyId === row.id" name="check" size="xs" :stroke-width="2" />
                <Icon v-else name="clipboard" size="xs" />
              </button>
            </div>
          </template>

          <template #cell-name="{ value, row }">
            <div class="flex min-w-0 items-center gap-1.5">
              <span class="truncate font-medium text-ink">{{ value }}</span>
              <!--
                Was a blue shield. Blue is the accent here, and the accent may
                not carry state — this is a configuration marker, so it is ink.
                The icon itself is not a labelling surface, hence the wrapper.
              -->
              <span
                v-if="row.ip_whitelist?.length > 0 || row.ip_blacklist?.length > 0"
                role="img"
                class="inline-flex shrink-0 text-ink-tertiary"
                :title="t('keys.ipRestrictionEnabled')"
                :aria-label="t('keys.ipRestrictionEnabled')"
              >
                <Icon name="shield" size="xs" />
              </span>
            </div>
          </template>

          <template #cell-group="{ row }">
            <button
              :ref="(el) => setGroupButtonRef(row.id, el)"
              type="button"
              data-group-cell
              class="-mx-1.5 -my-0.5 flex max-w-full items-center gap-2 rounded px-1.5 py-0.5 text-left transition-colors duration-fast hover:bg-surface-hover"
              :title="t('keys.clickToChangeGroup')"
              :aria-label="t('keys.clickToChangeGroup')"
              aria-haspopup="listbox"
              :aria-expanded="groupSelectorKeyId === row.id"
              @click="openGroupSelector(row)"
            >
              <GroupBadge
                v-if="row.group"
                :name="row.group.name"
                :platform="row.group.platform"
                :subscription-type="row.group.subscription_type"
                :rate-multiplier="row.group.rate_multiplier"
                :user-rate-multiplier="userGroupRates[row.group.id]"
                :peak-rate-enabled="row.group.peak_rate_enabled"
                :peak-start="row.group.peak_start"
                :peak-end="row.group.peak_end"
                :peak-rate-multiplier="row.group.peak_rate_multiplier"
              />
              <span v-else class="text-xs text-ink-tertiary">{{ t('keys.noGroup') }}</span>
              <Icon name="chevronDown" size="xs" class="shrink-0 text-ink-tertiary" />
            </button>
          </template>

          <!--
            Was an emerald chip whenever concurrency > 0. Live traffic is the
            healthy case; painting it green spends the signal budget on nothing.
          -->
          <template #cell-current_concurrency="{ value }">
            <NumCell :value="value" />
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-[10rem] space-y-1">
              <div class="flex items-baseline justify-between gap-3">
                <span class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('keys.today') }}
                </span>
                <NumCell
                  :value="usageStats[row.id]?.today_actual_cost"
                  :precision="4"
                  :unit="CURRENCY"
                />
              </div>
              <div class="flex items-baseline justify-between gap-3">
                <span class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('keys.total') }}
                </span>
                <NumCell
                  :value="usageStats[row.id]?.total_actual_cost"
                  :precision="4"
                  :unit="CURRENCY"
                />
              </div>
              <div v-if="row.quota > 0" class="space-y-1 pt-1">
                <Meter
                  :label="t('keys.quota')"
                  :value="row.quota_used ?? 0"
                  :max="row.quota"
                  :danger-at="1"
                  :show-value="false"
                />
                <div class="flex items-baseline justify-end gap-1">
                  <NumCell
                    :value="row.quota_used"
                    :precision="2"
                    :tone="ratioTone(row.quota_used, row.quota)"
                  />
                  <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                  <NumCell :value="row.quota" :precision="2" :unit="CURRENCY" />
                </div>
              </div>
            </div>
          </template>

          <template #cell-rate_limit="{ row }">
            <div v-if="rateWindows(row).length > 0" class="min-w-[11rem] space-y-2">
              <div v-for="win in rateWindows(row)" :key="win.key" class="space-y-1">
                <Meter
                  :label="win.label"
                  :value="win.used ?? 0"
                  :max="win.limit"
                  :danger-at="1"
                  :show-value="false"
                />
                <div class="flex flex-wrap items-baseline gap-x-2">
                  <span
                    v-if="formatResetTime(win.resetAt)"
                    class="inline-flex items-center gap-1 text-2xs text-ink-tertiary"
                  >
                    <Icon name="refresh" size="xs" />
                    <span class="font-mono tabular-nums">{{ formatResetTime(win.resetAt) }}</span>
                  </span>
                  <span class="ml-auto inline-flex items-baseline gap-1">
                    <NumCell
                      :value="win.used"
                      :precision="2"
                      :tone="ratioTone(win.used, win.limit)"
                    />
                    <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                    <NumCell :value="win.limit" :precision="2" :unit="CURRENCY" />
                  </span>
                </div>
              </div>
              <Button
                v-if="hasRateLimitUsage(row)"
                variant="quiet"
                size="xs"
                :title="t('keys.resetRateLimitUsage')"
                @click.stop="confirmResetRateLimitFromTable(row)"
              >
                <template #icon>
                  <Icon name="refresh" size="xs" />
                </template>
                {{ t('keys.resetUsage') }}
              </Button>
            </div>
            <span v-else class="text-ink-disabled" :aria-label="t('common.noValue')">–</span>
          </template>

          <template #cell-expires_at="{ value }">
            <span
              v-if="value"
              class="font-mono text-xs tabular-nums"
              :class="isExpired(value) ? 'text-danger' : 'text-ink-secondary'"
            >
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-xs text-ink-tertiary">{{ t('keys.noExpiration') }}</span>
          </template>

          <!--
            Dot + word, never a tinted row. `active` is the unremarkable case and
            gets no hue at all; only exhausted/expired spend colour, which is the
            whole point of a signal budget on a list this long.
          -->
          <template #cell-status="{ value }">
            <StatusDot
              :tone="statusTone(value)"
              :label="t('keys.status.' + value)"
              :muted="value === 'active'"
            />
          </template>

          <template #cell-last_used_at="{ value }">
            <span v-if="value" class="font-mono text-xs tabular-nums text-ink-secondary">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-ink-disabled" :aria-label="t('common.noValue')">–</span>
          </template>

          <template #cell-last_used_ip="{ value }">
            <span v-if="value" class="font-mono text-xs text-ink-secondary">{{ value }}</span>
            <span v-else class="text-ink-disabled" :aria-label="t('common.noValue')">–</span>
          </template>

          <template #cell-created_at="{ value }">
            <span v-if="value" class="font-mono text-xs tabular-nums text-ink-secondary">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-ink-disabled" :aria-label="t('common.noValue')">–</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <Button
                variant="ghost"
                size="xs"
                :title="t('keys.useKey')"
                @click="openUseKeyModal(row)"
              >
                <template #icon>
                  <Icon name="terminal" size="xs" />
                </template>
                {{ t('keys.useKey') }}
              </Button>
              <Button
                v-if="!publicSettings?.hide_ccs_import_button"
                variant="ghost"
                size="xs"
                :title="t('keys.importToCcSwitch')"
                @click="importToCcswitch(row)"
              >
                <template #icon>
                  <Icon name="upload" size="xs" />
                </template>
                {{ t('keys.importToCcSwitch') }}
              </Button>
              <Button
                variant="ghost"
                size="xs"
                :title="row.status === 'active' ? t('keys.disable') : t('keys.enable')"
                @click="toggleKeyStatus(row)"
              >
                <template #icon>
                  <Icon :name="row.status === 'active' ? 'ban' : 'checkCircle'" size="xs" />
                </template>
                {{ row.status === 'active' ? t('keys.disable') : t('keys.enable') }}
              </Button>
              <Button variant="ghost" size="xs" :title="t('common.edit')" @click="editKey(row)">
                <template #icon>
                  <Icon name="edit" size="xs" />
                </template>
                {{ t('common.edit') }}
              </Button>
              <Button
                variant="ghost"
                size="xs"
                :title="t('common.delete')"
                @click="confirmDelete(row)"
              >
                <template #icon>
                  <Icon name="trash" size="xs" />
                </template>
                {{ t('common.delete') }}
              </Button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('keys.noKeysYet')"
              :description="t('keys.createFirstKey')"
              :action-text="t('keys.createKey')"
              @action="showCreateModal = true"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create/Edit Modal -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('keys.editKey') : t('keys.createKey')"
      width="normal"
      @close="closeModals"
    >
      <form id="key-form" class="space-y-4" @submit.prevent="handleSubmit">
        <FormField :label="t('keys.nameLabel')" required>
          <template #default="{ id, describedBy }">
            <input
              :id="id"
              v-model="formData.name"
              type="text"
              required
              class="input"
              :aria-describedby="describedBy"
              :placeholder="t('keys.namePlaceholder')"
              data-tour="key-form-name"
            />
          </template>
        </FormField>

        <!--
          `keys.groupRequired` used to exist only as a toast: the field that
          caused it never said anything. It now renders in the reserved message
          row, which is also why submitting no longer shifts the dialog.
        -->
        <FormField :label="t('keys.groupLabel')" required :error="groupError">
          <template #default="{ id, describedBy, invalid }">
            <Select
              v-model="formData.group_id"
              :id="id"
              :options="groupOptions"
              :placeholder="t('keys.selectGroup')"
              :searchable="true"
              :search-placeholder="t('keys.searchGroup')"
              :error="invalid"
              :aria-describedby="describedBy"
              data-tour="key-form-group"
            >
              <template #selected="{ option }">
                <GroupBadge
                  v-if="option"
                  :name="(option as unknown as GroupOption).label"
                  :platform="(option as unknown as GroupOption).platform"
                  :subscription-type="(option as unknown as GroupOption).subscriptionType"
                  :rate-multiplier="(option as unknown as GroupOption).rate"
                  :user-rate-multiplier="(option as unknown as GroupOption).userRate"
                  :peak-rate-enabled="(option as unknown as GroupOption).peakRateEnabled"
                  :peak-start="(option as unknown as GroupOption).peakStart"
                  :peak-end="(option as unknown as GroupOption).peakEnd"
                  :peak-rate-multiplier="(option as unknown as GroupOption).peakRateMultiplier"
                />
                <span v-else class="text-ink-disabled">{{ t('keys.selectGroup') }}</span>
              </template>
              <template #option="{ option, selected }">
                <GroupOptionItem
                  :name="(option as unknown as GroupOption).label"
                  :platform="(option as unknown as GroupOption).platform"
                  :subscription-type="(option as unknown as GroupOption).subscriptionType"
                  :rate-multiplier="(option as unknown as GroupOption).rate"
                  :user-rate-multiplier="(option as unknown as GroupOption).userRate"
                  :peak-rate-enabled="(option as unknown as GroupOption).peakRateEnabled"
                  :peak-start="(option as unknown as GroupOption).peakStart"
                  :peak-end="(option as unknown as GroupOption).peakEnd"
                  :peak-rate-multiplier="(option as unknown as GroupOption).peakRateMultiplier"
                  :description="(option as unknown as GroupOption).description"
                  :selected="selected"
                />
              </template>
            </Select>
          </template>
        </FormField>

        <!-- Custom Key Section (only for create) -->
        <div v-if="!showEditModal" class="space-y-2">
          <div class="flex items-center justify-between gap-4">
            <span class="text-xs font-medium text-ink-secondary">
              {{ t('keys.customKeyLabel') }}
            </span>
            <button
              type="button"
              role="switch"
              :aria-checked="formData.use_custom_key"
              :aria-label="t('keys.customKeyLabel')"
              class="switch"
              :class="formData.use_custom_key && 'switch-active'"
              @click="formData.use_custom_key = !formData.use_custom_key"
            >
              <span class="switch-thumb" />
            </button>
          </div>
          <!--
            `keys.customKeyRequired` was another toast-only string. Both custom
            key errors now land on the field.
          -->
          <FormField
            v-if="formData.use_custom_key"
            :hint="t('keys.customKeyHint')"
            :error="customKeyFieldError"
          >
            <template #default="{ id, describedBy, invalid }">
              <input
                :id="id"
                v-model="formData.custom_key"
                type="text"
                class="input font-mono"
                :class="invalid && 'input-error'"
                :aria-describedby="describedBy"
                :aria-invalid="invalid || undefined"
                :placeholder="t('keys.customKeyPlaceholder')"
              />
            </template>
          </FormField>
        </div>

        <FormField v-if="showEditModal" :label="t('keys.statusLabel')">
          <template #default="{ id, describedBy }">
            <Select
              v-model="formData.status"
              :id="id"
              :options="statusOptions"
              :placeholder="t('keys.selectStatus')"
              :aria-describedby="describedBy"
            />
          </template>
        </FormField>

        <!-- IP Restriction -->
        <div class="space-y-2">
          <div class="flex items-center justify-between gap-4">
            <span class="text-xs font-medium text-ink-secondary">
              {{ t('keys.ipRestriction') }}
            </span>
            <button
              type="button"
              role="switch"
              :aria-checked="formData.enable_ip_restriction"
              :aria-label="t('keys.ipRestriction')"
              class="switch"
              :class="formData.enable_ip_restriction && 'switch-active'"
              @click="formData.enable_ip_restriction = !formData.enable_ip_restriction"
            >
              <span class="switch-thumb" />
            </button>
          </div>

          <div v-if="formData.enable_ip_restriction" class="space-y-2 pt-1">
            <FormField :label="t('keys.ipWhitelist')" :hint="t('keys.ipWhitelistHint')">
              <template #default="{ id, describedBy }">
                <textarea
                  :id="id"
                  v-model="formData.ip_whitelist"
                  rows="3"
                  class="input font-mono"
                  :aria-describedby="describedBy"
                  :placeholder="t('keys.ipWhitelistPlaceholder')"
                />
              </template>
            </FormField>

            <FormField :label="t('keys.ipBlacklist')" :hint="t('keys.ipBlacklistHint')">
              <template #default="{ id, describedBy }">
                <textarea
                  :id="id"
                  v-model="formData.ip_blacklist"
                  rows="3"
                  class="input font-mono"
                  :aria-describedby="describedBy"
                  :placeholder="t('keys.ipBlacklistPlaceholder')"
                />
              </template>
            </FormField>
          </div>
        </div>

        <!-- Quota -->
        <div class="space-y-2">
          <FormField :label="t('keys.quotaLimit')" :hint="t('keys.quotaAmountHint')">
            <template #default="{ id, describedBy }">
              <div class="relative">
                <span
                  class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 font-mono text-xs text-ink-tertiary"
                  aria-hidden="true"
                >
                  $
                </span>
                <input
                  :id="id"
                  v-model.number="formData.quota"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :aria-describedby="describedBy"
                  :placeholder="t('keys.quotaAmountPlaceholder')"
                />
              </div>
            </template>
          </FormField>

          <div v-if="showEditModal && selectedKey && selectedKey.quota > 0" class="flex gap-2">
            <div class="min-w-0 flex-1 space-y-1 rounded border border-line bg-surface-sunken px-3 py-2">
              <Meter
                :label="t('keys.quotaUsed')"
                :value="selectedKey.quota_used ?? 0"
                :max="selectedKey.quota"
                :danger-at="1"
                :show-value="false"
              />
              <div class="flex items-baseline justify-end gap-1">
                <NumCell
                  :value="selectedKey.quota_used"
                  :precision="4"
                  :tone="ratioTone(selectedKey.quota_used, selectedKey.quota)"
                />
                <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                <NumCell :value="selectedKey.quota" :precision="2" :unit="CURRENCY" />
              </div>
            </div>
            <Button
              type="button"
              size="md"
              :title="t('keys.resetQuotaUsed')"
              @click="confirmResetQuota"
            >
              {{ t('keys.reset') }}
            </Button>
          </div>
        </div>

        <!-- Rate Limit -->
        <div class="space-y-2">
          <div class="flex items-center justify-between gap-4">
            <span class="text-xs font-medium text-ink-secondary">
              {{ t('keys.rateLimitSection') }}
            </span>
            <button
              type="button"
              role="switch"
              :aria-checked="formData.enable_rate_limit"
              :aria-label="t('keys.rateLimitSection')"
              class="switch"
              :class="formData.enable_rate_limit && 'switch-active'"
              @click="formData.enable_rate_limit = !formData.enable_rate_limit"
            >
              <span class="switch-thumb" />
            </button>
          </div>

          <div v-if="formData.enable_rate_limit" class="space-y-2 pt-1">
            <p class="text-2xs text-ink-tertiary">{{ t('keys.rateLimitHint') }}</p>

            <div v-for="field in rateLimitFields" :key="field.key" class="space-y-1.5">
              <FormField :label="field.label">
                <template #default="{ id, describedBy }">
                  <div class="relative">
                    <span
                      class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 font-mono text-xs text-ink-tertiary"
                      aria-hidden="true"
                    >
                      $
                    </span>
                    <input
                      :id="id"
                      v-model.number="formData[field.model]"
                      type="number"
                      step="0.01"
                      min="0"
                      class="input pl-7"
                      :aria-describedby="describedBy"
                      placeholder="0"
                    />
                  </div>
                </template>
              </FormField>

              <div
                v-if="showEditModal && selectedKey && selectedKey[field.model] > 0"
                class="space-y-1 rounded border border-line bg-surface-sunken px-3 py-2"
              >
                <Meter
                  :label="field.label"
                  :value="selectedKey[field.usage] ?? 0"
                  :max="selectedKey[field.model]"
                  :danger-at="1"
                  :show-value="false"
                />
                <div class="flex items-baseline justify-end gap-1">
                  <NumCell
                    :value="selectedKey[field.usage]"
                    :precision="4"
                    :tone="ratioTone(selectedKey[field.usage], selectedKey[field.model])"
                  />
                  <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                  <NumCell :value="selectedKey[field.model]" :precision="2" :unit="CURRENCY" />
                </div>
              </div>
            </div>

            <div v-if="showEditModal && selectedKey && hasAnyRateLimit(selectedKey)">
              <Button type="button" size="md" @click="confirmResetRateLimit">
                {{ t('keys.resetRateLimitUsage') }}
              </Button>
            </div>
          </div>
        </div>

        <!-- Expiration -->
        <div class="space-y-2">
          <div class="flex items-center justify-between gap-4">
            <span class="text-xs font-medium text-ink-secondary">{{ t('keys.expiration') }}</span>
            <button
              type="button"
              role="switch"
              :aria-checked="formData.enable_expiration"
              :aria-label="t('keys.expiration')"
              class="switch"
              :class="formData.enable_expiration && 'switch-active'"
              @click="formData.enable_expiration = !formData.enable_expiration"
            >
              <span class="switch-thumb" />
            </button>
          </div>

          <div v-if="formData.enable_expiration" class="space-y-3 pt-1">
            <!-- Segmented: hairlines collapse so the group reads as one object. -->
            <div class="inline-flex -space-x-px" role="group">
              <button
                v-for="days in EXPIRATION_PRESETS"
                :key="days"
                type="button"
                :aria-pressed="formData.expiration_preset === days"
                :class="[
                  SEGMENT,
                  formData.expiration_preset === days ? SEGMENT_ON : SEGMENT_OFF,
                ]"
                @click="setExpirationDays(parseInt(days, 10))"
              >
                {{ showEditModal ? t('keys.extendDays', { days }) : t('keys.expiresInDays', { days }) }}
              </button>
              <button
                type="button"
                :aria-pressed="formData.expiration_preset === 'custom'"
                :class="[
                  SEGMENT,
                  formData.expiration_preset === 'custom' ? SEGMENT_ON : SEGMENT_OFF,
                ]"
                @click="formData.expiration_preset = 'custom'"
              >
                {{ t('keys.customDate') }}
              </button>
            </div>

            <FormField :label="t('keys.expirationDate')" :hint="t('keys.expirationDateHint')">
              <template #default="{ id, describedBy }">
                <input
                  :id="id"
                  v-model="formData.expiration_date"
                  type="datetime-local"
                  class="input"
                  :aria-describedby="describedBy"
                />
              </template>
            </FormField>

            <div
              v-if="showEditModal && selectedKey?.expires_at"
              class="flex items-baseline justify-between gap-3 border-t border-line-subtle pt-2"
            >
              <span class="text-xs text-ink-secondary">{{ t('keys.currentExpiration') }}</span>
              <span class="font-mono text-xs tabular-nums text-ink">
                {{ formatDateTime(selectedKey.expires_at) }}
              </span>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2">
          <Button type="button" size="md" @click="closeModals">
            {{ t('common.cancel') }}
          </Button>
          <!--
            The label does not change while saving. `loading` keeps the label box
            and overlays the spinner, so the button never resizes under the
            cursor mid-click.
          -->
          <Button
            form="key-form"
            type="submit"
            tone="accent"
            variant="solid"
            size="md"
            :loading="submitting"
            data-tour="key-form-submit"
            data-testid="key-form-submit"
          >
            {{ showEditModal ? t('common.update') : t('common.create') }}
          </Button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('keys.deleteKey')"
      :message="t('keys.deleteConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaDialog"
      :title="t('keys.resetQuotaTitle')"
      :message="
        t('keys.resetQuotaConfirmMessage', {
          name: selectedKey?.name,
          used: selectedKey?.quota_used?.toFixed(4),
        })
      "
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetQuotaUsed"
      @cancel="showResetQuotaDialog = false"
    />

    <!-- Reset Rate Limit Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetRateLimitDialog"
      :title="t('keys.resetRateLimitTitle')"
      :message="t('keys.resetRateLimitConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetRateLimitUsage"
      @cancel="showResetRateLimitDialog = false"
    />

    <!-- Use Key Modal -->
    <UseKeyModal
      :show="showUseKeyModal"
      :api-key="selectedKey?.key || ''"
      :base-url="publicSettings?.api_base_url || ''"
      :platform="selectedKey?.group?.platform || null"
      :allow-messages-dispatch="selectedKey?.group?.allow_messages_dispatch || false"
      @close="closeUseKeyModal"
    />

    <!-- CCS Client Selection Dialog for Antigravity -->
    <BaseDialog
      :show="showCcsClientSelect"
      :title="t('keys.ccsClientSelect.title')"
      width="narrow"
      @close="closeCcsClientSelect"
    >
      <div class="space-y-4">
        <p class="text-sm text-ink-secondary">{{ t('keys.ccsClientSelect.description') }}</p>
        <div class="grid grid-cols-2 gap-2">
          <button
            v-for="client in CCS_CLIENTS"
            :key="client.type"
            type="button"
            class="flex flex-col items-center gap-1.5 rounded border border-line bg-surface p-4 text-center transition-colors duration-fast hover:border-line-strong hover:bg-surface-hover"
            @click="handleCcsClientSelect(client.type)"
          >
            <Icon :name="client.icon" size="lg" class="text-ink-tertiary" />
            <span class="text-sm font-medium text-ink">{{ t(client.labelKey) }}</span>
            <span class="text-2xs text-ink-tertiary">{{ t(client.descKey) }}</span>
          </button>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <Button type="button" size="md" @click="closeCcsClientSelect">
            {{ t('common.cancel') }}
          </Button>
        </div>
      </template>
    </BaseDialog>

    <!-- Group Selector Dropdown (Teleported to body to avoid overflow clipping) -->
    <Teleport to="body">
      <div
        v-if="groupSelectorKeyId !== null && dropdownPosition"
        ref="dropdownRef"
        class="fixed z-[100000020] w-max max-w-[calc(100vw-16px)] overflow-hidden rounded border border-line bg-surface-raised shadow-popover sm:min-w-[380px]"
        style="pointer-events: auto !important"
        :style="{
          top: dropdownPosition.top !== undefined ? dropdownPosition.top + 'px' : undefined,
          bottom: dropdownPosition.bottom !== undefined ? dropdownPosition.bottom + 'px' : undefined,
          left: dropdownPosition.left + 'px',
        }"
        role="listbox"
        :aria-label="t('keys.selectGroup')"
      >
        <div class="border-b border-line-subtle p-2">
          <div class="relative">
            <Icon
              name="search"
              size="xs"
              class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-tertiary"
            />
            <input
              v-model="groupSearchQuery"
              type="text"
              class="input h-7 pl-8 text-xs"
              :placeholder="t('keys.searchGroup')"
              :aria-label="t('keys.searchGroup')"
              @click.stop
            />
          </div>
        </div>
        <div class="max-h-80 overflow-y-auto">
          <button
            v-for="option in filteredGroupOptions"
            :key="option.value ?? 'null'"
            type="button"
            role="option"
            :aria-selected="isGroupSelected(option.value)"
            :class="[
              'flex w-full items-center justify-between border-b border-line-subtle px-3 py-2 text-left text-sm last:border-0',
              'transition-colors duration-fast',
              isGroupSelected(option.value)
                ? 'bg-accent-tint shadow-[inset_2px_0_0_0_rgb(var(--ds-accent))]'
                : 'hover:bg-surface-hover',
            ]"
            :title="option.description || undefined"
            @click="changeGroup(selectedKeyForGroup!, option.value)"
          >
            <GroupOptionItem
              :name="option.label"
              :platform="option.platform"
              :subscription-type="option.subscriptionType"
              :rate-multiplier="option.rate"
              :user-rate-multiplier="option.userRate"
              :peak-rate-enabled="option.peakRateEnabled"
              :peak-start="option.peakStart"
              :peak-end="option.peakEnd"
              :peak-rate-multiplier="option.peakRateMultiplier"
              :description="option.description"
              :selected="isGroupSelected(option.value)"
            />
          </button>
          <p
            v-if="filteredGroupOptions.length === 0"
            class="px-3 py-6 text-center text-xs text-ink-tertiary"
          >
            {{ t('keys.noGroupFound') }}
          </p>
        </div>
      </div>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'

import { keysAPI, authAPI, usageAPI, userGroupsAPI } from '@/api'
import type { BatchApiKeyUsageStats } from '@/api/usage'
/*
 * Direct paths, never the `components/common/index.ts` barrel: that barrel
 * re-exports LocaleSwitcher, which drags `createI18n` into the module graph and
 * breaks every spec that mocks `vue-i18n`. This has already happened once here.
 */
import BaseDialog from '@/components/common/BaseDialog.vue'
import Button from '@/components/common/Button.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import FormField from '@/components/common/FormField.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Meter from '@/components/common/Meter.vue'
import NumCell from '@/components/common/NumCell.vue'
import Pagination from '@/components/common/Pagination.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'
import type { Column } from '@/components/common/types'
import Icon from '@/components/icons/Icon.vue'
import EndpointPopover from '@/components/keys/EndpointPopover.vue'
import UseKeyModal from '@/components/keys/UseKeyModal.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useAppStore } from '@/stores/app'
import { useOnboardingStore } from '@/stores/onboarding'
import type {
  ApiKey,
  Group,
  PublicSettings,
  SubscriptionType,
  GroupPlatform,
  UpdateApiKeyRequest,
} from '@/types'
import { buildCcSwitchImportDeeplink, type CcSwitchClientType } from '@/utils/ccswitchImport'
import { formatDateTime } from '@/utils/format'
import { maskApiKey } from '@/utils/maskApiKey'

const { t } = useI18n()

/** Every money column in this view is settled in USD server-side. */
const CURRENCY = 'USD'

const EXPIRATION_PRESETS = ['7', '30', '90'] as const

/**
 * Segmented control. `-space-x-px` collapses the hairlines so the group reads as
 * one object, and only ground + border change on press. No pill track.
 */
const SEGMENT =
  'h-7 border px-2.5 text-xs font-medium transition-colors duration-fast first:rounded-l last:rounded-r'
const SEGMENT_ON = 'relative z-10 border-accent-solid bg-accent-solid text-accent-on'
const SEGMENT_OFF = 'border-line bg-surface text-ink-secondary hover:bg-surface-hover hover:text-ink'

const CCS_CLIENTS = [
  {
    type: 'claude' as CcSwitchClientType,
    icon: 'terminal' as const,
    labelKey: 'keys.ccsClientSelect.claudeCode',
    descKey: 'keys.ccsClientSelect.claudeCodeDesc',
  },
  {
    type: 'gemini' as CcSwitchClientType,
    icon: 'sparkles' as const,
    labelKey: 'keys.ccsClientSelect.geminiCli',
    descKey: 'keys.ccsClientSelect.geminiCliDesc',
  },
]

// Helper to format date for datetime-local input
const formatDateTimeLocal = (isoDate: string): string => {
  const date = new Date(isoDate)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

interface GroupOption {
  value: number
  label: string
  description: string | null
  rate: number
  userRate: number | null
  peakRateEnabled: boolean
  peakStart: string
  peakEnd: string
  peakRateMultiplier: number
  subscriptionType: SubscriptionType
  platform: GroupPlatform
}

const appStore = useAppStore()
const onboardingStore = useOnboardingStore()
const { copyToClipboard: clipboardCopy } = useClipboard()

const allColumns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name'), sortable: true },
  { key: 'id', label: t('keys.id'), sortable: true },
  { key: 'key', label: t('keys.apiKey'), sortable: false },
  { key: 'group', label: t('keys.group'), sortable: false },
  { key: 'current_concurrency', label: t('keys.currentConcurrency'), sortable: true },
  { key: 'usage', label: t('keys.usage'), sortable: false },
  { key: 'rate_limit', label: t('keys.rateLimitColumn'), sortable: false },
  { key: 'expires_at', label: t('keys.expiresAt'), sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'last_used_at', label: t('keys.lastUsedAt'), sortable: true },
  { key: 'last_used_ip', label: t('keys.lastUsedIP'), sortable: false },
  { key: 'created_at', label: t('keys.created'), sortable: true },
  { key: 'actions', label: t('common.actions'), sortable: false },
])

const ALWAYS_VISIBLE_COLUMNS = new Set(['name', 'actions'])
const DEFAULT_HIDDEN_COLUMNS = ['id', 'rate_limit', 'last_used_at', 'last_used_ip']
const HIDDEN_COLUMNS_KEY = 'api-key-hidden-columns'
const COLUMN_SETTINGS_VERSION_KEY = 'api-key-column-settings-version'
const COLUMN_SETTINGS_VERSION = 3
const VERSION_NEW_HIDDEN_COLUMNS: Record<number, string[]> = {
  2: ['last_used_ip'],
  3: ['id'],
}

const toggleableColumns = computed(() =>
  allColumns.value.filter((col) => !ALWAYS_VISIBLE_COLUMNS.has(col.key))
)

const hiddenColumns = reactive<Set<string>>(new Set())

const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
    localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
  } catch (error) {
    console.error('Failed to save API key table columns:', error)
  }
}

const loadSavedColumns = () => {
  hiddenColumns.clear()
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      const validColumnKeys = new Set(allColumns.value.map((col) => col.key))
      parsed
        .filter(
          (key) =>
            typeof key === 'string' && validColumnKeys.has(key) && !ALWAYS_VISIBLE_COLUMNS.has(key)
        )
        .forEach((key) => hiddenColumns.add(key))
      const storedVersion = Number(localStorage.getItem(COLUMN_SETTINGS_VERSION_KEY) ?? '1')
      if (storedVersion < COLUMN_SETTINGS_VERSION) {
        for (let v = storedVersion + 1; v <= COLUMN_SETTINGS_VERSION; v++) {
          for (const key of VERSION_NEW_HIDDEN_COLUMNS[v] ?? []) {
            if (validColumnKeys.has(key) && !ALWAYS_VISIBLE_COLUMNS.has(key)) {
              hiddenColumns.add(key)
            }
          }
        }
        saveColumnsToStorage()
      } else {
        localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
      }
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
      localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
    }
  } catch (error) {
    console.error('Failed to load API key table columns:', error)
    DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
  }
}

const toggleColumn = (key: string) => {
  if (ALWAYS_VISIBLE_COLUMNS.has(key)) return
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const columns = computed<Column[]>(() =>
  allColumns.value.filter(
    (col) => ALWAYS_VISIBLE_COLUMNS.has(col.key) || !hiddenColumns.has(col.key)
  )
)

const apiKeys = ref<ApiKey[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
const submitting = ref(false)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null
let copiedTimer: ReturnType<typeof setTimeout> | null = null
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({})
const userGroupRates = ref<Record<number, number>>({})

const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0,
})
const sortState = ref({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc',
})

// Filter state
const filterSearch = ref('')
const filterStatus = ref('')
const filterGroupId = ref<string | number>('')

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showResetQuotaDialog = ref(false)
const showResetRateLimitDialog = ref(false)
const showUseKeyModal = ref(false)
const showCcsClientSelect = ref(false)
const showColumnDropdown = ref(false)
const pendingCcsRow = ref<ApiKey | null>(null)
const selectedKey = ref<ApiKey | null>(null)
const copiedKeyId = ref<number | null>(null)
const groupSelectorKeyId = ref<number | null>(null)
const publicSettings = ref<PublicSettings | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const columnDropdownRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<{ top?: number; bottom?: number; left: number } | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
let abortController: AbortController | null = null

// Get the currently selected key for group change
const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const isGroupSelected = (value: number | null) =>
  selectedKeyForGroup.value?.group_id === value ||
  (!selectedKeyForGroup.value?.group_id && value === null)

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

const formData = ref({
  name: '',
  group_id: null as number | null,
  status: 'active' as 'active' | 'inactive',
  use_custom_key: false,
  custom_key: '',
  enable_ip_restriction: false,
  ip_whitelist: '',
  ip_blacklist: '',
  // Quota settings (empty = unlimited)
  enable_quota: false,
  quota: null as number | null,
  // Rate limit settings
  enable_rate_limit: false,
  rate_limit_5h: null as number | null,
  rate_limit_1d: null as number | null,
  rate_limit_7d: null as number | null,
  enable_expiration: false,
  expiration_preset: '30' as '7' | '30' | '90' | 'custom',
  expiration_date: '',
})

/**
 * Set on the first submit attempt so "required" messages appear next to the
 * field rather than only as a toast, without shouting at a form nobody has
 * touched yet.
 */
const submitAttempted = ref(false)

// 自定义Key验证
const customKeyError = computed(() => {
  if (!formData.value.use_custom_key || !formData.value.custom_key) {
    return ''
  }
  const key = formData.value.custom_key
  if (key.length < 16) {
    return t('keys.customKeyTooShort')
  }
  // 检查字符：只允许字母、数字、下划线、连字符
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) {
    return t('keys.customKeyInvalidChars')
  }
  return ''
})

const customKeyFieldError = computed(() => {
  if (showEditModal.value || !formData.value.use_custom_key) return ''
  if (!formData.value.custom_key) {
    return submitAttempted.value ? t('keys.customKeyRequired') : ''
  }
  return customKeyError.value
})

const groupError = computed(() =>
  submitAttempted.value && formData.value.group_id === null ? t('keys.groupRequired') : ''
)

type RateLimitKey = 'rate_limit_5h' | 'rate_limit_1d' | 'rate_limit_7d'
type RateUsageKey = 'usage_5h' | 'usage_1d' | 'usage_7d'

interface RateLimitField {
  key: string
  label: string
  model: RateLimitKey
  usage: RateUsageKey
}

const rateLimitFields = computed<RateLimitField[]>(() => [
  { key: '5h', label: t('keys.rateLimit5h'), model: 'rate_limit_5h', usage: 'usage_5h' },
  { key: '1d', label: t('keys.rateLimit1d'), model: 'rate_limit_1d', usage: 'usage_1d' },
  { key: '7d', label: t('keys.rateLimit7d'), model: 'rate_limit_7d', usage: 'usage_7d' },
])

interface RateWindow {
  key: string
  label: string
  used: number | null
  limit: number
  resetAt: string | null
}

/** Only windows that actually cap something. A meter with max 0 measures nothing. */
const rateWindows = (row: ApiKey): RateWindow[] =>
  [
    { key: '5h', label: '5H', used: row.usage_5h, limit: row.rate_limit_5h, resetAt: row.reset_5h_at },
    { key: '1d', label: '1D', used: row.usage_1d, limit: row.rate_limit_1d, resetAt: row.reset_1d_at },
    { key: '7d', label: '7D', used: row.usage_7d, limit: row.rate_limit_7d, resetAt: row.reset_7d_at },
  ].filter((w) => w.limit > 0)

const hasRateLimitUsage = (row: ApiKey) =>
  row.usage_5h > 0 || row.usage_1d > 0 || row.usage_7d > 0

const hasAnyRateLimit = (key: ApiKey) =>
  key.rate_limit_5h > 0 || key.rate_limit_1d > 0 || key.rate_limit_7d > 0

/**
 * Signal budget: a quota that is fine gets NO colour. The previous version
 * painted every healthy bar emerald, which is how a colour stops meaning
 * anything — by the time something is wrong the table is already green.
 */
const ratioTone = (used: number | null | undefined, limit: number | null | undefined): Tone => {
  if (used == null || !limit || limit <= 0) return 'neutral'
  if (used >= limit) return 'danger'
  if (used >= limit * 0.8) return 'warn'
  return 'neutral'
}

const statusTone = (status: string): Tone => {
  if (status === 'quota_exhausted') return 'warn'
  if (status === 'expired') return 'danger'
  return 'neutral'
}

const isExpired = (value: string) => {
  const ts = new Date(value).getTime()
  return Number.isFinite(ts) && ts < now.value.getTime()
}

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') },
])

const shouldSubmitEditStatus = (key: ApiKey, status: 'active' | 'inactive') => {
  if (key.status === 'quota_exhausted' || key.status === 'expired') {
    return status === 'active'
  }
  return true
}

// Filter dropdown options
const groupFilterOptions = computed(() => [
  { value: '', label: t('keys.allGroups') },
  { value: 0, label: t('keys.noGroup') },
  ...groups.value.map((g) => ({ value: g.id, label: g.name })),
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('keys.allStatus') },
  { value: 'active', label: t('keys.status.active') },
  { value: 'inactive', label: t('keys.status.inactive') },
  { value: 'quota_exhausted', label: t('keys.status.quota_exhausted') },
  { value: 'expired', label: t('keys.status.expired') },
])

const onFilterChange = () => {
  pagination.value.page = 1
  loadApiKeys()
}

const onGroupFilterChange = (value: string | number | boolean | null) => {
  filterGroupId.value = value as string | number
  onFilterChange()
}

const onStatusFilterChange = (value: string | number | boolean | null) => {
  filterStatus.value = value as string
  onFilterChange()
}

// Convert groups to Select options format with rate multiplier and subscription type
const groupOptions = computed(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    rate: group.rate_multiplier,
    userRate: userGroupRates.value[group.id] ?? null,
    peakRateEnabled: group.peak_rate_enabled,
    peakStart: group.peak_start,
    peakEnd: group.peak_end,
    peakRateMultiplier: group.peak_rate_multiplier,
    subscriptionType: group.subscription_type,
    platform: group.platform,
  }))
)

// Group dropdown search
const groupSearchQuery = ref('')
const filteredGroupOptions = computed(() => {
  const query = groupSearchQuery.value.trim().toLowerCase()
  if (!query) return groupOptions.value
  return groupOptions.value.filter((opt) => {
    return (
      opt.label.toLowerCase().includes(query) ||
      (opt.description && opt.description.toLowerCase().includes(query))
    )
  })
})

const copyToClipboard = async (text: string, keyId: number) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedKeyId.value = keyId
    if (copiedTimer) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => {
      copiedKeyId.value = null
      copiedTimer = null
    }, 800)
  }
}

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const loadApiKeys = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  try {
    // Build filters
    const filters: {
      search?: string
      status?: string
      group_id?: number | string
      sort_by?: string
      sort_order?: 'asc' | 'desc'
    } = {}
    if (filterSearch.value) filters.search = filterSearch.value
    if (filterStatus.value) filters.status = filterStatus.value
    if (filterGroupId.value !== '') filters.group_id = filterGroupId.value
    filters.sort_by = sortState.value.sort_by
    filters.sort_order = sortState.value.sort_order

    const response = await keysAPI.list(
      pagination.value.page,
      pagination.value.page_size,
      filters,
      { signal }
    )
    if (signal.aborted) return
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    // Load usage stats for all API keys in the list
    if (response.items.length > 0) {
      const keyIds = response.items.map((k) => k.id)
      try {
        const usageResponse = await usageAPI.getDashboardApiKeysUsage(keyIds, { signal })
        if (signal.aborted) return
        usageStats.value = usageResponse.stats
      } catch (e) {
        if (!isAbortError(e)) {
          console.error('Failed to load usage stats:', e)
        }
      }
    }
  } catch (error) {
    if (isAbortError(error)) {
      return
    }
    appStore.showError(t('keys.failedToLoad'))
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await userGroupsAPI.getAvailable()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const loadUserGroupRates = async () => {
  try {
    userGroupRates.value = await userGroupsAPI.getUserGroupRates()
  } catch (error) {
    console.error('Failed to load user group rates:', error)
  }
}

const loadPublicSettings = async () => {
  try {
    publicSettings.value = await authAPI.getPublicSettings()
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
}

const openUseKeyModal = (key: ApiKey) => {
  selectedKey.value = key
  showUseKeyModal.value = true
}

const closeUseKeyModal = () => {
  showUseKeyModal.value = false
  selectedKey.value = null
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.value.sort_by = key
  sortState.value.sort_order = order
  pagination.value.page = 1
  loadApiKeys()
}

const editKey = (key: ApiKey) => {
  selectedKey.value = key
  submitAttempted.value = false
  const hasIPRestriction = key.ip_whitelist?.length > 0 || key.ip_blacklist?.length > 0
  const hasExpiration = !!key.expires_at
  formData.value = {
    name: key.name,
    group_id: key.group_id,
    status: key.status === 'quota_exhausted' || key.status === 'expired' ? 'inactive' : key.status,
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: hasIPRestriction,
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n'),
    enable_quota: key.quota > 0,
    quota: key.quota > 0 ? key.quota : null,
    enable_rate_limit: hasAnyRateLimit(key),
    rate_limit_5h: key.rate_limit_5h || null,
    rate_limit_1d: key.rate_limit_1d || null,
    rate_limit_7d: key.rate_limit_7d || null,
    enable_expiration: hasExpiration,
    expiration_preset: 'custom',
    expiration_date: key.expires_at ? formatDateTimeLocal(key.expires_at) : '',
  }
  showEditModal.value = true
}

const toggleKeyStatus = async (key: ApiKey) => {
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  try {
    await keysAPI.toggleStatus(key.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('keys.keyEnabledSuccess') : t('keys.keyDisabledSuccess')
    )
    loadApiKeys()
  } catch {
    appStore.showError(t('keys.failedToUpdateStatus'))
  }
}

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  } else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      const dropdownEstHeight = 400 // estimated max dropdown height
      const dropdownEstWidth = Math.min(380, window.innerWidth - 16)
      const spaceBelow = window.innerHeight - rect.bottom
      const spaceAbove = rect.top
      // 夹取 left，避免窄屏下浮层超出视口右缘
      const left = Math.max(8, Math.min(rect.left, window.innerWidth - dropdownEstWidth - 8))

      if (spaceBelow < dropdownEstHeight && spaceAbove > spaceBelow) {
        // Not enough space below, pop upward
        dropdownPosition.value = {
          bottom: window.innerHeight - rect.top + 4,
          left,
        }
      } else {
        // Default: pop downward
        dropdownPosition.value = {
          top: rect.bottom + 4,
          left,
        }
      }
    }
    groupSelectorKeyId.value = key.id
    groupSearchQuery.value = ''
  }
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
  if (key.group_id === newGroupId) return

  try {
    await keysAPI.update(key.id, { group_id: newGroupId })
    appStore.showSuccess(t('keys.groupChangedSuccess'))
    loadApiKeys()
  } catch {
    appStore.showError(t('keys.failedToChangeGroup'))
  }
}

const closeGroupSelector = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // Check if click is inside the dropdown or the trigger button
  if (!target.closest('[data-group-cell]') && !dropdownRef.value?.contains(target)) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  }
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

const confirmDelete = (key: ApiKey) => {
  selectedKey.value = key
  showDeleteDialog.value = true
}

const handleSubmit = async () => {
  submitAttempted.value = true

  // Validate group_id is required
  if (formData.value.group_id === null) {
    appStore.showError(t('keys.groupRequired'))
    return
  }

  // Validate custom key if enabled
  if (!showEditModal.value && formData.value.use_custom_key) {
    if (!formData.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    }
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    }
  }

  // Parse IP lists only if IP restriction is enabled
  const parseIPList = (text: string): string[] =>
    text
      .split('\n')
      .map((ip) => ip.trim())
      .filter((ip) => ip.length > 0)
  const ipWhitelist = formData.value.enable_ip_restriction
    ? parseIPList(formData.value.ip_whitelist)
    : []
  const ipBlacklist = formData.value.enable_ip_restriction
    ? parseIPList(formData.value.ip_blacklist)
    : []

  // Calculate quota value (null/empty/0 = unlimited, stored as 0)
  const quota = formData.value.quota && formData.value.quota > 0 ? formData.value.quota : 0

  // Calculate expiration
  let expiresInDays: number | undefined
  let expiresAt: string | null | undefined
  if (formData.value.enable_expiration && formData.value.expiration_date) {
    if (!showEditModal.value) {
      // Create mode: calculate days from date
      const expDate = new Date(formData.value.expiration_date)
      const currentTime = new Date()
      const diffDays = Math.ceil(
        (expDate.getTime() - currentTime.getTime()) / (1000 * 60 * 60 * 24)
      )
      expiresInDays = diffDays > 0 ? diffDays : 1
    } else {
      // Edit mode: use custom date directly
      expiresAt = new Date(formData.value.expiration_date).toISOString()
    }
  } else if (showEditModal.value) {
    // Edit mode: if expiration disabled or date cleared, send empty string to clear
    expiresAt = ''
  }

  // Calculate rate limit values (send 0 when toggle is off)
  const rateLimitData = formData.value.enable_rate_limit
    ? {
        rate_limit_5h:
          formData.value.rate_limit_5h && formData.value.rate_limit_5h > 0
            ? formData.value.rate_limit_5h
            : 0,
        rate_limit_1d:
          formData.value.rate_limit_1d && formData.value.rate_limit_1d > 0
            ? formData.value.rate_limit_1d
            : 0,
        rate_limit_7d:
          formData.value.rate_limit_7d && formData.value.rate_limit_7d > 0
            ? formData.value.rate_limit_7d
            : 0,
      }
    : { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }

  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      const updates: UpdateApiKeyRequest = {
        name: formData.value.name,
        group_id: formData.value.group_id,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: quota,
        expires_at: expiresAt,
        rate_limit_5h: rateLimitData.rate_limit_5h,
        rate_limit_1d: rateLimitData.rate_limit_1d,
        rate_limit_7d: rateLimitData.rate_limit_7d,
      }
      if (shouldSubmitEditStatus(selectedKey.value, formData.value.status)) {
        updates.status = formData.value.status
      }
      await keysAPI.update(selectedKey.value.id, updates)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const customKey = formData.value.use_custom_key ? formData.value.custom_key : undefined
      await keysAPI.create(
        formData.value.name,
        formData.value.group_id,
        customKey,
        ipWhitelist,
        ipBlacklist,
        quota,
        expiresInDays,
        rateLimitData
      )
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
      // Only advance tour if active, on submit step, and creation succeeded
      if (onboardingStore.isCurrentStep('[data-tour="key-form-submit"]')) {
        onboardingStore.nextStep(500)
      }
    }
    closeModals()
    loadApiKeys()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToSave')
    appStore.showError(errorMsg)
    // Don't advance tour on error
  } finally {
    submitting.value = false
  }
}

/**
 * 处理删除 API Key 的操作
 * 优化：错误处理改进，优先显示后端返回的具体错误消息（如权限不足等），
 * 若后端未返回消息则显示默认的国际化文本
 */
const handleDelete = async () => {
  if (!selectedKey.value) return

  try {
    await keysAPI.delete(selectedKey.value.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    showDeleteDialog.value = false
    loadApiKeys()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } catch (error: any) {
    // 优先使用后端返回的错误消息，提供更具体的错误信息给用户
    const errorMsg = error?.message || t('keys.failedToDelete')
    appStore.showError(errorMsg)
  }
}

const closeModals = () => {
  showCreateModal.value = false
  showEditModal.value = false
  selectedKey.value = null
  submitAttempted.value = false
  formData.value = {
    name: '',
    group_id: null,
    status: 'active',
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: false,
    ip_whitelist: '',
    ip_blacklist: '',
    enable_quota: false,
    quota: null,
    enable_rate_limit: false,
    rate_limit_5h: null,
    rate_limit_1d: null,
    rate_limit_7d: null,
    enable_expiration: false,
    expiration_preset: '30',
    expiration_date: '',
  }
}

// Show reset quota confirmation dialog
const confirmResetQuota = () => {
  showResetQuotaDialog.value = true
}

// Set expiration date based on quick select days
const setExpirationDays = (days: number) => {
  formData.value.expiration_preset = days.toString() as '7' | '30' | '90'
  const expDate = new Date()
  expDate.setDate(expDate.getDate() + days)
  formData.value.expiration_date = formatDateTimeLocal(expDate.toISOString())
}

// Reset quota used for an API key
const resetQuotaUsed = async () => {
  if (!selectedKey.value) return
  showResetQuotaDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_quota: true })
    appStore.showSuccess(t('keys.quotaResetSuccess'))
    // Update local state
    if (selectedKey.value) {
      selectedKey.value.quota_used = 0
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetQuota')
    appStore.showError(errorMsg)
  }
}

// Show reset rate limit confirmation dialog (from edit modal)
const confirmResetRateLimit = () => {
  showResetRateLimitDialog.value = true
}

// Show reset rate limit confirmation dialog (from table row)
const confirmResetRateLimitFromTable = (row: ApiKey) => {
  selectedKey.value = row
  showResetRateLimitDialog.value = true
}

// Reset rate limit usage for an API key
const resetRateLimitUsage = async () => {
  if (!selectedKey.value) return
  showResetRateLimitDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_rate_limit_usage: true })
    appStore.showSuccess(t('keys.rateLimitResetSuccess'))
    // Refresh key data
    await loadApiKeys()
    // Update the editing key with fresh data
    const refreshedKey = apiKeys.value.find((k) => k.id === selectedKey.value!.id)
    if (refreshedKey) {
      selectedKey.value = refreshedKey
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetRateLimit')
    appStore.showError(errorMsg)
  }
}

const importToCcswitch = (row: ApiKey) => {
  const platform = row.group?.platform || 'anthropic'

  // For antigravity platform, show client selection dialog
  if (platform === 'antigravity') {
    pendingCcsRow.value = row
    showCcsClientSelect.value = true
    return
  }

  // For other platforms, execute directly
  executeCcsImport(row, platform === 'gemini' ? 'gemini' : 'claude')
}

const executeCcsImport = (row: ApiKey, clientType: CcSwitchClientType) => {
  const baseUrl = publicSettings.value?.api_base_url || window.location.origin
  const platform = row.group?.platform || 'anthropic'

  const usageScript = `({
    request: {
      url: "{{baseUrl}}/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`
  const providerName = (publicSettings.value?.site_name || 'sub2api').trim() || 'sub2api'
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl,
    platform,
    clientType,
    providerName,
    apiKey: row.key,
    usageScript,
  })

  try {
    window.open(deeplink, '_self')

    // Check if the protocol handler worked by detecting if we're still focused
    setTimeout(() => {
      if (document.hasFocus()) {
        // Still focused means the protocol handler likely failed
        appStore.showError(t('keys.ccSwitchNotInstalled'))
      }
    }, 100)
  } catch {
    appStore.showError(t('keys.ccSwitchNotInstalled'))
  }
}

const handleCcsClientSelect = (clientType: CcSwitchClientType) => {
  if (pendingCcsRow.value) {
    executeCcsImport(pendingCcsRow.value, clientType)
  }
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

const closeCcsClientSelect = () => {
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

/**
 * Time until a rate-limit window rolls over. An unparseable timestamp used to
 * fall through to `NaNm` in the cell; it now reads as "unknown" and renders
 * nothing at all.
 */
function formatResetTime(resetAt: string | null): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (Number.isNaN(diff)) return ''
  if (diff <= 0) return t('keys.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

onMounted(() => {
  loadSavedColumns()
  loadApiKeys()
  loadGroups()
  loadUserGroupRates()
  loadPublicSettings()
  document.addEventListener('click', closeGroupSelector)
  resetTimer = setInterval(() => {
    now.value = new Date()
  }, 60000)
})

onUnmounted(() => {
  document.removeEventListener('click', closeGroupSelector)
  if (resetTimer) clearInterval(resetTimer)
  if (copiedTimer) clearTimeout(copiedTimer)
})
</script>

<!--
  No `<style scoped>`.
  Row geometry, header rule, hairline separators and the sticky column edges all
  come from `TablePageLayout` and `style.css`, which own the data surface for
  every table in the app. Nothing here needs a local override, and adding one
  would put this view's density out of step with the other fifteen.
-->
