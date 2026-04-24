<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useSecuritySettingsContext } from "../composables/useSecuritySettings";
import { enabledFanclubProviders } from "../fanclub/registry";
import { getFanclubLinkStatus } from "../fanclub/me";

const { t } = useI18n();
const {
  me,
  secret,
  uri,
  qrDataUrl,
  code,
  err,
  msg,
  loading,
  webPushConfig,
  webPushBusy,
  webPushPermission,
  webPushBrowserSubscribed,
  webPushStatusLabel,
  setup,
  enable,
  connectPatreonMember,
  connectPatreonCreator,
  disconnectMember,
  disconnectCreator,
  enableWebPushNotifications,
  disableWebPushNotifications,
  fanclubLinking,
} = useSecuritySettingsContext();

const providers = enabledFanclubProviders();
const selectedProviderId = ref(providers[0]?.id ?? "");

const webPushPermissionDisplay = computed(() =>
  webPushPermission.value === "unsupported"
    ? t("views.settings.security.webPush.permissionUnsupported")
    : webPushPermission.value,
);

const webPushSubscribedLabel = computed(() =>
  webPushBrowserSubscribed.value
    ? t("views.settings.security.webPush.subscribedOn")
    : t("views.settings.security.webPush.subscribedOff"),
);

const selectedProvider = computed(() => providers.find((p) => p.id === selectedProviderId.value) ?? null);
const selectedLinkStatus = computed(() =>
  selectedProvider.value ? getFanclubLinkStatus(me.value, selectedProvider.value.id) : null,
);

</script>

