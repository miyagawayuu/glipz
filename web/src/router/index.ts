import { createRouter, createWebHistory } from "vue-router";
import { getAccessToken } from "../auth";
import { applyDocumentTitle } from "../i18n";
import { isNativeApp } from "../lib/runtime";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: () => (getAccessToken() ? "/feed" : isNativeApp() ? "/login" : "/about") },
    {
      path: "/login",
      component: () => import("../views/LoginView.vue"),
      meta: { titleKey: "routes.login" },
    },
    {
      path: "/register",
      component: () => import("../views/RegisterView.vue"),
      meta: { titleKey: "routes.register" },
    },
    {
      path: "/register/verify",
      component: () => import("../views/RegisterVerifyView.vue"),
      meta: { titleKey: "routes.registerVerify" },
    },
    {
      path: "/mfa",
      component: () => import("../views/MfaView.vue"),
      meta: { titleKey: "routes.mfa" },
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
      name: "notes-list",
      component: () => import("../views/NotesListView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.notes" },
    },
    {
      path: "/notes/new",
      name: "note-new",
      component: () => import("../views/NoteEditView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.noteNew" },
    },
    {
      path: "/notes/:noteId/edit",
      name: "note-edit",
      component: () => import("../views/NoteEditView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.noteEdit" },
    },
    {
      path: "/notes/:noteId",
      name: "note-detail",
      component: () => import("../views/NoteDetailView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.noteDetail" },
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
      path: "/admin/federation",
      component: () => import("../views/FederationAdminView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.federationAdmin" },
    },
    {
      path: "/admin/reports",
      component: () => import("../views/AdminReportsView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.adminReports" },
    },
    {
      path: "/admin/user-badges",
      component: () => import("../views/AdminUserBadgesView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.adminUserBadges" },
    },
    {
      path: "/admin/custom-emojis",
      component: () => import("../views/AdminCustomEmojiView.vue"),
      meta: { requiresAuth: true, titleKey: "routes.adminCustomEmojis" },
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
      path: "/legal/api-guidelines",
      component: () => import("../views/ApiOpenApiReferenceView.vue"),
      meta: { requiresAuth: false, wideMain: true, guestSimpleLayout: true, titleKey: "routes.apiReference" },
    },
    ...(!isNativeApp()
      ? [
        {
          path: "/about",
          component: () => import("../views/AboutView.vue"),
          meta: { requiresAuth: false, wideMain: true, titleKey: "routes.about" },
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

router.beforeEach((to) => {
  if (to.meta.requiresAuth && !getAccessToken()) {
    return { path: "/login", query: { next: to.fullPath } };
  }
  return true;
});

router.afterEach((to) => {
  applyDocumentTitle(typeof to.meta.titleKey === "string" ? to.meta.titleKey : undefined);
});
