<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import {
  APP_VERSION,
  FEDERATION_EVENT_SCHEMA_VERSION,
  FEDERATION_PROTOCOL_VERSION,
  FEDERATION_SUPPORTED_PROTOCOL_VERSIONS,
} from "../lib/appInfo";
import { sanitizeInlineHtml } from "../lib/sanitizeHtml";
import { useBackLink } from "../lib/useBackLink";

type FederationEventSchemaRow = {
  kind: string;
  required: string;
  optional: string;
};

const publicOriginExample = "https://api.example.com";
const frontendOriginExample = "https://example.com";
const supportedProtocolVersionsJSON = JSON.stringify(FEDERATION_SUPPORTED_PROTOCOL_VERSIONS);
const { t, tm } = useI18n();
const overviewPoints = computed(() => tm("federationGuidelines.overviewPoints") as string[]);
const supportItems = computed(() => tm("federationGuidelines.supportItems") as string[]);
const versionPolicyItems = computed(() => tm("federationGuidelines.versionPolicyItems") as string[]);
const premiseParagraphs = computed(() => tm("federationGuidelines.premiseParagraphs") as string[]);
const endpointItems = computed(() => tm("federationGuidelines.endpointItems") as string[]);
const discoveryItems = computed(() => tm("federationGuidelines.discoveryItems") as string[]);
const signatureHeaders = computed(() => tm("federationGuidelines.signatureHeaders") as string[]);
const signatureItems = computed(() => tm("federationGuidelines.signatureItems") as string[]);
const signatureFlowItems = computed(() => tm("federationGuidelines.signatureFlowItems") as string[]);
const eventItems = computed(() => tm("federationGuidelines.eventItems") as string[]);
const eventSchemaHeaders = computed(() => tm("federationGuidelines.eventSchemaHeaders") as Record<string, string>);
const eventSchemaRows = computed(() => tm("federationGuidelines.eventSchemaRows") as FederationEventSchemaRow[]);
const followItems = computed(() => tm("federationGuidelines.followItems") as string[]);
const errorCodeItems = computed(() => tm("federationGuidelines.errorCodeItems") as string[]);
const rateLimitItems = computed(() => tm("federationGuidelines.rateLimitItems") as string[]);
const opsItems = computed(() => tm("federationGuidelines.opsItems") as string[]);
const cautionItems = computed(() => tm("federationGuidelines.cautionItems") as string[]);
const cautionParagraphs = computed(() => tm("federationGuidelines.cautionParagraphs") as string[]);

const backLink = useBackLink({ fallbackTo: "/register" });
const safeHtml = (value: string) => sanitizeInlineHtml(value);
</script>

