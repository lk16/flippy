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

//! Weights extraction/transform tool (`TASKS.md` Task 2).
//!
//! Reads Edax's `eval.dat`, validates its header, slices out the packed
//! weight blocks for plies 2..53 (the only range `eval_open` ever loads),
//! transforms the slice (see `weights_transform`), gzip-compresses it, and
//! writes the compressed blob plus a manifest to an output directory.
//!
//! `eval.dat` is a 13 MB binary artifact external to this repo (see
//! `TASKS.md` decision on licensing/distribution) and is not committed; this
//! tool's output is regenerated from a local Edax checkout rather than
//! checked in either. To run it:
//!
//! ```text
//! EDAX_HOST_DIR=/path/to/edax-reversi \
//!     cargo run --manifest-path wasm/edax-eval/Cargo.toml --bin extract_weights -- [output_dir]
//! ```
//!
//! `EDAX_HOST_DIR` is the same env var `sandbox.sh`/`docker-compose.yml` use
//! elsewhere in this repo for the edax-reversi checkout; `eval.dat` is read
//! from `$EDAX_HOST_DIR/data/eval.dat`. Set `EVAL_DAT_PATH` instead to point
//! directly at an `eval.dat` file outside that layout.

use std::env;
use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::process::ExitCode;

use flate2::write::GzEncoder;
use flate2::Compression;

use edax_eval::weights_transform::{encode, N_PLIES, N_W};

/// Packed weights per ply, as read from `eval.dat` (`eval.c`: `n_w`).
const RAW_N_W: usize = 114364;
/// Total plies `eval_open` reads from the file's block loop (`eval.h`: `EVAL_N_PLY`).
const EVAL_N_PLY: usize = 54;
/// First ply actually used; `eval_open` skips ply 0 and 1 (`eval.c:663`).
const FIRST_PLY: usize = 2;

const HEADER_LEN: usize = 28;

// `const.h`: EDAX/EVAL identify a normally-endian file; XADE/LAVE identify a byte-swapped one.
const EDAX_MAGIC: u32 = 0x4544_4158;
const EVAL_MAGIC: u32 = 0x4556_414c;
const XADE_MAGIC: u32 = 0x5841_4445;
const LAVE_MAGIC: u32 = 0x4c41_5645;

struct Header {
    byte_swapped: bool,
    version: u32,
    release: u32,
    build: u32,
}

fn parse_header(bytes: &[u8]) -> Result<Header, String> {
    if bytes.len() < HEADER_LEN {
        return Err(format!(
            "eval.dat header truncated: got {} bytes, want {HEADER_LEN}",
            bytes.len()
        ));
    }

    let magic1 = u32::from_le_bytes(bytes[0..4].try_into().unwrap());
    let magic2 = u32::from_le_bytes(bytes[4..8].try_into().unwrap());

    // Mirrors eval.c:659's validation exactly: accept either the normal or byte-swapped magic
    // pairing, fail loudly on anything else rather than guessing the format.
    let normal = magic1 == EDAX_MAGIC || magic2 == EVAL_MAGIC;
    let swapped = magic1 == XADE_MAGIC || magic2 == LAVE_MAGIC;
    if !normal && !swapped {
        return Err(format!(
            "eval.dat magic bytes don't match Edax's eval file format: got {magic1:#010x} {magic2:#010x}"
        ));
    }
    // eval.c:653 gates the byte-swap specifically on edax_header == XADE, not on either magic.
    let byte_swapped = magic1 == XADE_MAGIC;

    let mut version = u32::from_le_bytes(bytes[8..12].try_into().unwrap());
    let mut release = u32::from_le_bytes(bytes[12..16].try_into().unwrap());
    let mut build = u32::from_le_bytes(bytes[16..20].try_into().unwrap());
    if byte_swapped {
        version = version.swap_bytes();
        release = release.swap_bytes();
        build = build.swap_bytes();
    }

    Ok(Header {
        byte_swapped,
        version,
        release,
        build,
    })
}

/// Locates `eval.dat` from `EVAL_DAT_PATH` (direct path) or `EDAX_HOST_DIR` (edax-reversi
/// checkout root, same env var `sandbox.sh`/`docker-compose.yml` use for it elsewhere in this
/// repo), in that priority order.
fn locate_eval_dat() -> Result<PathBuf, String> {
    if let Ok(path) = env::var("EVAL_DAT_PATH") {
        return Ok(PathBuf::from(path));
    }
    if let Ok(host_dir) = env::var("EDAX_HOST_DIR") {
        return Ok(PathBuf::from(host_dir).join("data").join("eval.dat"));
    }
    Err(
        "neither EVAL_DAT_PATH nor EDAX_HOST_DIR is set; set EVAL_DAT_PATH to eval.dat's path, \
         or EDAX_HOST_DIR to an edax-reversi checkout root containing data/eval.dat"
            .to_string(),
    )
}

