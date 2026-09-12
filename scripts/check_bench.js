const fs = require("fs");

function analyzeLog(content) {
  const match = content.match(/#\s+p95:\s+(\d+(?:\.\d+)?)\s+ns/);
  if (!match) {
    console.error("Error: could not find '# p95: <value> ns' in benchmark output!");
    process.exit(1);
  }

  const p95 = parseFloat(match[1]);
  const threshold = 5000.0;
  console.log(`==> IPC Dispatch p95 Latency: ${p95} ns (Threshold: <= ${threshold} ns)`);

  if (p95 > threshold) {
    console.error(`VIOLATION: IPC Dispatch p95 latency (${p95} ns) exceeds threshold (${threshold} ns)!`);
    process.exit(1);
  }

  console.log(`PASS: IPC Dispatch p95 latency (${p95} ns) is within SLA.`);
}

const targetFile = process.argv[2] || "bench.log";

if (fs.existsSync(targetFile)) {
  const content = fs.readFileSync(targetFile, "utf-8");
  analyzeLog(content);
} else if (!process.stdin.isTTY) {
  let content = "";
  process.stdin.setEncoding("utf-8");
  process.stdin.on("data", (chunk) => {
    content += chunk;
  });
  process.stdin.on("end", () => {
    analyzeLog(content);
  });
} else {
  console.error(`Error: benchmark log file '${targetFile}' not found and no stdin provided!`);
  process.exit(1);
}
