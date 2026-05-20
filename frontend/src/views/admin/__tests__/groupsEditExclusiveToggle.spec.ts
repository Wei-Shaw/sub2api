import { describe, expect, it } from "vitest";
import { isEditExclusiveToggleVisible } from "../groupsEditExclusiveToggle";

/**
 * Regression tests for https://github.com/Wei-Shaw/sub2api/issues/2584
 *
 * The edit form's exclusive/public toggle must always be visible, regardless
 * of the group's subscription_type. Previously a `v-if` condition hid the
 * toggle for subscription-type groups, preventing admins from ever switching
 * them from exclusive to public.
 */
describe("isEditExclusiveToggleVisible", () => {
  it("shows the toggle for subscription-type groups (regression: issue #2584)", () => {
    expect(isEditExclusiveToggleVisible("subscription")).toBe(true);
  });

  it("shows the toggle for standard-type groups", () => {
    expect(isEditExclusiveToggleVisible("standard")).toBe(true);
  });

  it("shows the toggle for any unknown subscription type", () => {
    expect(isEditExclusiveToggleVisible("")).toBe(true);
    expect(isEditExclusiveToggleVisible("future_type")).toBe(true);
  });
});
