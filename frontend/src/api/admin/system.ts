/**
 * System API endpoints for admin operations
 */

import { apiClient REDACTED from '../client';

export interface ReleaseInfo {
  name: string;
  body: string;
  published_at: string;
  html_url: string;
REDACTED

export interface VersionInfo {
  current_version: string;
  latest_version: string;
  has_update: boolean;
  release_info?: ReleaseInfo;
  cached: boolean;
  warning?: string;
  build_type: string; // "source" for manual builds, "release" for CI builds
REDACTED

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string REDACTED> {
  const { data REDACTED = await apiClient.get<{ version: string REDACTED>('/admin/system/version');
  return data;
REDACTED

/**
 * Check for updates
 * @param force - Force refresh from GitHub API
 */
export async function checkUpdates(force = false): Promise<VersionInfo> {
  const { data REDACTED = await apiClient.get<VersionInfo>('/admin/system/check-updates', {
    params: force ? { force: 'true' REDACTED : undefined,
  REDACTED);
  return data;
REDACTED

export interface UpdateResult {
  message: string;
  need_restart: boolean;
REDACTED

/**
 * Perform system update
 * Downloads and applies the latest version
 */
export async function performUpdate(): Promise<UpdateResult> {
  const { data REDACTED = await apiClient.post<UpdateResult>('/admin/system/update');
  return data;
REDACTED

/**
 * Rollback to previous version
 */
export async function rollback(): Promise<UpdateResult> {
  const { data REDACTED = await apiClient.post<UpdateResult>('/admin/system/rollback');
  return data;
REDACTED

/**
 * Restart the service
 */
export async function restartService(): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>('/admin/system/restart');
  return data;
REDACTED

export const systemAPI = {
  getVersion,
  checkUpdates,
  performUpdate,
  rollback,
  restartService,
REDACTED;

export default systemAPI;
