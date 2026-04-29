# Changelog

All notable Glipz application changes should be recorded here.

Glipz uses `web/package.json` as the source of truth for the application
version. Federation protocol compatibility is versioned separately with
`glipz-federation/{major}` and `event_schema_version`.

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