/// FNV-1a 64-bit hash, used only to fingerprint the extracted raw slice in the manifest (not a
/// security checksum, just a build-artifact identity check).
fn fnv1a64(data: &[u8]) -> u64 {
    const OFFSET_BASIS: u64 = 0xcbf2_9ce4_8422_2325;
    const PRIME: u64 = 0x0000_0100_0000_01b3;
    data.iter().fold(OFFSET_BASIS, |hash, &byte| {
        (hash ^ byte as u64).wrapping_mul(PRIME)
    })
}

/// Parses the header and slices out the ply-major raw weights (plies `FIRST_PLY..EVAL_N_PLY`) from
/// a full `eval.dat` file buffer. Pure function, split out from `run()` so tests can exercise it
/// directly against the real file without touching the filesystem for output.
fn extract_raw_slice(file: &[u8]) -> Result<(Header, Vec<i16>), String> {
    assert_eq!(RAW_N_W, N_W, "RAW_N_W must match weights_transform::N_W");
    assert_eq!(
        EVAL_N_PLY - FIRST_PLY,
        N_PLIES,
        "ply range must match weights_transform::N_PLIES"
    );

    let header = parse_header(file)?;

    let block_bytes = RAW_N_W * 2;
    let want_len = HEADER_LEN + EVAL_N_PLY * block_bytes;
    if file.len() < want_len {
        return Err(format!(
            "eval.dat is too short: got {} bytes, want at least {want_len} ({EVAL_N_PLY} blocks of {RAW_N_W} shorts after the header)",
            file.len()
        ));
    }

    // Ply-major raw slice: plies FIRST_PLY..EVAL_N_PLY, each RAW_N_W shorts, byte-swapped if the
    // file was written on a different-endian machine (eval.c:665-666).
    let mut raw = vec![0i16; N_W * N_PLIES];
    for ply in FIRST_PLY..EVAL_N_PLY {
        let block_start = HEADER_LEN + ply * block_bytes;
        let block = &file[block_start..block_start + block_bytes];
        let out_ply = ply - FIRST_PLY;
        for i in 0..RAW_N_W {
            let bytes = [block[i * 2], block[i * 2 + 1]];
            let v = if header.byte_swapped {
                i16::from_be_bytes(bytes)
            } else {
                i16::from_le_bytes(bytes)
            };
            raw[out_ply * N_W + i] = v;
        }
    }

    Ok((header, raw))
}

