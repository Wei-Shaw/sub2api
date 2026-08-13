<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-3 2xl:flex-row 2xl:items-center 2xl:justify-between">
            <div class="grid w-full grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-[260px_160px_144px_152px] 2xl:w-auto">
              <div class="min-w-0">
                <SearchInput
                  v-model="filters.taskName"
                  :placeholder="t('batchImage.filters.searchTaskName')"
                  class="w-full"
                  @search="applyFilters"
                />
              </div>
              <Select
                v-model="filters.apiKeyId"
                :options="apiKeyFilterOptions"
                :aria-label="t('batchImage.columns.apiKey')"
                class="w-full"
                @change="applyFilters"
              />
              <Select
                v-model="filters.status"
                :options="statusFilterOptions"
                :aria-label="t('common.status')"
                class="w-full"
                @change="applyFilters"
              />
              <Select
                v-model="filters.downloaded"
                :options="downloadFilterOptions"
                :aria-label="t('batchImage.columns.downloadStatus')"
                class="w-full"
                @change="applyFilters"
              />
            </div>
            <div class="flex flex-wrap items-center justify-start gap-2 sm:justify-end 2xl:flex-shrink-0">
              <Button variant="outline" size="md" :disabled="loadingJobs" @click="resetFilters">
                {{ t('common.reset') }}
              </Button>
              <!--
                Icon-only, so the name lives in `aria-label`/`title`. `loading`
                keeps the box and overlays a spinner rather than spinning the
                glyph, and it already implies `disabled` + `aria-busy`.
              -->
              <Button
                variant="outline"
                size="md"
                :loading="loadingKeys || loadingJobs"
                :title="t('common.refresh')"
                :aria-label="t('common.refresh')"
                @click="refreshPage"
              >
                <template #icon>
                  <Icon name="refresh" size="sm" />
                </template>
              </Button>
              <Button variant="outline" size="md" @click="showGuideModal = true">
                <template #icon>
                  <Icon name="book" size="sm" />
                </template>
                {{ t('batchImage.actions.usageGuide') }}
              </Button>
              <!-- The one accent-filled control on the page. -->
              <Button tone="accent" variant="solid" size="md" @click="openCreateModal">
                <template #icon>
                  <Icon name="plus" size="sm" />
                </template>
                {{ t('batchImage.actions.createJob') }}
              </Button>
            </div>
          </div>

          <div
            v-if="selectedJobIds.size"
            class="flex flex-wrap items-center justify-between gap-3 rounded border border-line bg-surface px-3 py-2"
          >
            <i18n-t
              keypath="batchImage.list.selectedJobs"
              tag="span"
              scope="global"
              :plural="selectedJobIds.size"
              class="text-xs text-ink-secondary"
            >
              <template #count>
                <span class="font-mono font-medium tabular-nums text-ink">{{ selectedJobIds.size }}</span>
              </template>
            </i18n-t>
            <div class="flex flex-wrap items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                :loading="bulkDownloading"
                :disabled="selectedDownloadableRows.length === 0"
                @click="downloadSelectedJobs"
              >
                <template #icon>
                  <Icon name="download" size="xs" />
                </template>
                {{ t('batchImage.actions.downloadSelected') }}
              </Button>
              <Button
                variant="outline"
                tone="danger"
                size="sm"
                :loading="bulkDeleting"
                @click="deleteSelectedJobs"
              >
                <template #icon>
                  <Icon name="trash" size="xs" />
                </template>
                {{ t('batchImage.actions.deleteRecords') }}
              </Button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="visibleBatchJobs"
          :loading="loadingKeys || loadingJobs"
          :expandable-actions="false"
          row-key="id"
        >
          <!--
            The hand-rolled `select` column stays instead of DataTable's own
            `selectable` prop: DataTable pins columns by index and treats
            `columns[0].key === 'select'` as the signal to pin the first TWO
            columns, so a native checkbox column (which it does not give a
            `sticky-col` class) would scroll out from under the pinned task
            name. What was missing here was the accessible name, not the column.
          -->
          <template #header-select>
            <input
              type="checkbox"
              class="ds-focus-inset h-4 w-4 rounded-sm border-line-strong bg-surface text-accent"
              :checked="allVisibleSelected"
              :indeterminate="someVisibleSelected"
              :aria-label="t('common.selectAll')"
              @change="toggleAllVisible(($event.target as HTMLInputElement).checked)"
            />
          </template>

          <template #cell-select="{ row }">
            <input
              type="checkbox"
              class="ds-focus-inset h-4 w-4 rounded-sm border-line-strong bg-surface text-accent"
              :checked="selectedJobIds.has(row.id)"
              :aria-label="row.task_name || defaultTaskName(row.created_at)"
              @change="toggleJobSelection(row.id, ($event.target as HTMLInputElement).checked)"
              @click.stop
            />
          </template>

          <template #cell-id="{ row }">
            <div class="flex w-[240px] items-start gap-1" :class="row.is_child ? 'pl-5' : ''">
              <button
                v-if="row.child_count > 0 && !row.is_child"
                type="button"
                class="ds-focus-inset mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-sm text-ink-tertiary transition-colors duration-fast hover:bg-surface-hover hover:text-ink"
                :title="childToggleLabel(row)"
                :aria-label="childToggleLabel(row)"
                :aria-expanded="expandedParentIds.has(row.id)"
                @click.stop="toggleChildRows(row.id)"
              >
                <Icon :name="expandedParentIds.has(row.id) ? 'chevronDown' : 'chevronRight'" size="xs" />
              </button>
              <span v-else class="w-5 flex-shrink-0" aria-hidden="true" />
              <button
                type="button"
                class="ds-focus-inset -mx-1 min-w-0 flex-1 rounded-sm px-1 py-0.5 text-left transition-colors duration-fast hover:bg-surface-hover"
                @click="selectJob(row.id)"
              >
                <span
                  class="flex min-w-0 items-center gap-1.5 text-sm font-medium"
                  :class="row.task_name ? 'text-ink' : 'text-ink-tertiary'"
                >
                  <span class="min-w-0 truncate">{{ row.task_name || defaultTaskName(row.created_at) }}</span>
                  <!-- Structure, not status: a subtask marker gets no hue. -->
                  <Badge v-if="row.child_count > 0 && !row.is_child" tone="neutral" mono>
                    {{ t('batchImage.list.childCount', { n: row.child_count }, row.child_count) }}
                  </Badge>
                  <Badge v-if="row.is_child" tone="neutral">
                    {{ t('batchImage.list.childBadge') }}
                  </Badge>
                </span>
                <span class="mt-0.5 block font-mono text-2xs tabular-nums text-ink-tertiary">
                  {{ formatDate(row.created_at) }}
                </span>
              </button>
            </div>
          </template>

          <!-- A model name is an identifier, not a quantity: mono, never NumCell. -->
          <template #cell-model="{ row }">
            <p class="max-w-[180px] truncate font-mono text-xs text-ink-secondary" :title="row.model">
              {{ row.model }}
            </p>
          </template>

          <template #cell-api_key_name="{ value }">
            <span v-if="value" class="block truncate text-sm text-ink-secondary">{{ value }}</span>
            <span v-else class="text-xs text-ink-tertiary">{{ t('batchImage.list.keyNotRecorded') }}</span>
          </template>

          <!--
            Dot + word, never a tinted row or a bare colour. In-flight states are
            `info`, not the accent — the accent means interaction and selection
            in this system and may not carry status.
          -->
          <template #cell-status="{ row }">
            <StatusDot
              :tone="statusTone(displayJob(row))"
              :label="statusLabel(displayJob(row))"
              :muted="statusTone(displayJob(row)) === 'neutral'"
            />
          </template>

          <!--
            Success is the unremarkable case and spends no colour; only a
            non-zero failure count does.
          -->
          <template #cell-counts="{ row }">
            <div class="flex items-baseline justify-end gap-1.5">
              <NumCell :value="displayJob(row).success_count" />
              <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
              <NumCell
                :value="displayJob(row).fail_count"
                :tone="displayJob(row).fail_count > 0 ? 'danger' : 'neutral'"
              />
              <i18n-t
                keypath="batchImage.list.totalCount"
                tag="span"
                scope="global"
                class="text-2xs text-ink-tertiary"
              >
                <template #n>
                  <NumCell :value="displayJob(row).item_count" />
                </template>
              </i18n-t>
            </div>
          </template>

          <template #cell-cost="{ row }">
            <i18n-t
              v-if="costIsHold(displayJob(row))"
              keypath="batchImage.detail.holdCost"
              tag="span"
              scope="global"
              class="inline-flex items-baseline gap-1 text-2xs text-ink-tertiary"
            >
              <template #amount>
                <NumCell :value="displayJob(row).hold_amount" :precision="2" :unit="CURRENCY" />
              </template>
            </i18n-t>
            <NumCell v-else :value="settledCost(displayJob(row))" :precision="2" :unit="CURRENCY" />
          </template>

          <!--
            A timestamp is not a measurement, so it is mono/tabular rather than a
            NumCell — `Intl.NumberFormat` would turn it into an en dash.
          -->
          <template #cell-downloaded="{ row }">
            <span v-if="row.downloaded_at" class="font-mono text-xs tabular-nums text-ink-secondary">
              {{ formatDate(row.downloaded_at) }}
            </span>
            <span v-else class="text-xs text-ink-tertiary">{{ t('batchImage.list.notDownloaded') }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="xs"
                :title="t('batchImage.actions.viewDetail')"
                @click.stop="selectJob(row.id)"
              >
                <template #icon>
                  <Icon name="eye" size="xs" />
                </template>
                {{ t('common.view') }}
              </Button>
              <Button
                variant="ghost"
                size="xs"
                :loading="isDownloadingJob(row.id)"
                :disabled="!canDownload(row) || downloading"
                :title="t('batchImage.actions.downloadZip')"
                @click.stop="downloadJob(row)"
              >
                <template #icon>
                  <Icon name="download" size="xs" />
                </template>
                {{ t('batchImage.actions.download') }}
              </Button>
              <Button
                v-if="canRetry(row) || canDeleteRecord(row)"
                variant="ghost"
                size="xs"
                :title="t('batchImage.actions.moreActions')"
                aria-haspopup="true"
                :aria-expanded="openMoreJobId === row.id"
                :class="openMoreJobId === row.id ? 'bg-surface-active text-ink' : ''"
                @click.stop="toggleMoreMenu(row, $event)"
              >
                <template #icon>
                  <Icon name="more" size="xs" />
                </template>
                {{ t('common.more') }}
              </Button>
            </div>
          </template>

          <!-- Type-led. The 48px decorative glyph carried no information. -->
          <template #empty>
            <div class="flex min-h-[240px] flex-col items-center justify-center px-4 py-10 text-center">
              <p class="text-sm font-medium text-ink">{{ t('batchImage.list.empty') }}</p>
              <p class="mt-1.5 max-w-sm text-xs leading-5 text-ink-tertiary">
                {{ t('batchImage.list.emptyHint') }}
              </p>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <div
          v-if="visibleBatchJobs.length > 0 || pagination.page > 1"
          class="flex flex-col gap-3 border-t border-line bg-surface px-4 py-2.5 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-ink-secondary">
            <!-- A page number is an ordinal, not a measurement: mono, not NumCell. -->
            <i18n-t keypath="batchImage.pagination.pageNumber" tag="span" scope="global">
              <template #page>
                <span class="font-mono font-medium tabular-nums text-ink">{{ pagination.page }}</span>
              </template>
            </i18n-t>
            <i18n-t keypath="batchImage.pagination.pageItems" tag="span" scope="global">
              <template #count>
                <NumCell :value="visibleBatchJobs.length" />
              </template>
            </i18n-t>
            <div class="flex items-center gap-2">
              <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                {{ t('pagination.perPage') }}
              </span>
              <Select
                v-model="pagination.page_size"
                :options="batchPageSizeOptions"
                :aria-label="t('pagination.perPage')"
                class="w-20"
                @change="handlePageSizeChange"
              />
            </div>
          </div>
          <div class="flex items-center justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              :disabled="pagination.page <= 1 || loadingJobs"
              @click="handlePageChange(pagination.page - 1)"
            >
              <template #icon>
                <Icon name="chevronLeft" size="xs" />
              </template>
              {{ t('pagination.previous') }}
            </Button>
            <Button
              variant="outline"
              size="sm"
              :disabled="!pagination.has_more || loadingJobs"
              @click="handlePageChange(pagination.page + 1)"
            >
              {{ t('pagination.next') }}
              <template #trailing>
                <Icon name="chevronRight" size="xs" />
              </template>
            </Button>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <!-- Genuinely floating, so elevation is earned: one popover shadow, no ring. -->
    <Teleport to="body">
      <div
        v-if="openMoreJobId"
        class="fixed z-[9999] w-44 overflow-hidden rounded border border-line bg-surface-raised py-1 shadow-popover"
        :style="moreMenuStyle"
        role="menu"
        @click.stop
      >
        <template v-for="job in batchJobs" :key="job.id">
          <template v-if="job.id === openMoreJobId">
            <button
              v-if="canRetry(job)"
              type="button"
              role="menuitem"
              class="ds-focus-inset flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-ink-secondary transition-colors duration-fast hover:bg-surface-hover hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
              :disabled="retryingBatchId === job.id"
              @click="retryFailedJob(job)"
            >
              <Icon name="refresh" size="xs" :class="retryingBatchId === job.id ? 'animate-spin' : ''" />
              {{ t('batchImage.actions.retryFailedItems') }}
            </button>
            <button
              v-if="canDeleteRecord(job)"
              type="button"
              role="menuitem"
              class="ds-focus-inset flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-danger transition-colors duration-fast hover:bg-surface-hover disabled:cursor-not-allowed disabled:opacity-40"
              :disabled="deletingBatchId === job.id"
              @click="deleteJob(job)"
            >
              <Icon
                :name="deletingBatchId === job.id ? 'refresh' : 'trash'"
                size="xs"
                :class="deletingBatchId === job.id ? 'animate-spin' : ''"
              />
              {{ t('batchImage.actions.deleteRecords') }}
            </button>
          </template>
        </template>
      </div>
    </Teleport>

    <Teleport to="body">
      <div
        v-if="promptPopover.visible"
        class="batch-prompt-popover fixed z-[9999] rounded border border-line bg-surface-raised p-3 shadow-popover"
        :style="promptPopover.style"
        @mouseenter="cancelPromptPopoverClose"
        @mouseleave="schedulePromptPopoverClose"
      >
        <div class="mb-2 flex items-center justify-between gap-3 border-b border-line-subtle pb-2">
          <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('batchImage.promptPopover.title') }}
          </span>
          <Button variant="quiet" tone="accent" size="xs" @click="copyPromptPopover">
            {{ t('common.copy') }}
          </Button>
        </div>
        <!-- The prompt is what the reader came for: full text, own scroll box. -->
        <p class="max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-sm leading-6 text-ink-secondary">
          {{ promptPopover.text }}
        </p>
      </div>
    </Teleport>

    <BaseDialog :show="!!currentJob" :title="t('batchImage.detail.title')" width="extra-wide" @close="closeDetail">
      <div v-if="currentJob" class="space-y-6">
        <!--
          A definition list, not four centred cards. Labels are the small
          uppercase rank; the numbers are the largest thing in the row.
        -->
        <dl class="grid gap-x-8 gap-y-4 border border-line bg-surface-sunken px-4 py-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="min-w-0">
            <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
              {{ t('common.status') }}
            </dt>
            <dd class="mt-1.5">
              <StatusDot
                :tone="statusTone(currentDisplayJob || currentJob)"
                :label="statusLabel(currentDisplayJob || currentJob)"
              />
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
              {{ hasChildJobs(currentJob.id) ? t('batchImage.detail.aggregatedResult') : t('batchImage.detail.result') }}
            </dt>
            <dd class="mt-1.5 flex items-baseline gap-1.5">
              <NumCell :value="(currentDisplayJob || currentJob).success_count" />
              <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
              <NumCell
                :value="(currentDisplayJob || currentJob).fail_count"
                :tone="(currentDisplayJob || currentJob).fail_count > 0 ? 'danger' : 'neutral'"
              />
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
              {{ t('batchImage.detail.cost') }}
            </dt>
            <dd class="mt-1.5">
              <i18n-t
                v-if="costIsHold(currentDisplayJob || currentJob)"
                keypath="batchImage.detail.holdCost"
                tag="span"
                scope="global"
                class="inline-flex items-baseline gap-1 text-2xs text-ink-tertiary"
              >
                <template #amount>
                  <NumCell
                    :value="(currentDisplayJob || currentJob).hold_amount"
                    :precision="2"
                    :unit="CURRENCY"
                  />
                </template>
              </i18n-t>
              <NumCell
                v-else
                :value="settledCost(currentDisplayJob || currentJob)"
                :precision="2"
                :unit="CURRENCY"
              />
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
              {{ t('batchImage.detail.downloadStatus') }}
            </dt>
            <dd
              v-if="currentJob.downloaded_at"
              class="mt-1.5 truncate font-mono text-xs tabular-nums text-ink"
            >
              {{ formatDate(currentJob.downloaded_at) }}
            </dd>
            <dd v-else class="mt-1.5 text-xs text-ink-tertiary">
              {{ t('batchImage.list.notDownloaded') }}
            </dd>
          </div>
        </dl>

        <div>
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line pb-2">
            <h3 class="text-sm font-semibold text-ink">{{ t('batchImage.detail.items') }}</h3>
            <Button
              variant="outline"
              size="sm"
              :loading="refreshing || loadingItems"
              @click="refreshDetail"
            >
              <template #icon>
                <Icon name="refresh" size="xs" />
              </template>
              {{ t('common.refresh') }}
            </Button>
          </div>

          <!--
            `.table` rather than DataTable. DataTable owns a virtualizer that
            derives its scroll height from the flex chain of `TablePageLayout`;
            inside a dialog body that chain does not exist, and this table also
            needs a fixed percentage colgroup plus a thumbnail cell taller than
            the estimated row height. `.table` is already tokenized and carries
            the same hairline/header/no-zebra rules with none of that machinery.
          -->
          <div class="table-container mt-4">
            <table v-if="items.length" class="table min-w-[860px] table-fixed">
              <colgroup>
                <col class="w-[18%]" />
                <col class="w-[34%]" />
                <col class="w-[12%]" />
                <col class="w-[10%]" />
                <col class="w-[26%]" />
              </colgroup>
              <thead>
                <tr>
                  <th scope="col" class="is-numeric">Custom ID</th>
                  <th scope="col">Prompt</th>
                  <th scope="col">{{ t('common.status') }}</th>
                  <th scope="col">{{ t('batchImage.detail.preview') }}</th>
                  <th scope="col">{{ t('batchImage.detail.result') }}</th>
                </tr>
              </thead>
              <tbody>
                <!--
                  A superseded row is dimmed, never tinted. Tinting rows is the
                  second signal that competes with cell-level status, and the row
                  already says "Recovered by retry" in words.
                -->
                <tr v-for="item in items" :key="itemPreviewKey(item)">
                  <td class="is-numeric">
                    <span
                      class="block min-w-0 truncate"
                      :class="isRecoveredOriginalFailure(item) ? 'text-ink-disabled' : 'text-ink'"
                      :title="item.custom_id"
                    >
                      {{ item.custom_id }}
                    </span>
                  </td>
                  <td :class="isRecoveredOriginalFailure(item) ? 'text-ink-disabled' : 'text-ink-secondary'">
                    <div
                      class="batch-prompt-trigger cursor-default truncate rounded-sm text-sm"
                      tabindex="0"
                      @pointerenter="schedulePromptPopoverOpen($event, item.prompt_preview || '-')"
                      @pointerleave="schedulePromptPopoverClose"
                      @mouseenter="schedulePromptPopoverOpen($event, item.prompt_preview || '-')"
                      @mouseleave="schedulePromptPopoverClose"
                      @click="showPromptPopover($event, item.prompt_preview || '-')"
                      @focus="showPromptPopover($event, item.prompt_preview || '-')"
                      @focusin="showPromptPopover($event, item.prompt_preview || '-')"
                      @blur="schedulePromptPopoverClose"
                    >
                      {{ item.prompt_preview || '–' }}
                    </div>
                  </td>
                  <td>
                    <StatusDot
                      :tone="itemStatusTone(item)"
                      :label="itemDisplayStatusLabel(item)"
                      :muted="itemStatusTone(item) === 'neutral'"
                    />
                  </td>
                  <td>
                    <div class="h-10 w-10 overflow-hidden rounded-sm border border-line-subtle bg-surface-sunken">
                      <button
                        v-if="itemPreviewUrls[itemPreviewKey(item)] && !previewErrorIds.has(itemPreviewKey(item))"
                        type="button"
                        class="ds-focus-inset block h-full w-full overflow-hidden"
                        :title="t('batchImage.detail.previewZoom', { id: item.custom_id })"
                        :aria-label="t('batchImage.detail.previewZoom', { id: item.custom_id })"
                        @click="openImagePreview(item)"
                      >
                        <img
                          :src="itemPreviewUrls[itemPreviewKey(item)]"
                          class="h-full w-full object-cover"
                          alt=""
                          @error="handlePreviewError(itemPreviewKey(item))"
                        />
                      </button>
                      <button
                        v-else-if="canLoadItemPreview(item)"
                        type="button"
                        class="ds-focus-inset flex h-full w-full items-center justify-center text-ink-tertiary transition-colors duration-fast hover:bg-surface-hover hover:text-ink disabled:cursor-wait disabled:opacity-40"
                        :disabled="previewLoadingIds.has(itemPreviewKey(item))"
                        :title="previewButtonLabel(item)"
                        :aria-label="previewButtonLabel(item)"
                        @click="loadItemPreview(item)"
                      >
                        <Icon
                          :name="previewLoadingIds.has(itemPreviewKey(item)) ? 'refresh' : 'eye'"
                          size="xs"
                          :class="previewLoadingIds.has(itemPreviewKey(item)) ? 'animate-spin' : ''"
                        />
                      </button>
                      <div
                        v-else
                        class="flex h-full w-full items-center justify-center text-ink-disabled"
                        :title="item.image_count > 0 ? t('batchImage.detail.previewUnavailable') : t('batchImage.detail.noImage')"
                      >
                        <Icon name="document" size="xs" />
                      </div>
                    </div>
                  </td>
                  <td>
                    <Badge :tone="itemResultTone(item)" class="max-w-full">
                      <span class="min-w-0 truncate" :title="itemResultLabel(item)">
                        {{ itemResultLabel(item) }}
                      </span>
                    </Badge>
                  </td>
                </tr>
              </tbody>
            </table>
            <div v-else class="bg-surface-sunken px-4 py-10 text-center">
              <p class="text-sm font-medium text-ink">
                {{ loadingItems ? t('batchImage.detail.loadingItems') : t('batchImage.detail.noItems') }}
              </p>
              <p v-if="!loadingItems" class="mx-auto mt-1.5 max-w-md text-xs leading-5 text-ink-tertiary">
                {{ t('batchImage.detail.noItemsHint') }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <Button
          variant="outline"
          size="md"
          :loading="cancelling"
          :disabled="!currentJob || !canCancel(currentJob)"
          @click="cancelSelected"
        >
          {{ t('batchImage.actions.cancelJob') }}
        </Button>
        <Button
          v-if="currentJob && currentDisplayJob && canRetry(currentDisplayJob)"
          variant="outline"
          size="md"
          :loading="retryingBatchId === currentJob.id"
          @click="retrySelected"
        >
          <template #icon>
            <Icon name="refresh" size="xs" />
          </template>
          {{ t('batchImage.actions.retryFailedItems') }}
        </Button>
        <Button
          tone="accent"
          variant="solid"
          size="md"
          :loading="!!currentJob && isDownloadingJob(currentJob.id)"
          :disabled="!currentJob || !canDownload(currentJob) || downloading"
          @click="downloadSelected"
        >
          <template #icon>
            <Icon name="download" size="xs" />
          </template>
          {{ t('batchImage.actions.downloadZip') }}
        </Button>
      </template>
    </BaseDialog>

    <BaseDialog :show="!!previewImageItem" :title="previewImageItem?.custom_id || t('batchImage.imagePreview.title')" width="extra-wide" :z-index="60" @close="closeImagePreview">
      <div class="space-y-3">
        <!-- `warn` is a status token and this is a genuine caveat, so it may carry hue. -->
        <p class="rounded-sm border border-warn/40 bg-warn-tint px-3 py-2 text-xs leading-5 text-warn">
          {{ t('batchImage.imagePreview.notice') }}
        </p>
        <div class="flex min-h-[420px] items-center justify-center border border-line bg-surface-sunken p-4">
          <img
            v-if="previewImageUrl"
            :src="previewImageUrl"
            class="max-h-[70vh] max-w-full rounded-sm object-contain"
            :alt="previewImageItem?.custom_id || ''"
          />
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="showCreateModal" :title="t('batchImage.create.title')" width="wide" @close="closeCreateModal">
      <form class="space-y-6" @submit.prevent="submitJob">
        <!--
          Every validated control is wrapped in FormField: it owns the id/`for`
          pairing, the `aria-describedby`/`aria-invalid` wiring, and a reserved
          message line so an error does not shove the rest of the form down.
          Before this, three of the form's failure modes (no key, no model, too
          many outputs) were only reachable as a toast — the field that caused
          them said nothing.
        -->
        <div class="grid items-start gap-x-4 md:grid-cols-2">
          <FormField
            class="md:col-span-2"
            :label="t('batchImage.create.taskName')"
            :hint="t('batchImage.create.taskNamePlaceholder')"
          >
            <template #default="{ id, describedBy }">
              <input
                :id="id"
                v-model="form.taskName"
                type="text"
                maxlength="255"
                class="input"
                :aria-describedby="describedBy"
                :placeholder="t('batchImage.create.taskNamePlaceholder')"
              />
            </template>
          </FormField>

          <FormField
            class="md:col-span-2"
            :label="t('batchImage.columns.apiKey')"
            required
            :error="formErrors.apiKey"
            :hint="noKeysHint"
          >
            <!--
              `aria-label` is passed even though FormField already renders a
              `<label for>`: Select falls back to a hardcoded English
              "Select option" when the prop is absent, and an `aria-label` on the
              trigger overrides the associated label rather than adding to it.
            -->
            <template #default="{ id, describedBy, invalid }">
              <Select
                :id="id"
                :model-value="form.apiKeyId"
                :options="apiKeySelectOptions"
                :disabled="loadingKeys"
                :error="invalid"
                :aria-label="t('batchImage.columns.apiKey')"
                :aria-describedby="describedBy"
                @update:model-value="onCreateApiKeyChange"
              />
            </template>
          </FormField>

          <FormField
            :label="t('batchImage.create.model')"
            required
            :error="formErrors.model || modelLoadError"
            :hint="modelHint"
          >
            <template #default="{ id, describedBy, invalid }">
              <Select
                :id="id"
                :model-value="form.model"
                :options="modelSelectOptions"
                :disabled="loadingModels || availableBatchImageModels.length === 0"
                :error="invalid"
                :aria-label="t('batchImage.create.model')"
                :aria-describedby="describedBy"
                :placeholder="loadingModels ? batchImageText('loadingModels') : batchImageText('noModels')"
                @update:model-value="onCreateModelChange"
              />
            </template>
          </FormField>

          <FormField :label="t('batchImage.create.outputFormat')">
            <template #default="{ id, describedBy }">
              <Select
                :id="id"
                :model-value="form.responseMimeType"
                :options="outputFormatOptions"
                :aria-label="t('batchImage.create.outputFormat')"
                :aria-describedby="describedBy"
                @update:model-value="onOutputFormatChange"
              />
            </template>
          </FormField>

          <!--
            Read-only facts, so they are not inputs. Rendering them as a disabled
            `.input` claimed an interaction that does not exist.
          -->
          <div class="flex flex-col">
            <span class="mb-1.5 text-xs font-medium text-ink-secondary">
              {{ t('batchImage.create.imageSize') }}
            </span>
            <p class="flex h-9 items-center border border-line-subtle bg-surface-sunken px-3 font-mono text-sm tabular-nums text-ink">
              1K
            </p>
            <p class="input-hint">{{ t('batchImage.create.imageSizeHint') }}</p>
          </div>

          <div class="flex flex-col">
            <span class="mb-1.5 text-xs font-medium text-ink-secondary">
              {{ t('batchImage.create.estimatedOutput') }}
            </span>
            <i18n-t
              keypath="batchImage.create.estimatedOutputValue"
              tag="p"
              scope="global"
              class="flex h-9 items-center gap-1 border border-line-subtle bg-surface-sunken px-3 text-sm text-ink-secondary"
            >
              <template #images>
                <NumCell :value="estimatedOutputCount" />
              </template>
              <template #prompts>
                <NumCell :value="promptRows.length" />
              </template>
            </i18n-t>
          </div>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between gap-3 border-b border-line pb-1.5">
            <h4 class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">Prompt</h4>
            <span class="text-2xs text-ink-tertiary">
              {{ t('batchImage.create.promptAdded', { count: promptRows.length }) }}
            </span>
          </div>
          <div class="border border-line p-3">
            <label class="sr-only" for="batch-image-prompt-draft">Prompt</label>
            <textarea
              id="batch-image-prompt-draft"
              v-model="promptDraft"
              rows="3"
              class="input min-h-[76px] resize-y leading-5"
              :placeholder="t('batchImage.create.promptPlaceholder')"
            />
            <div class="mt-2 grid gap-2 md:grid-cols-[minmax(0,1fr)_112px_132px_112px] md:items-center">
              <input
                v-model="customIdDraft"
                type="text"
                maxlength="255"
                class="input"
                :aria-label="t('batchImage.create.customIdPlaceholder')"
                :placeholder="t('batchImage.create.customIdPlaceholder')"
              />
              <Select
                :model-value="outputCountDraft"
                :options="outputCountSelectOptions"
                :aria-label="t('batchImage.create.outputCountPerPrompt')"
                @update:model-value="onOutputCountChange"
              />
              <!-- A label wrapping the file input, so it must be styled as `.btn`. -->
              <label
                class="btn btn-secondary cursor-pointer justify-center"
                :class="referenceImageDrafts.length >= selectedModelReferenceLimit ? 'pointer-events-none opacity-40' : ''"
              >
                <Icon name="upload" size="xs" />
                {{ t('batchImage.create.referenceImage') }}
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  multiple
                  class="sr-only"
                  :disabled="referenceImageDrafts.length >= selectedModelReferenceLimit"
                  @change="handleReferenceImageFiles"
                />
              </label>
              <Button variant="outline" size="md" :disabled="!promptDraft.trim()" @click="addPromptRow">
                <template #icon>
                  <Icon name="plus" size="xs" />
                </template>
                {{ t('common.add') }}
              </Button>
            </div>
            <div v-if="referenceImageDrafts.length" class="mt-3 flex flex-wrap gap-2">
              <span
                v-for="(ref, refIndex) in referenceImageDrafts"
                :key="`${ref.name}-${refIndex}`"
                class="inline-flex max-w-full items-center gap-1 rounded-sm border border-line bg-surface-sunken px-1.5 py-0.5 text-2xs text-ink-secondary"
              >
                <span class="max-w-[180px] truncate">{{ ref.name }}</span>
                <button
                  type="button"
                  class="ds-focus-inset rounded-sm text-ink-tertiary transition-colors duration-fast hover:text-danger"
                  :title="t('batchImage.create.removeReferenceImage')"
                  :aria-label="t('batchImage.create.removeReferenceImage')"
                  @click="removeReferenceImageDraft(refIndex)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </span>
            </div>
            <p class="mt-2 text-2xs leading-4 text-ink-tertiary">
              {{ t('batchImage.create.limitsHint', { maxPerItem: BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM, maxPerJob: BATCH_IMAGE_MAX_OUTPUTS_PER_JOB, refLimit: selectedModelReferenceLimit }) }}
            </p>
          </div>
          <p v-if="formErrors.prompts" class="text-2xs text-danger" role="alert">
            {{ formErrors.prompts }}
          </p>
          <div v-if="promptRows.length" class="border border-line">
            <div
              v-for="(row, index) in promptRows"
              :key="row.localId"
              class="flex items-center gap-3 border-b border-line-subtle px-3 py-1.5 last:border-b-0"
            >
              <!-- A custom id is an identifier: mono + tabular, never a NumCell. -->
              <span
                class="w-24 flex-shrink-0 truncate font-mono text-2xs tabular-nums text-ink-tertiary"
                :title="row.custom_id"
              >
                {{ row.custom_id }}
              </span>
              <p class="min-w-0 flex-1 truncate text-sm text-ink">{{ row.prompt }}</p>
              <span
                v-if="row.output_count > 1"
                class="inline-flex flex-shrink-0 items-baseline gap-0.5 text-2xs text-ink-tertiary"
              >
                <span aria-hidden="true">×</span>
                <NumCell :value="row.output_count" />
              </span>
              <span v-if="row.reference_images.length" class="flex-shrink-0 text-2xs text-ink-tertiary">
                {{ t('batchImage.create.referenceCount', { n: row.reference_images.length }, row.reference_images.length) }}
              </span>
              <Button
                variant="ghost"
                tone="danger"
                size="xs"
                class="flex-shrink-0"
                :title="t('common.delete')"
                :aria-label="t('common.delete')"
                @click="removePromptRow(index)"
              >
                <template #icon>
                  <Icon name="trash" size="xs" />
                </template>
              </Button>
            </div>
          </div>
          <p v-else class="border border-line bg-surface-sunken px-3 py-6 text-center text-xs text-ink-tertiary">
            {{ t('batchImage.create.noPrompts') }}
          </p>
        </div>

        <p class="rounded-sm border border-warn/40 bg-warn-tint px-3 py-2 text-xs leading-5 text-warn">
          {{ t('batchImage.create.cancelNotice') }}
        </p>
        <!-- Progress, not a status: it spends no hue. -->
        <p
          v-if="submitting"
          role="status"
          class="border border-line bg-surface-sunken px-3 py-2 text-xs leading-5 text-ink-secondary"
        >
          {{ t('batchImage.create.submittingNotice') }}
        </p>
      </form>

      <template #footer>
        <Button variant="outline" size="md" :disabled="submitting" @click="closeCreateModal">
          {{ t('common.cancel') }}
        </Button>
        <!--
          `loading` keeps the label. Swapping "Submit job" for "Submitting…"
          resized the button under the cursor mid-click.
        -->
        <Button
          tone="accent"
          variant="solid"
          size="md"
          class="min-w-[120px]"
          :loading="submitting"
          :disabled="loadingModels || (parsedItems.length === 0 && !promptDraft.trim()) || !selectedApiKey || !form.model"
          @click="submitJob"
        >
          {{ t('batchImage.actions.submitJob') }}
        </Button>
      </template>
    </BaseDialog>

    <!--
      The one long-form reading surface in this view, so it is set as prose
      rather than as panels: one column on a ~68ch measure, hairlines between
      sections, and the space ABOVE a heading larger than the space below it so
      the heading reads as belonging to the section that follows.
    -->
    <BaseDialog :show="showGuideModal" :title="t('batchImage.guide.title')" width="wide" @close="showGuideModal = false">
      <div class="max-w-[68ch]">
        <section>
          <h3 class="text-lg font-medium text-ink">{{ t('batchImage.guide.uiTitle') }}</h3>
          <!--
            Mono step numbers on a hairline-separated list. No pastel bubbles, no
            connector line: the rule between rows already says "in order".
          -->
          <ol class="mt-5 border-t border-line-subtle">
            <li
              v-for="step in guideSteps"
              :key="step.index"
              class="flex gap-4 border-b border-line-subtle py-3"
            >
              <span
                class="w-6 shrink-0 pt-0.5 font-mono text-2xs tabular-nums text-ink-tertiary"
                aria-hidden="true"
              >
                {{ step.label }}
              </span>
              <p class="min-w-0 flex-1 text-sm leading-6 text-ink-secondary">{{ step.text }}</p>
            </li>
          </ol>
        </section>

        <section class="mt-12">
          <div class="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
            <div class="min-w-0">
              <h3 class="text-lg font-medium text-ink">{{ t('batchImage.guide.skillTitle') }}</h3>
              <p class="mt-1 text-xs leading-5 text-ink-tertiary">
                {{ t('batchImage.guide.skillDesc') }}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              class="shrink-0"
              :title="t('batchImage.actions.copyInstruction')"
              :aria-label="t('batchImage.actions.copyInstruction')"
              @click="copyInstruction"
            >
              <template #icon>
                <Icon name="copy" size="xs" />
              </template>
              {{ t('common.copy') }}
            </Button>
          </div>
          <!--
            A flat panel, not a fake terminal. It is what the reader came here to
            read, so it is mono, selectable, keyboard-scrollable, and it scrolls
            inside its own box — the dialog never scrolls sideways because of it.
          -->
          <pre
            class="batch-instruction mt-5 max-h-[440px] overflow-auto rounded-sm border border-line bg-surface-sunken p-4 font-mono text-xs leading-6 text-ink"
            tabindex="0"
            :aria-label="t('batchImage.guide.skillTitle')"
          >{{ agentInstruction }}</pre>
        </section>
      </div>

      <template #footer>
        <Button variant="outline" size="md" @click="showGuideModal = false">
          {{ t('common.close') }}
        </Button>
        <Button tone="accent" variant="solid" size="md" @click="copyInstruction">
          <template #icon>
            <Icon name="copy" size="xs" />
          </template>
          {{ t('batchImage.actions.copyInstruction') }}
        </Button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
