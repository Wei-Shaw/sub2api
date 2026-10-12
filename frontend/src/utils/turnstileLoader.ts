let pending: Promise<void> | undefined

export function loadTurnstile(): Promise<void> {
  if (window.turnstile) return Promise.resolve()
  if (pending) return pending

  pending = new Promise<void>((resolve, reject) => {
    const selector = 'script[src*="challenges.cloudflare.com/turnstile/v0/api.js"]'
    let script = document.querySelector<HTMLScriptElement>(selector)
    const created = !script
    script ??= document.createElement('script')
    const element = script
    let finished = false
    const finish = (error?: Error) => {
      if (finished) return
      finished = true
      clearTimeout(timer)
      element.removeEventListener('error', onError)
      element.removeEventListener('load', onLoad)
      if (window.onTurnstileLoad === onLoad) delete window.onTurnstileLoad
      if (error) {
        element.remove()
        reject(error)
      } else {
        resolve()
      }
    }
    const onError = () => finish(new Error('Turnstile script failed to load'))
    const onLoad = () => {
      if (window.turnstile) finish()
    }
    const timer = setTimeout(() => finish(new Error('Turnstile load timed out')), 15000)
    window.onTurnstileLoad = onLoad
    element.addEventListener('error', onError)
    element.addEventListener('load', onLoad)
    if (created) {
      element.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?onload=onTurnstileLoad&render=explicit'
      element.async = true
      element.defer = true
      document.head.appendChild(element)
    }
  }).finally(() => { pending = undefined })
  return pending
}
