const base = process.env.BIFROST_TEST_URL || "http://127.0.0.1:8080";
const targets = ["/?name=Bench", "/stream", "/about"];
const levels = [1, 2, 4, 8, 16];
const requests = 200;
const warmup = 20;

const percentile = (sorted, fraction) => sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * fraction))];
const format = (value) => value.toFixed(2);
const cell = (value, width, suffix = "") => (value === undefined ? "-" : `${format(value)}${suffix}`).padStart(width);

async function fetchOnce(url) {
  const start = performance.now();
  const response = await fetch(url);
  await response.arrayBuffer();
  return { elapsed: performance.now() - start, status: response.status };
}

async function measure(url, concurrency) {
  let next = 0;
  let overloaded = 0;
  const latencies = [];
  const worker = async () => {
    while (true) {
      const index = next++;
      if (index >= requests) return;
      const { elapsed, status } = await fetchOnce(url);
      if (status === 503) {
        overloaded++;
        continue;
      }
      if (status !== 200) throw new Error(`${status} from ${url}`);
      latencies.push(elapsed);
    }
  };
  const started = performance.now();
  await Promise.all(Array.from({ length: concurrency }, worker));
  const wall = performance.now() - started;
  latencies.sort((a, b) => a - b);
  return { concurrency, throughput: (latencies.length / wall) * 1000, p50: percentile(latencies, 0.5), p95: percentile(latencies, 0.95), p99: percentile(latencies, 0.99), overloaded };
}

console.log(`${requests} requests per level against ${base}`);
console.log(`${"clients".padStart(7)}  ${"req/s".padStart(9)}  ${"p50".padStart(8)}  ${"p95".padStart(8)}  ${"p99".padStart(8)}  ${"503".padStart(5)}`);

for (const target of targets) {
  const url = `${base}${target}`;
  for (let index = 0; index < warmup; index++) {
    const { status } = await fetchOnce(url);
    if (status !== 200) throw new Error(`${status} from ${url}`);
  }
  console.log(target);
  for (const level of levels) {
    const result = await measure(url, level);
    const cells = [String(result.concurrency).padStart(7), cell(result.throughput, 9, "/s"), cell(result.p50, 8, "ms"), cell(result.p95, 8, "ms"), cell(result.p99, 8, "ms"), String(result.overloaded).padStart(5)];
    console.log(cells.join("  "));
  }
}