/*
 * Primitives are imported by direct path, never through
 * `components/common/index.ts`. The barrel pulls `createI18n` into the module
 * graph, which breaks any spec that mocks `vue-i18n` with a partial factory.
 */
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import FormField from '@/components/common/FormField.vue'
import NumCell from '@/components/common/NumCell.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getPersistedPageSize, setPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useAppStore } from '@/stores/app'
import { keysAPI } from '@/api'
import {
  cancelBatchImageJob,
  deleteBatchImageJobRecord,
  downloadBatchImageZip,
  getBatchImageItemContent,
  getBatchImageJob,
  listBatchImageJobs,
  listBatchImageItems,
  listBatchImageModels,
  saveBlob,
  submitBatchImageJob,
  type BatchImageItem,
  type BatchImageJob,
  type BatchImageJobsListOptions,
  type BatchImageReferenceImage,
  type BatchImageStatus,
  type BatchImageSubmitItem,
} from '@/api/batchImage'
import type { ApiKey } from '@/types'
import type { Column } from '@/components/common/types'

type BatchImageJobRow = Pick<BatchImageJob, 'id' | 'task_name' | 'parent_batch_id' | 'status' | 'model' | 'provider' | 'item_count' | 'success_count' | 'fail_count' | 'estimated_cost' | 'hold_amount' | 'actual_cost' | 'created_at' | 'downloaded_at'> & {
  api_key_id: number
  api_key_name: string
  child_count: number
  is_child?: boolean
}

