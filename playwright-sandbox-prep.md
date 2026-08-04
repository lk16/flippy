# Playwright e2e testing — sandbox prep notes

Scratch notes from a sandbox investigation into adding Playwright tests for
the frontend (`static/`). Not implemented yet — this file just records what
the *next* sandbox session needs from the host side so we don't have to
re-derive it. Not meant to be merged.

## Network domains to allowlist

Everything below was confirmed blocked by the sandbox's default-deny policy
(not corporate/system policy), so it's addable with `sbx policy allow
network <domain>`, or permanently via `.sbx/kit/spec.yaml`'s
`caps.network.allow` list (see the existing entries there for the pattern).

- `registry.npmjs.org` — no `package.json` exists in this repo yet; needed
  for `npm install -D @playwright/test`.
- `cdn.playwright.dev` — Playwright's browser-binary download host (current
  versions). Older Playwright releases used `playwright.azureedge.net`
  instead — if `npx playwright install` still 403s after allowing
  `cdn.playwright.dev`, check `sbx policy log` for the actual host it tried
  and add that too.

Already allowlisted in `.sbx/kit/spec.yaml` and confirmed working, no
action needed:

- Docker Hub pull chain (`registry-1.docker.io`, `auth.docker.io`,
  `production.cloudfront.docker.com`) — covers `postgres:17` / `redis:8` for
  the local dev stack.
- `archive.ubuntu.com` / `security.ubuntu.com` — should also cover
  `npx playwright install --with-deps`'s apt-installed browser system libs
  (libnss3, fonts-liberation, etc.), same mechanism as the existing
  gcc/libc6-dev install.

## Node version mismatch

- CI (`.github/workflows/ci.yml`, `js-test` job) pins **Node 22**
  (`node-version: "22"`).
- This sandbox's base image ships **Node 22** (`v22.22.1`) — matches CI.
- The host machine runs **Node 26** (current latest) — mismatch vs.
  both CI and the sandbox.

Decide before writing tests: pin Playwright to Node 22 to match existing CI,
or bump CI + sandbox to Node 26 to match the host and "latest". Either way,
add an explicit version pin (`.nvmrc` and/or `package.json` `engines`) once
`package.json` exists, so this doesn't silently drift again.

To bring Node 26 into a sandbox session (base image only has 22
preinstalled), the straightforward options both need network access not
currently allowlisted — confirmed blocked by default-deny:

- `nodejs.org` — direct tarball download from `nodejs.org/dist/`.
- `raw.githubusercontent.com` — nvm's install script
  (`nvm-sh/nvm/master/install.sh`), then `nvm install 26`.
- `deb.nodesource.com` — NodeSource's apt repo, another way to install a
  specific Node major version via `apt-get`.

Pick whichever fits the eventual "pin to 22 or bump to 26" decision above,
allowlist that one domain, and prepare accordingly. If bumping to 26, do it
in both the sandbox (so local Playwright runs match) and
`.github/workflows/ci.yml`'s `node-version` (so CI matches too).

## Still needed once in the sandbox (no host prep required)

- `npm init` + `npm install -D @playwright/test` (net-new `package.json`).
- `npx playwright install --with-deps chromium` (add firefox/webkit only if
  cross-browser coverage is actually wanted).
- Start the app the way `local.sh` does (docker-compose Postgres/Redis,
  `migrate`, `go run ./cmd/loader seed`, `go build`/run server) so Playwright
  has a real `localhost:8080` to drive.
