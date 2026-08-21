import { describe, expect, it REDACTED from "vitest";

import {
  createDefaultMessagesDispatchFormState,
  messagesDispatchConfigToFormState,
  messagesDispatchFormStateToConfig,
  resetMessagesDispatchFormState,
  supportsMessagesDispatchPlatform,
REDACTED from "../groupsMessagesDispatch";

describe("groupsMessagesDispatch", () => {
  it("supports OpenAI and composite groups", () => {
    expect(supportsMessagesDispatchPlatform("openai")).toBe(true);
    expect(supportsMessagesDispatchPlatform("composite")).toBe(true);
    expect(supportsMessagesDispatchPlatform("anthropic")).toBe(false);
  REDACTED);

  it("returns the expected default form state", () => {
    expect(createDefaultMessagesDispatchFormState()).toEqual({
      allow_messages_dispatch: false,
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: [],
    REDACTED);
  REDACTED);

  it("sanitizes exact model mapping rows when converting to config", () => {
    const config = messagesDispatchFormStateToConfig({
      allow_messages_dispatch: true,
      opus_mapped_model: " gpt-5.4 ",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: " gpt-5.4-mini ",
      exact_model_mappings: [
        {
          claude_model: " claude-sonnet-4-5-20250929 ",
          target_model: " gpt-5.2 ",
        REDACTED,
        { claude_model: "", target_model: "gpt-5.4" REDACTED,
        { claude_model: "claude-opus-4-6", target_model: " " REDACTED,
      ],
    REDACTED);

    expect(config).toEqual({
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: {
        "claude-sonnet-4-5-20250929": "gpt-5.2",
      REDACTED,
    REDACTED);
  REDACTED);

  it("hydrates form state from api config", () => {
    expect(
      messagesDispatchConfigToFormState({
        opus_mapped_model: "gpt-5.4",
        sonnet_mapped_model: "gpt-5.2",
        haiku_mapped_model: "gpt-5.4-mini",
        exact_model_mappings: {
          "claude-opus-4-6": "gpt-5.4",
          "REDACTED": "gpt-5.4-mini",
        REDACTED,
      REDACTED),
    ).toEqual({
      allow_messages_dispatch: false,
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.2",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: [
        {
          claude_model: "REDACTED",
          target_model: "gpt-5.4-mini",
        REDACTED,
        { claude_model: "claude-opus-4-6", target_model: "gpt-5.4" REDACTED,
      ],
    REDACTED);
  REDACTED);

  it("resets mutable form state when platform switches away from openai", () => {
    const state = {
      allow_messages_dispatch: true,
      opus_mapped_model: "gpt-5.2",
      sonnet_mapped_model: "gpt-5.4",
      haiku_mapped_model: "gpt-5.1",
      exact_model_mappings: [
        { claude_model: "claude-opus-4-6", target_model: "gpt-5.4" REDACTED,
      ],
    REDACTED;

    resetMessagesDispatchFormState(state);

    expect(state).toEqual({
      allow_messages_dispatch: false,
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: [],
    REDACTED);
  REDACTED);
REDACTED);
