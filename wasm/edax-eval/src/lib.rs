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

//! A bit-exact Rust port of Edax's board representation ([`board`]), evaluation function
//! ([`eval`], [`weights`], [`weights_transform`]), and full-width alpha-beta search ([`search`]) at
//! level 10, exposed to the browser via a raw wasm ABI ([`wasm_api`]) -- see `TASKS.md` for the
//! full design background and the decisions this crate is built on.

pub mod board;
pub mod eval;
#[cfg(test)]
pub(crate) mod flip_variants;
pub mod search;
pub(crate) mod stability;
pub mod wasm_api;
pub mod weights;
pub mod weights_transform;

/// The crate version, as declared in `Cargo.toml`.
pub fn version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn version_is_set() {
        assert!(!version().is_empty());
    }
}
