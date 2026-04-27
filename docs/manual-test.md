# Inflate Manual Test Checklist

Run before each release.

1. **Fresh install, no config.** Delete `~/.config/inflate/`. Run `inflate`. Expected: intake wizard runs; after answers, exits with "no config.toml" hint.
2. **Brand-new dir, no git.** `mkdir /tmp/empty && cd /tmp/empty && inflate`. Expected: TUI shows `git✗ jsonl✗ shell~` and inflations stay skeletal.
3. **JSONL detection.** Open Terminal A, run `claude` in /tmp/empty, send any prompt. In Terminal B's `inflate`, status flips `jsonl✗` → `jsonl✓` within ~2s.
4. **Inflation E2E.** Type "fix the bug" in Inflate. Expected: 600ms after typing stops, preview shows a Role/Context/Task/Constraints/Output prompt referencing your repo state.
5. **Enter copies.** Press Enter. Toast: `copied ✓`. Switch to Terminal A and paste — inflated prompt is in Claude Code's input.
6. **Network down.** `sudo ip link set lo down` (or block API hostname). Type a fragment. Expected: status: `timeout` after 30s; raw seed still copies on Enter.
7. **Bad API key.** Edit config, set `api_key_env` to a missing var. Restart. Expected: fatal error naming the missing env var.
8. **Lockfile.** Start `inflate` twice. Second instance: refuses with "already running (PID N)". Then `kill <pid>` the first; the second can now start.
9. **Esc clears.** Type, then Esc. Preview + input both clear, no toast.
10. **Quit clean.** Ctrl-C. No goroutine leak warnings, no orphaned processes.
