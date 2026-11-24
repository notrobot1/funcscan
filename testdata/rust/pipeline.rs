fn main() {
    match run_pipeline("input.csv") {
        Ok(result) => println!("result: {}", result),
        Err(e) => eprintln!("error: {}", e),
    }
}

fn run_pipeline(path: &str) -> Result<String, String> {
    let raw = load_file(path)?;
    let validated = validate(&raw)?;
    Ok(transform(&validated))
}

fn load_file(path: &str) -> Result<String, String> {
    Ok(format!("data from {}", path))
}

fn validate(data: &str) -> Result<String, String> {
    if data.is_empty() {
        return Err("empty data".to_string());
    }
    Ok(sanitize(data))
}

fn sanitize(data: &str) -> String {
    data.trim().to_string()
}

fn transform(data: &str) -> String {
    encode(data)
}

fn encode(s: &str) -> String {
    format!("[{}]", s)
}
