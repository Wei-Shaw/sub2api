import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import EmailNotificationPolicyCard from "../EmailNotificationPolicyCard.vue";

const { getPolicy, updatePolicy, getDeliveries, retryDelivery, showError, showSuccess } = vi.hoisted(() => ({
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
      id: "refund_admin",
      enabled: false,
      recipient_kind: "group" as const,
      recipient_group: "finance",
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
  ],
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
    getDeliveries.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 });
    updatePolicy.mockImplementation(async (payload) => ({
      version: 1,
      configured: true,
      ...payload,
    }));
  });

  it("loads legacy recipients and saves channel-level routing", async () => {
    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    expect((wrapper.find("input[type='email']").element as HTMLInputElement).value).toBe("legacy@example.com");
    expect(wrapper.text()).toContain("admin.settings.emailPolicy.legacyUnverified");

    const refundRow = wrapper
      .findAll("section > div")
      .find((node) => node.text().includes("admin.settings.emailPolicy.channels.refundAdmin"));
    expect(refundRow).toBeDefined();
    await refundRow!.find("button[role='switch']").trigger("click");

    const saveButton = wrapper
      .findAll("button")
      .find((node) => node.text().includes("common.save"));
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(updatePolicy).toHaveBeenCalledTimes(1);
    const request = updatePolicy.mock.calls[0][0];
    expect(request.channels.find((channel: { id: string }) => channel.id === "refund_admin").enabled).toBe(true);
    expect(request.recipient_groups[0].members[0].status).toBe("legacy_unverified");
    expect(showSuccess).toHaveBeenCalled();
  });

  it("shows redacted delivery state and retries transient failures", async () => {
    getDeliveries.mockResolvedValue({
      items: [{
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
      }],
      total: 1,
      page: 1,
      page_size: 20,
    });
    retryDelivery.mockResolvedValue(undefined);

    const wrapper = mount(EmailNotificationPolicyCard);
    await flushPromises();

    expect(wrapper.text()).toContain("a***e@e***.com");
    expect(wrapper.text()).toContain("smtp timeout");
    const retryButton = wrapper.find(`button[title="admin.settings.emailPolicy.deliveries.retry"]`);
    expect(retryButton.exists()).toBe(true);
    await retryButton.trigger("click");
    await flushPromises();
    expect(retryDelivery).toHaveBeenCalledWith(41);
  });
});
