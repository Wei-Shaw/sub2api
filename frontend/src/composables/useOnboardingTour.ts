import { onBeforeUnmount, onMounted, onUnmounted, nextTick REDACTED from 'vue'
import { driver, type Driver, type DriveStep REDACTED from 'driver.js'
import 'driver.js/dist/driver.css'
import { useAuthStore as useUserStore REDACTED from '@/stores/auth'
import { useOnboardingStore REDACTED from '@/stores/onboarding'
import { useI18n REDACTED from 'vue-i18n'
import { getAdminSteps, getUserSteps REDACTED from '@/components/Guide/steps'

export interface OnboardingOptions {
  steps: DriveStep[]
  storageKey?: string
  autoStart?: boolean
  onComplete?: () => void
REDACTED

export function useOnboardingTour(options: OnboardingOptions) {
  const { t REDACTED = useI18n()
  const userStore = useUserStore()
  const onboardingStore = useOnboardingStore()
  const storageVersion = 'v4_interactive' // Bump version for new tour type

  // Timing constants for better maintainability
  const TIMING = {
    INTERACTIVE_WAIT_MS: 800,        // Default wait time for interactive steps
    SELECT_WAIT_MS: 2500,            // Extended wait for Select components
    ELEMENT_TIMEOUT_MS: 8000,        // Timeout for element detection
    AUTO_START_DELAY_MS: 1000        // Delay before auto-starting tour
  REDACTED as const

  // 使用 store 管理的全局 driver 实例
  let driverInstance: Driver | null = onboardingStore.getDriverInstance()
  let currentClickListener: {
    element: HTMLElement
    handler: () => void
    keyHandler?: (e: KeyboardEvent) => void
    originalTabIndex?: string | null
    eventTypes?: string[] // Track which event types were added
  REDACTED | null = null
  let autoStartTimer: ReturnType<typeof setTimeout> | null = null
  let globalKeyboardHandler: ((e: KeyboardEvent) => void) | null = null

  const getStorageKey = () => {
    const baseKey = options.storageKey ?? 'onboarding_tour'
    const userId = userStore.user?.id ?? 'guest'
    const role = userStore.user?.role ?? 'user'
    return `${baseKeyREDACTED_${userIdREDACTED_${roleREDACTED_${storageVersionREDACTED`
  REDACTED

  const hasSeen = () => {
    return localStorage.getItem(getStorageKey()) === 'true'
  REDACTED

  const markAsSeen = () => {
    localStorage.setItem(getStorageKey(), 'true')
  REDACTED

  const clearSeen = () => {
    localStorage.removeItem(getStorageKey())
  REDACTED

  /**
   * 检查元素是否存在，如果不存在则重试
   */
  const ensureElement = async (selector: string, timeout = 5000): Promise<boolean> => {
    const startTime = Date.now()
    while (Date.now() - startTime < timeout) {
      const element = document.querySelector(selector)
      if (element && element.getBoundingClientRect().height > 0) {
        return true
      REDACTED
      await new Promise((resolve) => setTimeout(resolve, 150))
    REDACTED
    return false
  REDACTED

  const startTour = async (startIndex = 0) => {
    // 动态获取当前用户角色和步骤
    const isAdmin = userStore.user?.role === 'admin'
    const isSimpleMode = userStore.isSimpleMode
    const steps = isAdmin ? getAdminSteps(t, isSimpleMode) : getUserSteps(t)

    // 确保 DOM 就绪
    await nextTick()

    // 如果指定了起始步骤，确保元素可见
    const currentStep = steps[startIndex]
    if (currentStep?.element && typeof currentStep.element === 'string') {
      await ensureElement(currentStep.element, TIMING.ELEMENT_TIMEOUT_MS)
    REDACTED

    if (driverInstance) {
      driverInstance.destroy()
    REDACTED

    // 创建新的 driver 实例并存储到 store
    driverInstance = driver({
      showProgress: true,
      steps,
      animate: true,
      allowClose: false, // 禁止点击遮罩关闭
      stagePadding: 4,
      popoverClass: 'theme-tour-popover',
      nextBtnText: t('common.next'),
      prevBtnText: t('common.back'),
      doneBtnText: t('common.confirm'),

      // 导航处理
      onNextClick: async (_el, _step, { config, state REDACTED) => {
        // 如果是最后一步，点击则是"完成"
        if (state.activeIndex === (config.steps?.length ?? 0) - 1) {
          markAsSeen()
          driverInstance?.destroy()
          onboardingStore.setDriverInstance(null)
        REDACTED else {
          // 注意：交互式步骤通常隐藏 Next 按钮，此处逻辑为防御性编程
          const currentIndex = state.activeIndex ?? 0
          const currentStep = steps[currentIndex]
          const isInteractiveStep = currentStep?.popover?.showButtons?.length === 1 &&
                                     currentStep?.popover.showButtons[0] === 'close'

          if (isInteractiveStep && currentStep.element) {
            const targetElement = typeof currentStep.element === 'string'
              ? document.querySelector(currentStep.element) as HTMLElement
              : currentStep.element as HTMLElement

            if (targetElement) {
              const isClickable = !['INPUT', 'TEXTAREA', 'SELECT'].includes(targetElement.tagName)
              if (isClickable) {
                targetElement.click()
                return
              REDACTED
            REDACTED
          REDACTED
          driverInstance?.moveNext()
        REDACTED
      REDACTED,
      onPrevClick: () => {
        driverInstance?.movePrevious()
      REDACTED,
      onCloseClick: () => {
        markAsSeen()
        driverInstance?.destroy()
        onboardingStore.setDriverInstance(null)
      REDACTED,

      // 渲染时重组 Footer 布局
      onPopoverRender: (popover, { config, state REDACTED) => {
        // Class name constants for easier maintenance
        const CLASS_REORGANIZED = 'reorganized'
        const CLASS_FOOTER_LEFT = 'footer-left'
        const CLASS_FOOTER_RIGHT = 'footer-right'
        const CLASS_SKIP_BTN = 'header-skip-btn'
        const CLASS_DONE_BTN = 'driver-popover-done-btn'
        const CLASS_TITLE_TEXT = 'driver-popover-title-text'
        const CLASS_PROGRESS_TEXT = 'driver-popover-progress-text'
        const CLASS_NEXT_BTN = 'driver-popover-next-btn'
        const CLASS_PREV_BTN = 'driver-popover-prev-btn'

        try {
          const { title: titleEl, footer: footerEl, nextButton, previousButton REDACTED = popover

          // Defensive check: ensure popover elements exist
          if (!titleEl || !footerEl) {
            console.warn('Onboarding: Missing popover elements')
            return
          REDACTED

          // 1.5 交互式步骤提示
          const currentStep = steps[state.activeIndex ?? 0]
          const isInteractive = currentStep?.popover?.showButtons?.length === 1 &&
                               currentStep?.popover?.showButtons[0] === 'close'

          if (isInteractive && popover.description) {
            const hintClass = 'driver-popover-description-hint'
            if (!popover.description.querySelector(`.${hintClassREDACTED`)) {
              const hint = document.createElement('div')
              hint.className = `${hintClassREDACTED mt-2 text-xs text-gray-500 flex items-center gap-1`

              const iconSpan = document.createElement('span')
              iconSpan.className = 'i-mdi-keyboard-return mr-1'

              const textNode = document.createTextNode(
                t('onboarding.interactiveHint', 'Press Enter or Click to continue'),
              )

              hint.appendChild(iconSpan)
              hint.appendChild(textNode)
              popover.description.appendChild(hint)
            REDACTED
          REDACTED

          // 2. 底部：DOM 重组
          if (!footerEl.classList.contains(CLASS_REORGANIZED)) {
            footerEl.classList.add(CLASS_REORGANIZED)

            const progressEl = footerEl.querySelector(`.${CLASS_PROGRESS_TEXTREDACTED`)
            const nextBtnEl = nextButton || footerEl.querySelector(`.${CLASS_NEXT_BTNREDACTED`)
            const prevBtnEl = previousButton || footerEl.querySelector(`.${CLASS_PREV_BTNREDACTED`)

            const leftContainer = document.createElement('div')
            leftContainer.className = CLASS_FOOTER_LEFT

            const rightContainer = document.createElement('div')
            rightContainer.className = CLASS_FOOTER_RIGHT

            if (progressEl) leftContainer.appendChild(progressEl)

            const shortcutsEl = document.createElement('div')
            shortcutsEl.className = 'footer-shortcuts'

            const shortcut1 = document.createElement('span')
            shortcut1.className = 'shortcut-item'
            const kbd1 = document.createElement('kbd')
            kbd1.textContent = '←'
            const kbd2 = document.createElement('kbd')
            kbd2.textContent = '→'
            shortcut1.appendChild(kbd1)
            shortcut1.appendChild(kbd2)
            shortcut1.appendChild(
              document.createTextNode(` ${t('onboarding.navigation.flipPage')REDACTED`),
            )

            const shortcut2 = document.createElement('span')
            shortcut2.className = 'shortcut-item'
            const kbd3 = document.createElement('kbd')
            kbd3.textContent = 'ESC'
            shortcut2.appendChild(kbd3)
            shortcut2.appendChild(
              document.createTextNode(` ${t('onboarding.navigation.exit')REDACTED`),
            )

            shortcutsEl.appendChild(shortcut1)
            shortcutsEl.appendChild(shortcut2)
            leftContainer.appendChild(shortcutsEl)

            if (prevBtnEl) rightContainer.appendChild(prevBtnEl)
            if (nextBtnEl) rightContainer.appendChild(nextBtnEl)

            footerEl.innerHTML = ''
            footerEl.appendChild(leftContainer)
            footerEl.appendChild(rightContainer)
          REDACTED

          // 3. 状态更新
          const isLastStep = state.activeIndex === (config.steps?.length ?? 0) - 1
          const activeNextBtn = nextButton || footerEl.querySelector(`.${CLASS_NEXT_BTNREDACTED`)

          if (activeNextBtn) {
             if (isLastStep) {
               activeNextBtn.classList.add(CLASS_DONE_BTN)
             REDACTED else {
               activeNextBtn.classList.remove(CLASS_DONE_BTN)
             REDACTED
          REDACTED
        REDACTED catch (e) {
          console.error('Onboarding Tour Render Error:', e)
        REDACTED
      REDACTED,

      // 步骤高亮时触发
      onHighlightStarted: async (element, step) => {
        // 清理之前的监听器
        if (currentClickListener) {
          const { element: el, handler, keyHandler, originalTabIndex, eventTypes REDACTED = currentClickListener
          // Remove all tracked event types
          if (eventTypes) {
            eventTypes.forEach(type => el.removeEventListener(type, handler))
          REDACTED
          if (keyHandler) el.removeEventListener('keydown', keyHandler)
          if (originalTabIndex !== undefined) {
             if (originalTabIndex === null) el.removeAttribute('tabindex')
             else el.setAttribute('tabindex', originalTabIndex)
          REDACTED
          currentClickListener = null
        REDACTED

        // 尝试等待元素
        if (!element && step.element && typeof step.element === 'string') {
           const exists = await ensureElement(step.element, 8000)
           if (!exists) {
             console.warn(`Tour element not found after 8s: ${step.elementREDACTED`)
             return
           REDACTED
           element = document.querySelector(step.element) as HTMLElement
        REDACTED

        const isInteractiveStep = step.popover?.showButtons?.length === 1 &&
                                   step.popover.showButtons[0] === 'close'

        if (isInteractiveStep && element) {
          const htmlElement = element as HTMLElement

          // Check if this is a submit button - if so, don't bind auto-advance listeners
          // Let business code (e.g., handleCreateGroup) manually call nextStep after success
          const isSubmitButton = htmlElement.getAttribute('type') === 'submit' ||
                                (htmlElement.tagName === 'BUTTON' && htmlElement.closest('form'))

          if (isSubmitButton) {
            console.log('Submit button detected, skipping auto-advance listener')
            return // Don't bind any click listeners for submit buttons
          REDACTED

          const originalTabIndex = htmlElement.getAttribute('tabindex')
          if (!htmlElement.isContentEditable && htmlElement.tabIndex === -1) {
             htmlElement.setAttribute('tabindex', '0')
          REDACTED

          // Enhanced Select component detection - check both children and self
          const isSelectComponent = htmlElement.querySelector('.select-trigger') !== null ||
                                    htmlElement.classList.contains('select-trigger')

          // Single-execution protection flag
          let hasExecuted = false

          const clickHandler = async () => {
            // Prevent duplicate execution
            if (hasExecuted) {
              console.warn('Click handler already executed, skipping')
              return
            REDACTED
            hasExecuted = true

            // For Select components, wait longer to allow user to make a selection
            const waitTime = isSelectComponent ? TIMING.SELECT_WAIT_MS : TIMING.INTERACTIVE_WAIT_MS
            await new Promise(resolve => setTimeout(resolve, waitTime))

            // Verify driver is still active and not destroyed
            if (!driverInstance || !driverInstance.isActive()) {
              console.warn('Driver instance destroyed or inactive during navigation')
              return
            REDACTED

            const currentIndex = driverInstance.getActiveIndex() ?? 0
            const nextStep = steps[currentIndex + 1]

            if (nextStep?.element && typeof nextStep.element === 'string') {
              // 增加超时时间到 8 秒，给路由导航更多时间
              const exists = await ensureElement(nextStep.element, TIMING.ELEMENT_TIMEOUT_MS)
              if (!exists) {
                console.warn('Next step element not found after timeout, aborting auto-advance')
                console.warn('Expected element:', nextStep.element)
                return
              REDACTED
            REDACTED

            // Final check before moving
            if (driverInstance && driverInstance.isActive()) {
              driverInstance.moveNext()
            REDACTED
          REDACTED

          // For input fields, advance on input/change events instead of click
          const isInputField = ['INPUT', 'TEXTAREA', 'SELECT'].includes(htmlElement.tagName)

          if (isInputField) {
            const inputHandler = () => {
              // Remove listener after first input
              htmlElement.removeEventListener('input', inputHandler)
              htmlElement.removeEventListener('change', inputHandler)
              clickHandler()
            REDACTED

            htmlElement.addEventListener('input', inputHandler)
            htmlElement.addEventListener('change', inputHandler)

            currentClickListener = {
              element: htmlElement,
              handler: inputHandler,
              originalTabIndex,
              eventTypes: ['input', 'change']
            REDACTED
          REDACTED else if (isSelectComponent) {
            // For Select components, listen for option selection clicks
            const selectOptionClickHandler = (e: Event) => {
              const target = e.target as HTMLElement
              // Type safety: ensure target is an Element before using closest
              if (!(target instanceof Element)) {
                return
              REDACTED
              // Check if the clicked element is a select option
              if (target.closest('.select-option')) {
                // User selected an option, proceed to next step
                clickHandler()
              REDACTED
            REDACTED

            const keyHandler = (e: KeyboardEvent) => {
               if (['Enter', ' '].includes(e.key)) {
                  e.preventDefault()
                  // For select components, Enter/Space should open dropdown, not advance
                  // Only advance if an option is focused
                  const focusedOption = htmlElement.querySelector('.select-option:focus')
                  if (focusedOption) {
                    clickHandler()
                  REDACTED
               REDACTED
            REDACTED

            htmlElement.addEventListener('click', selectOptionClickHandler)
            htmlElement.addEventListener('keydown', keyHandler)

            currentClickListener = {
              element: htmlElement,
              handler: selectOptionClickHandler as () => void,
              keyHandler,
              originalTabIndex,
              eventTypes: ['click']
            REDACTED
          REDACTED else {
            const keyHandler = (e: KeyboardEvent) => {
               if (['Enter', ' '].includes(e.key)) {
                  e.preventDefault()
                  clickHandler()
               REDACTED
            REDACTED

            htmlElement.addEventListener('click', clickHandler, { once: true REDACTED)
            htmlElement.addEventListener('keydown', keyHandler)

            currentClickListener = {
              element: htmlElement,
              handler: clickHandler as () => void,
              keyHandler,
              originalTabIndex,
              eventTypes: ['click']
            REDACTED
          REDACTED
        REDACTED
      REDACTED,

      onDestroyed: () => {
        if (currentClickListener) {
          const { element: el, handler, keyHandler, originalTabIndex, eventTypes REDACTED = currentClickListener
          // Remove all tracked event types
          if (eventTypes) {
            eventTypes.forEach(type => el.removeEventListener(type, handler))
          REDACTED
          if (keyHandler) el.removeEventListener('keydown', keyHandler)
          if (originalTabIndex !== undefined) {
             if (originalTabIndex === null) el.removeAttribute('tabindex')
             else el.setAttribute('tabindex', originalTabIndex)
          REDACTED
          currentClickListener = null
        REDACTED
        // 清理全局监听器 (由此处唯一管理)
        if (globalKeyboardHandler) {
          document.removeEventListener('keydown', globalKeyboardHandler, { capture: true REDACTED)
          globalKeyboardHandler = null
        REDACTED
        onboardingStore.setDriverInstance(null)
      REDACTED
    REDACTED)

    onboardingStore.setDriverInstance(driverInstance)

    // 添加全局键盘监听器
    globalKeyboardHandler = (e: KeyboardEvent) => {
      if (!driverInstance?.isActive()) return

      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        markAsSeen()
        driverInstance.destroy()
        onboardingStore.setDriverInstance(null)
        return
      REDACTED

      if (e.key === 'ArrowRight') {
        const target = e.target as HTMLElement
        // 允许在输入框中使用方向键
        if (['INPUT', 'TEXTAREA'].includes(target?.tagName)) {
           return
        REDACTED

        e.preventDefault()
        e.stopPropagation()

        // 对于交互式步骤，箭头键应该触发交互而非跳过
        const currentIndex = driverInstance!.getActiveIndex() ?? 0
        const currentStep = steps[currentIndex]
        const isInteractiveStep = currentStep?.popover?.showButtons?.length === 1 &&
                                   currentStep?.popover.showButtons[0] === 'close'

        if (isInteractiveStep && currentStep.element) {
          const targetElement = typeof currentStep.element === 'string'
            ? document.querySelector(currentStep.element) as HTMLElement
            : currentStep.element as HTMLElement

          if (targetElement) {
            // 对于非输入类元素，提示用户需要点击或按Enter
            const isClickable = !['INPUT', 'TEXTAREA', 'SELECT'].includes(targetElement.tagName)
            if (isClickable) {
              // 不自动触发，只是停留提示
              return
            REDACTED
          REDACTED
        REDACTED

        // 非交互式步骤才允许箭头键翻页
        driverInstance!.moveNext()
      REDACTED
      else if (e.key === 'Enter') {
        const target = e.target as HTMLElement
        // 允许在输入框中使用回车
        if (['INPUT', 'TEXTAREA'].includes(target?.tagName)) {
           return
        REDACTED

        e.preventDefault()
        e.stopPropagation()

        // 回车键处理交互式步骤
        const currentIndex = driverInstance!.getActiveIndex() ?? 0
        const currentStep = steps[currentIndex]
        const isInteractiveStep = currentStep?.popover?.showButtons?.length === 1 &&
                                   currentStep?.popover.showButtons[0] === 'close'

        if (isInteractiveStep && currentStep.element) {
          const targetElement = typeof currentStep.element === 'string'
            ? document.querySelector(currentStep.element) as HTMLElement
            : currentStep.element as HTMLElement

          if (targetElement) {
            const isClickable = !['INPUT', 'TEXTAREA', 'SELECT'].includes(targetElement.tagName)
            if (isClickable) {
              targetElement.click()
              return
            REDACTED
          REDACTED
        REDACTED
        driverInstance!.moveNext()
      REDACTED
      else if (e.key === 'ArrowLeft') {
        const target = e.target as HTMLElement
        // 允许在输入框中使用方向键
        if (['INPUT', 'TEXTAREA', 'SELECT'].includes(target?.tagName) || target?.isContentEditable) {
           return
        REDACTED

        e.preventDefault()
        e.stopPropagation()
        driverInstance.movePrevious()
      REDACTED
    REDACTED

    document.addEventListener('keydown', globalKeyboardHandler, { capture: true REDACTED)
    driverInstance.drive(startIndex)
  REDACTED

  const nextStep = async (delay = 300) => {
    if (!driverInstance?.isActive()) return
    if (delay > 0) {
      await new Promise(resolve => setTimeout(resolve, delay))
    REDACTED
    driverInstance.moveNext()
  REDACTED

  const isCurrentStep = (elementSelector: string): boolean => {
    if (!driverInstance?.isActive()) return false
    const activeElement = driverInstance.getActiveElement()
    return activeElement?.matches(elementSelector) ?? false
  REDACTED

  const replayTour = () => {
    clearSeen()
    void startTour()
  REDACTED

  onMounted(async () => {
    onboardingStore.setControlMethods({
      nextStep,
      isCurrentStep
    REDACTED)

    if (onboardingStore.isDriverActive()) {
      console.log('Tour already active, skipping auto-start')
      driverInstance = onboardingStore.getDriverInstance()
      return
    REDACTED

    // 简易模式下禁用新手引导
    if (userStore.isSimpleMode) {
      console.log('Simple mode detected, skipping onboarding tour')
      return
    REDACTED

    // 只在管理员+标准模式下自动启动
    const isAdmin = userStore.user?.role === 'admin'
    if (!isAdmin) {
      console.log('Non-admin user, skipping auto-start')
      return
    REDACTED

    if (!options.autoStart || hasSeen()) return
    autoStartTimer = setTimeout(() => {
      void startTour()
    REDACTED, TIMING.AUTO_START_DELAY_MS)
  REDACTED)

  onBeforeUnmount(() => {
    // 保持 driver 实例活跃，支持路由切换
  REDACTED)

  onUnmounted(() => {
    if (autoStartTimer) {
      clearTimeout(autoStartTimer)
      autoStartTimer = null
    REDACTED
    // 关键修复：不再此处清理 globalKeyboardHandler，交由 driver.onDestroyed 管理
    onboardingStore.clearControlMethods()
  REDACTED)

  return {
    startTour,
    replayTour,
    nextStep,
    isCurrentStep,
    hasSeen,
    markAsSeen,
    clearSeen
  REDACTED
REDACTED
