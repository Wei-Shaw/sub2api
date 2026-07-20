import { mount REDACTED from "@vue/test-utils";
import { describe, expect, it REDACTED from "vitest";
import { createPinia REDACTED from "pinia";
import { createI18n REDACTED from "vue-i18n";
import type { SubscriptionPlan REDACTED from "@/types/payment";
import SubscriptionPlanCard from "../SubscriptionPlanCard.vue";

const i18n = createI18n({
  legacy: false,
  locale: "en",
  fallbackWarn: false,
  missingWarn: false,
  messages: {
    en: {
      payment: {
        days: "days",
        models: "Models",
        planCard: {
          quota: "Quota",
          rate: "Rate",
          unlimited: "Unlimited",
        REDACTED,
        subscribeNow: "Subscribe now",
      REDACTED,
    REDACTED,
  REDACTED,
REDACTED);

const mountPlanCard = (groupPlatform: string, overrides: Partial<SubscriptionPlan> = {REDACTED) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: {
        id: 1,
        group_id: 10,
        group_platform: groupPlatform,
        name: "Pro",
        price: 10,
        amount: 1000,
        features: [],
        rate_multiplier: 1,
        validity_days: 30,
        validity_unit: "day",
        supported_model_scopes: ["claude", "gemini_text", "gemini_image"],
        is_active: true,
        ...overrides,
      REDACTED,
    REDACTED,
    global: { plugins: [i18n, createPinia()] REDACTED,
  REDACTED);

describe("SubscriptionPlanCard", () => {
  it("does not show Antigravity model scopes for OpenAI plans", () => {
    const text = mountPlanCard("openai").text();

    expect(text).not.toContain("Claude");
    expect(text).not.toContain("Gemini");
    expect(text).not.toContain("Imagen");
  REDACTED);

  it("shows model scopes for Antigravity plans", () => {
    const text = mountPlanCard("antigravity").text();

    expect(text).toContain("Claude");
    expect(text).toContain("Gemini");
    expect(text).toContain("Imagen");
  REDACTED);

  it("uses the configured currency symbol while preserving USD for legacy plans", () => {
    const cnyPlan = mountPlanCard("openai", { currency: "CNY", original_price: 20 REDACTED).text();

    expect(cnyPlan).toContain("¥10CNY");
    expect(cnyPlan).toContain("¥20CNY");
    expect(mountPlanCard("openai", { currency: "USD" REDACTED).text()).toContain("$10USD");
    expect(mountPlanCard("openai", { currency: "" REDACTED).text()).toContain("$10");
  REDACTED);
REDACTED);
