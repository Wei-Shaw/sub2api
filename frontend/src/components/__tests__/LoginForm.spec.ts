/**
 * LoginView 组件核心逻辑测试
 * 测试登录表单提交、验证、2FA 等场景
 */
import { describe, it, expect, vi, beforeEach REDACTED from 'vitest'
import { mount, flushPromises REDACTED from '@vue/test-utils'
import { setActivePinia, createPinia REDACTED from 'pinia'
import { defineComponent, reactive, ref REDACTED from 'vue'
import { useAuthStore REDACTED from '@/stores/auth'

// Mock 所有外部依赖
const mockLogin = vi.fn()
const mockLogin2FA = vi.fn()
const mockPush = vi.fn()

vi.mock('@/api', () => ({
  authAPI: {
    login: (...args: any[]) => mockLogin(...args),
    login2FA: (...args: any[]) => mockLogin2FA(...args),
    logout: vi.fn(),
    getCurrentUser: vi.fn().mockResolvedValue({ data: {REDACTED REDACTED),
    register: vi.fn(),
    refreshToken: vi.fn(),
  REDACTED,
  isTotp2FARequired: (response: any) => response?.requires_2fa === true,
REDACTED))

vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(),
REDACTED))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn().mockResolvedValue({REDACTED),
REDACTED))

/**
 * 创建一个简化的测试组件来封装登录逻辑
 * 避免引入 LoginView.vue 的全部依赖（AuthLayout、i18n、Icon 等）
 */
const LoginFormTestComponent = defineComponent({
  setup() {
    const authStore = useAuthStore()
    const formData = reactive({ email: '', password: '' REDACTED)
    const isLoading = ref(false)
    const errorMessage = ref('')

    const handleLogin = async () => {
      if (!formData.email || !formData.password) {
        errorMessage.value = '请输入邮箱和密码'
        return
      REDACTED

      isLoading.value = true
      errorMessage.value = ''

      try {
        const response = await authStore.login({
          email: formData.email,
          password: formData.password,
        REDACTED)

        // 2FA 流程由调用方处理
        if ((response as any)?.requires_2fa) {
          errorMessage.value = '需要 2FA 验证'
          return
        REDACTED

        mockPush('/dashboard')
      REDACTED catch (error: any) {
        errorMessage.value = error.message || '登录失败'
      REDACTED finally {
        isLoading.value = false
      REDACTED
    REDACTED

    return { formData, isLoading, errorMessage, handleLogin REDACTED
  REDACTED,
  template: `
    <form @submit.prevent="handleLogin">
      <input id="email" v-model="formData.email" type="email" />
      <input id="password" v-model="formData.password" type="password" />
      <p v-if="errorMessage" class="error">{{ errorMessage REDACTEDREDACTED</p>
      <button type="submit" :disabled="isLoading">登录</button>
    </form>
  `,
REDACTED)

describe('LoginForm 核心逻辑', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  REDACTED)

  it('成功登录后跳转到 dashboard', async () => {
    mockLogin.mockResolvedValue({
      access_token: 'token',
      token_type: 'Bearer',
      user: { id: 1, username: 'test', email: 'test@example.com', role: 'user', balance: 0, concurrency: 5, status: 'active', allowed_groups: null, created_at: '', updated_at: '' REDACTED,
    REDACTED)

    const wrapper = mount(LoginFormTestComponent)

    await wrapper.find('#email').setValue('test@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockLogin).toHaveBeenCalledWith({
      email: 'test@example.com',
      password: 'password123',
    REDACTED)
    expect(mockPush).toHaveBeenCalledWith('/dashboard')
  REDACTED)

  it('登录失败时显示错误信息', async () => {
    mockLogin.mockRejectedValue(new Error('Invalid credentials'))

    const wrapper = mount(LoginFormTestComponent)

    await wrapper.find('#email').setValue('test@example.com')
    await wrapper.find('#password').setValue('wrong')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('.error').text()).toBe('Invalid credentials')
  REDACTED)

  it('空表单提交显示验证错误', async () => {
    const wrapper = mount(LoginFormTestComponent)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('.error').text()).toBe('请输入邮箱和密码')
    expect(mockLogin).not.toHaveBeenCalled()
  REDACTED)

  it('需要 2FA 时不跳转', async () => {
    mockLogin.mockResolvedValue({
      requires_2fa: true,
      temp_token: 'temp-123',
    REDACTED)

    const wrapper = mount(LoginFormTestComponent)

    await wrapper.find('#email').setValue('test@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mockPush).not.toHaveBeenCalled()
    expect(wrapper.find('.error').text()).toBe('需要 2FA 验证')
  REDACTED)

  it('提交过程中按钮被禁用', async () => {
    let resolveLogin: (v: any) => void
    mockLogin.mockImplementation(
      () => new Promise((resolve) => { resolveLogin = resolve REDACTED)
    )

    const wrapper = mount(LoginFormTestComponent)

    await wrapper.find('#email').setValue('test@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit')

    expect(wrapper.find('button').attributes('disabled')).toBeDefined()

    resolveLogin!({
      access_token: 'token',
      token_type: 'Bearer',
      user: { id: 1, username: 'test', email: 'test@example.com', role: 'user', balance: 0, concurrency: 5, status: 'active', allowed_groups: null, created_at: '', updated_at: '' REDACTED,
    REDACTED)
    await flushPromises()

    expect(wrapper.find('button').attributes('disabled')).toBeUndefined()
  REDACTED)
REDACTED)