<template>
  <div class="w-full min-w-0 px-4 py-8 text-neutral-900 sm:px-6 lg:px-8">
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-8">
      <RouterLink :to="backLink.to" class="text-sm font-medium text-lime-700 hover:text-lime-800" @click="backLink.onClick">{{ backLink.label }}</RouterLink>

      <section class="overflow-hidden rounded-[2rem] border border-lime-200 bg-white dark:border-lime-800/70 dark:bg-neutral-950">
        <div class="grid gap-8 px-6 py-10 sm:px-8 lg:grid-cols-[minmax(0,1.1fr)_24rem] lg:items-center lg:px-10">
          <div class="max-w-3xl">
            <p class="text-sm font-semibold uppercase tracking-[0.2em] text-lime-700">{{ $t("federationGuidelines.badge") }}</p>
            <h1 class="mt-4 text-4xl font-bold tracking-tight text-neutral-900 sm:text-5xl">
              {{ ($tm("federationGuidelines.title") as string[])[0] }}
              <br />
              {{ ($tm("federationGuidelines.title") as string[])[1] }}
            </h1>
            <p class="mt-5 max-w-2xl text-sm leading-7 text-neutral-700 sm:text-base">
              {{ $t("federationGuidelines.description", { protocol: FEDERATION_PROTOCOL_VERSION }) }}
            </p>
            <div class="mt-6 flex flex-wrap gap-2 text-xs text-neutral-600">
              <span class="rounded-full border border-neutral-200 bg-white px-3 py-1 dark:border-neutral-200 dark:bg-neutral-900">App {{ APP_VERSION }}</span>
              <span class="rounded-full border border-neutral-200 bg-white px-3 py-1 dark:border-neutral-200 dark:bg-neutral-900">{{ $t("common.labels.protocol") }} {{ FEDERATION_PROTOCOL_VERSION }}</span>
              <span class="rounded-full border border-neutral-200 bg-white px-3 py-1 dark:border-neutral-200 dark:bg-neutral-900">{{ $t("federationGuidelines.eventSchemaLabel") }}</span>
              <span class="rounded-full border border-neutral-200 bg-white px-3 py-1 dark:border-neutral-200 dark:bg-neutral-900">{{ $t("common.labels.updated", { date: "2026-04-22" }) }}</span>
            </div>
          </div>

          <div class="rounded-3xl border border-neutral-200 bg-white/90 p-5 shadow-sm dark:border-neutral-200 dark:bg-neutral-900/90">
            <p class="text-sm font-semibold text-neutral-900">{{ $t("federationGuidelines.overviewTitle") }}</p>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="point in overviewPoints" :key="point" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span>{{ point }}</span>
              </li>
            </ul>
            <div class="mt-5 rounded-2xl border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm leading-6 text-neutral-700 dark:border-neutral-200 dark:bg-neutral-800">
              {{ $t("federationGuidelines.overviewNote") }}
            </div>
          </div>
        </div>
      </section>

      <section class="rounded-[2rem] border border-neutral-200 bg-white px-6 py-8 dark:border-neutral-200 dark:bg-neutral-950 sm:px-8">
        <div class="space-y-8">
          <section>
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.supportTitle") }}</h2>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in supportItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.versionPolicyTitle") }}</h2>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in versionPolicyItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.premiseTitle") }}</h2>
            <p v-for="paragraph in premiseParagraphs" :key="paragraph" class="mt-3 text-sm leading-7 text-neutral-700">
              {{ paragraph }}
            </p>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.endpointTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.endpointDescription") }}
            </p>
            <div class="mt-4 overflow-x-auto rounded-2xl bg-neutral-900 p-4 text-xs text-lime-100">
              <pre>GET  {{ publicOriginExample }}/.well-known/glipz-federation
GET  {{ publicOriginExample }}/federation/profile/{handle}
GET  {{ publicOriginExample }}/federation/dm-keys/{handle}
GET  {{ publicOriginExample }}/federation/posts/{handle}
POST {{ publicOriginExample }}/federation/posts/{postID}/unlock
POST {{ publicOriginExample }}/federation/follow
POST {{ publicOriginExample }}/federation/unfollow
POST {{ publicOriginExample }}/federation/events</pre>
            </div>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in endpointItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <p class="mt-4 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.endpointNote") }}
            </p>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.discoveryTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">{{ $t("federationGuidelines.discoveryDescription") }}</p>
            <div class="mt-4 overflow-x-auto rounded-2xl bg-neutral-900 p-4 text-xs text-lime-100">
              <pre>{
  "resource": "alice@example.com",
  "server": {
    "protocol_version": "{{ FEDERATION_PROTOCOL_VERSION }}",
    "supported_protocol_versions": {{ supportedProtocolVersionsJSON }},
    "server_software": "glipz",
    "server_version": "{{ APP_VERSION }}",
    "event_schema_version": {{ FEDERATION_EVENT_SCHEMA_VERSION }},
    "host": "example.com",
    "origin": "{{ publicOriginExample }}",
    "key_id": "{{ publicOriginExample }}/.well-known/glipz-federation#default",
    "public_key": "BASE64_ED25519_PUBLIC_KEY",
    "events_url": "{{ publicOriginExample }}/federation/events",
    "follow_url": "{{ publicOriginExample }}/federation/follow",
    "unfollow_url": "{{ publicOriginExample }}/federation/unfollow",
    "dm_keys_url": "{{ publicOriginExample }}/federation/dm-keys",
    "known_instances": ["trusted.example", "friend.example"]
  },
  "account": {
    "acct": "alice@example.com",
    "handle": "alice",
    "domain": "example.com",
    "display_name": "Alice",
    "summary": "profile bio",
    "avatar_url": "https://cdn.example.com/avatar.jpg",
    "profile_url": "{{ frontendOriginExample }}/@alice",
    "posts_url": "{{ publicOriginExample }}/federation/posts/alice"
  }
}</pre>
            </div>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in discoveryItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <p class="mt-4 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.discoveryResourceNote") }}
            </p>
            <p class="mt-3 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.discoveryProfileNote") }}
            </p>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.signatureTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.signatureDescription") }}
            </p>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in signatureHeaders" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <div class="mt-4 overflow-x-auto rounded-2xl bg-neutral-900 p-4 text-xs text-lime-100">
              <pre>UPPERCASE_HTTP_METHOD
