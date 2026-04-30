import CalendarSidebarWidget from "./CalendarSidebarWidget.vue";
import { registerSidebarWidgetPlugin } from "../../../lib/sidebarWidgetPlugins";

registerSidebarWidgetPlugin({
  id: "glipz.calendar",
  name: "Glipz Calendar",
  nameKey: "plugins.calendar.pluginName",
  description: "Shows a compact monthly calendar in the right sidebar.",
  descriptionKey: "plugins.calendar.pluginDescription",
  widgets: [
    {
      id: "monthly-calendar",
      title: "Calendar",
      titleKey: "plugins.calendar.widgetTitle",
      placement: "right-sidebar",
      order: 20,
      component: CalendarSidebarWidget,
    },
  ],
});