type BatchImageDetailItem = BatchImageItem & {
  batch_id: string
  source_task_name: string
}

type PromptRow = {
  localId: string
  custom_id: string
  prompt: string
  output_count: number
  reference_images: BatchImageReferenceImage[]
}

type ReferenceImageDraft = BatchImageReferenceImage & {
  name: string
  size: number
}

type PreviewCacheRecord = {
  key: string
  blob: Blob
  size: number
  createdAt: number
  lastAccessedAt: number
}

type PreviewImageSource = ImageBitmap | HTMLImageElement

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'cancelled', 'output_deleted'])
const PREVIEW_CACHE_DB_NAME = 'sub2api-batch-image-preview-cache'
const PREVIEW_CACHE_STORE_NAME = 'thumbnails'
const PREVIEW_THUMBNAIL_MAX_EDGE = 360
const PREVIEW_THUMBNAIL_QUALITY = 0.72
const PREVIEW_CACHE_MAX_AGE_MS = 3 * 24 * 60 * 60 * 1000
const PREVIEW_CACHE_MAX_ENTRIES = 120
const PREVIEW_CACHE_MAX_BYTES = 48 * 1024 * 1024
const BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM = 4
const BATCH_IMAGE_MAX_OUTPUTS_PER_JOB = 200
const outputCountOptions = Array.from({ length: BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM }, (_, index) => index + 1)
const batchPageSizeOptions: SelectOption[] = [20, 50, 100].map(size => ({ value: size, label: String(size) }))
/** Passed to `NumCell` as the unit, so it renders a step down from the number. */
const CURRENCY = 'USD'
const outputFormatOptions: SelectOption[] = [
  { value: 'image/png', label: 'PNG' },
  { value: 'image/jpeg', label: 'JPEG' },
  { value: 'image/webp', label: 'WebP' },
]

const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const { t, locale } = useI18n()

/*
 * Quantities align on the decimal, so their columns — and their headers, which
 * DataTable aligns from this same class string — are right-aligned. Everything
 * else reads from the left.
 */
const columns = computed<Column[]>(() => [
  { key: 'select', label: '', sortable: false, class: 'w-11 text-center' },
  { key: 'id', label: t('batchImage.columns.taskName'), sortable: false, class: 'w-[260px] max-w-[260px]' },
  { key: 'model', label: t('batchImage.columns.model'), sortable: false, class: 'w-[180px] max-w-[180px]' },
  { key: 'api_key_name', label: t('batchImage.columns.apiKey'), sortable: false, class: 'w-40 max-w-40' },
  { key: 'status', label: t('common.status'), sortable: false, class: 'w-32' },
  { key: 'counts', label: t('batchImage.columns.result'), sortable: false, class: 'w-40 text-right' },
  { key: 'cost', label: t('batchImage.columns.cost'), sortable: false, class: 'w-36 text-right' },
  { key: 'downloaded', label: t('batchImage.columns.downloadStatus'), sortable: false, class: 'w-44' },
  { key: 'actions', label: t('common.actions'), sortable: false, class: 'w-52 text-right' },
])

const statusFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('batchImage.filters.allStatuses') },
  { value: 'queued', label: t('batchImage.status.queued') },
  { value: 'running', label: t('batchImage.status.running') },
  { value: 'processing_results', label: t('batchImage.status.processingResults') },
  { value: 'settling', label: t('batchImage.status.settling') },
  { value: 'completed', label: t('batchImage.status.completed') },
  { value: 'failed', label: t('batchImage.status.failed') },
  { value: 'cancelled', label: t('batchImage.status.cancelled') },
  { value: 'output_deleted', label: t('batchImage.status.outputDeleted') },
])

const downloadFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('batchImage.filters.allDownloadStates') },
  { value: 'true', label: t('batchImage.filters.downloaded') },
  { value: 'false', label: t('batchImage.filters.notDownloaded') },
])

const form = reactive({
  apiKeyId: 0,
  taskName: '',
  model: '',
  responseMimeType: 'image/png',
})

const filters = reactive({
  taskName: '',
  apiKeyId: '',
  status: '',
  downloaded: '',
})

/**
 * Inline validation messages for the create form.
 *
 * `validateForm` used to fail entirely through `appStore.showError`, so the
 * field that was actually wrong never said so and the message vanished with the
 * toast. These are rendered by the owning `FormField` (which also wires
 * `aria-invalid`/`aria-describedby`); the toast still fires, so behaviour is
 * additive.
 */
const formErrors = reactive({
  apiKey: '',
  model: '',
  prompts: '',
})

const pagination = reactive({
  page: 1,
  page_size: Math.min(getPersistedPageSize(20), 100),
  has_more: false,
})

const apiKeys = ref<ApiKey[]>([])
const loadingKeys = ref(false)
const loadingJobs = ref(false)
const submitting = ref(false)
const refreshing = ref(false)
const cancelling = ref(false)
const downloading = ref(false)
const downloadingBatchId = ref('')
const retryingBatchId = ref('')
const bulkDownloading = ref(false)
const bulkDeleting = ref(false)
const deletingBatchId = ref('')
const loadingItems = ref(false)
const loadingModels = ref(false)
const showCreateModal = ref(false)
const showGuideModal = ref(false)
const currentJob = ref<BatchImageJob | null>(null)
const selectedBatchId = ref('')
const selectedBatchApiKeyId = ref(0)
const items = ref<BatchImageDetailItem[]>([])
const batchJobs = ref<BatchImageJobRow[]>([])
const selectedJobIds = ref(new Set<string>())
const expandedParentIds = ref(new Set<string>())
const promptRows = ref<PromptRow[]>([])
const promptDraft = ref('')
const customIdDraft = ref('')
const outputCountDraft = ref(1)
const referenceImageDrafts = ref<ReferenceImageDraft[]>([])
const itemPreviewUrls = reactive<Record<string, string>>({})
const previewLoadingIds = ref(new Set<string>())
const previewErrorIds = ref(new Set<string>())
const previewImageItem = ref<BatchImageItem | null>(null)
const availableBatchImageModels = ref<Array<{ value: string; label: string }>>([])
const modelLoadError = ref('')
const openMoreJobId = ref('')
const moreMenuStyle = ref<Record<string, string>>({})
const promptPopover = reactive({
  visible: false,
  text: '',
  style: {} as Record<string, string>,
})
let modelRequestSeq = 0
let pollTimer: ReturnType<typeof setInterval> | null = null
let previewCacheDBPromise: Promise<IDBDatabase | null> | null = null
let previewCacheCleanupTimer: ReturnType<typeof setInterval> | null = null
let promptPopoverCloseTimer: ReturnType<typeof setTimeout> | null = null
let promptPopoverOpenTimer: ReturnType<typeof setTimeout> | null = null
let activePromptPopoverTarget: HTMLElement | null = null

const geminiApiKeys = computed(() =>
  apiKeys.value.filter((key) =>
    key.status === 'active' &&
    key.group?.platform === 'gemini' &&
    key.group?.allow_batch_image_generation === true,
  ),
)

const selectedApiKey = computed(() =>
  geminiApiKeys.value.find((key) => key.id === Number(form.apiKeyId)) || null,
)

const filteredApiKeys = computed(() => {
  const selectedFilterID = Number(filters.apiKeyId || 0)
  if (!selectedFilterID) return geminiApiKeys.value
  return geminiApiKeys.value.filter(key => key.id === selectedFilterID)
})

const apiKeyFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('batchImage.filters.allApiKeys') },
  ...geminiApiKeys.value.map(key => ({
    value: String(key.id),
    label: key.name || `API Key #${key.id}`,
  })),
])

/*
 * The create form's three native `<select class="input">` controls are replaced
 * by the design-system `Select`, which accepts the `id` and `aria-describedby`
 * that `FormField` generates. A native select cannot consume either, so the
 * labels above them were unassociated `<label>` elements pointing at nothing.
 */
const apiKeySelectOptions = computed<SelectOption[]>(() => [
  {
    value: 0,
    label: loadingKeys.value
      ? t('batchImage.create.loadingKeys')
      : t('batchImage.create.selectKeyPlaceholder'),
  },
  ...geminiApiKeys.value.map(key => ({
    value: key.id,
    label: `${key.name} · ${key.group?.name || 'Gemini'}`,
  })),
])

const modelSelectOptions = computed<SelectOption[]>(() =>
  availableBatchImageModels.value.map(model => ({ value: model.value, label: model.label })),
)

const outputCountSelectOptions = computed<SelectOption[]>(() =>
  outputCountOptions.map(count => ({
    value: count,
    label: t('batchImage.create.outputCountOption', { n: count }, count),
  })),
)

const noKeysHint = computed(() =>
  !loadingKeys.value && geminiApiKeys.value.length === 0 ? t('batchImage.create.noKeysHint') : '',
)

const modelHint = computed(() =>
  selectedApiKey.value && !loadingModels.value && availableBatchImageModels.value.length === 0
    ? batchImageText('noModelsHint')
    : '',
)

function onCreateApiKeyChange(value: string | number | boolean | null) {
  form.apiKeyId = Number(value) || 0
  formErrors.apiKey = ''
}

function onCreateModelChange(value: string | number | boolean | null) {
  form.model = value === null || typeof value === 'boolean' ? '' : String(value)
  formErrors.model = ''
}

function onOutputFormatChange(value: string | number | boolean | null) {
  if (value === null || typeof value === 'boolean') return
  form.responseMimeType = String(value)
}

function onOutputCountChange(value: string | number | boolean | null) {
  outputCountDraft.value = normalizeOutputCount(value)
}

function childToggleLabel(row: BatchImageJobRow) {
  return expandedParentIds.value.has(row.id)
    ? t('batchImage.list.collapseChildren')
    : t('batchImage.list.expandChildren', { n: row.child_count }, row.child_count)
}

function previewButtonLabel(item: BatchImageDetailItem) {
  return previewErrorIds.value.has(itemPreviewKey(item))
    ? t('batchImage.detail.previewReload')
    : t('batchImage.detail.previewLoad')
}

/**
 * The `stepN` strings already carry their own "1." / "2." prefix, so the mono
 * step number would print twice. Strip whatever ordinal the locale wrote and
 * render the index instead.
 */
const guideSteps = computed(() =>
  ([1, 2, 3, 4] as const).map(step => ({
    index: step,
    label: String(step).padStart(2, '0'),
    text: t(`batchImage.guide.step${step}`).replace(/^\s*\d+\s*[.、．)]\s*/, ''),
  })),
)

const selectedRows = computed(() =>
  batchJobs.value.filter(job => selectedJobIds.value.has(job.id)),
)

const childrenByParent = computed(() => {
  const groups = new Map<string, BatchImageJobRow[]>()
  for (const job of batchJobs.value) {
    if (!job.parent_batch_id) continue
    const rows = groups.get(job.parent_batch_id) || []
    rows.push(job)
    groups.set(job.parent_batch_id, rows)
  }
  for (const rows of groups.values()) {
    rows.sort((a, b) => a.created_at - b.created_at)
  }
  return groups
})

const visibleBatchJobs = computed(() => {
  const rows: BatchImageJobRow[] = []
  for (const job of batchJobs.value.filter(item => !item.parent_batch_id)) {
    rows.push(job)
    if (expandedParentIds.value.has(job.id)) {
      rows.push(...(childrenByParent.value.get(job.id) || []).map(child => ({ ...child, is_child: true })))
    }
  }
  return rows
})

const selectedDownloadableRows = computed(() =>
  selectedRows.value.filter(job => canDownload(job)),
)

const allVisibleSelected = computed(() =>
  visibleBatchJobs.value.length > 0 && visibleBatchJobs.value.every(job => selectedJobIds.value.has(job.id)),
)

const someVisibleSelected = computed(() =>
  visibleBatchJobs.value.some(job => selectedJobIds.value.has(job.id)) && !allVisibleSelected.value,
)

const previewImageUrl = computed(() => {
  const item = previewImageItem.value
  if (!item) return ''
  return itemPreviewUrls[itemPreviewKey(item)] || ''
})

const recoveredOriginalCustomIds = computed(() => {
  const rootBatchId = detailRootBatchId()
  if (!rootBatchId) return new Set<string>()
  const ids = new Set<string>()
  for (const item of items.value) {
    if (!isChildDetailItem(item) || !isSuccessfulImageItem(item)) continue
    const sourceCustomID = retrySourceCustomID(item.custom_id)
    if (sourceCustomID) ids.add(sourceCustomID)
  }
  return ids
})

const currentDisplayJob = computed(() => {
  if (!currentJob.value) return null
  return displayJob(currentJob.value)
})

const endpointBase = computed(() => {
  const configured = appStore.apiBaseUrl?.trim()
  if (configured) return configured.replace(/\/+$/, '')
  if (typeof window !== 'undefined') return window.location.origin.replace(/\/+$/, '')
  return '<你的 Sub2API API 端点>'
})

const selectedModelReferenceLimit = computed(() => referenceImageLimitForModel(form.model))

const estimatedOutputCount = computed(() =>
  promptRows.value.reduce((sum, row) => sum + normalizeOutputCount(row.output_count), 0),
)

const parsedItems = computed<BatchImageSubmitItem[]>(() => {
  const used = new Set<string>()
  return promptRows.value
    .map((row, index) => {
      const customID = uniqueCustomID(row.custom_id || `img_${String(index + 1).padStart(3, '0')}`, used, index)
      const item: BatchImageSubmitItem = { custom_id: customID, prompt: row.prompt.trim() }
      const outputCount = normalizeOutputCount(row.output_count)
      if (outputCount > 1) {
        item.output_count = outputCount
      }
      if (row.reference_images.length) {
        item.reference_images = row.reference_images
      }
      return item
    })
    .filter(item => item.prompt)
})

function referenceImageLimitForModel(model: string) {
  const normalized = String(model || '').toLowerCase()
  if (normalized.includes('pro-image')) return 14
  if (normalized.includes('flash-image')) return 3
  return 0
}

const agentInstruction = computed(() => `---
name: sub2api-batch-image
description: 当用户希望用 Gemini/Vertex 批量生成图片、批量跑提示词、下载批量生图结果、重试失败图片时使用。
---

你是 Codex 中的批量生图执行 Agent。用户不需要手动填写页面表单；你应从当前聊天、用户给的文件、目录或上下文中整理任务名称、prompt 列表和输出目录，只有缺少关键决策时才向用户提问。

默认端点：
${endpointBase.value}

你需要自己完成：
1. 从用户聊天或附件中提取 prompt。每条 prompt 保留完整文本，按顺序生成稳定 custom_id，例如 img_001、img_002。
2. 从用户要求或上下文推断任务名称；没有明确名称时用当前时间生成任务名。
3. 从用户要求或上下文推断输出目录；如果用户没有说保存到哪里，才询问用户。
4. 提交前必须先计算 expected_output_count = 所有 item 的 output_count 之和。单个批量任务硬性最多 200 张输出图；超过 200 张必须拆成多组任务，不能提交一个超大任务，也不能把参考图附件上限当成生成张数上限。
5. 如果用户提供参考图，把参考图按用途绑定到具体 item。参考图只是输入附件，不是输出图数量。模型单条限制必须按模型执行：Gemini 2.5 Flash Image 每条最多 3 张参考图；Gemini 3 Pro Image 每条最多 14 张参考图。不要把后端附件风控理解成 Pro 单条能力：按 output_count 展开后，所有 item 的参考图附件总数还有内部保护阈值 1000 个，inline base64 参考图解码后总量最多 128MB。这个 1000 只是服务器拒绝异常请求的保护阈值，不是推荐规模；参考图很多或总请求体较大时应主动拆分任务。
6. 参考图会按 output_count 重复消耗输入 token；大量任务、重复复用同一张参考图或参考图总体积较大时，优先使用 gs:// file_uri 或拆分成多组任务。
7. 选择 API Key 和模型：先获取当前可用的批量生图 Key/模型；如果用户指定模型且该 Key 支持，则使用用户指定模型；否则使用该 Key 可用模型中的默认/第一个。不要展示或询问内部 provider 名称。
8. 调用批量生图 API 提交、轮询、下载，不要求用户去页面里手填。

API 调用规范：
- 模型：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/models')}
- 提交：POST ${joinEndpointPath(endpointBase.value, '/v1/images/batches')}
- 查询：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}')}
- 明细：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}/items')}
- 下载：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}/download')}
- 取消：POST ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}/cancel')}

提交请求体：
{
  "model": "<按所选 Key 可用模型填写>",
  "task_name": "<从聊天推断；为空则用当前时间>",
  "image_size": "1K",
  "response_mime_type": "image/png",
  "items": [
    {
      "custom_id": "img_001",
      "prompt": "<第一条完整 prompt>",
      "output_count": 1,
      "reference_images": [
        {
          "id": "face",
          "type": "subject",
          "mime_type": "image/png",
          "data": "<base64，不含 data:image/png;base64, 前缀>"
        }
      ]
    }
  ]
}

必须遵守：
- 不要把 API Key 写入仓库、日志、提交记录或最终回复。
- 不要把参考图 base64 写入最终回复、日志或公开文件。恢复记录中只保存参考图文件名、用途、数量和请求 JSON 文件路径；若请求 JSON 文件包含 base64，应保存在用户指定输出目录且不要提交到仓库。
- output_count 表示同一 prompt 和参考图重复生成几张，默认 1，每条最多 4；这不是依赖 Gemini 单次请求返回多图，而是系统展开成多个真实任务项。提交前必须确认预计输出图总数不超过 200，超过就拆分成多组任务。绝不能因为参考图附件有更高的内部保护阈值，就提交会生成超过 200 张图的任务。
- 当前对用户的批量生图计费仍按成功输出图片数量结算，不单独对参考图加价。可以向用户说明：参考图会产生少量上游输入 token 和临时存储成本，且会随 output_count 重复计算；页面显示的冻结/结算金额按输出图片数量计算。
- 提交成功后，必须立刻在输出目录写入本地恢复记录，例如 batch-image-resume.json。不要在恢复记录里保存 API Key。
- 恢复记录至少包含：endpoint、task_name、batch_id、model、output_dir、request_file、submitted_at、last_status、status_url、items_url、download_url、prompt_count、expected_output_count，以及可用于失败重试的 custom_id 到 prompt 映射或请求 JSON 文件路径。
- 每次查询状态后更新恢复记录，写入 last_checked_at、last_status、成功数、失败数、实际扣费和失败摘要。会话中断或暂停后，下次必须能凭该文件继续查询、下载或重试。
- 不要高频轮询。首次查询等待约 20 到 30 秒；queued 状态每 60 到 120 秒查询一次；如果连续 3 次仍是 queued，就先停止主动查询，告诉用户任务仍在排队，并保留恢复记录，之后可继续其他任务或等待用户稍后让你恢复。
- running 状态每约 60 秒查询一次，服务器压力大或大批量任务时可以更久；processing_results 等接近完成的状态可每 20 到 45 秒查询一次。
- 任务完成后报告任务名、任务 id、成功数、失败数、实际扣费和保存路径。
- 只下载成功图片。部分失败时，先展示失败 custom_id、错误码、错误来源和简要原因。
- 重试只能重试失败项，不能重复提交已成功项。若历史任务没有保存失败项 prompt，必须告诉用户无法自动重试，并询问用户是否提供原 prompt。
- 取消任务前必须提醒：已被系统索引为成功的图片仍会按成功项结算扣费，其余冻结金额会释放。
- 图片预览按需加载；不要为了查看列表自动批量加载图片内容。`)

