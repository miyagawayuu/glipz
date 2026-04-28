<script setup lang="ts">
import { ref } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { uploadMediaFile } from "../lib/api";
import { createCommunity } from "../lib/communities";

const router = useRouter();
const name = ref("");
const description = ref("");
const iconFile = ref<File | null>(null);
const headerFile = ref<File | null>(null);
const busy = ref(false);
const err = ref("");

function selectIcon(e: Event) {
  const input = e.target as HTMLInputElement;
  iconFile.value = input.files?.[0] ?? null;
}

function selectHeader(e: Event) {
  const input = e.target as HTMLInputElement;
  headerFile.value = input.files?.[0] ?? null;
}

async function submit() {
  if (!name.value.trim()) {
    err.value = "required";
    return;
  }
  const token = getAccessToken();
  if (!token) {
    err.value = "login_required";
    return;
  }
  busy.value = true;
  err.value = "";
  try {
    const icon = iconFile.value ? await uploadMediaFile(token, iconFile.value) : null;
    const header = headerFile.value ? await uploadMediaFile(token, headerFile.value) : null;
    const community = await createCommunity({
      name: name.value,
      description: description.value,
      icon_object_key: icon?.object_key,
      header_object_key: header?.object_key,
    });
    await router.push(`/communities/${community.id}`);
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : "create_failed";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="border-b border-neutral-200 px-4 py-4">
    <RouterLink to="/communities" class="text-sm font-medium text-lime-800 hover:underline">
      {{ $t("views.communities.backToList") }}
    </RouterLink>
    <h1 class="mt-3 text-2xl font-bold text-neutral-900">{{ $t("views.communityCreate.title") }}</h1>
    <p class="mt-1 text-sm text-neutral-600">{{ $t("views.communityCreate.description") }}</p>
  </div>

  <form class="space-y-4 px-4 py-5" @submit.prevent="submit">
    <div v-if="err" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      {{ $t("views.communityCreate.failed") }}
    </div>
    <div>
      <label class="mb-1 block text-sm font-medium text-neutral-700" for="community-name">
        {{ $t("views.communityCreate.name") }}
      </label>
      <input
        id="community-name"
        v-model="name"
        maxlength="80"
        class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
      />
    </div>
    <div>
      <label class="mb-1 block text-sm font-medium text-neutral-700" for="community-description">
        {{ $t("views.communityCreate.body") }}
      </label>
      <textarea
        id="community-description"
        v-model="description"
        maxlength="500"
        rows="5"
        class="w-full resize-none rounded-xl border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
      />
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="mb-1 block text-sm font-medium text-neutral-700" for="community-icon">
          {{ $t("views.communityCreate.iconImage") }}
        </label>
        <input
          id="community-icon"
          type="file"
          accept="image/*"
          class="block w-full text-sm text-neutral-700 file:mr-3 file:rounded-full file:border-0 file:bg-neutral-100 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-neutral-800 hover:file:bg-neutral-200"
          @change="selectIcon"
        />
      </div>
      <div>
        <label class="mb-1 block text-sm font-medium text-neutral-700" for="community-header">
          {{ $t("views.communityCreate.headerImage") }}
        </label>
        <input
          id="community-header"
          type="file"
          accept="image/*"
          class="block w-full text-sm text-neutral-700 file:mr-3 file:rounded-full file:border-0 file:bg-neutral-100 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-neutral-800 hover:file:bg-neutral-200"
          @change="selectHeader"
        />
      </div>
    </div>
    <p class="text-xs text-neutral-500">{{ $t("views.communityCreate.urlIdHint") }}</p>
    <button
      type="submit"
      class="rounded-full bg-lime-500 px-5 py-2 text-sm font-semibold text-white hover:bg-lime-600 disabled:cursor-not-allowed disabled:opacity-50"
      :disabled="busy"
    >
      {{ busy ? $t("views.communityCreate.creating") : $t("views.communityCreate.submit") }}
    </button>
  </form>
</template>
