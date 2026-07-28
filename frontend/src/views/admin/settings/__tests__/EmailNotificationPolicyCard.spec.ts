import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import EmailNotificationPolicyCard from "../EmailNotificationPolicyCard.vue";

const {
  getPolicy,
  updatePolicy,
  getDeliveries,
  retryDelivery,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  getPolicy: vi.fn(),
  updatePolicy: vi.fn(),
  getDeliveries: vi.fn(),
  retryDelivery: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock("@/api", () => ({
  adminAPI: {
    settings: {
      getNotificationEmailPolicy: getPolicy,
      updateNotificationEmailPolicy: updatePolicy,
      getNotificationEmailDeliveries: getDeliveries,
      retryNotificationEmailDelivery: retryDelivery,
    },
  },
}));

vi.mock("@/stores", () => ({
  useAppStore: () => ({ showError, showSuccess }),
}));

vi.mock("@/utils/apiError", () => ({
  extractApiErrorMessage: () => "request failed",
}));

vi.mock("vue-i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("vue-i18n")>()),
  useI18n: () => ({ t: (key: string) => key }),
}));

const policyFixture = {
  version: 1,
  configured: false,
  channels: [
    {
      id: "auth_verification",
      enabled: true,
      recipient_kind: "explicit" as const,
      events: ["auth.verify_code"],
    },
    {
      id: "password_reset",
      enabled: true,
      recipient_kind: "explicit" as const,
      events: ["auth.password_reset"],
    },
    {
      id: "subscription",
      enabled: true,
      recipient_kind: "user" as const,
      include_user_primary: true,
      events: ["subscription.purchase_success", "subscription.expiry_reminder"],
    },
    {
      id: "balance",
      enabled: true,
      recipient_kind: "user" as const,
      allow_user_primary: true,
      allow_verified_additional: true,
      include_user_primary: true,
      include_verified_additional: false,
      events: ["balance.low", "balance.recharge_success"],
    },
    {
      id: "refund_admin",
      enabled: false,
      recipient_kind: "group" as const,
      recipient_group: "finance",
      events: [],
    },
    {
      id: "account_quota",
      enabled: false,
      recipient_kind: "group" as const,
      recipient_group: "account_quota",
      events: [],
    },
    {
      id: "ops_alert",
      enabled: false,
      recipient_kind: "group" as const,
      recipient_group: "ops_alert",
      events: [],
    },
    {
      id: "ops_report",
      enabled: false,
      recipient_kind: "group" as const,
      recipient_group: "ops_report",
      events: [],
    },
  ],
  recipient_groups: [
    {
      id: "finance",
      members: [
        {
          email: "legacy@example.com",
          enabled: false,
          status: "legacy_unverified" as const,
        },
      ],
    },
    { id: "account_quota", members: [] },
    { id: "security", members: [] },
    { id: "ops_alert", members: [] },
    { id: "ops_report", members: [] },
  ],
  feature_settings: {
    subscription_expiry_enabled: true,
    balance_low_enabled: false,
    balance_low_threshold: 0,
    balance_low_recharge_url: "",
  },
};