fn run() -> Result<(), String> {
    let eval_dat_path = locate_eval_dat()?;
    let file = fs::read(&eval_dat_path)
        .map_err(|e| format!("failed to read {}: {e}", eval_dat_path.display()))?;

    let (header, raw) = extract_raw_slice(&file)?;

    let raw_bytes: Vec<u8> = raw.iter().flat_map(|v| v.to_le_bytes()).collect();
    let raw_checksum = fnv1a64(&raw_bytes);

    let transformed = encode(&raw, N_W, N_PLIES);

    let mut encoder = GzEncoder::new(Vec::new(), Compression::best());
    encoder
        .write_all(&transformed)
        .map_err(|e| format!("gzip encoding failed: {e}"))?;
    let compressed = encoder
        .finish()
        .map_err(|e| format!("gzip encoding failed: {e}"))?;

    let output_dir = env::args()
        .nth(1)
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("generated"));
    fs::create_dir_all(&output_dir)
        .map_err(|e| format!("failed to create {}: {e}", output_dir.display()))?;

    let blob_path = output_dir.join("weights.bin.gz");
    fs::write(&blob_path, &compressed)
        .map_err(|e| format!("failed to write {}: {e}", blob_path.display()))?;

    let manifest = format!(
        "{{\n\
         \x20\x20\"format\": \"edax-eval-weights-v1\",\n\
         \x20\x20\"transform\": \"transpose-delta-byteplane\",\n\
         \x20\x20\"compression\": \"gzip\",\n\
         \x20\x20\"source_version\": {},\n\
         \x20\x20\"source_release\": {},\n\
         \x20\x20\"source_build\": {},\n\
         \x20\x20\"source_byte_swapped\": {},\n\
         \x20\x20\"n_w\": {N_W},\n\
         \x20\x20\"n_plies\": {N_PLIES},\n\
         \x20\x20\"first_ply\": {FIRST_PLY},\n\
         \x20\x20\"raw_slice_bytes\": {},\n\
         \x20\x20\"raw_slice_checksum_fnv1a64\": \"{:#018x}\",\n\
         \x20\x20\"compressed_bytes\": {}\n\
         }}\n",
        header.version,
        header.release,
        header.build,
        header.byte_swapped,
        raw_bytes.len(),
        raw_checksum,
        compressed.len(),
    );
    let manifest_path = output_dir.join("weights_manifest.json");
    fs::write(&manifest_path, &manifest)
        .map_err(|e| format!("failed to write {}: {e}", manifest_path.display()))?;

    eprintln!(
        "wrote {} ({} bytes) and {} (source {}.{}.{}, raw slice {} bytes -> {} bytes compressed)",
        blob_path.display(),
        compressed.len(),
        manifest_path.display(),
        header.version,
        header.release,
        header.build,
        raw_bytes.len(),
        compressed.len(),
    );

    Ok(())
}

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("error: {e}");
            ExitCode::FAILURE
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_normal_header() {
        let mut bytes = vec![0u8; HEADER_LEN];
        bytes[0..4].copy_from_slice(&EDAX_MAGIC.to_le_bytes());
        bytes[4..8].copy_from_slice(&EVAL_MAGIC.to_le_bytes());
        bytes[8..12].copy_from_slice(&3u32.to_le_bytes());
        bytes[12..16].copy_from_slice(&2u32.to_le_bytes());
        bytes[16..20].copy_from_slice(&5u32.to_le_bytes());

        let header = parse_header(&bytes).unwrap();
        assert!(!header.byte_swapped);
        assert_eq!((header.version, header.release, header.build), (3, 2, 5));
    }

    #[test]
    fn parses_byte_swapped_header() {
        let mut bytes = vec![0u8; HEADER_LEN];
        bytes[0..4].copy_from_slice(&XADE_MAGIC.to_le_bytes());
        bytes[4..8].copy_from_slice(&LAVE_MAGIC.to_le_bytes());
        bytes[8..12].copy_from_slice(&3u32.swap_bytes().to_le_bytes());
        bytes[12..16].copy_from_slice(&2u32.swap_bytes().to_le_bytes());
        bytes[16..20].copy_from_slice(&5u32.swap_bytes().to_le_bytes());

        let header = parse_header(&bytes).unwrap();
        assert!(header.byte_swapped);
        assert_eq!((header.version, header.release, header.build), (3, 2, 5));
    }

    #[test]
    fn rejects_bad_magic() {
        let bytes = vec![0u8; HEADER_LEN];
        assert!(parse_header(&bytes).is_err());
    }

    #[test]
    fn rejects_truncated_header() {
        let bytes = vec![0u8; HEADER_LEN - 1];
        assert!(parse_header(&bytes).is_err());
    }

    #[test]
    fn fnv1a64_is_stable_and_sensitive() {
        assert_eq!(fnv1a64(b""), 0xcbf2_9ce4_8422_2325);
        assert_ne!(fnv1a64(b"a"), fnv1a64(b"b"));
    }

    /// Mirrors the EDAX_PATH-gated skip pattern in `internal/edax/process_test.go`: runs only
    /// when a real eval.dat is available, skips (doesn't fail) otherwise, so it needs no separate
    /// CI exclusion.
    #[test]
    fn extracts_and_round_trips_the_real_eval_dat() {
        let Ok(eval_dat_path) = locate_eval_dat() else {
            eprintln!("EVAL_DAT_PATH/EDAX_HOST_DIR not set; skipping");
            return;
        };
        let Ok(file) = fs::read(&eval_dat_path) else {
            eprintln!("{} not readable; skipping", eval_dat_path.display());
            return;
        };

        let (header, raw) = extract_raw_slice(&file).expect("real eval.dat should parse");

        // These are facts read directly from the real file (TASKS.md Task 2 "Correction" note):
        // format version 3.2.5, not byte-swapped, exactly the raw slice size the transform table
        // in TASKS.md was measured against.
        assert!(!header.byte_swapped);
        assert_eq!((header.version, header.release, header.build), (3, 2, 5));
        assert_eq!(raw.len(), N_W * N_PLIES);
        let raw_bytes: Vec<u8> = raw.iter().flat_map(|v| v.to_le_bytes()).collect();
        assert_eq!(raw_bytes.len(), 11_893_856);

        let transformed = encode(&raw, N_W, N_PLIES);
        assert_eq!(
            edax_eval::weights_transform::decode(&transformed, N_W, N_PLIES),
            raw,
            "decode(encode(raw)) must reconstruct the real weights exactly"
        );
    }
}
