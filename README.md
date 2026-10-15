# spools

Making agent threads/sessions accessible across all machines.

```bash
spools zed list
spools zed push <your_machine> <thread-id>
spools zed pull <your_machine> <thread-id>
spools zed export <thread-id> > thread.json
spools zed import thread.json   # or `-` for stdin

spools claude list --project <name>
spools claude pull <your_machine> <session-id>
spools claude pull <your_machine> --project <name>   # every session for that project
```

Supported: `zed` (native agent threads) and `claude` (Claude Code sessions). Importing a claude session also adds it to Zed's sidebar if Zed is installed and not running.

`push`, `pull` and `import` take `--dry-run` and `--project <path>`.

## Setup

spools needs nothing beyond working ssh between your machines.

1. Make sure you can ssh in without a password. If you don't have a key yet, `ssh-keygen -t ed25519`, then `ssh-copy-id <user>@<host>`.
2. Optionally save the user in `~/.ssh/config` so you can type `<host>` instead of `<user>@<host>`:

   ```
   Host <host>
       User <user>
   ```

3. Install spools on both machines, somewhere on the non-interactive `PATH`:

   ```bash
   go install github.com/cachebag/spools/cmd/spools@latest
   sudo install -m 755 ~/go/bin/spools /usr/local/bin/spools
   ```

4. Check it: `ssh <host> 'command -v spools'` should print `/usr/local/bin/spools`.

The other machine needs an ssh server running. Linux usually has one; on macOS turn on Remote Login in System Settings.

## Moving threads between machines

`push`/`pull` shell out to your own `ssh`, so keys, agent, `~/.ssh/config` and tailscale all work as usual.

- **Quit Zed on the receiving machine first.** spools refuses to import while it's running.
- **`spools` must be on the non-interactive `PATH` on both machines.** `ssh host cmd` doesn't read `.zshrc`, which is why the setup above uses `/usr/local/bin`.
- Pulling a whole project keeps going if one session fails and reports the failures at the end.
- The thread gets attached to a local checkout of the same project: `--project` if given, else the original path if it exists here, else the most recently opened project in the tool (e.g. Zed's recent workspaces) with the same git remote. If none match, open the project in the tool once and retry.
- Importing a thread id that already exists replaces it.

## License

[MIT](LICENSE)
