import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import BackupView from '../BackupView.vue'

const {
  getS3Config,
  getImageStorageConfig,
  getSchedule,
  listBackups,
  getDownloadURL,
REDACTED = vi.hoisted(() => ({
  getS3Config: vi.fn(),
  getImageStorageConfig: vi.fn(),
  getSchedule: vi.fn(),
  listBackups: vi.fn(),
  getDownloadURL: vi.fn(),
REDACTED))

vi.mock('@/api', () => ({
  adminAPI: {
    backup: {
      getS3Config,
      updateS3Config: vi.fn(),
      testS3Connection: vi.fn(),
      getImageStorageConfig,
      updateImageStorageConfig: vi.fn(),
      testImageStorageConnection: vi.fn(),
      getSchedule,
      updateSchedule: vi.fn(),
      createBackup: vi.fn(),
      listBackups,
      getBackup: vi.fn(),
      deleteBackup: vi.fn(),
      getDownloadURL,
      restoreBackup: vi.fn(),
    REDACTED,
  REDACTED,
REDACTED))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  REDACTED),
REDACTED))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (fn: () => unknown) => fn() REDACTED),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.index === undefined ? key : `${keyREDACTED:${params.indexREDACTED`,
  REDACTED),
REDACTED))

const baseRecord = (id: string, parts?: unknown[]) => ({
  id,
  status: 'completed',
  backup_type: 'postgres',
  file_name: `${idREDACTED.sql.gz`,
  s3_key: `backups/${idREDACTED.sql.gz`,
  parts,
  size_bytes: 10,
  triggered_by: 'manual',
  started_at: '2026-08-09T00:00:00Z',
REDACTED)

function mountBackupView() {
  return mount(BackupView, {
    global: {
      stubs: {
        TotpStepUpDialog: true,
      REDACTED,
    REDACTED,
  REDACTED)
REDACTED

describe('admin BackupView 分卷备份', () => {
  beforeEach(() => {
    getS3Config.mockResolvedValue({REDACTED)
    getImageStorageConfig.mockResolvedValue({ config: {REDACTED, secret_configured: false REDACTED)
    getSchedule.mockResolvedValue({ enabled: false, cron_expr: '', retain_days: 14, retain_count: 10 REDACTED)
    getDownloadURL.mockReset()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {REDACTED)
  REDACTED)

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  REDACTED)

  it('显示分卷数并在下载时列出每个分卷链接', async () => {
    listBackups.mockResolvedValue({
      items: [baseRecord('split', [{ index: 1 REDACTED, { index: 2 REDACTED, { index: 3 REDACTED])],
    REDACTED)
    getDownloadURL.mockResolvedValue({
      parts: [
        { index: 1, size_bytes: 5, url: 'https://example.test/part-1' REDACTED,
        { index: 2, size_bytes: 6, url: 'https://example.test/part-2' REDACTED,
        { index: 3, size_bytes: 7, url: 'https://example.test/part-3' REDACTED,
      ],
    REDACTED)

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.text()).toContain('3')
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    expect(downloadButton).toBeDefined()
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:1')
    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:3')
    expect(document.body.querySelector('a[href="https://example.test/part-2"]')).not.toBeNull()
  REDACTED)

  it('旧单文件记录仍使用单个下载地址', async () => {
    listBackups.mockResolvedValue({ items: [baseRecord('legacy')] REDACTED)
    getDownloadURL.mockResolvedValue({ url: 'https://example.test/legacy.sql.gz' REDACTED)

    const wrapper = mountBackupView()
    await flushPromises()
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(getDownloadURL).toHaveBeenCalledWith('legacy')
    expect(document.body.textContent).not.toContain('admin.backup.actions.downloadParts')
  REDACTED)

  it('运行中的备份不显示删除入口', async () => {
    listBackups.mockResolvedValue({
      items: [{ ...baseRecord('running'), status: 'running', progress: 'uploading' REDACTED],
    REDACTED)

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('-')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  REDACTED)
REDACTED)
