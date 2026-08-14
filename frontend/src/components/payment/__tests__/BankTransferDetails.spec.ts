import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { createI18n } from "vue-i18n";
import BankTransferDetails from "../BankTransferDetails.vue";

const copyToClipboard = vi.fn(async () => true);

vi.mock("@/composables/useClipboard", () => ({
  useClipboard: () => ({ copied: { value: false }, copyToClipboard }),
}));

const i18n = createI18n({
  legacy: false,
  locale: "en",
  fallbackWarn: false,
  missingWarn: false,
  messages: {
    en: {
      common: { copy: "Copy", copied: "Copied" },
      payment: {
        qr: {
          transfer: {
            title: "Or transfer manually",
            hint: "Keep the transfer content unchanged",
            bank: "Bank",
            accountNumber: "Account Number",
            accountName: "Account Name",
            amount: "Amount",
            content: "Transfer Content",
          },
        },
      },
    },
  },
});

const TRANSFER = {
  bank_code: "MBBANK",
  bank_bin: "970422",
  account_number: "0123456789",
  account_name: "CONG TY ABC",
  amount: "500000",
  content: "SUB123456",
};

const mountDetails = (props: Record<string, unknown> = {}) =>
  mount(BankTransferDetails, {
    props: { transfer: TRANSFER, currency: "VND", ...props },
    global: { plugins: [i18n] },
  });

describe("BankTransferDetails", () => {
  it("renders every field a payer has to type into their banking app", () => {
    const text = mountDetails().text();

    expect(text).toContain("MBBANK");
    expect(text).toContain("0123456789");
    expect(text).toContain("CONG TY ABC");
    expect(text).toContain("SUB123456");
    // Dong is zero-decimal: the minor-unit string is already the payable figure.
    expect(text).toContain("500,000");
  });

  it("copies the raw amount, not the formatted one — bank forms reject separators", async () => {
    copyToClipboard.mockClear();
    const wrapper = mountDetails();

    await wrapper.get('[data-testid="copy-transfer-amount"]').trigger("click");

    expect(copyToClipboard).toHaveBeenCalledWith("500000");
  });

  it("copies the transfer content verbatim — SePay matches the order by it alone", async () => {
    copyToClipboard.mockClear();
    const wrapper = mountDetails();

    await wrapper.get('[data-testid="copy-transfer-content"]').trigger("click");

    expect(copyToClipboard).toHaveBeenCalledWith("SUB123456");
  });

  it("renders nothing for a provider that returns no transfer instructions", () => {
    expect(mountDetails({ transfer: undefined }).find("section").exists()).toBe(false);
  });

  it("falls back to the BIN when the bank has no short code", () => {
    const wrapper = mountDetails({ transfer: { ...TRANSFER, bank_code: "" } });

    expect(wrapper.get('[data-testid="transfer-bank"]').text()).toBe("970422");
  });
});
