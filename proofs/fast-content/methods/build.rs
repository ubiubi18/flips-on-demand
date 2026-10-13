include!("../../guest-build.rs");

fn main() {
    embed(
        "fast-content-guest",
        "fast-content/methods",
        &[
            "fast-content-core",
            "fast-delivery-core",
            "registration/core",
            "flip-core",
            "session-core",
        ],
    );
}