function joinEndpointPath(base: string, path: string): string {
  return `${base.replace(/\/+$/, '')}/${path.replace(/^\/+/, '')}`
}

function uniqueCustomID(raw: string, used: Set<string>, index: number): string {
  const base = raw.replace(/[^\w.-]+/g, '_').replace(/^_+|_+$/g, '') || `img_${String(index + 1).padStart(3, '0')}`
  let candidate = base
  let suffix = 2
  while (used.has(candidate)) {
    candidate = `${base}_${suffix}`
    suffix += 1
  }
  used.add(candidate)
  return candidate
}

function normalizeOutputCount(value: unknown): number {
  const parsed = Math.floor(Number(value || 1))
  if (!Number.isFinite(parsed)) return 1
  return Math.min(BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM, Math.max(1, parsed))
}

function addPromptRow() {
  const prompt = promptDraft.value.trim()
  if (!prompt) return
  const outputCount = normalizeOutputCount(outputCountDraft.value)
  const used = new Set(promptRows.value.map(row => row.custom_id))
  const customID = uniqueCustomID(customIdDraft.value || `img_${String(promptRows.value.length + 1).padStart(3, '0')}`, used, promptRows.value.length)
  promptRows.value = [
    ...promptRows.value,
    {
      localId: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      custom_id: customID,
      prompt,
      output_count: outputCount,
      reference_images: referenceImageDrafts.value.map(({ name: _name, size: _size, ...ref }) => ref),
    },
  ]
  promptDraft.value = ''
  customIdDraft.value = ''
  outputCountDraft.value = 1
  referenceImageDrafts.value = []
  formErrors.prompts = ''
}

function removePromptRow(index: number) {
  promptRows.value = promptRows.value.filter((_, currentIndex) => currentIndex !== index)
  formErrors.prompts = ''
}

function removeReferenceImageDraft(index: number) {
  referenceImageDrafts.value = referenceImageDrafts.value.filter((_, currentIndex) => currentIndex !== index)
}

async function handleReferenceImageFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (files.length === 0) return
  const limit = selectedModelReferenceLimit.value
  if (limit <= 0) {
    appStore.showError(t('batchImage.create.modelNoReferenceImages'))
    return
  }
  const slots = Math.max(0, limit - referenceImageDrafts.value.length)
  if (slots <= 0) {
    appStore.showError(t('batchImage.create.refLimitReached', { limit }))
    return
  }
  const accepted = files.slice(0, slots)
  if (accepted.length < files.length) {
    appStore.showError(t('batchImage.create.refLimitExceededIgnored', { limit }))
  }
  const next: ReferenceImageDraft[] = []
  for (const file of accepted) {
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      appStore.showError(t('batchImage.create.refFormatUnsupported'))
      continue
    }
    if (file.size > 10 * 1024 * 1024) {
      appStore.showError(t('batchImage.create.refFileTooLarge', { name: file.name }))
      continue
    }
    const data = await readFileAsBase64(file)
    next.push({
      id: file.name,
      type: 'reference',
      mime_type: file.type,
      data,
      name: file.name,
      size: file.size,
    })
  }
  referenceImageDrafts.value = [...referenceImageDrafts.value, ...next]
}

function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.onload = () => {
      const result = String(reader.result || '')
      resolve(result.includes(',') ? result.slice(result.indexOf(',') + 1) : result)
    }
    reader.readAsDataURL(file)
  })
}

async function loadApiKeys() {
  loadingKeys.value = true
  try {
    const response = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    apiKeys.value = response.items || []
    if (!selectedApiKey.value && geminiApiKeys.value.length > 0) {
      form.apiKeyId = geminiApiKeys.value[0].id
    }
    if (filters.apiKeyId && !geminiApiKeys.value.some(key => String(key.id) === filters.apiKeyId)) {
      filters.apiKeyId = ''
    }
    if (!selectedApiKey.value) {
      availableBatchImageModels.value = []
      form.model = ''
    }
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadKeysFailed')))
  } finally {
    loadingKeys.value = false
  }
}

async function loadAvailableModels() {
  const key = selectedApiKey.value
  const requestID = ++modelRequestSeq
  modelLoadError.value = ''
  availableBatchImageModels.value = []
  form.model = ''
  if (!key) return

  loadingModels.value = true
  try {
    const result = await listBatchImageModels(key.key)
    if (requestID !== modelRequestSeq) return
    const seen = new Set<string>()
    availableBatchImageModels.value = (result.data || [])
      .map(model => String(model.id || '').trim())
      .filter((model) => {
        if (!model || seen.has(model)) return false
        seen.add(model)
        return true
      })
      .map(model => ({ value: model, label: model }))
    form.model = availableBatchImageModels.value[0]?.value || ''
  } catch (error: any) {
    if (requestID !== modelRequestSeq) return
    modelLoadError.value = batchImageErrorMessage(error, batchImageText('loadModelsFailed'))
  } finally {
    if (requestID === modelRequestSeq) {
      loadingModels.value = false
    }
  }
}

async function refreshPage() {
  await loadApiKeys()
  await loadBatchJobs()
}

function applyFilters() {
  pagination.page = 1
  selectedJobIds.value = new Set()
  void loadBatchJobs()
}

function resetFilters() {
  filters.taskName = ''
  filters.apiKeyId = ''
  filters.status = ''
  filters.downloaded = ''
  applyFilters()
}

function listOptions(): BatchImageJobsListOptions {
  const options: BatchImageJobsListOptions = {
    limit: pagination.page_size,
    cursor: String((pagination.page - 1) * pagination.page_size),
  }
  if (filters.taskName.trim()) options.taskName = filters.taskName.trim()
  if (filters.status) options.status = filters.status
  if (filters.downloaded) options.downloaded = filters.downloaded
  return options
}

function toJobRow(job: BatchImageJob, key = selectedApiKey.value): BatchImageJobRow {
  return {
    id: job.id,
    task_name: job.task_name || defaultTaskName(job.created_at),
    parent_batch_id: job.parent_batch_id || null,
    status: job.status,
    model: job.model,
    provider: job.provider,
    item_count: job.item_count,
    success_count: job.success_count,
    fail_count: job.fail_count,
    estimated_cost: job.estimated_cost,
    hold_amount: job.hold_amount,
    actual_cost: job.actual_cost,
    created_at: job.created_at,
    downloaded_at: job.downloaded_at,
    api_key_id: key?.id || 0,
    api_key_name: key?.name || '',
    child_count: 0,
  }
}

function applyChildCounts(rows: BatchImageJobRow[]) {
  const counts = new Map<string, number>()
  for (const row of rows) {
    if (!row.parent_batch_id) continue
    counts.set(row.parent_batch_id, (counts.get(row.parent_batch_id) || 0) + 1)
  }
  return rows.map(row => ({ ...row, child_count: counts.get(row.id) || 0 }))
}

function displayJob<T extends Pick<BatchImageJob, 'id' | 'parent_batch_id' | 'status' | 'item_count' | 'success_count' | 'fail_count' | 'estimated_cost' | 'hold_amount' | 'actual_cost'>>(job: T): T {
  if (job.parent_batch_id) return job
  const children = childrenByParent.value.get(job.id) || []
  if (!children.length) return job

  const childSuccess = children.reduce((sum, child) => sum + child.success_count, 0)
  const childEstimated = children.reduce((sum, child) => sum + child.estimated_cost, 0)
  const childHold = children.reduce((sum, child) => sum + child.hold_amount, 0)
  const childActual = children.reduce((sum, child) => sum + (child.actual_cost || 0), 0)
  const childActualReady = children.every(child => child.actual_cost !== null)
  const successCount = Math.min(job.item_count, job.success_count + childSuccess)
  const failCount = Math.max(0, job.item_count - successCount)
  const actualCost = job.actual_cost === null
    ? (childActualReady ? childActual : null)
    : job.actual_cost + childActual

  return {
    ...job,
    success_count: successCount,
    fail_count: failCount,
    status: failCount === 0 && TERMINAL_STATUSES.has(job.status) ? 'completed' : job.status,
    estimated_cost: job.estimated_cost + childEstimated,
    hold_amount: job.hold_amount + childHold,
    actual_cost: actualCost,
  }
}

function hasChildJobs(batchId: string) {
  return (childrenByParent.value.get(batchId) || []).length > 0
}

function toggleChildRows(batchId: string) {
  const next = new Set(expandedParentIds.value)
  if (next.has(batchId)) next.delete(batchId)
  else next.add(batchId)
  expandedParentIds.value = next
}

function closeMoreMenu() {
  openMoreJobId.value = ''
}

function toggleMoreMenu(job: BatchImageJobRow, event: MouseEvent) {
  if (openMoreJobId.value === job.id) {
    closeMoreMenu()
    return
  }
  const trigger = event.currentTarget as HTMLElement | null
  const rect = trigger?.getBoundingClientRect()
  if (!rect) return
  const menuWidth = 176
  const margin = 8
  const left = Math.max(margin, Math.min(rect.right - menuWidth, window.innerWidth - menuWidth - margin))
  const top = Math.min(rect.bottom + margin, window.innerHeight - 96)
  moreMenuStyle.value = {
    left: `${left}px`,
    top: `${Math.max(margin, top)}px`,
  }
  openMoreJobId.value = job.id
}

function cancelPromptPopoverClose() {
  if (!promptPopoverCloseTimer) return
  clearTimeout(promptPopoverCloseTimer)
  promptPopoverCloseTimer = null
}

function cancelPromptPopoverOpen() {
  if (!promptPopoverOpenTimer) return
  clearTimeout(promptPopoverOpenTimer)
  promptPopoverOpenTimer = null
}

function closePromptPopover() {
  cancelPromptPopoverOpen()
  cancelPromptPopoverClose()
  promptPopover.visible = false
  promptPopover.text = ''
  promptPopover.style = {}
  activePromptPopoverTarget = null
}

function schedulePromptPopoverClose() {
  cancelPromptPopoverOpen()
  cancelPromptPopoverClose()
  promptPopoverCloseTimer = setTimeout(() => {
    closePromptPopover()
  }, 180)
}

function schedulePromptPopoverOpen(event: MouseEvent | PointerEvent, text: string) {
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const value = String(text || '').trim()
  if (!value || value === '-') return
  activePromptPopoverTarget = target
  cancelPromptPopoverOpen()
  cancelPromptPopoverClose()
  promptPopoverOpenTimer = setTimeout(() => {
    if (activePromptPopoverTarget !== target || !document.body.contains(target)) return
    openPromptPopover(target, value)
  }, 520)
}

function showPromptPopover(event: MouseEvent | FocusEvent, text: string) {
  const value = String(text || '').trim()
  if (!value || value === '-') return
  const target = event.currentTarget as HTMLElement | null
  cancelPromptPopoverClose()
  cancelPromptPopoverOpen()
  if (!target) return
  activePromptPopoverTarget = target
  openPromptPopover(target, value)
}

function openPromptPopover(target: HTMLElement, value: string) {
  const rect = target.getBoundingClientRect()
  if (!rect) return
  const viewportWidth = window.innerWidth || 1280
  const viewportHeight = window.innerHeight || 720
  const width = Math.min(440, Math.max(320, viewportWidth - 32))
  const left = Math.max(16, Math.min(rect.left, viewportWidth - width - 16))
  const estimatedHeight = 178
  const preferredTop = rect.bottom + 8
  const top = preferredTop + estimatedHeight > viewportHeight
    ? Math.max(16, rect.top - estimatedHeight - 8)
    : preferredTop
  promptPopover.text = value
  promptPopover.style = {
    left: `${left}px`,
    top: `${top}px`,
    width: `${width}px`,
  }
  promptPopover.visible = true
}

function copyPromptPopover() {
  if (!promptPopover.text) return
  void copyToClipboard(promptPopover.text, t('batchImage.promptPopover.copied'))
}

async function loadBatchJobs() {
  const keys = filteredApiKeys.value
  if (!keys.length) {
    batchJobs.value = []
    pagination.has_more = false
    return
  }
  loadingJobs.value = true
  closeMoreMenu()
  try {
    const options = listOptions()
    const results = await Promise.all(keys.map(async (key) => {
      const result = await listBatchImageJobs(key.key, options)
      return {
        hasMore: Boolean(result.has_more),
        rows: (result.data || []).map(job => toJobRow(job, key)),
      }
    }))
    batchJobs.value = applyChildCounts(results
      .flatMap(result => result.rows)
      .sort((a, b) => b.created_at - a.created_at)
      .slice(0, pagination.page_size))
    pagination.has_more = results.some(result => result.hasMore)
    selectedJobIds.value = new Set([...selectedJobIds.value].filter(id => visibleBatchJobs.value.some(job => job.id === id)))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadJobsFailed')))
  } finally {
    loadingJobs.value = false
  }
}

