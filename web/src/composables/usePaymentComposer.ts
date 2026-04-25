import { type Ref, ref, watch } from "vue";

type Translate = (key: string) => string;

export function usePaymentComposer(opts: {
  viewPassword: Ref<string>;
  viewPasswordConfirm: Ref<string>;
  composerPasswordOpen: Ref<boolean>;
  membershipUsePatreon: Ref<boolean>;
}) {
  const composerPaymentOpen = ref(false);
  const paymentUsePayPal = ref(false);
  const payPalPlanId = ref("");

  watch([opts.viewPassword, opts.viewPasswordConfirm], () => {
    if (opts.viewPassword.value.trim() || opts.viewPasswordConfirm.value.trim()) {
      // Password + payment is allowed; do nothing.
    }
  });

  watch(opts.membershipUsePatreon, (v) => {
    if (v) {
      paymentUsePayPal.value = false;
      payPalPlanId.value = "";
    }
  });

  function resetPaymentComposerState() {
    composerPaymentOpen.value = false;
    paymentUsePayPal.value = false;
    payPalPlanId.value = "";
  }

  function validatePaymentForSubmit(t: Translate): string | null {
    if (opts.membershipUsePatreon.value && paymentUsePayPal.value) {
      return t("views.compose.errors.paymentWithMembership");
    }
    if (paymentUsePayPal.value && !payPalPlanId.value.trim()) {
      return t("views.compose.errors.paypalPlanRequired");
    }
    return null;
  }

  function applyPaymentToBody(body: Record<string, unknown>) {
    if (!paymentUsePayPal.value) return;
    body.payment = {
      provider: "paypal",
      plan_id: payPalPlanId.value.trim(),
    };
  }

  return {
    composerPaymentOpen,
    paymentUsePayPal,
    payPalPlanId,
    resetPaymentComposerState,
    validatePaymentForSubmit,
    applyPaymentToBody,
  };
}

