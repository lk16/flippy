# Next steps

Features and fixes we don't have yet. Mostly behavior from the pre-rewrite
implementation (`origin/main`) that was deliberately left out; none of it
is required — pick items up when a real need appears.

## Verification still owed

- Manual smoke test: worker + server + edax end-to-end on a few jobs.
- Manual smoke test: frontend pages (game, stats, clients) in a browser.

## Features

- **Auth**: no request auth at all. Old had X-Token/Basic Auth middleware
  on the API and separate Basic Auth on the HTML admin pages.
- **`GET /version`**: no version/build-info endpoint.
- **Stricter evaluation validation**: submitted results check
  level/depth/score bounds, but not the confidence enum
  ({73,87,95,98,99,100}) or an explicit level floor (`TargetLevel` never
  assigns below 28, so the floor holds by construction, not by input
  validation).
- **PGN illegal-move tolerance**: old auto-inserted a pass when a recorded
  move was illegal, recovering from bad data; our parser errors instead.
  Only matters if a real-world PGN turns up that needs it.
- **Desktop GUI**: no `cmd/gui` (Go raylib) or pygame equivalent —
  free-play board, PGN stepping with alt-move exploration, move-frequency
  stats, live-game screen-scraping.
- **CLI tools**: incremental PGN folder-watch importer (last-processed
  tracking), `book validate` (recompute disc counts, re-check evaluation
  invariants), `pgn_organizer` (sort PGNs by date/variant, download from
  playok.com/Othello Quest), `recent_games`, `show_board`.
  (`pgn_analyzer`'s job is covered by the web PGN-analysis page.)
- **`FLIPPY_EDAX_VERBOSE`**: no debug flag to dump edax
  command/cwd/stdin/stdout.
- **Config niceties**: no `PROJECT_ROOT` placeholder substitution in env
  vars, no `LOG_LEVEL` verbosity control.

## Testing gaps

- The websocket client's reconnect/queueing logic has no JS unit tests;
  `static/test/` covers board logic only.
