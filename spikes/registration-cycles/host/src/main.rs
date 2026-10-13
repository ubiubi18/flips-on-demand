//! Reports what the registration guest's primitives cost in the zkVM.
//!
//! It runs the guest with the executor, which produces no receipt: the guest
//! times itself with its own cycle counter and commits the deltas, so a
//! measurement costs execution time rather than proving time. Nothing here is
//! verification evidence; the numbers are the point.
//!
//!   cargo run --release -- <mode> [payload KiB]
//!
//! Modes: baseline, sha256, aes, ecdh, flip, raw (the slice ingestion path).

use risc0_zkvm::{default_executor, ExecutorEnv};

use k256::elliptic_curve::sec1::ToEncodedPoint;

fn mode_code(name: &str) -> u32 {
    match name {
        "baseline" => 0,
        "sha256" => 1,
        "aes" => 2,
        "ecdh" => 3,
        "flip" => 4,
        "raw" => 5,
        "read" => 6,
        other => panic!("unknown mode {other}: baseline, read, raw, sha256, aes, ecdh, flip"),
    }
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let mode = args.first().cloned().unwrap_or_else(|| "flip".to_string());
    let kib: usize = args.get(1).map(|v| v.parse().expect("payload KiB")).unwrap_or(2);

    // A payload of the requested size, and a scalar plus a matching public point
    // for the ECDH: the same shape the pinned construction uses.
    let payload = vec![0x5au8; kib * 1024];
    let secret = (1u8..=32).collect::<Vec<u8>>();
    let scalar = k256::SecretKey::from_slice(&secret).expect("secret");
    let counterpart = scalar.public_key().to_encoded_point(false).as_bytes().to_vec();

    let mut builder = ExecutorEnv::builder();
    builder.write(&mode_code(&mode)).expect("write mode");
    if mode == "raw" {
        builder
            .write(&(payload.len() as u32))
            .expect("write length")
            .write_slice(&payload);
    } else {
        builder.write(&payload).expect("write payload");
    }
    let env = builder
        .write(&secret)
        .expect("write secret")
        .write(&counterpart)
        .expect("write point")
        .build()
        .expect("build env");

    let started = std::time::Instant::now();
    let session = default_executor()
        .execute(env, methods::CYCLES_GUEST_ELF)
        .expect("execute");
    let seconds = started.elapsed().as_secs_f64();

    let journal = &session.journal.bytes;
    if journal.len() != 32 {
        panic!("unexpected journal of {} bytes", journal.len());
    }
    let number = |at: usize| u64::from_le_bytes(journal[at..at + 8].try_into().expect("word"));
    let ingest = number(8);
    let work = number(16);

    println!("mode          {mode}");
    println!("payload       {kib} KiB");
    println!("ingest cycles {ingest}");
    println!("work cycles   {work}");
    println!("total cycles  {}", ingest + work);
    println!("elapsed       {seconds:.2} s (executed, not proved)");
    if kib > 0 {
        println!("work per KiB  {:.0} cycles", work as f64 / kib as f64);
    }
}
