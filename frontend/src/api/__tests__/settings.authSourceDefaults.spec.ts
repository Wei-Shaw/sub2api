import { describe, expect, it REDACTED from 'vitest'

import {
  appendAuthSourceDefaultsToUpdateRequest,
  buildAuthSourceDefaultsState,
  type UpdateSettingsRequest,
REDACTED from '@/api/admin/settings'

describe('admin settings auth source defaults helpers', () => {
  it('builds auth source defaults state from flat settings fields', () => {
    const state = buildAuthSourceDefaultsState({
      auth_source_default_email_balance: 9.5,
      auth_source_default_email_concurrency: 3,
      auth_source_default_email_subscriptions: [
        { group_id: 1, validity_days: 30 REDACTED,
      ],
      auth_source_default_email_grant_on_signup: false,
      auth_source_default_email_grant_on_first_bind: true,
      auth_source_default_linuxdo_balance: 6,
      auth_source_default_linuxdo_concurrency: 8,
      auth_source_default_linuxdo_subscriptions: [
        { group_id: 2, validity_days: 60 REDACTED,
      ],
      auth_source_default_linuxdo_grant_on_signup: true,
      auth_source_default_linuxdo_grant_on_first_bind: false,
    REDACTED)

    expect(state.email).toEqual({
      balance: 9.5,
      concurrency: 3,
      subscriptions: [{ group_id: 1, validity_days: 30 REDACTED],
      grant_on_signup: false,
      grant_on_first_bind: true,
    REDACTED)
    expect(state.linuxdo).toEqual({
      balance: 6,
      concurrency: 8,
      subscriptions: [{ group_id: 2, validity_days: 60 REDACTED],
      grant_on_signup: true,
      grant_on_first_bind: false,
    REDACTED)
    expect(state.oidc).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: true,
      grant_on_first_bind: false,
    REDACTED)
    expect(state.wechat).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: true,
      grant_on_first_bind: false,
    REDACTED)
  REDACTED)

  it('appends auth source defaults back onto update payload', () => {
    const payload: UpdateSettingsRequest = {
      site_name: 'Sub2API',
    REDACTED

    appendAuthSourceDefaultsToUpdateRequest(payload, {
      email: {
        balance: 1.25,
        concurrency: 2,
        subscriptions: [{ group_id: 3, validity_days: 7 REDACTED],
        grant_on_signup: true,
        grant_on_first_bind: false,
      REDACTED,
      linuxdo: {
        balance: 0,
        concurrency: 6,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: true,
      REDACTED,
      oidc: {
        balance: 4,
        concurrency: 9,
        subscriptions: [{ group_id: 9, validity_days: 90 REDACTED],
        grant_on_signup: true,
        grant_on_first_bind: true,
      REDACTED,
      wechat: {
        balance: 2,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
      REDACTED,
    REDACTED)

    expect(payload).toMatchObject({
      site_name: 'Sub2API',
      auth_source_default_email_balance: 1.25,
      auth_source_default_email_concurrency: 2,
      auth_source_default_email_subscriptions: [{ group_id: 3, validity_days: 7 REDACTED],
      auth_source_default_email_grant_on_signup: true,
      auth_source_default_email_grant_on_first_bind: false,
      auth_source_default_linuxdo_balance: 0,
      auth_source_default_linuxdo_concurrency: 6,
      auth_source_default_linuxdo_subscriptions: [],
      auth_source_default_linuxdo_grant_on_signup: false,
      auth_source_default_linuxdo_grant_on_first_bind: true,
      auth_source_default_oidc_balance: 4,
      auth_source_default_oidc_concurrency: 9,
      auth_source_default_oidc_subscriptions: [{ group_id: 9, validity_days: 90 REDACTED],
      auth_source_default_oidc_grant_on_signup: true,
      auth_source_default_oidc_grant_on_first_bind: true,
      auth_source_default_wechat_balance: 2,
      auth_source_default_wechat_concurrency: 5,
      auth_source_default_wechat_subscriptions: [],
      auth_source_default_wechat_grant_on_signup: false,
      auth_source_default_wechat_grant_on_first_bind: false,
    REDACTED)
  REDACTED)
REDACTED)
