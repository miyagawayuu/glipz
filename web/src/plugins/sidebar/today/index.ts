import TodaySidebarWidget from "./TodaySidebarWidget.vue";
import { registerSidebarWidgetPlugin } from "../../../lib/sidebarWidgetPlugins";

registerSidebarWidgetPlugin({
  id: "glipz.today",
  name: "Today in History",
  nameKey: "plugins.today.pluginName",
  description: "Shows a small fact for today's date in the right sidebar.",
  descriptionKey: "plugins.today.pluginDescription",
  widgets: [
    {
      id: "today-fact",
      title: "Today",
      titleKey: "plugins.today.widgetTitle",
      placement: "right-sidebar",
      order: 30,
      component: TodaySidebarWidget,
    },
  ],
});
