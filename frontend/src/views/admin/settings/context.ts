import { inject, type InjectionKey } from "vue";

import type { SettingsFormContext } from "./useSettingsForm";

/**
 * SettingsView hands its whole state surface to the ten tab sections through
 * this key. Props/emits would mean threading well over a hundred bindings into
 * each section for no isolation benefit — every section reads and writes the
 * same `form`.
 */
export const SETTINGS_FORM_CONTEXT: InjectionKey<SettingsFormContext> =
  Symbol("settings-form-context");

export function useSettingsFormContext(): SettingsFormContext {
  // Injected without a default on purpose: a missing provider is a wiring bug,
  // and failing at mount is far cheaper to diagnose than undefined bindings
  // surfacing as a blank tab.
  const ctx = inject(SETTINGS_FORM_CONTEXT);
  if (!ctx) {
    throw new Error("SettingsView context is missing a provider");
  }
  return ctx;
}
