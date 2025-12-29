export const ADMIN_TOUR_STEP_KEYS = [
  'admin.welcome',
  'admin.groupManage',
  'admin.createGroup',
  'admin.groupName',
  'admin.groupPlatform',
  'admin.groupMultiplier',
  'admin.groupExclusive',
  'admin.groupSubmit',
  'admin.accountManage',
  'admin.createAccount',
  'admin.accountName',
  'admin.accountPlatform',
  'admin.accountType',
  'admin.accountPriority',
  'admin.accountGroups',
  'admin.accountSubmit',
  'admin.keyManage',
  'admin.createKey',
  'admin.keyName',
  'admin.keyGroup',
  'admin.keySubmit'
] as const

export const USER_TOUR_STEP_KEYS = [
  'user.welcome',
  'user.keyManage',
  'user.createKey',
  'user.keyName',
  'user.keyGroup',
  'user.keySubmit'
] as const

export const TOUR_STEP_KEYS = [...ADMIN_TOUR_STEP_KEYS, ...USER_TOUR_STEP_KEYS] as const

export type TourStepKey = (typeof TOUR_STEP_KEYS)[number]

export const TOUR_STEP_COMPONENTS: Record<TourStepKey, string> = {
  'admin.welcome': 'AdminWelcomeDescription',
  'admin.groupManage': 'AdminGroupManageDescription',
  'admin.createGroup': 'AdminCreateGroupDescription',
  'admin.groupName': 'AdminGroupNameDescription',
  'admin.groupPlatform': 'AdminGroupPlatformDescription',
  'admin.groupMultiplier': 'AdminGroupMultiplierDescription',
  'admin.groupExclusive': 'AdminGroupExclusiveDescription',
  'admin.groupSubmit': 'AdminGroupSubmitDescription',
  'admin.accountManage': 'AdminAccountManageDescription',
  'admin.createAccount': 'AdminCreateAccountDescription',
  'admin.accountName': 'AdminAccountNameDescription',
  'admin.accountPlatform': 'AdminAccountPlatformDescription',
  'admin.accountType': 'AdminAccountTypeDescription',
  'admin.accountPriority': 'AdminAccountPriorityDescription',
  'admin.accountGroups': 'AdminAccountGroupsDescription',
  'admin.accountSubmit': 'AdminAccountSubmitDescription',
  'admin.keyManage': 'AdminKeyManageDescription',
  'admin.createKey': 'AdminCreateKeyDescription',
  'admin.keyName': 'AdminKeyNameDescription',
  'admin.keyGroup': 'AdminKeyGroupDescription',
  'admin.keySubmit': 'AdminKeySubmitDescription',
  'user.welcome': 'UserWelcomeDescription',
  'user.keyManage': 'UserKeyManageDescription',
  'user.createKey': 'UserCreateKeyDescription',
  'user.keyName': 'UserKeyNameDescription',
  'user.keyGroup': 'UserKeyGroupDescription',
  'user.keySubmit': 'UserKeySubmitDescription'
REDACTED

export const useTourStepDescription = () => {
  const getComponentName = (stepKey: TourStepKey) => TOUR_STEP_COMPONENTS[stepKey]

  const isTourStepKey = (value: string): value is TourStepKey =>
    Object.prototype.hasOwnProperty.call(TOUR_STEP_COMPONENTS, value)

  return {
    getComponentName,
    isTourStepKey,
    stepKeys: TOUR_STEP_KEYS
  REDACTED
REDACTED
