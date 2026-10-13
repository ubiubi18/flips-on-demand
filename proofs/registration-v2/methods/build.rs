include!("../../guest-build.rs");

fn main() {
    embed(
        "registration-v2-guest",
        "registration-v2/methods",
        &["flip-core", "registration/core", "session-core"],
    );
}