<template>
  <div class="space-y-10">
    <section>
      <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
        {{ $t("views.settings.sections.twoFactor") }}
      </h2>
      <div class="mt-3 space-y-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
        <div v-if="me">
          <p class="text-sm text-neutral-600">
            {{ $t("views.settings.security.summarySignedIn", { email: me.email }) }}
            <span class="whitespace-nowrap">
              · {{ $t("views.settings.security.summaryTwoFactor") }}
              <span :class="me.totp_enabled ? 'font-medium text-lime-700' : 'text-amber-600'">
                {{ me.totp_enabled ? $t("views.settings.security.twoFactorEnabled") : $t("views.settings.security.twoFactorDisabled") }}
              </span>
            </span>
          </p>
          <p v-if="me.id" class="mt-2 text-xs text-neutral-500">
            {{ $t("views.settings.security.uuidLabel") }}
            <code class="ml-1 break-all rounded bg-neutral-100 px-1.5 py-0.5 font-mono text-[11px] text-neutral-800">
              {{ me.id }}
            </code>
          </p>
        </div>

        <div
          v-if="me && !me.totp_enabled"
          class="space-y-4 rounded-xl border border-lime-200 bg-lime-50/50 p-4"
        >
          <button
            type="button"
            class="rounded-md bg-lime-500 px-3 py-2 text-sm font-medium text-white hover:bg-lime-600 disabled:opacity-50"
            :disabled="loading"
            @click="setup"
          >
            {{ $t("views.settings.security.mfa.setupButton") }}
          </button>
          <div v-if="uri" class="space-y-4 text-sm text-neutral-700">
            <p>{{ $t("views.settings.security.mfa.setupIntro") }}</p>
            <div
              v-if="qrDataUrl"
              class="mx-auto w-fit overflow-hidden rounded-xl border border-lime-200 bg-white p-3 shadow-sm"
            >
              <img
                :src="qrDataUrl"
                width="240"
                height="240"
                class="block h-60 w-60 max-w-full"
                :alt="$t('views.settings.security.mfa.qrAlt')"
              />
            </div>
            <p v-else class="text-center text-xs text-neutral-500">{{ $t("views.settings.security.mfa.qrLoading") }}</p>
            <p class="text-xs text-neutral-600">
              {{ $t("views.settings.security.mfa.secretLabel") }}
              <code class="ml-1 break-all rounded bg-white px-1.5 py-0.5 font-mono text-[13px] text-neutral-900">
                {{ secret }}
              </code>
            </p>
            <details class="text-xs text-neutral-500">
              <summary class="cursor-pointer text-neutral-600 hover:text-neutral-800">{{ $t("views.settings.security.mfa.uriToggle") }}</summary>
              <p class="mt-2 break-all rounded-md border border-lime-100 bg-white p-2 text-[11px] text-neutral-800">
                {{ uri }}
              </p>
            </details>
          </div>
          <div v-if="uri" class="space-y-2">
            <label class="block text-sm font-medium text-neutral-700">{{ $t("views.settings.security.mfa.codeLabel") }}</label>
            <div class="flex gap-2">
              <input
                v-model="code"
                type="text"
                maxlength="8"
                class="flex-1 rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
              />
              <button
                type="button"
                class="rounded-md bg-lime-600 px-3 py-2 text-sm font-medium text-white hover:bg-lime-700 disabled:opacity-50"
                :disabled="loading || !code"
                @click="enable"
              >
                {{ $t("views.settings.security.mfa.enableButton") }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 通知（Web Push） -->
    <section>
      <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
        {{ $t("views.settings.sections.notifications") }}
      </h2>
      <div class="mt-3 space-y-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
        <h3 class="text-base font-semibold text-neutral-900">{{ $t("views.settings.security.webPush.heading") }}</h3>
        <p class="text-sm leading-relaxed text-neutral-600">{{ $t("views.settings.security.webPush.intro") }}</p>
        <div class="rounded-xl border border-neutral-200 bg-neutral-50/80 p-4">
          <p class="text-sm font-medium text-neutral-800">{{ webPushStatusLabel }}</p>
          <p class="mt-2 text-xs text-neutral-500">
            {{
              $t("views.settings.security.webPush.permissionLine", {
                permission: webPushPermissionDisplay,
                subscribed: webPushSubscribedLabel,
                count: webPushConfig?.subscription_count ?? 0,
              })
            }}
          </p>
          <div class="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              class="rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-white hover:bg-neutral-800 disabled:opacity-50"
              :disabled="webPushBusy || !webPushConfig?.available || webPushBrowserSubscribed"
              @click="enableWebPushNotifications"
            >
              {{
                webPushBusy && !webPushBrowserSubscribed
                  ? $t("views.settings.security.webPush.enableBusy")
                  : $t("views.settings.security.webPush.enableButton")
              }}
            </button>
            <button
              type="button"
              class="rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
              :disabled="webPushBusy || !webPushBrowserSubscribed"
              @click="disableWebPushNotifications"
            >
              {{
                webPushBusy && webPushBrowserSubscribed
                  ? $t("views.settings.security.webPush.disableBusy")
                  : $t("views.settings.security.webPush.disableButton")
              }}
            </button>
          </div>
        </div>
      </div>
    </section>

    <section v-if="providers.length">
      <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
        {{ $t("views.settings.sections.fanclubLinks") }}
      </h2>

      <div class="mt-3 space-y-3">
        <div class="w-full max-w-sm">
          <label class="mb-1 block text-xs font-semibold uppercase tracking-wide text-neutral-500">
            {{ $t("views.settings.security.fanclubs.providerLabel") }}
          </label>
          <select
            v-model="selectedProviderId"
            class="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          >
            <option v-for="p in providers" :key="p.id" :value="p.id">
              {{ p.labelKey ? $t(p.labelKey) : p.label ?? p.id }}
            </option>
          </select>
        </div>

        <div v-if="selectedProvider?.id === 'patreon'" class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
          <p class="text-sm text-neutral-700">
            {{ $t("views.settings.security.patreon.description") }}
          </p>

          <div class="grid gap-3 sm:grid-cols-2">
            <div class="rounded-xl border border-neutral-200 bg-neutral-50 p-3">
              <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
                {{ $t("views.settings.security.patreon.memberTitle") }}
              </p>
              <p class="mt-1 text-sm text-neutral-700">
                {{
                  selectedLinkStatus?.member_linked
                    ? $t("views.settings.security.patreon.linked")
                    : $t("views.settings.security.patreon.notLinked")
                }}
              </p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-if="!selectedLinkStatus?.member_linked"
                  type="button"
                  class="rounded-md bg-lime-500 px-3 py-2 text-sm font-medium text-white hover:bg-lime-600 disabled:opacity-50"
                  :disabled="fanclubLinking"
                  @click="connectPatreonMember"
                >
                  {{ fanclubLinking ? $t("views.settings.security.patreon.connectBusy") : $t("views.settings.security.patreon.connect") }}
                </button>
                <button
                  v-else
                  type="button"
                  class="rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
                  :disabled="fanclubLinking"
                  @click="disconnectMember"
                >
                  {{ fanclubLinking ? $t("views.settings.security.patreon.disconnectBusy") : $t("views.settings.security.patreon.disconnect") }}
                </button>
              </div>
            </div>

            <div class="rounded-xl border border-neutral-200 bg-neutral-50 p-3">
              <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
                {{ $t("views.settings.security.patreon.creatorTitle") }}
              </p>
              <p class="mt-1 text-sm text-neutral-700">
                {{
                  selectedLinkStatus?.creator_linked
                    ? $t("views.settings.security.patreon.linked")
                    : $t("views.settings.security.patreon.notLinked")
                }}
              </p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-if="!selectedLinkStatus?.creator_linked"
                  type="button"
                  class="rounded-md bg-lime-500 px-3 py-2 text-sm font-medium text-white hover:bg-lime-600 disabled:opacity-50"
                  :disabled="fanclubLinking"
                  @click="connectPatreonCreator"
                >
                  {{ fanclubLinking ? $t("views.settings.security.patreon.connectBusy") : $t("views.settings.security.patreon.connect") }}
                </button>
                <button
                  v-else
                  type="button"
                  class="rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
                  :disabled="fanclubLinking"
                  @click="disconnectCreator"
                >
                  {{ fanclubLinking ? $t("views.settings.security.patreon.disconnectBusy") : $t("views.settings.security.patreon.disconnect") }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <p v-else class="text-sm text-neutral-600">
          {{ $t("views.settings.security.fanclubs.noProviders") }}
        </p>
      </div>
    </section>

    <p v-if="msg" class="text-sm font-medium text-lime-700">{{ msg }}</p>
    <p v-if="err" class="text-sm text-red-600">{{ err }}</p>
  </div>
</template>
