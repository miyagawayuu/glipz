# Changelog

All notable Glipz application changes should be recorded here.

Glipz uses `web/package.json` as the source of truth for the application
version. Federation protocol compatibility is versioned separately with
`glipz-federation/{major}` and `event_schema_version`.

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

### Release Notes

- Record Docker image tags as immutable release tags such as `glipz:v0.0.1`.
- Note any required DB migrations in this section before publishing a release.
