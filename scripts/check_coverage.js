const fs = require("fs");
const { execSync } = require("child_process");

const coverageFile = process.argv[2] || "coverage.out";

if (!fs.existsSync(coverageFile)) {
  console.error(`Error: coverage file ${coverageFile} not found!`);
  process.exit(1);
}

try {
  const output = execSync(`go tool cover -func=${coverageFile}`).toString();
  const match = output.match(/total:\s+\(statements\)\s+([\d\.]+)%/);
  if (!match) {
    console.error("Error: could not parse total coverage percentage from go tool cover!");
    process.exit(1);
  }

  const coverage = parseFloat(match[1]);
  console.log(`==> Total Internal Core Statement Coverage: ${coverage}%`);

  const threshold = 80.0;
  if (coverage < threshold) {
    console.error(`VIOLATION: Coverage ${coverage}% is below the required ${threshold}% threshold!`);
    process.exit(1);
  }

  console.log(`PASS: Test coverage requirement (>= ${threshold}%) respected.`);
} catch (err) {
  console.error("Coverage analysis failed:", err);
  process.exit(1);
}
