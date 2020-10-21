use anyhow::{Context, Result};
use cbindgen::{Builder, Language};
use std::env;

fn main() -> Result<()> {
    Builder::new()
        .with_crate(env::var("CARGO_MANIFEST_DIR")?)
        .with_language(Language::C)
        .generate()
        .context("generate bindings")?
        .write_to_file("include/api.h");

    Ok(())
}