describe("EmailNotificationPolicyCard", () => {
  beforeEach(() => {
    getPolicy.mockReset();
    updatePolicy.mockReset();
    getDeliveries.mockReset();
    retryDelivery.mockReset();
    showError.mockReset();
    showSuccess.mockReset();
    getPolicy.mockResolvedValue(structuredClone(policyFixture));
    getDeliveries.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
    });
    updatePolicy.mockImplementation(async (payload) => ({
      version: 1,
      configured: true,
      ...payload,
    }));
  });

  it("loads legacy recipients and saves channel-level routing", async () => {
    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    const financeGroup = wrapper.find(`[data-recipient-group-id="finance"]`);
    expect(
      (financeGroup.find("input[type='email']").element as HTMLInputElement)
        .value,
    ).toBe("legacy@example.com");
    expect(wrapper.text()).toContain(
      "admin.settings.emailPolicy.legacyUnverified",
    );
    expect(wrapper.text()).not.toContain(
      "admin.settings.emailPolicy.channels.authVerification",
    );
    expect(wrapper.text()).not.toContain(
      "admin.settings.emailPolicy.channels.passwordReset",
    );

    const refundRow = wrapper.find(`[data-channel-id="refund_admin"]`);
    expect(refundRow.exists()).toBe(true);
    await refundRow.find("button[role='switch']").trigger("click");

    const saveButton = wrapper
      .findAll("button")
      .find((node) => node.text().includes("common.save"));
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(updatePolicy).toHaveBeenCalledTimes(1);
    const request = updatePolicy.mock.calls[0][0];
    expect(
      request.channels.find(
        (channel: { id: string }) => channel.id === "refund_admin",
      ).enabled,
    ).toBe(true);
    expect(request.recipient_groups[0].members[0].status).toBe(
      "legacy_unverified",
    );
    expect(request.feature_settings.subscription_expiry_enabled).toBe(true);
    expect(showSuccess).toHaveBeenCalled();
  });

  it("adds one admin address to every active recipient group without exposing an unused group", async () => {
    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    expect(wrapper.find(`[data-recipient-group-id="security"]`).exists()).toBe(
      false,
    );
    const bulkInput = wrapper.get(`[data-testid="bulk-admin-email"]`);
    await bulkInput.setValue("ADMIN@example.com");
    const addAllButton = wrapper
      .findAll("button")
      .find((node) =>
        node.text().includes("admin.settings.emailPolicy.addToAllGroups"),
      );
    expect(addAllButton).toBeDefined();
    await addAllButton!.trigger("click");

    for (const groupID of [
      "finance",
      "account_quota",
      "ops_alert",
      "ops_report",
    ]) {
      const group = wrapper.get(`[data-recipient-group-id="${groupID}"]`);
      const emails = group
        .findAll("input[type='email']")
        .map((input) => (input.element as HTMLInputElement).value);
      expect(emails).toContain("admin@example.com");
    }
    expect(showSuccess).toHaveBeenCalledWith(
      "admin.settings.emailPolicy.addedToAllGroups",
    );
  });

  it("saves integrated subscription and low-balance settings in the policy request", async () => {
    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    const subscriptionRow = wrapper.get(`[data-channel-id="subscription"]`);
    expect(subscriptionRow.text()).toContain(
      "admin.settings.emailPolicy.subscriptionExpiry.title",
    );
    const balanceRow = wrapper.get(`[data-channel-id="balance"]`);
    const balanceToggles = balanceRow.findAll("button[role='switch']");
    expect(balanceToggles.length).toBeGreaterThanOrEqual(2);
    await balanceToggles[1].trigger("click");
    await balanceRow.find('input[type="number"]').setValue("15");
    await balanceRow
      .find('input[type="url"]')
      .setValue("https://example.com/recharge");

    const saveButton = wrapper
      .findAll("button")
      .find((node) => node.text().includes("common.save"));
    await saveButton!.trigger("click");
    await flushPromises();

    expect(updatePolicy).toHaveBeenCalledTimes(1);
    expect(updatePolicy.mock.calls[0][0].feature_settings).toEqual({
      subscription_expiry_enabled: true,
      balance_low_enabled: true,
      balance_low_threshold: 15,
      balance_low_recharge_url: "https://example.com/recharge",
    });
  });

  it("shows redacted delivery state and retries transient failures", async () => {
    getDeliveries.mockResolvedValue({
      items: [
        {
          id: 41,
          event: "ops.alert",
          channel: "ops_alert",
          recipient: "a***e@e***.com",
          source_type: "ops_incident",
          source_id: "incident-41",
          status: "failed",
          attempt_count: 5,
          max_attempts: 5,
          next_attempt_at: "2026-07-26T08:00:00Z",
          last_error_category: "transport",
          last_error: "smtp timeout",
          created_at: "2026-07-26T08:00:00Z",
          updated_at: "2026-07-26T08:05:00Z",
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    });
    retryDelivery.mockResolvedValue(undefined);

    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    expect(wrapper.text()).toContain("a***e@e***.com");
    expect(wrapper.text()).toContain("smtp timeout");
    const retryButton = wrapper.find(
      `button[title="admin.settings.emailPolicy.deliveries.retry"]`,
    );
    expect(retryButton.exists()).toBe(true);
    await retryButton.trigger("click");
    await flushPromises();
    expect(retryDelivery).toHaveBeenCalledWith(41);
  });

  it("normalizes legacy null recipient members instead of crashing the policy card", async () => {
    getPolicy.mockResolvedValue({
      ...structuredClone(policyFixture),
      recipient_groups: [
        { id: "finance", members: null },
        { id: "ops_alert", members: null },
      ],
    });

    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    expect(wrapper.text()).toContain("admin.settings.emailPolicy.title");
    expect(wrapper.text()).toContain("admin.settings.emailPolicy.noRecipients");

    const addButtons = wrapper.findAll(
      `button[title="admin.settings.emailPolicy.addRecipient"]`,
    );
    expect(addButtons).toHaveLength(2);
    await addButtons[0].trigger("click");

    expect(
      wrapper
        .get(`[data-recipient-group-id="finance"]`)
        .findAll("input[type='email']"),
    ).toHaveLength(1);
  });
});
