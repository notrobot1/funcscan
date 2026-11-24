function main(): void {
  const result = runPipeline("input.csv");
  console.log("result:", result);
}

function runPipeline(path: string): string {
  const raw = loadFile(path);
  const validated = validate(raw);
  return transform(validated);
}

function loadFile(path: string): string {
  return `data from ${path}`;
}

function validate(data: string): string {
  if (!data) throw new Error("empty data");
  return sanitize(data);
}

function sanitize(data: string): string {
  return data.trim();
}

function transform(data: string): string {
  return encode(data);
}

function encode(s: string): string {
  return `[${s}]`;
}

main();
