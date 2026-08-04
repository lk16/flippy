// wasm/edax-eval: a Rust/WASM port of Edax's board, evaluation, and search
// logic, so browsers can reproduce Edax's evaluation score for a position
// without a server round-trip.
//
// Copyright (C) 2026  Luuk Verweij
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// This crate is a derivative work, translating board/evaluation/search logic
// from Edax 4.5.1 (https://github.com/abulmo/edax-reversi), also licensed
// under GPLv3.

// Package webassets embeds this crate's browser-facing files -- the JS wrapper (js/) and the
// built wasm module plus compressed weights blob (dist/) -- so internal/web can serve them
// without a frontend build step, the same way static.FS embeds static/'s hand-written JS/CSS.
// This one .go file living inside a Cargo crate directory is intentional, not a stray file:
// go:embed patterns can't reach outside their own directory tree, and this is the only directory
// containing both js/ and dist/ without duplicating either into static/ (see TASKS.md decision #4,
// which already carves this subtree out as the repo's one exception to "no frontend build step").
//
// dist/ is checked into git (unlike generated/ and target/, both gitignored scratch output) --
// regenerating it requires a local Edax checkout (EDAX_HOST_DIR/eval.dat), which CI doesn't have;
// see docs/next-steps.md for the regeneration steps.
package webassets

import "embed"

//go:embed js/edax-eval.js js/edax-eval-worker.js dist/edax_eval.wasm dist/weights.bin.gz dist/weights_manifest.json
var FS embed.FS
