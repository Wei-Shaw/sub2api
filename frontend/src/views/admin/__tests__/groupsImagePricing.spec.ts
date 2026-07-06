import { describe, expect, it REDACTED from "vitest";

import {
  imagePricingPlatforms,
  supportsImagePricingPlatform,
REDACTED from "../groupsImagePricing";

describe("groups image pricing platform support", () => {
  it("includes Grok media groups", () => {
    expect(supportsImagePricingPlatform("grok")).toBe(true);
    expect(imagePricingPlatforms.has("grok")).toBe(true);
  REDACTED);

  it("keeps non-media group platforms out of the image pricing controls", () => {
    expect(supportsImagePricingPlatform("anthropic")).toBe(false);
  REDACTED);
REDACTED);