function upsertJob(job: BatchImageJob) {
  const next = toJobRow(job)
  const index = batchJobs.value.findIndex(item => item.id === job.id)
  if (index >= 0) {
    const rows = [...batchJobs.value]
    rows[index] = { ...next, is_child: rows[index].is_child }
    batchJobs.value = applyChildCounts(rows)
    return
  }
  batchJobs.value = applyChildCounts([next, ...batchJobs.value].slice(0, pagination.page_size))
}

function handlePageChange(page: number) {
  if (page < 1 || page === pagination.page) return
  pagination.page = page
  selectedJobIds.value = new Set()
  void loadBatchJobs()
}

function handlePageSizeChange(value: string | number | boolean | null) {
  if (value === null || typeof value === 'boolean') return
  const nextSize = Math.min(Math.max(Number(value) || 20, 1), 100)
  pagination.page_size = nextSize
  pagination.page = 1
  setPersistedPageSize(nextSize)
  selectedJobIds.value = new Set()
  void loadBatchJobs()
}

function openCreateModal() {
  clearFormErrors()
  showCreateModal.value = true
  if (!apiKeys.value.length) {
    void loadApiKeys()
  }
}

function closeCreateModal() {
  if (submitting.value) return
  showCreateModal.value = false
  resetCreateDraft()
}

function resetCreateDraft() {
  form.taskName = ''
  form.responseMimeType = 'image/png'
  promptRows.value = []
  promptDraft.value = ''
  customIdDraft.value = ''
  outputCountDraft.value = 1
  referenceImageDrafts.value = []
  clearFormErrors()
}

function closeDetail() {
  closePromptPopover()
  currentJob.value = null
  selectedBatchId.value = ''
  selectedBatchApiKeyId.value = 0
  items.value = []
  clearItemPreviews()
}

function keyForSelectedBatch(): ApiKey | null {
  if (selectedBatchApiKeyId.value) {
    const key = geminiApiKeys.value.find(item => item.id === selectedBatchApiKeyId.value)
    if (key) return key
  }
  return selectedApiKey.value
}

function requireApiKey(): ApiKey | null {
  if (!selectedApiKey.value) {
    appStore.showError(batchImageText('selectApiKey'))
    return null
  }
  return selectedApiKey.value
}

function clearFormErrors() {
  formErrors.apiKey = ''
  formErrors.model = ''
  formErrors.prompts = ''
}

/**
 * Every branch now writes the message to the field that owns it AND raises the
 * toast, so the reason survives the toast timeout. `requireApiKey` is still the
 * gate for the non-form paths (retry, download, delete), which have no field to
 * annotate.
 */
function validateForm(): boolean {
  clearFormErrors()
  if (!requireApiKey()) {
    formErrors.apiKey = batchImageText('selectApiKey')
    return false
  }
  if (!form.model) {
    formErrors.model = availableBatchImageModels.value.length === 0
      ? batchImageText('noModelsForKey')
      : batchImageText('selectModel')
    appStore.showError(formErrors.model)
    return false
  }
  if (parsedItems.value.length === 0) {
    formErrors.prompts = batchImageText('promptRequired')
    appStore.showError(formErrors.prompts)
    return false
  }
  if (estimatedOutputCount.value > BATCH_IMAGE_MAX_OUTPUTS_PER_JOB) {
    formErrors.prompts = batchImageText('tooManyOutputImages')
    appStore.showError(formErrors.prompts)
    return false
  }
  const refLimit = selectedModelReferenceLimit.value
  if (promptRows.value.some(row => row.reference_images.length > refLimit)) {
    formErrors.prompts = batchImageText('tooManyReferenceImages')
    appStore.showError(formErrors.prompts)
    return false
  }
  return true
}

async function submitJob() {
  if (submitting.value) return
  if (promptDraft.value.trim()) addPromptRow()
  if (!validateForm()) return
  const key = requireApiKey()
  if (!key) return
	  submitting.value = true
	  try {
	    const job = await submitBatchImageJob(
	      key.key,
	      {
	        model: form.model,
        task_name: form.taskName.trim() || defaultTaskName(),
        image_size: '1K',
        response_mime_type: form.responseMimeType,
        items: parsedItems.value,
	      },
	      `sub2api-ui-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
	    )
	    currentJob.value = job
	    selectedBatchId.value = job.id
	    selectedBatchApiKeyId.value = key.id
	    items.value = []
	    upsertJob(job)
	    showCreateModal.value = false
	    resetCreateDraft()
	    appStore.showSuccess(batchImageText('submitted'))
	    void loadItems()
	    startPolling()
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('submitFailed')))
  } finally {
    submitting.value = false
  }
}

async function refreshSelected() {
  if (!selectedBatchId.value) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  refreshing.value = true
  try {
    const job = await getBatchImageJob(key.key, selectedBatchId.value)
    currentJob.value = job
    upsertJob(job)
    if (TERMINAL_STATUSES.has(job.status)) stopPolling()
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('refreshFailed')))
  } finally {
    refreshing.value = false
  }
}

async function refreshDetail() {
  await Promise.all([
    refreshSelected(),
    loadItems(),
  ])
}

function selectJob(batchId: string) {
  const row = batchJobs.value.find(job => job.id === batchId)
  if (row?.api_key_id && geminiApiKeys.value.some(key => key.id === row.api_key_id)) {
    form.apiKeyId = row.api_key_id
    selectedBatchApiKeyId.value = row.api_key_id
  } else {
    selectedBatchApiKeyId.value = 0
  }
  selectedBatchId.value = batchId
  currentJob.value = null
  items.value = []
  void refreshSelected()
  void loadItems()
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => {
    if (!currentJob.value || TERMINAL_STATUSES.has(currentJob.value.status)) {
      stopPolling()
      return
    }
    void refreshSelected()
  }, 8000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function canCancel(job: Pick<BatchImageJob, 'status'>) {
  return !TERMINAL_STATUSES.has(job.status)
}

function canDownload(job: Pick<BatchImageJob, 'status' | 'success_count'>) {
  return job.status === 'completed' && job.success_count > 0
}

function canRetry(job: Pick<BatchImageJob, 'status' | 'fail_count'>) {
  const display = 'id' in job ? displayJob(job as BatchImageJob) : job
  return TERMINAL_STATUSES.has(display.status) && display.fail_count > 0
}

function isDownloadingJob(batchId: string) {
  return downloading.value && downloadingBatchId.value === batchId
}

function applyJobApiKey(job: BatchImageJobRow | Pick<BatchImageJob, 'id'>) {
  if ('api_key_id' in job && job.api_key_id && geminiApiKeys.value.some(key => key.id === job.api_key_id)) {
    form.apiKeyId = job.api_key_id
  }
}

function apiKeyForJob(job: BatchImageJobRow | Pick<BatchImageJob, 'id'>): ApiKey | null {
  if ('api_key_id' in job && job.api_key_id) {
    return geminiApiKeys.value.find(key => key.id === job.api_key_id) || null
  }
  return selectedApiKey.value
}

function toggleJobSelection(batchId: string, checked: boolean) {
  const next = new Set(selectedJobIds.value)
  if (checked) next.add(batchId)
  else next.delete(batchId)
  selectedJobIds.value = next
}

function toggleAllVisible(checked: boolean) {
  const next = new Set(selectedJobIds.value)
  for (const job of visibleBatchJobs.value) {
    if (checked) next.add(job.id)
    else next.delete(job.id)
  }
  selectedJobIds.value = next
}

function canDeleteRecord(job: Pick<BatchImageJob, 'status'>) {
  return TERMINAL_STATUSES.has(job.status)
}

async function cancelSelected() {
  if (!currentJob.value) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  if (!window.confirm(batchImageText('cancelConfirm'))) return
  cancelling.value = true
  try {
    const job = await cancelBatchImageJob(key.key, currentJob.value.id)
    currentJob.value = job
    upsertJob(job)
    appStore.showSuccess(batchImageText('cancelled'))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('cancelFailed')))
  } finally {
    cancelling.value = false
  }
}

async function downloadSelected() {
  if (!currentJob.value) return
  await downloadJob(currentJob.value)
}

async function retrySelected() {
  if (!currentJob.value) return
  await retryFailedJob(currentJob.value)
}

async function retryFailedJob(job: BatchImageJobRow | BatchImageJob) {
  if (!canRetry(job) || retryingBatchId.value) return
  closeMoreMenu()
  const key = apiKeyForJob(job) || keyForSelectedBatch() || requireApiKey()
  if (!key) return
  retryingBatchId.value = job.id
  try {
    const sourceItems = await ensureItemsForRetry(key.key, job.id)
    const failedItems = sourceItems
      .filter(item => item.status === 'failed')
      .map(item => ({ custom_id: retryCustomID(item.custom_id), prompt: String(item.prompt_preview || '').trim() }))
      .filter(item => item.prompt)
    if (failedItems.length === 0) {
      appStore.showError(batchImageText('retryMissingPrompts'))
      return
    }
    const retryJob = await submitBatchImageJob(
      key.key,
      {
        model: job.model,
        task_name: `${job.task_name || defaultTaskName()} ${t('batchImage.messages.retryTaskNameSuffix')}`,
        parent_batch_id: rootBatchIdForRetry(job),
        provider: job.provider,
        image_size: '1K',
        response_mime_type: form.responseMimeType,
        items: failedItems,
      },
      `sub2api-ui-retry-${job.id}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
    )
    currentJob.value = retryJob
    selectedBatchId.value = retryJob.id
    selectedBatchApiKeyId.value = key.id
    items.value = []
    upsertJob(retryJob)
    if (retryJob.parent_batch_id) {
      expandedParentIds.value = new Set([...expandedParentIds.value, retryJob.parent_batch_id])
    }
    appStore.showSuccess(batchImageText('retrySubmitted'))
    void loadItems()
    startPolling()
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('retryFailed')))
  } finally {
    retryingBatchId.value = ''
  }
}

async function ensureItemsForRetry(apiKey: string, batchId: string) {
  if (selectedBatchId.value === batchId && items.value.length > 0) {
    return items.value
  }
  const result = await listBatchImageItems(apiKey, batchId)
  return result.data || []
}

function retryCustomID(customID: string) {
  const base = String(customID || 'item').replace(/[^\w.-]+/g, '_').replace(/^_+|_+$/g, '') || 'item'
  return `${base}_retry_${Date.now().toString(36)}`
}

function rootBatchIdForRetry(job: BatchImageJobRow | BatchImageJob) {
  return job.parent_batch_id || job.id
}

async function downloadJob(job: (BatchImageJobRow | Pick<BatchImageJob, 'id'>)) {
  if (downloading.value) return
  closeMoreMenu()
  applyJobApiKey(job)
  const key = apiKeyForJob(job) || requireApiKey()
  if (!key) return
  downloading.value = true
  downloadingBatchId.value = job.id
  try {
    const blob = await downloadBatchImageZip(key.key, job.id)
    saveBlob(blob, `${job.id}.zip`)
    markJobDownloaded(job.id)
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('downloadFailed')))
  } finally {
    downloading.value = false
    downloadingBatchId.value = ''
  }
}

async function downloadSelectedJobs() {
  if (bulkDownloading.value || selectedDownloadableRows.value.length === 0) return
  bulkDownloading.value = true
  try {
    for (const row of selectedDownloadableRows.value) {
      const key = apiKeyForJob(row)
      if (!key) continue
      downloading.value = true
      downloadingBatchId.value = row.id
      const blob = await downloadBatchImageZip(key.key, row.id)
      saveBlob(blob, `${row.id}.zip`)
      markJobDownloaded(row.id)
    }
    appStore.showSuccess(batchImageText('batchDownloadStarted'))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('downloadFailed')))
  } finally {
    bulkDownloading.value = false
    downloading.value = false
    downloadingBatchId.value = ''
  }
}

async function deleteJob(job: BatchImageJobRow) {
  if (!canDeleteRecord(job) || deletingBatchId.value) return
  closeMoreMenu()
  const key = apiKeyForJob(job)
  if (!key) return
  if (!window.confirm(batchImageText('deleteConfirm'))) return
  deletingBatchId.value = job.id
  try {
    await deleteBatchImageJobRecord(key.key, job.id)
    removeJobFromList(job.id)
    appStore.showSuccess(batchImageText('deleted'))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('deleteFailed')))
  } finally {
    deletingBatchId.value = ''
  }
}

async function deleteSelectedJobs() {
  const rows = selectedRows.value.filter(job => canDeleteRecord(job))
  if (bulkDeleting.value || rows.length === 0) return
  if (!window.confirm(batchImageText('deleteSelectedConfirm'))) return
  bulkDeleting.value = true
  try {
    for (const row of rows) {
      const key = apiKeyForJob(row)
      if (!key) continue
      deletingBatchId.value = row.id
      await deleteBatchImageJobRecord(key.key, row.id)
      removeJobFromList(row.id)
    }
    appStore.showSuccess(batchImageText('deleted'))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('deleteFailed')))
  } finally {
    bulkDeleting.value = false
    deletingBatchId.value = ''
  }
}

function markJobDownloaded(batchId: string) {
  const downloadedAt = Math.floor(Date.now() / 1000)
  batchJobs.value = batchJobs.value.map(job => job.id === batchId ? { ...job, downloaded_at: job.downloaded_at || downloadedAt } : job)
  if (currentJob.value?.id === batchId && !currentJob.value.downloaded_at) {
    currentJob.value = { ...currentJob.value, downloaded_at: downloadedAt }
  }
}

function removeJobFromList(batchId: string) {
  batchJobs.value = batchJobs.value.filter(job => job.id !== batchId)
  toggleJobSelection(batchId, false)
  if (currentJob.value?.id === batchId) closeDetail()
}

function canLoadItemPreview(item: BatchImageItem) {
  return (item.status === 'succeeded' || item.status === 'success') && item.image_count > 0
}

