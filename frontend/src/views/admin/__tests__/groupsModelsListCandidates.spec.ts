import { describe, expect, it REDACTED from "vitest";

import {
  createModelsListCandidatesTracker,
REDACTED from "../groupsModelsListCandidates";

describe("groupsModelsListCandidates", () => {
  it("rejects stale candidate responses after a newer platform request starts", () => {
    const tracker = createModelsListCandidatesTracker();
    const first = {
      mode: "create" as const,
      groupID: 0,
      platform: "openai" as const,
    REDACTED;
    const second = {
      mode: "create" as const,
      groupID: 0,
      platform: "anthropic" as const,
    REDACTED;

    const firstID = tracker.next(first);
    const secondID = tracker.next(second);

    expect(tracker.isCurrent(firstID, first)).toBe(false);
    expect(tracker.isCurrent(secondID, second)).toBe(true);
  REDACTED);

  it("rejects responses for a previous edit group even with the same platform", () => {
    const tracker = createModelsListCandidatesTracker();
    const first = {
      mode: "edit" as const,
      groupID: 10,
      platform: "openai" as const,
    REDACTED;
    const second = {
      mode: "edit" as const,
      groupID: 11,
      platform: "openai" as const,
    REDACTED;

    const firstID = tracker.next(first);
    tracker.next(second);

    expect(tracker.isCurrent(firstID, first)).toBe(false);
  REDACTED);

  it("tracks create and edit requests independently", () => {
    const tracker = createModelsListCandidatesTracker();
    const editRequest = {
      mode: "edit" as const,
      groupID: 10,
      platform: "openai" as const,
    REDACTED;
    const createRequest = {
      mode: "create" as const,
      groupID: 0,
      platform: "anthropic" as const,
    REDACTED;

    const editID = tracker.next(editRequest);
    tracker.next(createRequest);

    expect(tracker.isCurrent(editID, editRequest)).toBe(true);
  REDACTED);
REDACTED);
