import { describe, expect, it } from "vitest";

import {
  addModelsListItems,
  buildModelsListConfig,
  createModelsListState,
  filterModelsListItems,
  hydrateModelsListState,
  invertModelsListSelection,
  moveModelsListItem,
  selectAllModelsListItems,
  setModelsListCandidates,
  toggleModelsListItem,
} from "../groupsModelsList";

describe("groupsModelsList", () => {
  it("selects all default candidates for a new disabled config", () => {
    const state = createModelsListState();

    setModelsListCandidates(state, ["gpt-5.5", "gpt-5.4"]);

    expect(state.enabled).toBe(false);
    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: true },
    ]);
  });

  it("keeps saved selections and marks new candidates as unselected when editing", () => {
    const state = createModelsListState({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4"],
    });

    setModelsListCandidates(state, ["gpt-5.4", "legacy-gpt", "gpt-5.5"]);

    expect(state.enabled).toBe(true);
    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: true },
      { id: "legacy-gpt", selected: false },
    ]);
  });

  it("preserves explicitly unselected saved candidates when candidates refresh", () => {
    const state = createModelsListState({
      enabled: true,
      models: ["gpt-5.5"],
    });

    setModelsListCandidates(state, ["gpt-5.5", "gpt-5.4"]);

    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: false },
    ]);
  });

  it("builds config with selected models in current display order", () => {
    const state = hydrateModelsListState({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4", "legacy-gpt"],
    }, ["gpt-5.5", "gpt-5.4", "legacy-gpt"]);

    toggleModelsListItem(state, "legacy-gpt");
    moveModelsListItem(state, 1, 0);

    expect(buildModelsListConfig(state)).toEqual({
      enabled: true,
      models: ["gpt-5.4", "gpt-5.5"],
    });
  });

  it("adds manual model IDs, normalizes duplicates, and selects existing candidates", () => {
    const state = hydrateModelsListState({
      enabled: true,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-image-2"]);

    addModelsListItems(state, " openai/gpt-image-2, gpt-image-2 ; openai/gpt-image-2 ");

    expect(buildModelsListConfig(state)).toEqual({
      enabled: true,
      models: ["gpt-5.5", "gpt-image-2", "openai/gpt-image-2"],
    });
  });

  it("preserves saved selections when a manual model is added before candidates load", () => {
    const state = createModelsListState({
      enabled: true,
      models: ["gpt-5.5"],
    });

    addModelsListItems(state, "custom-model");

    expect(buildModelsListConfig(state)).toEqual({
      enabled: true,
      models: ["gpt-5.5", "custom-model"],
    });
  });

  it("keeps selected models in payload even when disabled so reopening can restore choices", () => {
    const state = hydrateModelsListState({
      enabled: false,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-5.4"]);

    expect(buildModelsListConfig(state)).toEqual({
      enabled: false,
      models: ["gpt-5.5"],
    });
  });

  it("preserves saved models when candidates have not loaded yet", () => {
    const state = createModelsListState({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4"],
    });

    expect(buildModelsListConfig(state)).toEqual({
      enabled: true,
      models: ["gpt-5.5", "gpt-5.4"],
    });
  });

  it("selects all candidate models from the toolbar action", () => {
    const state = hydrateModelsListState({
      enabled: true,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini"]);

    selectAllModelsListItems(state);

    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "gpt-5.4", selected: true },
      { id: "gpt-5.4-mini", selected: true },
    ]);
  });

  it("inverts selected models from the toolbar action", () => {
    const state = hydrateModelsListState({
      enabled: true,
      models: ["gpt-5.5"],
    }, ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini"]);

    invertModelsListSelection(state);

    expect(state.items).toEqual([
      { id: "gpt-5.5", selected: false },
      { id: "gpt-5.4", selected: true },
      { id: "gpt-5.4-mini", selected: true },
    ]);
  });

  it("filters model IDs case-insensitively without changing their order", () => {
    const state = hydrateModelsListState(null, [
      "gpt-5.5",
      "openai/GPT-Image-2",
      "claude-sonnet-4",
    ]);

    expect(filterModelsListItems(state.items, "  gPt  ")).toEqual([
      { id: "gpt-5.5", selected: true },
      { id: "openai/GPT-Image-2", selected: true },
    ]);
    expect(filterModelsListItems(state.items, "missing-model")).toEqual([]);
    expect(filterModelsListItems(state.items, "")).toBe(state.items);
  });
});