function isSuccessfulImageItem(item: Pick<BatchImageItem, 'status' | 'image_count'>) {
  return (item.status === 'succeeded' || item.status === 'success') && item.image_count > 0
}

function detailRootBatchId() {
  return currentJob.value?.parent_batch_id || selectedBatchId.value || currentJob.value?.id || ''
}

function isChildDetailItem(item: Pick<BatchImageDetailItem, 'batch_id'>) {
  const rootBatchId = detailRootBatchId()
  return Boolean(rootBatchId && item.batch_id && item.batch_id !== rootBatchId)
}

function retrySourceCustomID(customID: string) {
  return String(customID || '').replace(/(?:_retry_[a-z0-9]+)+$/i, '')
}

function isRecoveredOriginalFailure(item: BatchImageDetailItem) {
  const rootBatchId = detailRootBatchId()
  return Boolean(
    rootBatchId
    && item.batch_id === rootBatchId
    && item.status === 'failed'
    && recoveredOriginalCustomIds.value.has(item.custom_id),
  )
}

function previewCacheSupported() {
  return typeof window !== 'undefined' && 'indexedDB' in window
}

function previewCacheKey(batchId: string, customID: string, imageIndex = 0) {
  return [batchId, customID, imageIndex].map(part => encodeURIComponent(String(part))).join(':')
}

function itemPreviewKey(item: Pick<BatchImageItem, 'batch_id' | 'custom_id'>) {
  return previewCacheKey(item.batch_id || selectedBatchId.value || currentJob.value?.id || '', item.custom_id, 0)
}

function idbRequest<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function openPreviewCacheDB(): Promise<IDBDatabase | null> {
  if (!previewCacheSupported()) return Promise.resolve(null)
  if (previewCacheDBPromise) return previewCacheDBPromise

  previewCacheDBPromise = new Promise((resolve) => {
    const request = window.indexedDB.open(PREVIEW_CACHE_DB_NAME, 1)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(PREVIEW_CACHE_STORE_NAME)) {
        const store = db.createObjectStore(PREVIEW_CACHE_STORE_NAME, { keyPath: 'key' })
        store.createIndex('lastAccessedAt', 'lastAccessedAt', { unique: false })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => resolve(null)
    request.onblocked = () => resolve(null)
  })
  return previewCacheDBPromise
}

async function getCachedPreviewBlob(cacheKey: string): Promise<Blob | null> {
  const db = await openPreviewCacheDB()
  if (!db) return null
  const record = await idbRequest<PreviewCacheRecord | undefined>(
    db.transaction(PREVIEW_CACHE_STORE_NAME, 'readonly').objectStore(PREVIEW_CACHE_STORE_NAME).get(cacheKey),
  ).catch(() => undefined)
  if (!record?.blob) return null

  const now = Date.now()
  if (now - record.createdAt > PREVIEW_CACHE_MAX_AGE_MS) {
    void deleteCachedPreview(cacheKey)
    return null
  }
  void touchCachedPreview(cacheKey, now)
  return record.blob
}

async function hydrateCachedItemPreviews(detailItems: BatchImageDetailItem[]) {
  const previewableItems = detailItems.filter(item => canLoadItemPreview(item))
  if (!previewableItems.length || !previewCacheSupported()) return

  await Promise.all(previewableItems.map(async (item) => {
    const batchId = item.batch_id || selectedBatchId.value || currentJob.value?.id || ''
    const previewKey = itemPreviewKey(item)
    if (!batchId || itemPreviewUrls[previewKey] || previewErrorIds.value.has(previewKey)) return
    const cached = await getCachedPreviewBlob(previewCacheKey(batchId, item.custom_id, 0)).catch(() => null)
    if (!cached || itemPreviewUrls[previewKey]) return
    itemPreviewUrls[previewKey] = URL.createObjectURL(cached)
  }))
}

async function putCachedPreviewBlob(cacheKey: string, blob: Blob) {
  const db = await openPreviewCacheDB()
  if (!db) return
  const now = Date.now()
  const record: PreviewCacheRecord = {
    key: cacheKey,
    blob,
    size: blob.size,
    createdAt: now,
    lastAccessedAt: now,
  }
  await idbRequest(db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME).put(record)).catch(() => null)
  void cleanupPreviewCache()
}

async function touchCachedPreview(cacheKey: string, lastAccessedAt: number) {
  const db = await openPreviewCacheDB()
  if (!db) return
  const record = await idbRequest<PreviewCacheRecord | undefined>(
    db.transaction(PREVIEW_CACHE_STORE_NAME, 'readonly').objectStore(PREVIEW_CACHE_STORE_NAME).get(cacheKey),
  ).catch(() => undefined)
  if (!record) return
  record.lastAccessedAt = lastAccessedAt
  await idbRequest(db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME).put(record)).catch(() => null)
}

async function deleteCachedPreview(cacheKey: string) {
  const db = await openPreviewCacheDB()
  if (!db) return
  await idbRequest(db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME).delete(cacheKey)).catch(() => null)
}

async function cleanupPreviewCache() {
  const db = await openPreviewCacheDB()
  if (!db) return
  const records = await idbRequest<PreviewCacheRecord[]>(
    db.transaction(PREVIEW_CACHE_STORE_NAME, 'readonly').objectStore(PREVIEW_CACHE_STORE_NAME).getAll(),
  ).catch(() => [])
  if (!records.length) return

  const now = Date.now()
  const sorted = [...records].sort((a, b) => a.lastAccessedAt - b.lastAccessedAt)
  const deleteKeys = new Set<string>()
  let totalBytes = 0
  let keptCount = 0

  for (const record of sorted) {
    if (now - record.createdAt > PREVIEW_CACHE_MAX_AGE_MS) {
      deleteKeys.add(record.key)
      continue
    }
    totalBytes += record.size || record.blob?.size || 0
    keptCount += 1
  }

  for (const record of sorted) {
    if (deleteKeys.has(record.key)) continue
    if (keptCount <= PREVIEW_CACHE_MAX_ENTRIES && totalBytes <= PREVIEW_CACHE_MAX_BYTES) break
    deleteKeys.add(record.key)
    totalBytes -= record.size || record.blob?.size || 0
    keptCount -= 1
  }

  if (!deleteKeys.size) return
  const store = db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME)
  for (const key of deleteKeys) {
    store.delete(key)
  }
}

async function createThumbnailBlob(blob: Blob): Promise<Blob> {
  const source = await loadPreviewImageSource(blob)
  const width = source.width
  const height = source.height
  const scale = Math.min(1, PREVIEW_THUMBNAIL_MAX_EDGE / Math.max(width, height))
  const targetWidth = Math.max(1, Math.round(width * scale))
  const targetHeight = Math.max(1, Math.round(height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = targetWidth
  canvas.height = targetHeight
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas unavailable')
  ctx.drawImage(source.image, 0, 0, targetWidth, targetHeight)
  source.close()
  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((thumbnail) => {
      if (thumbnail) resolve(thumbnail)
      else reject(new Error('thumbnail unavailable'))
    }, 'image/webp', PREVIEW_THUMBNAIL_QUALITY)
  })
}

async function loadPreviewImageSource(blob: Blob): Promise<{ image: PreviewImageSource, width: number, height: number, close: () => void }> {
  if ('createImageBitmap' in window) {
    const bitmap = await window.createImageBitmap(blob)
    return {
      image: bitmap,
      width: bitmap.width,
      height: bitmap.height,
      close: () => bitmap.close(),
    }
  }

  const url = URL.createObjectURL(blob)
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const img = new Image()
      img.onload = () => resolve(img)
      img.onerror = () => reject(new Error('image unavailable'))
      img.src = url
    })
    return {
      image,
      width: image.naturalWidth || image.width,
      height: image.naturalHeight || image.height,
      close: () => URL.revokeObjectURL(url),
    }
  } catch (error) {
    URL.revokeObjectURL(url)
    throw error
  }
}

async function loadItems() {
  const batchId = selectedBatchId.value || currentJob.value?.id || ''
  if (!batchId) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  loadingItems.value = true
  try {
    clearItemPreviews()
    const jobs = detailJobsForBatch(batchId)
    const results = await Promise.all(jobs.map(async (job) => {
      const result = await listBatchImageItems(key.key, job.id)
      return (result.data || []).map(item => ({
        ...item,
        batch_id: job.id,
        source_task_name: detailSourceName(job, batchId),
      }))
    }))
    const detailItems = results.flat()
    items.value = detailItems
    void hydrateCachedItemPreviews(detailItems)
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadItemsFailed')))
  } finally {
    loadingItems.value = false
  }
}

function detailJobsForBatch(batchId: string): BatchImageJobRow[] {
  const row = batchJobs.value.find(job => job.id === batchId)
  const base = row || (currentJob.value && currentJob.value.id === batchId ? toJobRow(currentJob.value, keyForSelectedBatch() || selectedApiKey.value) : null)
  if (!base) return []
  if (base.parent_batch_id) return [base]
  return [base, ...(childrenByParent.value.get(base.id) || [])]
}

function detailSourceName(job: Pick<BatchImageJobRow, 'id' | 'task_name' | 'parent_batch_id'>, rootBatchId: string) {
  const name = job.task_name || job.id
  if (job.id === rootBatchId) return t('batchImage.detail.mainTask', { name })
  return t('batchImage.detail.childTask', { name })
}

async function loadItemPreview(item: BatchImageItem) {
  const batchId = item.batch_id || selectedBatchId.value || currentJob.value?.id || ''
  const previewKey = itemPreviewKey(item)
  if (!batchId || !canLoadItemPreview(item) || (itemPreviewUrls[previewKey] && !previewErrorIds.value.has(previewKey))) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  const cacheKey = previewCacheKey(batchId, item.custom_id, 0)
  previewLoadingIds.value = new Set([...previewLoadingIds.value, previewKey])
  try {
    previewErrorIds.value = new Set([...previewErrorIds.value].filter(id => id !== previewKey))
    if (itemPreviewUrls[previewKey]) {
      URL.revokeObjectURL(itemPreviewUrls[previewKey])
      delete itemPreviewUrls[previewKey]
    }
    const cached = await getCachedPreviewBlob(cacheKey)
    if (cached) {
      itemPreviewUrls[previewKey] = URL.createObjectURL(cached)
      return
    }
    const blob = await getBatchImageItemContent(key.key, batchId, item.custom_id, 0)
    const thumbnail = await createThumbnailBlob(blob).catch(() => blob)
    itemPreviewUrls[previewKey] = URL.createObjectURL(thumbnail)
    if (thumbnail !== blob || thumbnail.size <= 1024 * 1024) {
      void putCachedPreviewBlob(cacheKey, thumbnail)
    }
  } catch (error: any) {
    previewErrorIds.value = new Set([...previewErrorIds.value, previewKey])
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadPreviewFailed')))
  } finally {
    const next = new Set(previewLoadingIds.value)
    next.delete(previewKey)
    previewLoadingIds.value = next
  }
}

function openImagePreview(item: BatchImageItem) {
  const previewKey = itemPreviewKey(item)
  if (!itemPreviewUrls[previewKey] || previewErrorIds.value.has(previewKey)) return
  previewImageItem.value = item
}

function closeImagePreview() {
  previewImageItem.value = null
}

function handlePreviewError(customID: string) {
  if (itemPreviewUrls[customID]) {
    URL.revokeObjectURL(itemPreviewUrls[customID])
    delete itemPreviewUrls[customID]
  }
  previewErrorIds.value = new Set([...previewErrorIds.value, customID])
}

function clearItemPreviews() {
  closePromptPopover()
  for (const url of Object.values(itemPreviewUrls)) {
    if (url) URL.revokeObjectURL(url)
  }
  for (const key of Object.keys(itemPreviewUrls)) {
    delete itemPreviewUrls[key]
  }
  previewLoadingIds.value = new Set()
  previewErrorIds.value = new Set()
  previewImageItem.value = null
}

function copyInstruction() {
  void copyToClipboard(agentInstruction.value, batchImageText('copiedInstruction'))
}

function statusLabel(jobOrStatus: BatchImageStatus | Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>) {
  const status = typeof jobOrStatus === 'string' ? jobOrStatus : jobOrStatus.status
  if (typeof jobOrStatus !== 'string' && status === 'completed' && jobOrStatus.fail_count > 0) {
    if (jobOrStatus.success_count > 0) return t('batchImage.status.partialSuccess')
    return t('batchImage.status.allFailed')
  }
  const statusKeys: Record<string, string> = {
    queued: 'queued',
    running: 'running',
    indexing: 'processingResults',
    processing_results: 'processingResults',
    settling: 'settling',
    completed: 'completed',
    failed: 'failed',
    cancelled: 'cancelled',
    output_deleted: 'outputDeleted',
  }
  const key = statusKeys[status]
  return key ? t(`batchImage.status.${key}`) : status
}

/**
 * Job status as a semantic tone.
 *
 * Two things changed from the badge classes this replaces. In-flight states were
 * `badge-primary`, i.e. the accent — but the accent means interaction and
 * selection in this system and may never carry status, so they are `info`. And
 * `cancelled` was `badge-danger`: a cancellation the user asked for is not a
 * failure, and painting it red leaves nothing louder for a job that actually
 * broke.
 */
function statusTone(
  jobOrStatus: BatchImageStatus | Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>,
): Tone {
  const status = typeof jobOrStatus === 'string' ? jobOrStatus : jobOrStatus.status
  if (typeof jobOrStatus !== 'string' && status === 'completed' && jobOrStatus.fail_count > 0) {
    return jobOrStatus.success_count > 0 ? 'warn' : 'danger'
  }
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'cancelled' || status === 'output_deleted') return 'neutral'
  return 'info'
}

function itemStatusLabel(status: string) {
  const statusKeys: Record<string, string> = {
    pending: 'pending',
    succeeded: 'succeeded',
    success: 'succeeded',
    failed: 'failed',
    cancelled: 'cancelled',
  }
  const key = statusKeys[status]
  return key ? t(`batchImage.itemStatus.${key}`) : status
}

function itemDisplayStatusLabel(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) return t('batchImage.itemStatus.recovered')
  return itemStatusLabel(item.status)
}

