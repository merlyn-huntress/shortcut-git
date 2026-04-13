# shortcut-git

A CLI tool that mirrors a [Shortcut](https://shortcut.com) objective as local markdown files inside a git repository. Edit stories and epics using Claude Code or in your favorite editor, track changes with familiar git commands, and sync back to Shortcut when ready.

## How it works

`shortcut-git` clones a Shortcut objective (with all its epics and stories) into a local directory. Each entity becomes a markdown file with YAML frontmatter. Git handles all local state management (diffing, staging, committing, history). When you're ready, push your changes back to Shortcut.

Shortcut is treated as the remote "origin". There is no branching -- just a single line of history synced with Shortcut's state.

## Prerequisites

- **Go 1.21+** (to build from source)
- **git** (installed and on PATH)
- **Shortcut API token** provided via one of the following (checked in order):
  1. **Environment variable:** `export SHORTCUT_API_TOKEN=your-token-here`
  2. **`~/.claude.json`** at the path `mcpServers.shortcut.env.SHORTCUT_API_TOKEN`.

## Installation

```bash
git clone https://github.com/merlyn-huntress/shortcut-git.git
cd shortcut-git
go build -o shortcut-git .
# Move to PATH, e.g. one of:
# mv shortcut-git /usr/local/bin/
# mv shortcut-git ~/.local/bin
```

## Quick start

```bash
# Clone an objective
shortcut-git clone https://app.shortcut.com/your-workspace/objective/12345

# Browse the files
cd your-objective-name/
ls
# _objective.md
# some-epic/
#   _epic.md
#   001-first-story.md
#   002-second-story.md

# Edit a story
vim some-epic/001-first-story.md

# See what changed
shortcut-git diff

# Stage changes
shortcut-git add some-epic/001-first-story.md

# Commit locally
shortcut-git commit -m "Updated story description"

# Preview what would be pushed
shortcut-git push --dry-run

# Push to Shortcut
shortcut-git push
```

## File structure

After cloning, your directory looks like this:

```
your-objective-slug/
  .shortcut-git/           # Internal metadata (hidden from git)
    repo/                  # Git repository data
    config.json            # Objective ID, workspace, cached lookups
    remote/                # Last-fetched state for each entity
  _objective.md            # The objective (always at root)
  first-epic/              # One directory per epic (slugified title)
    _epic.md               # The epic's metadata and description
    001-story-title.md     # Stories, ordered by Shortcut's position
    002-another-story.md
  second-epic/
    _epic.md
    001-a-story.md
```

Files prefixed with `_` (`_objective.md`, `_epic.md`) sort first in directory listings.

Story files have a three-digit position prefix (`001-`, `002-`, ...) that preserves Shortcut's "User Defined" sort order.

## Markdown file format

Each file uses YAML frontmatter followed by a markdown body:

```markdown
---
title: "Story title goes here"
informational_shortcut_id: "197608"
informational_shortcut_url: "https://app.shortcut.com/workspace/story/197608"
informational_owner: "Jane Smith"
informational_status: "In Progress"
informational_type: feature
informational_last_updated_at: "2026-04-10T14:30:00Z"
informational_github_pr_urls:
  - "https://github.com/org/repo/pull/123"
---

The story description goes here. Full markdown is supported.
```

### Editable fields

You can modify these fields and push them to Shortcut:

- **`title`** -- The entity's name. Changing the title also renames the file on the next commit.
- **Body text** -- Everything below the frontmatter `---` is the entity's description.

### Informational fields (read-only)

Fields prefixed with `informational_` are managed by shortcut-git. They are updated on fetch/pull and reflect the current state in Shortcut. Local edits to these fields are silently overwritten on the next pull.

| Field | Description |
|-------|-------------|
| `informational_shortcut_id` | Shortcut's numeric entity ID |
| `informational_shortcut_url` | Direct link to the entity in Shortcut's web UI |
| `informational_owner` | Assigned owner(s), resolved to display names |
| `informational_status` | Workflow state (e.g., "In Progress", "Done") |
| `informational_type` | Story type: `feature`, `bug`, or `chore` |
| `informational_archived` | Present (set to `true`) only when the entity is archived |
| `informational_last_updated_at` | UTC timestamp of the last remote update |
| `informational_github_pr_urls` | List of linked GitHub pull request URLs |

## Commands

### `shortcut-git clone <objective-url>`

Clone a Shortcut objective and all its epics and stories into a new local directory.

```bash
shortcut-git clone https://app.shortcut.com/your-workspace/objective/12345
```

- Creates a directory named from the slugified objective title
- Fetches all epics linked to the objective and all stories within those epics
- Initializes a hidden git repository at `.shortcut-git/repo/`
- Makes an initial commit with all files
- Detects and stores the team ID for creating new entities later

The objective URL can include or omit the SEO slug at the end. Both `/objective/176790` and `/objective/176790/some-slug` work.

### `shortcut-git fetch`

Download the latest state from Shortcut without modifying your local files.

```bash
shortcut-git fetch
# Fetched: 3 updated, 1 added, 0 removed
```

This updates the internal `.shortcut-git/remote/` state files. Your working tree is not touched. Use this to check what changed before pulling.

### `shortcut-git pull`

Fetch the latest state from Shortcut and apply changes to your local files.

```bash
shortcut-git pull
# Pulled: 1 info-only updates, 2 editable updates, 1 added, 0 removed
```

Pull requires a clean working tree (no unstaged or staged changes). If your tree is dirty, commit or reset first.

What pull does:
- Updates informational fields (status, owner, etc.) and auto-commits them separately as "shortcut-git: sync informational fields"
- Updates editable fields (title, description) and auto-commits as "shortcut-git: pull remote changes"
- Creates files for new stories/epics added in Shortcut
- Removes files for stories/epics deleted in Shortcut
- Moves story files between epic directories if a story was reassigned to a different epic
- Renames files/directories when titles change

### `shortcut-git push [--dry-run] [--force]`

Send your committed local changes to Shortcut.

```bash
# Preview what would be pushed
shortcut-git push --dry-run
# story 197608: title: "Old Title" -> "New Title"
# story 197608: description changed (3 lines -> 5 lines)
# [NEW] story: "My new story"

# Actually push
shortcut-git push
```

Push requires a clean working tree (all changes must be committed).

**What push sends to Shortcut:**
- Title changes (the `title` frontmatter field maps to Shortcut's `name` field)
- Description changes (the markdown body)
- Epic reassignment (if you moved a story file to a different epic directory)
- New entities (files without an `informational_shortcut_id` are created in Shortcut)

**Divergence detection:** If someone else modified an entity on Shortcut since your last fetch, push refuses with an error. Use `--force` to overwrite, or `pull` first to merge.

**New entity creation:** To create a new story, just create a markdown file in an epic directory:

```bash
cat > some-epic/my-new-story.md << 'EOF'
---
title: "My new story"
---

Description of the story.
EOF

shortcut-git add some-epic/my-new-story.md
shortcut-git commit -m "Add new story"
shortcut-git push
```

After push, `informational_shortcut_id` and `informational_shortcut_url` are written back into the file. New stories are assigned to the same team as existing stories in the objective.

You can also create new epics by creating a directory with an `_epic.md` file:

```bash
mkdir new-epic
cat > new-epic/_epic.md << 'EOF'
---
title: "New epic"
---

Epic description.
EOF

# Add stories to the epic too, then push
```

### `shortcut-git status`

Show the working tree status, prefixed with time since last fetch.

```bash
shortcut-git status
# Last fetch: 5m ago
#
# On branch main
# Changes not staged for commit:
#   modified:   some-epic/001-my-story.md
```

### `shortcut-git add <path>`

Stage changes for the next commit.

```bash
shortcut-git add some-epic/001-my-story.md
shortcut-git add .   # stage everything
```

### `shortcut-git commit -m <message>`

Record staged changes as a local commit. On commit, files are automatically renamed to match their `title` frontmatter if they've diverged.

```bash
shortcut-git commit -m "Update story descriptions for sprint 12"
```

### `shortcut-git diff [--staged]`

Show changes between your working tree and the last commit.

```bash
shortcut-git diff              # unstaged changes
shortcut-git diff --staged     # staged changes
```

### `shortcut-git log [--oneline]`

Show the local commit history.

```bash
shortcut-git log --oneline
# a1b2c3d shortcut-git: pull remote changes
# d4e5f6g Update story descriptions
# h7i8j9k shortcut-git: initial clone of objective 176790
```

### `shortcut-git reset [--soft | --hard]`

Reset the current state.

```bash
shortcut-git reset              # unstage all staged changes
shortcut-git reset --soft       # undo last commit, keep changes staged
shortcut-git reset --hard       # discard all uncommitted changes
```

### `shortcut-git checkout -- <path>`

Discard unstaged changes to specific files.

```bash
shortcut-git checkout -- some-epic/001-my-story.md
```

The `--` separator is required (to prevent accidentally switching git branches in the internal repo).

### `shortcut-git version`

Print the version.

```bash
shortcut-git version
# shortcut-git v0.1.0
```

## Filename conventions

- **Filenames are derived from the `title` frontmatter field.** The title is slugified (lowercased, non-alphanumeric characters replaced with hyphens, truncated to 80 characters).
- **Title is authoritative.** If you change the title in frontmatter, the file is automatically renamed on the next commit. If you rename the file without changing the title, the file is renamed back on the next commit.
- **Story files have position prefixes** (`001-`, `002-`, ...) that preserve Shortcut's ordering. The prefix is preserved across renames.
- **New files you create locally** don't need a position prefix. They get one assigned when pulled after a round-trip through Shortcut.

## Workflow examples

### Edit a story description offline

```bash
shortcut-git clone https://app.shortcut.com/myteam/objective/12345
cd my-objective/
vim some-epic/001-my-story.md   # edit the body text
shortcut-git add .
shortcut-git commit -m "Rewrite story acceptance criteria"
shortcut-git push
```

### Batch-create stories

```bash
cd my-objective/some-epic/
for i in 1 2 3; do
  cat > new-story-$i.md << EOF
---
title: "Sprint 14 task $i"
---

TODO: fill in details
EOF
done
shortcut-git add .
shortcut-git commit -m "Add sprint 14 stories"
shortcut-git push
```

### Sync before editing

```bash
shortcut-git pull         # get latest from Shortcut
vim some-epic/story.md    # make your edits
shortcut-git add .
shortcut-git commit -m "Updates"
shortcut-git push
```

### Review what changed on Shortcut

```bash
shortcut-git fetch        # download changes without applying
shortcut-git pull         # apply them to your files
shortcut-git log -1 -p    # see exactly what changed
```

## Scope and limitations

- **Objective-rooted:** You always clone an objective. All epics under that objective and all stories within those epics are included.
- **No branching:** There is one line of history. Shortcut is always "main".
- **Editable fields:** Only `title` and description body can be pushed. Status, owner, and other fields are informational (read-only).
- **No deletion via push:** Deleting a local file does not delete the entity on Shortcut. The file reappears on the next pull. (A warning is shown on push.)
- **Rate limiting:** The Shortcut API allows 200 requests per minute. Large objectives with many stories may take longer to clone/fetch. The tool retries automatically on rate limit errors.
- **Team assignment:** New stories and epics inherit the team detected from existing entities during clone.

## Building from source

```bash
go build -o shortcut-git .
go test ./...
```

## License

MIT
