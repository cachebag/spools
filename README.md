# spools

Making agent threads/sessions accessible across all machines.

```bash
spools zed list
spools zed push <your_machine> <thread-id>
spools zed pull <your_machine> <thread-id>
spools zed export <thread-id> > thread.json
spools zed import thread.json   # or `-` for stdin
```

`push`, `pull` and `import` take `--dry-run` and `--project <path>`.

## Moving threads between machines

`push`/`pull` shell out to your own `ssh`, so keys, agent, `~/.ssh/config` and tailscale all work as usual.

- **Quit Zed on the receiving machine first.** spools refuses to import while it's running.
- **`spools` must be on the non-interactive `PATH` on both machines.** `ssh host cmd` doesn't read `.zshrc`, so install it somewhere like `/usr/local/bin`. Check with `ssh <host> 'command -v spools'`.
- The thread gets attached to a local checkout of the same project: `--project` if given, else the original path if it exists here, else the most recently opened project in the tool (e.g. Zed's recent workspaces) with the same git remote. If none match, open the project in the tool once and retry.
- Importing a thread id that already exists replaces it.

## License

[MIT](LICENSE)