/request/path
RFC3339_TIMESTAMP
NONCE
BASE64(SHA256(request_body))</pre>
            </div>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in signatureItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <p class="mt-6 text-sm font-semibold text-neutral-900">{{ $t("federationGuidelines.signatureFlowTitle") }}</p>
            <ol class="mt-3 list-decimal space-y-2 pl-5 text-sm leading-7 text-neutral-700">
              <li v-for="item in signatureFlowItems" :key="item">
                <span v-html="safeHtml(item)" />
              </li>
            </ol>
            <p class="mt-4 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.signatureNote") }}
            </p>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.eventsTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">{{ $t("federationGuidelines.eventsDescription") }}</p>
            <div class="mt-4 overflow-x-auto rounded-2xl bg-neutral-900 p-4 text-xs text-lime-100">
              <pre>{
  "event_id": "EVENT_UUID",
  "v": {{ FEDERATION_EVENT_SCHEMA_VERSION }},
  "kind": "post_created | repost_created | post_updated | post_deleted | post_liked | post_unliked | poll_voted | poll_tally_updated | dm_invite | dm_accept | dm_reject | dm_message",
  "author": {
    "acct": "alice@example.com",
    "handle": "alice",
    "domain": "example.com",
    "display_name": "Alice",
    "avatar_url": "https://cdn.example.com/avatar.jpg",
    "profile_url": "{{ frontendOriginExample }}/@alice"
  },
  "post": { "id": "POST_UUID", "url": "{{ frontendOriginExample }}/posts/POST_UUID", "caption": "hello" },
  "dm": { "thread_id": "THREAD_UUID", "to_acct": "bob@example.com", "from_acct": "alice@example.com" }
}</pre>
            </div>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in eventItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <p class="mt-6 text-sm font-semibold text-neutral-900">{{ $t("federationGuidelines.eventSchemaTitle") }}</p>
            <p class="mt-2 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.eventSchemaDescription") }}
            </p>
            <div class="mt-4 overflow-x-auto rounded-2xl border border-neutral-200">
              <table class="min-w-full divide-y divide-neutral-200 text-left text-xs text-neutral-700">
                <thead class="bg-neutral-50 text-neutral-900">
                  <tr>
                    <th class="px-3 py-2 font-semibold">{{ eventSchemaHeaders.kind }}</th>
                    <th class="px-3 py-2 font-semibold">{{ eventSchemaHeaders.required }}</th>
                    <th class="px-3 py-2 font-semibold">{{ eventSchemaHeaders.optional }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-neutral-200 bg-white">
                  <tr v-for="row in eventSchemaRows" :key="row.kind" class="align-top">
                    <td class="px-3 py-2 font-mono text-[11px] text-neutral-900">{{ row.kind }}</td>
                    <td class="px-3 py-2 leading-6" v-html="safeHtml(row.required)" />
                    <td class="px-3 py-2 leading-6" v-html="safeHtml(row.optional)" />
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.followTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.followDescription") }}
            </p>
            <div class="mt-4 overflow-x-auto rounded-2xl bg-neutral-900 p-4 text-xs text-lime-100">
              <pre>{
  "follower_acct": "bob@remote.example",
  "target_acct": "alice@example.com",
  "inbox_url": "https://remote.example/federation/events"
}</pre>
            </div>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in followItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.errorCodeTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.errorCodeDescription") }}
            </p>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in errorCodeItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.rateLimitTitle") }}</h2>
            <p class="mt-3 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.rateLimitDescription") }}
            </p>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in rateLimitItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("federationGuidelines.operationsTitle") }}</h2>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in opsItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <p class="mt-4 text-sm leading-7 text-neutral-700">
              {{ $t("federationGuidelines.opsNote") }}
            </p>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-2xl font-semibold text-neutral-900">{{ $t("federationGuidelines.cautionTitle") }}</h2>
            <ul class="mt-5 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in cautionItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-amber-500" />
                <span v-html="safeHtml(item)" />
              </li>
            </ul>
            <p v-for="paragraph in cautionParagraphs" :key="paragraph" class="mt-4 text-sm leading-7 text-neutral-700">
              {{ paragraph }}
            </p>
          </section>
        </div>
      </section>
    </div>
  </div>
</template>
