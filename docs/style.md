# Style

## Docs (this folder)

Concise but precise. Plain language — not formal, no corporate lingo.
These files are read by AI agents every session, so keep token count low:
short sentences, no filler, don't repeat what the code already says.

## Code

- Idiomatic Go, gofmt-formatted, golangci-lint clean.
- Return errors; panic only on internal invariant violations that indicate
  a bug, never for expected failures (bad input, invalid moves, I/O).
- Comments: one line per exported symbol, plus only constraints the code
  can't express ("edax crashes on a position with no legal move" stays;
  design essays go). Never describe a change or the past — git history
  does that.
- All backend code except `cmd/*/main.go` entrypoints needs unit tests;
  prefer table-driven tests with testify.
- Frontend JS is dependency-free. `static/board.js` must stay in sync with
  `internal/othello`'s move generation/normalization output.
