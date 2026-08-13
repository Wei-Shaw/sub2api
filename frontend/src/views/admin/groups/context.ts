import { inject, type InjectionKey } from "vue";

import type { GroupsViewContext } from "./useGroupsView";

/**
 * GroupsView hands its whole state surface to the table and the four dialogs
 * through this key. Props/emits would mean threading well over a hundred
 * bindings through each child for no isolation benefit — every child already
 * reads and writes the same forms.
 */
export const GROUPS_VIEW_CONTEXT: InjectionKey<GroupsViewContext> =
  Symbol("groups-view-context");

export function useGroupsViewContext(): GroupsViewContext {
  // Injected without a default on purpose: a missing provider is a wiring bug,
  // and failing at mount is far cheaper to diagnose than undefined bindings
  // surfacing as blank dialogs.
  const ctx = inject(GROUPS_VIEW_CONTEXT);
  if (!ctx) {
    throw new Error("GroupsView context is missing a provider");
  }
  return ctx;
}
