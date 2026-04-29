package tui

const helpText = `Keys

  Enter        copy the inflated prompt to clipboard
  Tab          inflate now (skip the 600 ms idle wait)
  Esc          dismiss errors, then clear input + preview
  ?            toggle this help
  p            (in this overlay) switch between cloud and local Ollama
  Ctrl-C       quit

What inflate does

  Type a fragment in the box below. After 600 ms inflate
  expands it into a context-loaded prompt using your
  project state and recent Claude Code session. Press
  Enter to copy, then paste into Claude Code.

Status line

  Green   git is present, full project context available
  Yellow  git missing but at least 2 other sources OK
  Red     only profile, or nothing usable

  "Missing: …" shows which context blocks weren't found.
`
