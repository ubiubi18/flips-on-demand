//! The session evidence and the version 2 registration statement.
//!
//! This crate is deliberately separate from `fod-registration-core`: that crate
//! is compiled into the pinned registration guest, and adding a single unused
//! function to it moves the guest's image id, which would invalidate the seal
//! the escrow ships and every stored receipt (the host's sealed-artefact test
//! fails on exactly that). The version 2 program has its own image id, so its
//! evidence (`session`), its proof folding (`iavl`) and its public statement
//! (`statement`) live here until that program replaces the version 1 one.

#![no_std]

pub mod iavl;
pub mod session;
pub mod statement;
