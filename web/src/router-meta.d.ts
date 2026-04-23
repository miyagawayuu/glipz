import "vue-router";

declare module "vue-router" {
  interface RouteMeta {
    requiresAuth?: boolean;
    hideRightAside?: boolean;
    wideMain?: boolean;
    mobileEdgeToEdge?: boolean;
    hideMobileChrome?: boolean;
    guestSimpleLayout?: boolean;
    mobileOnly?: boolean;
    titleKey?: string;
  }
}
