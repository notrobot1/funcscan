def main():
    result = run_pipeline("input.csv")
    print("result:", result)


def run_pipeline(path):
    raw = load_file(path)
    validated = validate(raw)
    return transform(validated)


def load_file(path):
    return f"data from {path}"


def validate(data):
    if not data:
        raise ValueError("empty data")
    return sanitize(data)


def sanitize(data):
    return data.strip()


def transform(data):
    return encode(data)


def encode(s):
    return f"[{s}]"


if __name__ == "__main__":
    main()
