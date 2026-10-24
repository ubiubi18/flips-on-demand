//! Ownerless bonded curve market candidate. No administrator or proof bypass.
#![cfg_attr(target_arch = "wasm32", no_std)]
extern crate alloc;

pub mod curve;
pub mod market;
pub mod records;
pub mod proof;
#[path = "../../escrow/src/verifier.rs"]
mod verifier;
#[cfg(target_arch = "wasm32")]
mod host;

#[cfg(test)]
mod tests;
