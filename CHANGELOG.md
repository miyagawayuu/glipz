# Changelog

All notable Glipz application changes should be recorded here.

Glipz uses `web/package.json` as the source of truth for the application
version. Federation protocol compatibility is versioned separately with
`glipz-federation/{major}` and `event_schema_version`.

## [0.0.3] - Unreleased

### Added

- Appearance settings now include a custom theme builder with saved color
  palettes and live preview.
- Timeline settings now allow users to choose, reorder, hide, and create home
  timeline tabs with custom filters.
- Timeline settings can be exported/imported with share codes and are saved to
  the user's server-side account settings.
- Custom timeline tabs can fetch filtered feeds using a dedicated authenticated
  feed endpoint.
- Custom timeline tabs can use a recommended sort with tunable ranking weights
  for recency, popularity, author affinity, federated posts, and diversity.
- Law enforcement readiness tooling for admins, including legal request intake,
  status tracking, preservation hold creation, exportable disclosure packages,
  and a public law enforcement request policy page.
- DM reporting can include participant-submitted plaintext by explicit consent
  while preserving client-side encryption for protected message content.
- Tamper-resistant admin audit events record sensitive legal-compliance actions
  with hash chaining.
- Report workflows now classify local, federated, and DM reports with categories
  including legal and safety priority flags.
- Instance administrators can configure the minimum account creation age from
  instance settings.
- Right-sidebar widget plugins can be registered from the frontend and managed
  from a plugin manager page.
- Built-in sample sidebar plugins now include a compact calendar and a
  "Today in History" date-fact widget.
- A language settings page lets users switch between the supported app locales.
- Identity portability now supports a migration-file workflow that bundles the
  encrypted identity export, transfer session, transfer token, expiration, and
  import options into a downloadable/uploadable JSON file.

### Changed

- The Glipz logo and app icon assets were refreshed, with light/dark logo
  variants used across the app shell and auth pages.
- Admin report, federation, custom emoji, and legal request pages now follow the
  dashboard layout for page width, headings, cards, forms, and primary actions.
- Account deletion now respects active legal preservation holds.
- Legal disclosure exports now honor requested data types and include a manifest
  with section counts, SHA-256 hashes, and audit-event hash references.
- Post deletion now respects active legal preservation holds.
- Login, registration, MFA, and messages now use simpler route-level chrome
  controls for their dedicated layouts.
- Sidebar plugins are disabled by default until a user enables them, and users
  can choose plugin order plus collapse individual sidebar widgets.
- Supported UI locales are now managed from the language settings page; the
  incomplete German locale override was removed from the supported locale list.

### Fixed

- Escaped literal `@` characters in new timeline-setting translations to avoid
  vue-i18n message parse errors.
- Frontend production builds split large vendor and locale bundles to avoid Vite
  chunk-size warnings.

### Database Migrations

- Startup migrations add `user_timeline_settings` for server-saved per-user
  timeline customization.
- Startup migrations add law enforcement requests, legal preservation holds,
  admin audit events, and DM report storage.
- Startup migrations add report categories, access-event storage for legal
  exports, and `minimum_registration_age` site setting.

## [0.0.2] - Unreleased

### Added

- Optional SMTP mail delivery support for deployments that send registration and
  notification email outside the local development mail catcher.
- Community tags with create/edit support, tag search, tag chips on community
  cards/details, and a horizontally scrollable tag picker in the community
  directory.
- Markdown editing and sanitized preview/display for community details and rules.
- A console safety warning to discourage pasting untrusted code into DevTools.
- Right-sidebar operator announcements now display DB-backed announcements as a
  slider with navigation controls.

### Changed

- Refined community detail actions: edit is shown as a pencil button beside the
  member count, join status uses "Join/Member" style labels, and community
  deletion moved into the edit modal as a disband action.
- Community edit now opens in a modal instead of expanding inline.
- Removed visible community UUIDs from the community directory and detail header.
- Simplified right-sidebar policy links into compact inline links separated by
  `｜`.
- Enlarged the left-sidebar post button, removed its icon, and added the app
  version below it.
- Two-factor authentication now uses the same simple guest page shell as
  registration and login.

### Fixed

- Raised authenticated SSE connection limits so normal multi-tab use of feed,
  notification, and DM streams is less likely to hit `429 Too Many Requests`.

### Database Migrations

- Startup migrations add `communities.tags TEXT[]` for community tag storage.

## [0.0.1] - Unreleased

### Versioning

- Application version source: `web/package.json`.
- Federation protocol: `glipz-federation/3`.
- Supported federation protocol versions advertised in discovery:
  `glipz-federation/2`, `glipz-federation/3`.
- Event schema version: `3`.

### Database Migrations

- Initial schema: `infra/postgres/init.sql`.
- Existing one-time migration: `infra/postgres/migrate_posts_object_keys.sql`.
- Startup migrations now also cover ID portability transfer tables, bookmarks /
  follow portability support, community tables with `posts.group_id`, and profile
  pinned-post support for existing databases.

### Added

- Community directory, creation flow, owner-managed join requests, community
  timelines, and separate community posting via `community_id`.
- Community detail tabs for recommended posts, latest posts, media grid, and
  owner-editable details/rules.
- Community headers with editable icon/header images, member avatar previews, and
  compact member counts.
- Profile pinned posts and profile-style media tile support for communities.
- Reusable post composer form and sidebar compose modal for normal and community
  posting flows.

### Release Notes

- Record Docker image tags as immutable release tags such as `glipz:v0.0.1`.
- Note any required DB migrations in this section before publishing a release.
