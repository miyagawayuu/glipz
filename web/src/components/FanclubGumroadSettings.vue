<template>
  <div class="mt-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="text-sm font-medium text-neutral-900">{{ $t("views.settings.fanclubGumroad.title") }}</p>
        <p class="mt-1 text-xs text-neutral-500">{{ $t("views.settings.fanclubGumroad.lead") }}</p>
      </div>
      <span class="shrink-0 rounded-full bg-lime-50 px-2.5 py-1 text-xs font-medium text-lime-800">
        {{ $t("views.settings.fanclubGumroad.available") }}
      </span>
    </div>
    <p class="mt-2 text-xs text-neutral-500">{{ $t("views.settings.fanclubGumroad.hint") }}</p>
  </div>
</template>
