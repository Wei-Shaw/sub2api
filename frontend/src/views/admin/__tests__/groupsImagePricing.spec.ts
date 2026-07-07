import { describe, expect, it REDACTED from "vitest";

import {
  imagePricingPlatforms,
  imagePricingI18nKey,
  supportsImagePricingPlatform,
  supportsVideoPricingPlatform,
  videoPricingI18nKey,
REDACTED from "../groupsImagePricing";

describe("groups image pricing platform support", () => {
  it("includes Grok image groups", () => {
    expect(supportsImagePricingPlatform("grok")).toBe(true);
    expect(imagePricingPlatforms.has("grok")).toBe(true);
  REDACTED);

  it("enables video pricing controls for Grok only", () => {
    expect(supportsVideoPricingPlatform("grok")).toBe(true);
    expect(supportsVideoPricingPlatform("openai")).toBe(false);
  REDACTED);

  it("keeps non-media group platforms out of the image pricing controls", () => {
    expect(supportsImagePricingPlatform("anthropic")).toBe(false);
  REDACTED);

  it("keeps image and video pricing copy separate", () => {
    expect(imagePricingI18nKey("grok", "title")).toBe(
      "admin.groups.imagePricing.title",
    );
    expect(videoPricingI18nKey("title")).toBe("admin.groups.videoPricing.title");
  REDACTED);
REDACTED);
