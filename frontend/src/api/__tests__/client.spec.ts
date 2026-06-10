import { describe, it, expect, vi, beforeEach, afterEach REDACTED from 'vitest'
import axios from 'axios'
import type { AxiosInstance REDACTED from 'axios'

// 需要在导入 client 之前设置 mock
vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN',
REDACTED))

describe('API Client', () => {
  let apiClient: AxiosInstance

  beforeEach(async () => {
    localStorage.clear()
    // 每次测试重新导入以获取干净的模块状态
    vi.resetModules()
    const mod = await import('@/api/client')
    apiClient = mod.apiClient
  REDACTED)

  afterEach(() => {
    vi.restoreAllMocks()
  REDACTED)

  // --- 请求拦截器 ---

  describe('请求拦截器', () => {
    it('自动附加 Authorization 头', async () => {
      localStorage.setItem('auth_token', 'my-jwt-token')

      // 拦截实际请求
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {REDACTED REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await apiClient.get('/test')

      const config = adapter.mock.calls[0][0]
      expect(config.headers.get('Authorization')).toBe('Bearer my-jwt-token')
    REDACTED)

    it('无 token 时不附加 Authorization 头', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {REDACTED REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await apiClient.get('/test')

      const config = adapter.mock.calls[0][0]
      expect(config.headers.get('Authorization')).toBeFalsy()
    REDACTED)

    it('GET 请求自动附加 timezone 参数', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {REDACTED REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await apiClient.get('/test')

      const config = adapter.mock.calls[0][0]
      expect(config.params).toHaveProperty('timezone')
    REDACTED)

    it('POST 请求不附加 timezone 参数', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {REDACTED REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await apiClient.post('/test', { foo: 'bar' REDACTED)

      const config = adapter.mock.calls[0][0]
      expect(config.params?.timezone).toBeUndefined()
    REDACTED)

    it('请求默认带 withCredentials 以支持跨域 cookie', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {REDACTED REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await apiClient.post('/auth/oauth/bind-token')

      const config = adapter.mock.calls[0][0]
      expect(config.withCredentials).toBe(true)
    REDACTED)
  REDACTED)

  // --- 响应拦截器 ---

  describe('响应拦截器', () => {
    it('code=0 时解包 data 字段', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: { name: 'test' REDACTED, message: 'ok' REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      const response = await apiClient.get('/test')
      expect(response.data).toEqual({ name: 'test' REDACTED)
    REDACTED)

    it('code!=0 时拒绝并返回结构化错误', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 1001, message: '参数错误', data: null REDACTED,
        headers: {REDACTED,
        config: {REDACTED,
        statusText: 'OK',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await expect(apiClient.get('/test')).rejects.toEqual(
        expect.objectContaining({
          code: 1001,
          message: '参数错误',
        REDACTED)
      )
    REDACTED)

    it('部署与运营合规未确认时广播事件且保留登录态', async () => {
      localStorage.setItem('auth_token', 'admin-token')
      const listener = vi.fn()
      window.addEventListener('admin-compliance-required', listener)

      const adapter = vi.fn().mockRejectedValue({
        response: {
          status: 423,
          data: {
            code: 'ADMIN_COMPLIANCE_ACK_REQUIRED',
            message: 'administrator compliance acknowledgement is required',
            metadata: {
              version: 'v2026.06.10',
              document_path_zh: 'docs/legal/admin-compliance.zh.md',
              document_path_en: 'docs/legal/admin-compliance.en.md',
            REDACTED,
          REDACTED,
        REDACTED,
        config: {
          url: '/admin/users',
          headers: { Authorization: 'Bearer admin-token' REDACTED,
        REDACTED,
        code: 'ERR_BAD_REQUEST',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await expect(apiClient.get('/admin/users')).rejects.toEqual(
        expect.objectContaining({
          status: 423,
          code: 'ADMIN_COMPLIANCE_ACK_REQUIRED',
          metadata: expect.objectContaining({
            version: 'v2026.06.10',
          REDACTED),
        REDACTED)
      )

      expect(listener).toHaveBeenCalledTimes(1)
      expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual(
        expect.objectContaining({
          version: 'v2026.06.10',
        REDACTED)
      )
      expect(localStorage.getItem('auth_token')).toBe('admin-token')

      window.removeEventListener('admin-compliance-required', listener)
    REDACTED)
  REDACTED)

  // --- 401 Token 刷新 ---

  describe('401 Token 刷新', () => {
    it('无 refresh_token 时 401 清除 localStorage', async () => {
      localStorage.setItem('auth_token', 'expired-token')
      // 不设置 refresh_token

      // Mock window.location
      const originalLocation = window.location
      Object.defineProperty(window, 'location', {
        value: { ...originalLocation, pathname: '/dashboard', href: '/dashboard' REDACTED,
        writable: true,
      REDACTED)

      const adapter = vi.fn().mockRejectedValue({
        response: {
          status: 401,
          data: { code: 'TOKEN_EXPIRED', message: 'Token expired' REDACTED,
        REDACTED,
        config: {
          url: '/test',
          headers: { Authorization: 'Bearer expired-token' REDACTED,
        REDACTED,
        code: 'ERR_BAD_REQUEST',
      REDACTED)
      apiClient.defaults.adapter = adapter

      await expect(apiClient.get('/test')).rejects.toBeDefined()

      expect(localStorage.getItem('auth_token')).toBeNull()

      // 恢复 location
      Object.defineProperty(window, 'location', {
        value: originalLocation,
        writable: true,
      REDACTED)
    REDACTED)
  REDACTED)

  // --- 网络错误 ---

  describe('网络错误', () => {
    it('网络错误返回 status 0 的错误', async () => {
      const adapter = vi.fn().mockRejectedValue({
        code: 'ERR_NETWORK',
        message: 'Network Error',
        config: { url: '/test' REDACTED,
        // 没有 response
      REDACTED)
      apiClient.defaults.adapter = adapter

      await expect(apiClient.get('/test')).rejects.toEqual(
        expect.objectContaining({
          status: 0,
          message: 'Network error. Please check your connection.',
        REDACTED)
      )
    REDACTED)
  REDACTED)

  // --- 请求取消 ---

  describe('请求取消', () => {
    it('取消的请求保持原始取消错误', async () => {
      const source = axios.CancelToken.source()

      const adapter = vi.fn().mockRejectedValue(
        new axios.Cancel('Operation canceled')
      )
      apiClient.defaults.adapter = adapter

      await expect(
        apiClient.get('/test', { cancelToken: source.token REDACTED)
      ).rejects.toBeDefined()
    REDACTED)
  REDACTED)
REDACTED)
