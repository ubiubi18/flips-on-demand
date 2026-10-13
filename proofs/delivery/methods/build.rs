include!("../../guest-build.rs");

fn main() {
    embed("delivery-guest", "delivery/methods", &["delivery/core"]);
}
