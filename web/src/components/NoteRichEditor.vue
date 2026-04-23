<script setup lang="ts">
import Image from "@tiptap/extension-image";
import Placeholder from "@tiptap/extension-placeholder";
import StarterKit from "@tiptap/starter-kit";
import { EditorContent, useEditor } from "@tiptap/vue-3";
import { marked } from "marked";
import { onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import TurndownService from "turndown";
import { uploadNoteMedia } from "../lib/noteMediaUpload";

const props = defineProps<{
  markdown: string;
  uploadToken: string;
}>();

const emit = defineEmits<{
  "update:markdown": [value: string];
}>();
const { t } = useI18n();

function mdToHtml(md: string): string {
  const x = marked.parse(md || "", { async: false });
  return typeof x === "string" ? x : String(x);
}

const td = new TurndownService({ headingStyle: "atx" });
td.addRule("video", {
  filter(node: Node) {
    return node.nodeName === "VIDEO";
  },
  replacement(_content: string, node: Node) {
    const el = node as HTMLElement;
    const src = el.getAttribute("src");
    if (!src) return "";
    return `\n\n<video src="${src}" controls playsinline></video>\n\n`;
  },
});

let debounceTimer: ReturnType<typeof setTimeout> | null = null;

function scheduleEmitFromHtml(html: string) {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    debounceTimer = null;
    emit("update:markdown", td.turndown(html));
  }, 300);
}

const editor = useEditor({
  content: mdToHtml(props.markdown),
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      link: { openOnClick: false, autolink: true },
    }),
    Image.configure({ inline: true, allowBase64: false }),
    Placeholder.configure({ placeholder: t("components.noteEditor.placeholder") }),
  ],
  editorProps: {
    attributes: {
      class:
        "note-rich-editor min-h-[280px] max-w-none px-3 py-2 text-[15px] text-neutral-900 outline-none focus:outline-none prose prose-neutral prose-headings:font-bold prose-p:my-2 prose-ul:my-2 prose-ol:my-2 dark:text-neutral-100 dark:prose-invert",
    },
  },
  onUpdate({ editor: ed }) {
    scheduleEmitFromHtml(ed.getHTML());
  },
});

const fileImageRef = ref<HTMLInputElement | null>(null);
const fileVideoRef = ref<HTMLInputElement | null>(null);

async function onPickedImage(ev: Event) {
  const inp = ev.target as HTMLInputElement;
  const f = inp.files?.[0];
  inp.value = "";
  if (!f) return;
  try {
    const url = await uploadNoteMedia(f, props.uploadToken);
    editor.value?.chain().focus().setImage({ src: url }).run();
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("errors.uploadFailed"));
  }
}

async function onPickedVideo(ev: Event) {
  const inp = ev.target as HTMLInputElement;
  const f = inp.files?.[0];
  inp.value = "";
  if (!f) return;
  try {
    const url = await uploadNoteMedia(f, props.uploadToken);
    editor.value
      ?.chain()
      .focus()
      .insertContent(`<video src="${url}" controls playsinline class="max-w-full rounded-lg"></video><p></p>`)
      .run();
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("errors.uploadFailed"));
  }
}

watch(
  () => props.markdown,
  (md) => {
    const ed = editor.value;
    if (!ed) return;
    const htmlStr = mdToHtml(md);
    const roundTrip = td.turndown(ed.getHTML()).trim();
    if (roundTrip === (md || "").trim()) return;
    ed.commands.setContent(htmlStr, { emitUpdate: false });
  },
);

onBeforeUnmount(() => {
  if (debounceTimer) clearTimeout(debounceTimer);
  editor.value?.destroy();
});
</script>

<template>
  <div class="rounded-xl border border-neutral-200 bg-white dark:border-neutral-200 dark:bg-neutral-950">
    <div class="flex flex-wrap gap-1 border-b border-neutral-200 px-2 py-1.5 dark:border-neutral-200">
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-100 dark:text-neutral-200 dark:hover:bg-neutral-800"
        :title="$t('components.noteEditor.bold')"
        @click="editor?.chain().focus().toggleBold().run()"
      >
        <strong>B</strong>
      </button>
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-100 dark:text-neutral-200 dark:hover:bg-neutral-800"
        :title="$t('components.noteEditor.italic')"
        @click="editor?.chain().focus().toggleItalic().run()"
      >
        <em>I</em>
      </button>
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-100 dark:text-neutral-200 dark:hover:bg-neutral-800"
        @click="editor?.chain().focus().toggleHeading({ level: 2 }).run()"
      >
        H2
      </button>
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-100 dark:text-neutral-200 dark:hover:bg-neutral-800"
        @click="editor?.chain().focus().toggleBulletList().run()"
      >
        {{ $t("components.noteEditor.list") }}
      </button>
      <span class="mx-1 text-neutral-300 dark:text-neutral-600">|</span>
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs font-medium text-lime-800 hover:bg-lime-50 dark:text-lime-400 dark:hover:bg-lime-950/40"
        @click="fileImageRef?.click()"
      >
        {{ $t("components.noteEditor.image") }}
      </button>
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs font-medium text-lime-800 hover:bg-lime-50 dark:text-lime-400 dark:hover:bg-lime-950/40"
        @click="fileVideoRef?.click()"
      >
        {{ $t("components.noteEditor.video") }}
      </button>
      <input ref="fileImageRef" type="file" accept="image/*" class="hidden" @change="onPickedImage" />
      <input ref="fileVideoRef" type="file" accept="video/*" class="hidden" @change="onPickedVideo" />
    </div>
    <EditorContent :editor="editor" class="note-rich-wrap" />
  </div>
</template>

<style scoped>
.note-rich-wrap :deep(.ProseMirror) {
  min-height: 240px;
}
.note-rich-wrap :deep(.ProseMirror img),
.note-rich-wrap :deep(.ProseMirror video) {
  max-width: 100%;
  border-radius: 0.5rem;
}
</style>
