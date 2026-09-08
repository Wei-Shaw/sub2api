import { describe, expect, it } from "vitest";

import {
  addCustomModelAllowlistItem,
  createModelAllowlistState,
  type ModelAllowlistAddError,
} from "../groupModelAllowlist";

import {
  buildModelAllowlistConfig,
  createModelAllowlistState,
  hydrateModelAllowlistState,
  invertModelAllowlistSelection,
  moveModelAllowlistItem,
  selectAllModelAllowlistItems,
  setModelAllowlistCandidates,
  toggleModelAllowlistItem,
} from "../groupModelAllowlist";

describe("groupModelAllowlist", () => {
  it("selects all default candidates for a new disabled config", () => {
    const state = createModelAllowlistState();

    setModelAllowlistCandidates(state, ["gpt-5.5", "gpt-5.4"]);

    expect(state.enabled).toBe(false);
    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: true },
    ]);
  });

  it("keeps saved selections and marks new candidates as unselected when editing", () => {
    const state = createModelAllowlistState({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4"],
    });

    setModelAllowlistCandidates(state, ["gpt-5.4", "legacy-gpt", "gpt-5.5"]);

    expect(state.enabled).toBe(true);
    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: true },
      { id: "legacy-gpt", selected: false },
    ]);
  });

  it("preserves explicitly unselected saved candidates when candidates refresh", () => {
    const state = createModelAllowlistState({
      enabled: true,
      models: ["gpt-5.5"],
    });

    setModelAllowlistCandidates(state, ["gpt-5.5", "gpt-5.4"]);

    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: false },
    ]);
  });

  it("builds config with selected models in current display order", () => {
    const state = hydrateModelAllowlistState({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4", "legacy-gpt"],
    }, ["gpt-5.5", "gpt-5.4", "legacy-gpt"]);

    toggleModelAllowlistItem(state, "legacy-gpt");
    moveModelAllowlistItem(state, 1, 0);

    expect(buildModelAllowlistConfig(state)).toEqual({
      enabled: true,
      models: ["gpt-5.4", "gpt-5.5"],
    });
  });

  it("keeps selected models in payload even when disabled so reopening can restore choices", () => {
    const state = hydrateModelAllowlistState({
      enabled: false,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-5.4"]);

    expect(buildModelAllowlistConfig(state)).toEqual({
      enabled: false,
      models: ["gpt-5.5"],
    });
  });

  it("preserves saved models when candidates have not loaded yet", () => {
    const state = createModelAllowlistState({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4"],
    });

    expect(buildModelAllowlistConfig(state)).toEqual({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4"],
    });
  });

  it("selects all candidate models from the toolbar action", () => {
    const state = hydrateModelAllowlistState({
      enabled: true,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini"]);

    selectAllModelAllowlistItems(state);

    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: true },
      { id: "gpt-5.4-mini", selected: true },
    ]);
  });

  it("inverts selected models from the toolbar action", () => {
    const state = hydrateModelAllowlistState({
      enabled: true,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini"]);

    invertModelAllowlistSelection(state);

    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: false },
      { id: "gpt-5.4", selected: true },
      { id: "gpt-5.4-mini", selected: true },
    ]);
  });
});

describe("addCustomModelAllowlistItem", () => {
  const state = () => createModelAllowlistState({ enabled: true, models: ["gpt-5.4"] });

  it("appends a trimmed entry as selected", () => {
    const s = state();
    expect(addCustomModelAllowlistItem(s, "  claude-sonnet-4.5  ")).toBeNull();
    expect(s.items.at(-1)).toEqual({ id: "claude-sonnet-4.5", selected: true });
  });

  it("accepts trailing wildcard entries", () => {
    const s = state();
    expect(addCustomModelAllowlistItem(s, "gpt-5.5-*")).toBeNull();
    expect(s.items.at(-1)?.id).toBe("gpt-5.5-*");
  });

  it("rejects blank input", () => {
    expect(addCustomModelAllowlistItem(state(), "   ")).toBe<ModelAllowlistAddError>("empty");
  });

  it("rejects wildcards that are not trailing", () => {
    expect(addCustomModelAllowlistItem(state(), "gpt-*-5.4")).toBe<ModelAllowlistAddError>("invalid_wildcard");
    expect(addCustomModelAllowlistItem(state(), "gpt-*-codex-*")).toBe<ModelAllowlistAddError>("invalid_wildcard");
  });

  it("rejects duplicates case-insensitively", () => {
    const s = state();
    expect(addCustomModelAllowlistItem(s, "GPT-5.4")).toBe<ModelAllowlistAddError>("duplicate");
  });
});