function itemStatusTone(item: BatchImageDetailItem): Tone {
  if (isRecoveredOriginalFailure(item)) return 'neutral'
  if (item.status === 'succeeded' || item.status === 'success') return 'success'
  if (item.status === 'failed') return 'danger'
  if (item.status === 'cancelled') return 'neutral'
  return 'info'
}

function itemResultLabel(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) return t('batchImage.itemResult.recoveredByRetry')
  if (item.error) return friendlyItemError(item.error)
  if (item.status === 'succeeded' || item.status === 'success') {
    return itemPreviewUrls[itemPreviewKey(item)] ? t('batchImage.itemResult.readyPreview') : t('batchImage.itemResult.readyDownload')
  }
  if (item.status === 'failed') return t('batchImage.itemResult.noUsableImage')
  if (item.status === 'cancelled') return t('batchImage.itemResult.cancelled')
  return t('batchImage.itemResult.waiting')
}

function itemResultTone(item: BatchImageDetailItem): Tone {
  if (isRecoveredOriginalFailure(item)) return 'neutral'
  if (item.error || item.status === 'failed') return 'danger'
  if (item.status === 'cancelled') return 'neutral'
  if (item.status === 'succeeded' || item.status === 'success') return 'success'
  return 'neutral'
}

function friendlyItemError(error: BatchImageItem['error']) {
  if (!error) return '-'
  if (error.code === 'EMPTY_IMAGE_OUTPUT') return t('batchImage.itemResult.emptyImageOutput')
  if (error.code === 'PROVIDER_ITEM_FAILED') return t('batchImage.itemResult.providerItemFailed')
  return error.message || error.code || '-'
}

function terminalZeroCost(job: Pick<BatchImageJob, 'status' | 'actual_cost'>) {
  return job.actual_cost === null && (job.status === 'failed' || job.status === 'cancelled')
}

/**
 * Money is a quantity, so it goes through `NumCell` rather than a hand-rolled
 * `$0.00`. That means the single `costLabel` string has to split into "which
 * number" and "is that number a hold" — the template then renders the hold case
 * through `i18n-t` with a `NumCell` in the `{amount}` slot, so the number stays
 * locale-formatted, mono and tabular inside the sentence.
 */
function costIsHold(job: Pick<BatchImageJob, 'status' | 'actual_cost'>) {
  return job.actual_cost === null && !terminalZeroCost(job)
}

/**
 * A terminal job that never charged settled at a real 0. Returning `null` for it
 * would render an en dash, and "we did not charge you" is not the same fact as
 * "we have not measured this yet".
 */
function settledCost(job: Pick<BatchImageJob, 'status' | 'actual_cost'>) {
  if (job.actual_cost !== null) return job.actual_cost
  return terminalZeroCost(job) ? 0 : null
}

type BatchImageTextKey =
  | 'loadKeysFailed'
  | 'loadModelsFailed'
  | 'loadJobsFailed'
  | 'selectApiKey'
  | 'noModelsForKey'
  | 'selectModel'
  | 'promptRequired'
  | 'submitted'
  | 'submitFailed'
  | 'refreshFailed'
  | 'cancelConfirm'
  | 'cancelled'
  | 'cancelFailed'
  | 'batchDownloadStarted'
	  | 'downloadFailed'
	  | 'retrySubmitted'
	  | 'retryFailed'
	  | 'retryMissingPrompts'
  | 'deleteConfirm'
  | 'deleteSelectedConfirm'
  | 'deleted'
  | 'deleteFailed'
	  | 'loadItemsFailed'
	  | 'loadPreviewFailed'
  | 'copiedInstruction'
  | 'loadingModels'
  | 'noModels'
  | 'noModelsHint'
  | 'noCompatibleAccount'
  | 'unsupportedProvider'
  | 'providerSubmitFailed'
  | 'vertexGcsBucketMissing'
  | 'queueFailed'
  | 'billingHoldFailed'
  | 'groupDisabled'
  | 'pricingMissing'
  | 'insufficientBalance'
  | 'invalidModel'
  | 'invalidItems'
  | 'duplicateCustomId'
  | 'promptTooLong'
  | 'invalidReferenceImage'
  | 'tooManyReferenceImages'
  | 'referenceImagesTooLarge'
  | 'tooManyOutputImages'
  | 'idempotencyConflict'
  | 'notReady'
  | 'outputDeleted'
  | 'resultMissing'
  | 'itemFailed'
  | 'itemImageIndexOutOfRange'
  | 'downloadLimited'
  | 'downloadTooLarge'
  | 'deleteNotReady'
  | 'disabled'
  | 'authRequired'
  | 'adminReference'
  | 'errorReference'

function isZhLocale() {
  return String(locale.value || '').toLowerCase().startsWith('zh')
}

function batchImageText(key: BatchImageTextKey) {
  return t(`batchImage.messages.${key}`)
}

function batchImageErrorReference(error: any) {
  const parts: string[] = []
  const code = String(error?.code || '').trim()
  const requestId = String(error?.requestId || '').trim()
  const status = String(error?.status || '').trim()
  if (code) parts.push(t('batchImage.messages.errorCodeRef', { code }))
  if (requestId) parts.push(t('batchImage.messages.requestIdRef', { id: requestId }))
  if (!code && status) parts.push(t('batchImage.messages.httpStatusRef', { status }))
  if (!parts.length) return ''
  // The separator was already locale-gated but the brackets were not, so an
  // English error came back wrapped in fullwidth CJK parentheses.
  return isZhLocale() ? `（${parts.join('，')}）` : `(${parts.join(', ')})`
}

function batchImageAdminError(base: string, error: any) {
  const reference = batchImageErrorReference(error)
  return `${base}${reference ? ` ${reference}` : ''} ${batchImageText('adminReference')}`
}

function batchImagePlainError(base: string) {
  return base
}

function batchImageErrorMessage(error: any, fallback: string) {
  const code = String(error?.code || '').trim()
  const message = String(error?.message || '').trim()
  if (code === 'API_KEY_REQUIRED' || code === '401') {
    return batchImagePlainError(batchImageText('authRequired'))
  }
  if (code === 'BATCH_IMAGE_NO_ACCOUNT_AVAILABLE' || /no compatible batch image account/i.test(message)) {
    return batchImageAdminError(batchImageText('noCompatibleAccount'), error)
  }
  if (code === 'BATCH_IMAGE_UNSUPPORTED_PROVIDER' || /unsupported batch image provider/i.test(message)) {
    return batchImageAdminError(batchImageText('unsupportedProvider'), error)
  }
  if (code === 'BATCH_IMAGE_VERTEX_GCS_BUCKET_MISSING' || code === 'VERTEX_MANAGED_GCS_BUCKET_MISSING') {
    return batchImageAdminError(batchImageText('vertexGcsBucketMissing'), error)
  }
  if (
    code === 'BATCH_IMAGE_PROVIDER_SUBMIT_FAILED' ||
    code === 'BATCH_IMAGE_PROVIDER_MISSING_API_KEY' ||
    code === 'BATCH_IMAGE_PROVIDER_MISSING_SERVICE_ACCOUNT' ||
    code === 'BATCH_IMAGE_PROVIDER_UNSUPPORTED_ACCOUNT'
  ) {
    return batchImageAdminError(batchImageText('providerSubmitFailed'), error)
  }
  if (code === 'BATCH_IMAGE_QUEUE_FAILED' || code === 'BATCH_IMAGE_QUEUE_NOT_CONFIGURED') {
    return batchImageAdminError(batchImageText('queueFailed'), error)
  }
  if (code === 'BATCH_IMAGE_BILLING_HOLD_FAILED') {
    return batchImageAdminError(batchImageText('billingHoldFailed'), error)
  }
  if (code === 'BATCH_IMAGE_GROUP_DISABLED') {
    return batchImagePlainError(batchImageText('groupDisabled'))
  }
  if (code === 'BATCH_IMAGE_SETTLEMENT_PRICING_MISSING') {
    return batchImageAdminError(batchImageText('pricingMissing'), error)
  }
  if (code === 'BATCH_IMAGE_INSUFFICIENT_BALANCE') {
    return batchImagePlainError(batchImageText('insufficientBalance'))
  }
  if (code === 'BATCH_IMAGE_INVALID_MODEL') {
    return batchImageText('invalidModel')
  }
  if (code === 'BATCH_IMAGE_INVALID_ITEMS') {
    return batchImageText('invalidItems')
  }
  if (code === 'BATCH_IMAGE_DUPLICATE_CUSTOM_ID') {
    return batchImageText('duplicateCustomId')
  }
  if (code === 'BATCH_IMAGE_PROMPT_TOO_LONG') {
    return batchImageText('promptTooLong')
  }
  if (code === 'BATCH_IMAGE_INVALID_REFERENCE_IMAGE') {
    return batchImageText('invalidReferenceImage')
  }
  if (code === 'BATCH_IMAGE_TOO_MANY_REFERENCE_IMAGES') {
    return batchImageText('tooManyReferenceImages')
  }
  if (code === 'BATCH_IMAGE_REFERENCE_IMAGES_TOO_LARGE') {
    return batchImageText('referenceImagesTooLarge')
  }
  if (code === 'BATCH_IMAGE_TOO_MANY_OUTPUT_IMAGES') {
    return batchImageText('tooManyOutputImages')
  }
  if (code === 'BATCH_IMAGE_IDEMPOTENCY_CONFLICT') {
    return batchImagePlainError(batchImageText('idempotencyConflict'))
  }
  if (code === 'BATCH_IMAGE_NOT_READY') {
    return batchImageText('notReady')
  }
  if (code === 'BATCH_IMAGE_OUTPUT_DELETED') {
    return batchImageText('outputDeleted')
  }
  if (code === 'BATCH_IMAGE_RESULT_MISSING') {
    return batchImageAdminError(batchImageText('resultMissing'), error)
  }
  if (code === 'BATCH_IMAGE_ITEM_FAILED') {
    return batchImagePlainError(batchImageText('itemFailed'))
  }
  if (code === 'BATCH_IMAGE_ITEM_IMAGE_INDEX_OUT_OF_RANGE') {
    return batchImagePlainError(batchImageText('itemImageIndexOutOfRange'))
  }
  if (code === 'BATCH_IMAGE_DOWNLOAD_LIMITED') {
    return batchImageText('downloadLimited')
  }
  if (code === 'BATCH_IMAGE_DOWNLOAD_TOO_LARGE') {
    return batchImageText('downloadTooLarge')
  }
  if (code === 'BATCH_IMAGE_RECORD_DELETE_NOT_READY') {
    return batchImagePlainError(batchImageText('deleteNotReady'))
  }
  if (code === 'BATCH_IMAGE_DISABLED') {
    return batchImageAdminError(batchImageText('disabled'), error)
  }
  if (code === 'INTERNAL_ERROR' || code === '500') {
    return batchImageAdminError(fallback, error)
  }
  if (isZhLocale()) {
    const detail = message ? `${batchImageText('errorReference')}：${message}` : batchImageText('adminReference')
    return `${fallback}。${detail} ${batchImageErrorReference(error)}`
  }
  return message || fallback
}

function formatDate(timestamp: number) {
  if (!timestamp) return ''
  return new Date(timestamp * 1000).toLocaleString()
}

function defaultTaskName(timestamp?: number) {
  const date = timestamp ? new Date(timestamp * 1000) : new Date()
  return date.toLocaleString()
}

onMounted(() => {
  void appStore.fetchPublicSettings()
  void refreshPage()
  void cleanupPreviewCache()
  previewCacheCleanupTimer = setInterval(() => {
    void cleanupPreviewCache()
  }, 60 * 60 * 1000)
  document.addEventListener('click', closeMoreMenu)
  window.addEventListener('resize', closeMoreMenu)
  window.addEventListener('scroll', closeMoreMenu, true)
  window.addEventListener('resize', closePromptPopover)
  window.addEventListener('scroll', closePromptPopover, true)
})

watch(
  () => form.apiKeyId,
  () => {
    void loadAvailableModels()
  },
)

watch(
  () => form.model,
  () => {
    const limit = selectedModelReferenceLimit.value
    if (limit <= 0) {
      referenceImageDrafts.value = []
      return
    }
    if (referenceImageDrafts.value.length > limit) {
      referenceImageDrafts.value = referenceImageDrafts.value.slice(0, limit)
    }
  },
)

onBeforeUnmount(() => {
  stopPolling()
  if (previewCacheCleanupTimer) {
    clearInterval(previewCacheCleanupTimer)
    previewCacheCleanupTimer = null
  }
  clearItemPreviews()
  document.removeEventListener('click', closeMoreMenu)
  window.removeEventListener('resize', closeMoreMenu)
  window.removeEventListener('scroll', closeMoreMenu, true)
  window.removeEventListener('resize', closePromptPopover)
  window.removeEventListener('scroll', closePromptPopover, true)
})
</script>

<style scoped>
/*
 * What used to be here and is deliberately gone:
 *
 *   `.batch-row-action` — a stack of `!important` flex overrides propping up
 *     hand-built icon buttons. Those are `Button` components now, which own
 *     their own geometry, so there is nothing left to override.
 *
 *   `.batch-row-action:focus { outline: none }` and
 *   `.batch-prompt-trigger:focus { outline: none; box-shadow: none }` — two bare
 *     outline suppressions. The prompt cell is a real tab stop (it opens its
 *     popover on `focus`), so removing its focus ring left keyboard users with
 *     no idea where they were. Focus is the global `:focus-visible` outline in
 *     style.css and nothing in this file may cancel it.
 *
 *   `.batch-output-count-select` — a hardcoded 36px height patching a native
 *     `<select class="input">` into alignment. That control is a `Select` now and
 *     takes its height from the same token as every other control on the row.
 */
.batch-prompt-popover {
  user-select: text;
}

.batch-prompt-popover p {
  scrollbar-width: thin;
}

/*
 * The agent instruction is prose, not source, so it wraps at spaces the way the
 * old `<textarea>` did — `white-space: pre` would force the reader to scroll
 * sideways through every Chinese paragraph. Indentation in the embedded JSON is
 * still preserved, and `overflow: auto` on the element catches any single
 * unbreakable token (a long URL) so the dialog itself never scrolls sideways.
 */
.batch-instruction {
  white-space: pre-wrap;
  tab-size: 2;
  user-select: text;
}
</style>
