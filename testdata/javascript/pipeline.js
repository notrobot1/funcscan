function main() {
  const result = runPipeline("input.csv");
  console.log("result:", result);
}

function runPipeline(path) {
  const raw = loadFile(path);
  const validated = validate(raw);
  return transform(validated);
}

function loadFile(path) {
  return `data from ${path}`;
}

function validate(data) {
  if (!data) throw new Error("empty data");
  return sanitize(data);
}

function sanitize(data) {
  return data.trim();
}

function transform(data) {
  return encode(data);
}

function encode(s) {
  return `[${s}]`;
}

main();
