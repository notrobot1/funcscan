def main
  result = run_pipeline("input.csv")
  puts "result: #{result}"
end

def run_pipeline(path)
  raw = load_file(path)
  validated = validate(raw)
  transform(validated)
end

def load_file(path)
  "data from #{path}"
end

def validate(data)
  raise "empty data" if data.nil? || data.empty?
  sanitize(data)
end

def sanitize(data)
  data.strip
end

def transform(data)
  encode(data)
end

def encode(s)
  "[#{s}]"
end

main
