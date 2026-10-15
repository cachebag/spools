# Changelog

## v0.1.0 - 2026-10-15

### Zed

- `list`, `export`, `import`, `push`, `pull`, `rm` for native agent threads.
- Thread payloads are rewritten so project and home paths point at the local machine.
- Import refuses while Zed is running.

### Claude Code

- `list`, `export`, `import`, `push`, `pull`, `rm` for sessions. The tool-results directory travels with the session.
- Imported sessions are added to Zed's sidebar.

### General

- `push` and `pull` go over your own `ssh`.
- The local project is found from the original path, then from the tool's recently opened projects by git remote. `--project <path>` overrides.
- `pull <host> --project <name>` pulls every session for a project.
- `list --project <name>` filters, `list --json` for scripts.
- `--dry-run` on `import`, `push` and `pull`.
- `--version`.
