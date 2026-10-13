// Shared by the two methods build scripts. Only compile inputs enter Docker;
// local captures, order witnesses, vaults and target directories never do.
use risc0_build::{DockerOptionsBuilder, GuestOptionsBuilder};
use std::{collections::HashMap, env, fs, path::Path};

pub const BUILDER: &str =
    "r0.1.91.1@sha256:fafb377a44e1cfca415577c48d2f7012bda99ed36f2fae27f9a663b9fe6048f0";

fn copy_file(source: &Path, target: &Path) {
    let meta = fs::symlink_metadata(source).expect("guest source metadata");
    assert!(
        meta.is_file() && !meta.file_type().is_symlink(),
        "guest source must be a regular file: {}",
        source.display()
    );
    fs::create_dir_all(target.parent().unwrap()).expect("guest source directory");
    fs::copy(source, target).expect("copy guest source");
    println!("cargo:rerun-if-changed={}", source.display());
}

fn copy_src(source: &Path, target: &Path) {
    assert!(
        fs::symlink_metadata(source).unwrap().is_dir(),
        "guest source directory must not be a symlink"
    );
    println!("cargo:rerun-if-changed={}", source.display());
    for entry in fs::read_dir(source).unwrap() {
        let entry = entry.unwrap();
        let kind = entry.file_type().unwrap();
        assert!(
            !kind.is_symlink(),
            "guest source symlink refused: {}",
            entry.path().display()
        );
        if kind.is_dir() {
            copy_src(&entry.path(), &target.join(entry.file_name()));
        } else if entry.path().extension().is_some_and(|ext| ext == "rs") {
            copy_file(&entry.path(), &target.join(entry.file_name()));
        }
    }
}

fn copy_crate(root: &Path, staged: &Path, name: &str) {
    let source = root.join(name);
    // Check each component rather than following a symlink into a local vault.
    let mut part = root.to_path_buf();
    for component in Path::new(name).components() {
        part.push(component);
        assert!(
            !fs::symlink_metadata(&part)
                .unwrap()
                .file_type()
                .is_symlink(),
            "guest crate symlink refused"
        );
    }
    copy_file(
        &source.join("Cargo.toml"),
        &staged.join(name).join("Cargo.toml"),
    );
    if source.join("Cargo.lock").exists() {
        copy_file(
            &source.join("Cargo.lock"),
            &staged.join(name).join("Cargo.lock"),
        );
    }
    copy_src(&source.join("src"), &staged.join(name).join("src"));
}

pub fn embed(guest: &str, methods: &str, dependencies: &[&str]) {
    for name in [
        "RISC0_SKIP_BUILD",
        "RISC0_BUILD_DEBUG",
        "RISC0_DOCKER_CONTAINER_TAG",
    ] {
        println!("cargo:rerun-if-env-changed={name}");
        assert!(
            env::var_os(name).is_none_or(|value| value.is_empty()),
            "{name} is not allowed for pinned guest builds"
        );
    }
    let original = env::var_os("CARGO_MANIFEST_DIR").unwrap();
    let root = Path::new(&original).parent().unwrap().parent().unwrap();
    println!(
        "cargo:rerun-if-changed={}",
        root.join("guest-build.rs").display()
    );
    let staged = Path::new(&env::var_os("OUT_DIR").unwrap()).join("guest-source");
    if staged.exists() {
        fs::remove_dir_all(&staged).expect("replace generated guest compile inputs");
    }
    fs::create_dir(&staged).expect("stage guest compile inputs");
    // Stop Cargo from discovering the caller's workspace above OUT_DIR.
    fs::write(
        staged.join("Cargo.toml"),
        format!("[workspace]\nmembers = [{methods:?}]\nresolver = \"2\"\n"),
    )
    .unwrap();
    copy_crate(root, &staged, methods);
    copy_crate(root, &staged, &format!("{methods}/guest"));
    for dependency in dependencies {
        copy_crate(root, &staged, dependency);
    }

    let docker = DockerOptionsBuilder::default()
        .root_dir(&staged)
        .docker_container_tag(BUILDER)
        .build()
        .unwrap();
    let options = GuestOptionsBuilder::default()
        .use_docker(docker)
        .build()
        .unwrap();
    // SDK metadata must point into the source-only context. Docker compiles at
    // /src with its digest-pinned compiler and locked dependency graph.
    env::set_var("CARGO_MANIFEST_DIR", staged.join(methods));
    let entries = risc0_build::embed_methods_with_options(HashMap::from([(guest, options)]));
    env::set_var("CARGO_MANIFEST_DIR", original);
    assert_eq!(entries.len(), 1, "exactly one pinned guest is required");
}
