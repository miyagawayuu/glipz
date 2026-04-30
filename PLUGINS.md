# Sidebar Widget Plugins

Glipz supports frontend sidebar widget plugins. The current extension point is
the authenticated right sidebar; other placements can be added later by placing
additional plugin hosts in the app shell or feature views.

## User Behavior

- Plugins are disabled by default.
- Users enable or disable plugins from **Settings → Plugin manager**.
- Users can reorder plugins from the same page.
- Each rendered widget can be collapsed in the right sidebar.
- Plugin settings are stored client-side in `localStorage` under
  `glipz.sidebarWidgetSettings`.

## Built-in Plugins

- `glipz.calendar`: compact monthly calendar.
- `glipz.today`: "Today in History" date fact widget.

## Adding a Sidebar Plugin

Create a plugin directory under `web/src/plugins/sidebar/`:

```text
web/src/plugins/sidebar/example/
  ExampleSidebarWidget.vue
  index.ts
```

Register the plugin in `index.ts`:

```ts
import ExampleSidebarWidget from "./ExampleSidebarWidget.vue";
import { registerSidebarWidgetPlugin } from "../../../lib/sidebarWidgetPlugins";

registerSidebarWidgetPlugin({
  id: "glipz.example",
  name: "Example",
  nameKey: "plugins.example.pluginName",
  description: "Shows an example sidebar widget.",
  descriptionKey: "plugins.example.pluginDescription",
  widgets: [
    {
      id: "example-widget",
      title: "Example",
      titleKey: "plugins.example.widgetTitle",
      placement: "right-sidebar",
      order: 40,
      component: ExampleSidebarWidget,
    },
  ],
});
```

Then import the plugin directory from `web/src/plugins/sidebar/index.ts`:

```ts
import "./example";
```

## i18n Requirements

Plugin names, descriptions, and widget titles should use i18n keys so the plugin
manager and sidebar header follow the active locale. Add matching keys to every
supported locale file under `web/src/locales/`.

Use this shape:

```ts
plugins: {
  example: {
    pluginName: "Example",
    pluginDescription: "Shows an example sidebar widget.",
    widgetTitle: "Example",
  },
}
```

## Widget Guidelines

- Keep widgets compact; the sidebar is `350px` wide on desktop.
- Do not wrap widget content in its own outer card unless it intentionally needs
  nested visual grouping. `SidebarWidgetHost` already provides the card,
  collapsible header, border, and shadow.
- Avoid external network calls unless the backend CSP and privacy expectations
  are reviewed first.
- Prefer existing app utilities such as `vue-i18n`, `getLocaleTag()`, and local
  API helpers.
- Treat plugin state as user-facing. Preserve existing `localStorage` settings
  when changing plugin IDs or widget IDs.
