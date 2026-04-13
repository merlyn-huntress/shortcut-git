# CLAUDE.md

## Before finishing

When you think you are done, run the following:

1. Run tests: `GOCACHE="$(pwd)/tmp/gocache" go test ./...`
2. Build the binary: `GOCACHE="$(pwd)/tmp/gocache" go build -o shortcut-git .`

## Temporary files

Use the `tmp/` directory for all temporary files, including shortcut-git clones during testing. This directory is gitignored. Set `GOCACHE="$(pwd)/tmp/gocache"` when running Go commands (must be absolute path).

## Frontmatter rules

- All frontmatter fields prefixed with `informational_` are **read-only**. They are updated exclusively by fetch/pull and must never produce a local diff. Local edits to these fields are silently overwritten on next pull.
- The informational fields are: `informational_shortcut_id`, `informational_shortcut_url`, `informational_owner`, `informational_status`, `informational_last_updated_at`, `informational_github_pr_urls`.
- The Shortcut API uses `name` (not `title`) as the field name for entity titles.
- The `title` frontmatter field is authoritative for filenames. On commit/pull/push, files are auto-renamed to match the slugified title. Filename != title is always resolved in favor of the title.
