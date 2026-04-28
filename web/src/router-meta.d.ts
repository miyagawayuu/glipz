import "vue-router";

declare module "vue-router" {
  interface RouteMeta {
    requiresAuth?: boolean;
    hideRightAside?: boolean;
    wideMain?: boolean;
    containedMainScroll?: boolean;
    mobileEdgeToEdge?: boolean;
    hideMobileChrome?: boolean;
    guestSimpleLayout?: boolean;
    adminShell?: boolean;
    requiresAdmin?: boolean;
    mobileOnly?: boolean;
    titleKey?: string;
    descriptionKey?: string;
    canonicalPath?: string;
    noindex?: boolean;
  }
}
