<script setup lang="ts">
type PatreonTier = { id: string; title: string; amount_cents: number };
type PatreonCampaign = { id: string; name: string };

const props = defineProps<{
  campaignId: string;
  requiredTierId: string;
  campaigns: PatreonCampaign[];
  tiers: PatreonTier[];
  campaignsErr: string;
  tiersErr: string;
  onCampaignChange: () => void;
}>();

const emit = defineEmits<{
  (e: "update:campaignId", v: string): void;
  (e: "update:requiredTierId", v: string): void;
}>();
</script>

<template>
  <div class="mb-3">
    <label class="mb-1 block text-xs font-medium text-lime-800 dark:text-lime-300" for="note-patreon-campaign">{{
      $t("views.noteEdit.patreonCampaignLabel")
    }}</label>
    <select
      id="note-patreon-campaign"
      :value="campaignId"
      class="mb-2 w-full max-w-md rounded-xl border border-lime-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-400 focus:ring-2 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-neutral-100"
      @change="
        (e) => {
          emit('update:campaignId', (e.target as HTMLSelectElement).value);
          onCampaignChange();
        }
      "
    >
      <option value="">{{ $t("views.noteEdit.patreonCampaignDefault") }}</option>
      <option
        v-if="campaignId && !campaigns.some((x) => x.id === campaignId)"
        :value="campaignId"
      >
        {{ $t("views.noteEdit.patreonCurrentId", { id: campaignId }) }}
      </option>
      <option v-for="c in campaigns" :key="c.id" :value="c.id">
        {{ c.name || c.id }}
      </option>
    </select>
    <p v-if="campaignsErr" class="mb-2 text-xs text-lime-700/90 dark:text-lime-200/90">{{ campaignsErr }}</p>

    <label class="mb-1 block text-xs font-medium text-lime-800 dark:text-lime-300" for="note-patreon-tier">{{
      $t("views.noteEdit.patreonTierLabel")
    }}</label>
    <select
      id="note-patreon-tier"
      :value="requiredTierId"
      class="w-full max-w-md rounded-xl border border-lime-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-400 focus:ring-2 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-neutral-100"
      @change="(e) => emit('update:requiredTierId', (e.target as HTMLSelectElement).value)"
    >
      <option value="">{{ $t("views.noteEdit.patreonTierDefault") }}</option>
      <option
        v-if="requiredTierId && !tiers.some((x) => x.id === requiredTierId)"
        :value="requiredTierId"
      >
        {{ $t("views.noteEdit.patreonCurrentId", { id: requiredTierId }) }}
      </option>
      <option v-for="t in tiers" :key="t.id" :value="t.id">
        {{ t.title || $t("views.noteEdit.untitledTier") }}{{ t.amount_cents > 0 ? ` - ${(t.amount_cents / 100).toFixed(0)}` : "" }}
      </option>
    </select>
    <p v-if="tiersErr" class="mt-1 text-xs text-lime-700/90 dark:text-lime-200/90">{{ tiersErr }}</p>
  </div>
</template>

