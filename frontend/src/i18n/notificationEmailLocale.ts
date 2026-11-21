import { initializeNotificationEmailLocale } from '@/api/user'

type NotificationEmailLocale = 'en' | 'zh'

const INITIALIZATION_VERSION = 'v1'
const INITIALIZED_MARKER_VALUE = '1'
const initializationRequests = new Map<number, Promise<void>>()

function markerKey(userID: number): string {
  return `notification_email_locale_initialized:${INITIALIZATION_VERSION}:user:${userID}`
}

export function markNotificationEmailLocaleInitialized(userID: number): void {
  if (!Number.isInteger(userID) || userID <= 0) return
  localStorage.setItem(markerKey(userID), INITIALIZED_MARKER_VALUE)
}

export function initializeNotificationEmailLocaleOnce(
  userID: number,
  locale: NotificationEmailLocale,
): Promise<void> {
  if (!Number.isInteger(userID) || userID <= 0) return Promise.resolve()
  if (localStorage.getItem(markerKey(userID)) === INITIALIZED_MARKER_VALUE) {
    return Promise.resolve()
  }

  const inFlight = initializationRequests.get(userID)
  if (inFlight) return inFlight

  const request = initializeNotificationEmailLocale(locale).then(() => {
    markNotificationEmailLocaleInitialized(userID)
  })
  initializationRequests.set(userID, request)

  const clearRequest = () => {
    if (initializationRequests.get(userID) === request) {
      initializationRequests.delete(userID)
    }
  }
  request.then(clearRequest, clearRequest)
  return request
}
