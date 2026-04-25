# Operator-editable legal documents

Copy these Markdown files into the directory configured by `LEGAL_DOCS_DIR`.

Supported filenames:

- `terms.md`
- `privacy.md`
- `nsfw-guidelines.md`

Locale-specific variants take precedence when the viewer uses that locale:

- `terms.ja.md`
- `terms.en.md`
- `privacy.ja.md`
- `privacy.en.md`
- `nsfw-guidelines.ja.md`
- `nsfw-guidelines.en.md`

With the default Docker Compose setup, place the files under `data/legal-docs/`
on the host and restart the backend. The frontend keeps using the built-in
policy text when a file is missing.
