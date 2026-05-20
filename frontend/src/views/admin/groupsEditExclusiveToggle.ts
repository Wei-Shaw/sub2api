/**
 * Returns whether the exclusive/public toggle should be visible in the
 * group **edit** form.
 *
 * Unlike the *create* form — where subscription-type groups are always
 * exclusive and the toggle is intentionally hidden — the edit form must
 * expose the toggle for every subscription type so that admins can change
 * the setting on existing groups.
 *
 * @see https://github.com/Wei-Shaw/sub2api/issues/2584
 */
export function isEditExclusiveToggleVisible(_subscriptionType: string): boolean {
  return true
}
