use std::env;

const VERSION: &str = env!("CARGO_PKG_VERSION");

fn main() {
    match env::args().nth(1).as_deref() {
        Some("--capabilities") => {
            println!(
                "{{\"engine\":\"blast\",\"version\":\"{}\",\"protocols\":[\"http/1.1\"],\"status\":\"scaffold\"}}",
                VERSION
            );
        }
        Some("--version") => println!("tml-blast {VERSION}"),
        _ => {
            eprintln!("tml-blast {VERSION}: traffic execution is not implemented in the foundation slice");
            eprintln!("usage: tml-blast --capabilities | --version");
            std::process::exit(2);
        }
    }
}
