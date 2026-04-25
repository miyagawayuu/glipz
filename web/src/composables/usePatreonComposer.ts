import { computed, type Ref, ref, watch } from "vue";
import { fetchPatreonCampaigns, fetchPatreonStatus, startPatreonOAuth, type PatreonCampaignRow } from "../lib/fanclubPatreon";

export const patreonSettingsPath = "/settings";

type Translate = (key: string) => string;
type MembershipProvider = "patreon" | "gumroad";

export function usePatreonComposer(opts: {
  viewPassword: Ref<string>;
  viewPasswordConfirm: Ref<string>;
  composerPasswordOpen: Ref<boolean>;
}) {
  const patreonAvailable = ref(false);
  const patreonConnected = ref(false);
  const patreonCampaigns = ref<PatreonCampaignRow[]>([]);
  const composerMembershipOpen = ref(false);
  const membershipUsePatreon = ref(false);
  const membershipProvider = ref<MembershipProvider>("gumroad");
  const membershipCampaignId = ref("");
  const membershipTierId = ref("");
  const patreonConnectBusy = ref(false);

  const membershipTierOptions = computed(() => {
    const id = membershipCampaignId.value;
    const camp = patreonCampaigns.value.find((c) => c.id === id);
    return camp?.tiers ?? [];
  });

  watch([opts.viewPassword, opts.viewPasswordConfirm], () => {
    if (opts.viewPassword.value.trim() || opts.viewPasswordConfirm.value.trim()) {
      membershipUsePatreon.value = false;
    }
  });

  watch(membershipUsePatreon, (v) => {
    if (v) {
      opts.viewPassword.value = "";
      opts.viewPasswordConfirm.value = "";
      opts.composerPasswordOpen.value = false;
    }
  });

  watch(membershipCampaignId, () => {
    membershipTierId.value = "";
  });

  watch(membershipProvider, () => {
    membershipCampaignId.value = "";
    membershipTierId.value = "";
  });

  async function loadPatreon(token: string) {
    try {
      const s = await fetchPatreonStatus(token);
      const p = s.patreon;
      patreonAvailable.value = Boolean(p?.available);
      patreonConnected.value = Boolean(p?.available && p?.connected);
      if (patreonConnected.value) {
        try {
          patreonCampaigns.value = await fetchPatreonCampaigns(token);
        } catch {
          patreonCampaigns.value = [];
        }
      } else {
        patreonCampaigns.value = [];
      }
    } catch {
      patreonAvailable.value = false;
      patreonConnected.value = false;
      patreonCampaigns.value = [];
    }
  }

  function resetPatreonComposerState() {
    composerMembershipOpen.value = false;
    membershipUsePatreon.value = false;
    membershipProvider.value = "gumroad";
    membershipCampaignId.value = "";
    membershipTierId.value = "";
  }

  /** Returns a translated error message, or null if validation passes. */
  function validateMembershipForSubmit(pw: string, pw2: string, t: Translate): string | null {
    if (membershipUsePatreon.value && (pw.trim() || pw2.trim())) {
      return t("views.compose.errors.membershipWithPassword");
    }
    if (membershipUsePatreon.value && membershipProvider.value === "patreon") {
      if (!membershipCampaignId.value.trim() || !membershipTierId.value.trim()) {
        return t("views.compose.errors.membershipIdsRequired");
      }
    }
    if (membershipUsePatreon.value && membershipProvider.value === "gumroad" && !membershipCampaignId.value.trim()) {
      return t("views.compose.errors.gumroadProductRequired");
    }
    return null;
  }

  function applyMembershipToBody(body: Record<string, unknown>) {
    if (!membershipUsePatreon.value) return;
    body.membership = {
      provider: membershipProvider.value,
      creator_id: membershipCampaignId.value.trim(),
      tier_id: membershipProvider.value === "gumroad" ? "license" : membershipTierId.value.trim(),
    };
  }

  async function connectPatreonOAuth(returnTo: string): Promise<{ error?: string }> {
    if (patreonConnectBusy.value) return {};
    patreonConnectBusy.value = true;
    try {
      await startPatreonOAuth(returnTo);
      return {};
    } catch (e) {
      return { error: e instanceof Error ? e.message : "patreon_oauth_failed" };
    } finally {
      patreonConnectBusy.value = false;
    }
  }

  return {
    patreonAvailable,
    patreonConnected,
    patreonCampaigns,
    composerMembershipOpen,
    membershipUsePatreon,
    membershipProvider,
    membershipCampaignId,
    membershipTierId,
    patreonConnectBusy,
    membershipTierOptions,
    loadPatreon,
    resetPatreonComposerState,
    validateMembershipForSubmit,
    applyMembershipToBody,
    connectPatreonOAuth,
  };
}
