import { createRouter, createWebHistory } from "vue-router";
import { getAccessToken } from "../auth";
import { applyDocumentTitle, translate } from "../i18n";
import { api } from "../lib/api";
import { isNativeApp } from "../lib/runtime";
import { applySeoMeta } from "../lib/seo";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: () => (getAccessToken() ? "/feed" : isNativeApp() ? "/login" : "/about") },
    {
      path: "/login",
      component: () => import("../views/LoginView.vue"),
      meta: { titleKey: "routes.login", noindex: true },
    },
    {
      path: "/register",
      component: () => import("../views/RegisterView.vue"),
      meta: { titleKey: "routes.register", noindex: true },
    },
    {
      path: "/register/verify",
      component: () => import("../views/RegisterVerifyView.vue"),
      meta: { titleKey: "routes.registerVerify", noindex: true },
    },
    {
      path: "/mfa",
      component: () => import("../views/MfaView.vue"),
      meta: { titleKey: "routes.mfa", noindex: true },
    },
    {
      path: "/remote/profile",
      component: () => import("../views/RemoteProfileView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.remoteProfile" },
    },
    {
      path: "/@:fedUser@:fedHost",
      component: () => import("../views/RemoteProfileView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.remoteProfile" },
    },
    {
      path: "/@:handle/followers",
      component: () => import("../views/UserFollowListView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.userProfile" },
    },
    {
      path: "/@:handle/following",
      component: () => import("../views/UserFollowListView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.userProfile" },
    },
    {
      path: "/@:handle",
      component: () => import("../views/UserProfileView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.userProfile" },
    },
    {
      path: "/notes",
      redirect: "/feed",
    },
    {
      path: "/posts/federated/:incomingId",
      component: () => import("../views/PostDetailView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.postDetail" },
    },
    {
      path: "/posts/:postId",
      component: () => import("../views/PostDetailView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.postDetail" },
    },
    {
      path: "/feed",
      component: () => import("../views/FeedView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.feed" },
    },
    {
      path: "/communities",
      component: () => import("../views/CommunityListView.vue"),
      meta: { requiresAuth: false, mobileEdgeToEdge: true, titleKey: "routes.communities" },
    },
    {
      path: "/communities/new",
      component: () => import("../views/CommunityCreateView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.communityCreate" },
    },
    {
      path: "/communities/:id",
      component: () => import("../views/CommunityDetailView.vue"),
      meta: { requiresAuth: false, containedMainScroll: true, mobileEdgeToEdge: true, titleKey: "routes.communityDetail" },
    },
    {
      path: "/compose",
      component: () => import("../views/PostComposeView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.compose" },
    },
    {
      path: "/feed/scheduled",
      component: () => import("../views/ScheduledPostsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.scheduledPosts" },
    },
    {
      path: "/search",
      component: () => import("../views/SearchTopicsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.search" },
    },
    {
      path: "/notifications",
      component: () => import("../views/NotificationsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.notifications" },
    },
    {
      path: "/messages",
      component: () => import("../views/MessagesView.vue"),
      meta: { requiresAuth: true, hideRightAside: true, wideMain: true, mobileEdgeToEdge: true, titleKey: "routes.messages" },
    },
    {
      path: "/messages/:threadId",
      component: () => import("../views/MessagesView.vue"),
      meta: { requiresAuth: true, hideRightAside: true, wideMain: true, mobileEdgeToEdge: true, hideMobileChrome: true, titleKey: "routes.messages" },
    },
    {
      path: "/bookmarks",
      component: () => import("../views/BookmarksView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.bookmarks" },
    },
    {
      path: "/settings",
      component: () => import("../views/SettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.settings" },
    },
    {
      path: "/settings/custom-emojis",
      component: () => import("../views/CustomEmojiSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.customEmojiSettings" },
    },
    {
      path: "/settings/timeline",
      component: () => import("../views/TimelineSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.timelineSettings" },
    },
    {
      path: "/settings/mfa",
      component: () => import("../views/MfaSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.mfaSettings" },
    },
    {
      path: "/settings/identity-portability",
      component: () => import("../views/IdentityPortabilitySettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.identityPortabilitySettings" },
    },
    {
      path: "/settings/notifications",
      component: () => import("../views/NotificationSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.notificationSettings" },
    },
    {
      path: "/settings/account-deletion",
      component: () => import("../views/AccountDeletionSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.accountDeletionSettings" },
    },
    {
      path: "/settings/appearance",
      component: () => import("../views/AppearanceSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.appearanceSettings" },
    },
    {
      path: "/settings/direct-messages",
      component: () => import("../views/DirectMessageSettingsView.vue"),
      meta: { requiresAuth: true, mobileEdgeToEdge: true, titleKey: "routes.directMessageSettings" },
    },
    {
      path: "/security",
      redirect: (to) => ({ path: "/settings", query: to.query }),
    },
    {
      path: "/developer/api",
      component: () => import("../views/ApiDeveloperView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.apiDeveloper" },
    },
    {
      path: "/developer/oauth/authorize",
      component: () => import("../views/OAuthAuthorizeView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.oauthAuthorize" },
    },
    {
      path: "/admin",
      component: () => import("../components/admin/AdminLayout.vue"),
      meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.admin" },
      children: [
        {
          path: "",
          component: () => import("../views/AdminDashboardView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.admin" },
        },
        {
          path: "users",
          component: () => import("../views/AdminUsersView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.adminUsers" },
        },
        {
          path: "reports",
          component: () => import("../views/AdminReportsView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.adminReports" },
        },
        {
          path: "federation",
          component: () => import("../views/FederationAdminView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.federationAdmin" },
        },
        {
          path: "legal-requests",
          component: () => import("../views/AdminLegalRequestsView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.adminLegalRequests" },
        },
        {
          path: "custom-emojis",
          component: () => import("../views/AdminCustomEmojiView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.adminCustomEmojis" },
        },
        {
          path: "instance-settings",
          component: () => import("../views/AdminInstanceSettingsView.vue"),
          meta: { requiresAuth: true, requiresAdmin: true, adminShell: true, titleKey: "routes.adminInstanceSettings" },
        },
      ],
    },
    {
      path: "/legal/terms",
      component: () => import("../views/LegalTermsView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.legalTerms" },
    },
    {
      path: "/legal/privacy",
      component: () => import("../views/LegalPrivacyView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.legalPrivacy" },
    },
    {
      path: "/legal/nsfw-guidelines",
      component: () => import("../views/NSFWGuidelinesView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.nsfwGuidelines" },
    },
    {
      path: "/legal/law-enforcement",
      component: () => import("../views/LegalLawEnforcementView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.lawEnforcement" },
    },
    {
      path: "/legal/api-guidelines",
      component: () => import("../views/ApiOpenApiReferenceView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.apiReference" },
    },
    ...(!isNativeApp()
      ? [
        {
          path: "/about",
          component: () => import("../views/AboutView.vue"),
          meta: {
            requiresAuth: false,
            wideMain: true,
            titleKey: "routes.about",
            descriptionKey: "seo.about.description",
            canonicalPath: "/about",
          },
        },
      ]
      : []),
    {
      path: "/federation/guidelines",
      component: () => import("../views/FederationGuidelinesView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.federationGuidelines" },
    },
  ],
});

router.beforeEach(async (to) => {
  const token = getAccessToken();
  if (to.meta.requiresAuth && !token) {
    return { path: "/login", query: { next: to.fullPath } };
  }
  if (to.meta.requiresAdmin) {
    if (!token) {
      return { path: "/login", query: { next: to.fullPath } };
    }
    try {
      const me = await api<{ is_site_admin?: boolean }>("/api/v1/me", { method: "GET", token });
      if (!me.is_site_admin) return { path: "/feed" };
    } catch {
      return { path: "/feed" };
    }
  }
  return true;
});

router.afterEach((to) => {
  const titleKey = typeof to.meta.titleKey === "string" ? to.meta.titleKey : undefined;
  const descriptionKey = typeof to.meta.descriptionKey === "string" ? to.meta.descriptionKey : "seo.default.description";
  applyDocumentTitle(titleKey);
  applySeoMeta({
    title: typeof document === "undefined" ? undefined : document.title,
    description: translate(descriptionKey),
    canonicalPath: typeof to.meta.canonicalPath === "string" ? to.meta.canonicalPath : to.path,
    noindex: typeof to.meta.noindex === "boolean" ? to.meta.noindex : to.path !== "/about",
  });
});
