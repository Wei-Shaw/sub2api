import { describe, expect, it REDACTED from "vitest";

import {
  profitDecimalToPercent,
  profitPercentToDecimal,
  validateProfitControlFormState,
  type ProfitControlFormState,
REDACTED from "../groupsProfitControl";

const formState = (
  overrides: Partial<ProfitControlFormState> = {REDACTED,
): ProfitControlFormState => ({
  platform: "openai",
  profit_control_enabled: true,
  profit_min_margin_percent: 30,
  profit_safety_buffer_percent: 0,
  ...overrides,
REDACTED);

describe("profitPercentToDecimal", () => {
  it("converts percent input to backend decimal", () => {
    expect(profitPercentToDecimal(30)).toBe(0.3);
    expect(profitPercentToDecimal(5)).toBe(0.05);
    expect(profitPercentToDecimal(33.33)).toBe(0.3333);
    expect(profitPercentToDecimal(99.99)).toBe(0.9999);
  REDACTED);

  it("rounds to four decimal places matching decimal(10,4) storage", () => {
    expect(profitPercentToDecimal(33.333)).toBe(0.3333);
    expect(profitPercentToDecimal(0.005)).toBe(0.0001);
  REDACTED);

  it("treats empty, invalid and non-positive input as zero", () => {
    expect(profitPercentToDecimal("")).toBe(0);
    expect(profitPercentToDecimal(null)).toBe(0);
    expect(profitPercentToDecimal(undefined)).toBe(0);
    expect(profitPercentToDecimal("abc")).toBe(0);
    expect(profitPercentToDecimal(-5)).toBe(0);
    expect(profitPercentToDecimal(0)).toBe(0);
  REDACTED);
REDACTED);

describe("profitDecimalToPercent", () => {
  it("converts backend decimal to percent without float tail noise", () => {
    expect(profitDecimalToPercent(0.3)).toBe(30);
    expect(profitDecimalToPercent(0.05)).toBe(5);
    expect(profitDecimalToPercent(0.3333)).toBe(33.33);
    expect(profitDecimalToPercent(0.9999)).toBe(99.99);
  REDACTED);

  it("treats missing and non-positive values as zero", () => {
    expect(profitDecimalToPercent(null)).toBe(0);
    expect(profitDecimalToPercent(undefined)).toBe(0);
    expect(profitDecimalToPercent(0)).toBe(0);
    expect(profitDecimalToPercent(-0.3)).toBe(0);
  REDACTED);

  it("round-trips representative storage values", () => {
    for (const decimal of [0.05, 0.1, 0.3, 0.3333, 0.5, 0.75, 0.9999]) {
      expect(profitPercentToDecimal(profitDecimalToPercent(decimal))).toBe(
        decimal,
      );
    REDACTED
  REDACTED);
REDACTED);

describe("validateProfitControlFormState", () => {
  it("passes valid enabled configurations", () => {
    expect(validateProfitControlFormState(formState())).toBeNull();
    expect(
      validateProfitControlFormState(
        formState({
          profit_min_margin_percent: 0,
          profit_safety_buffer_percent: 0,
        REDACTED),
      ),
    ).toBeNull();
    expect(
      validateProfitControlFormState(
        formState({
          profit_min_margin_percent: 60,
          profit_safety_buffer_percent: 39.99,
        REDACTED),
      ),
    ).toBeNull();
  REDACTED);

  it("validates all five supported platforms and skips unsupported platforms", () => {
    expect(
      validateProfitControlFormState(
        formState({ profit_control_enabled: false, profit_min_margin_percent: 200 REDACTED),
      ),
    ).toBeNull();
    expect(
      validateProfitControlFormState(
        formState({ platform: "anthropic", profit_min_margin_percent: 200 REDACTED),
      ),
    ).toBe("marginRangeError");
    for (const platform of ["openai", "anthropic", "gemini", "grok", "antigravity"]) {
      expect(validateProfitControlFormState(formState({ platform REDACTED))).toBeNull();
    REDACTED
    expect(
      validateProfitControlFormState(
        formState({ platform: "composite", profit_min_margin_percent: 200 REDACTED),
      ),
    ).toBeNull();
  REDACTED);

  it("treats empty inputs as zero", () => {
    expect(
      validateProfitControlFormState(
        formState({
          profit_min_margin_percent: "",
          profit_safety_buffer_percent: null,
        REDACTED),
      ),
    ).toBeNull();
  REDACTED);

  it("rejects out-of-range margin and buffer", () => {
    expect(
      validateProfitControlFormState(
        formState({ profit_min_margin_percent: 100 REDACTED),
      ),
    ).toBe("marginRangeError");
    expect(
      validateProfitControlFormState(
        formState({ profit_min_margin_percent: -1 REDACTED),
      ),
    ).toBe("marginRangeError");
    expect(
      validateProfitControlFormState(
        formState({ profit_safety_buffer_percent: 100 REDACTED),
      ),
    ).toBe("bufferRangeError");
    expect(
      validateProfitControlFormState(
        formState({ profit_safety_buffer_percent: -0.1 REDACTED),
      ),
    ).toBe("bufferRangeError");
  REDACTED);

  it("rejects margin plus buffer reaching 100 percent", () => {
    expect(
      validateProfitControlFormState(
        formState({
          profit_min_margin_percent: 60,
          profit_safety_buffer_percent: 40,
        REDACTED),
      ),
    ).toBe("sumTooHigh");
  REDACTED);
REDACTED);
