const fs = require("fs");
const path = require("path");

const emojiRegex = /[\u{1F300}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}]/u;
let violations = 0;

function checkTypeScriptFile(full, content) {
  // 1. Check for prohibited emoji characters
  if (emojiRegex.test(content)) {
    console.error(`[HYGIENE VIOLATION] Prohibited emoji detected in ${full}`);
    violations++;
  }

  // 2. Check for empty catch blocks
  if (/catch\s*(\([^)]*\))?\s*\{\s*\}/.test(content)) {
    console.error(`[HYGIENE VIOLATION] Empty catch block detected in ${full}`);
    violations++;
  }

  // 3. Check for 'as any' bypasses
  if (/\bas\s+any\b/.test(content)) {
    console.error(`[HYGIENE VIOLATION] 'as any' type bypass detected in ${full}`);
    violations++;
  }

  // 4. Check for <em> emphasis tags
  if (/<em>/i.test(content)) {
    console.error(`[HYGIENE VIOLATION] <em> tag detected in ${full}`);
    violations++;
  }
}

function checkGoFile(full, content) {
  // 1. Check for prohibited emoji characters
  if (emojiRegex.test(content)) {
    console.error(`[HYGIENE VIOLATION] Prohibited emoji detected in ${full}`);
    violations++;
  }

  // 2. In non-test Go code, check for blank error discards without // errcheck:ok justification
  if (!full.endsWith("_test.go")) {
    const lines = content.split("\n");
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const prevLine = i > 0 ? lines[i - 1] : "";

      // Blank discards like `_ = ...` or `_, _ = ...`
      if (/^\s*(_\s*=|_\s*,\s*_\s*=)\s*/.test(line)) {
        // Exempt interface assertions: `var _ Interface = (*Impl)(nil)`
        if (!line.includes("var _") && !line.includes("// errcheck:ok") && !prevLine.includes("// errcheck:ok")) {
          console.error(
            `[HYGIENE VIOLATION] Unhandled blank discard at ${full}:${i + 1}: ${line.trim()} (must handle error or annotate with '// errcheck:ok <reason>')`
          );
          violations++;
        }
      }

      // Empty error check blocks: if err != nil { }
      if (/if\s+err\s*!=\s*nil\s*\{\s*\}/.test(line)) {
        if (!line.includes("// errcheck:ok") && !prevLine.includes("// errcheck:ok")) {
          console.error(
            `[HYGIENE VIOLATION] Empty error check at ${full}:${i + 1}: ${line.trim()} (must handle error or annotate with '// errcheck:ok <reason>')`
          );
          violations++;
        }
      }
    }
  }
}

function walk(dir) {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir)) {
    const full = path.join(dir, entry);
    const stat = fs.statSync(full);
    if (stat.isDirectory()) {
      walk(full);
    } else if (entry.endsWith(".ts") || entry.endsWith(".tsx")) {
      const content = fs.readFileSync(full, "utf8");
      checkTypeScriptFile(full, content);
    } else if (entry.endsWith(".go")) {
      const content = fs.readFileSync(full, "utf8");
      checkGoFile(full, content);
    }
  }
}

console.log("==> Running Code Hygiene Pre-Flight Verification...");
console.log("--> Scanning frontend/src (TypeScript / SolidJS)...");
walk(path.join(process.cwd(), "frontend", "src"));

console.log("--> Scanning internal & cmd (Go)...");
walk(path.join(process.cwd(), "internal"));
walk(path.join(process.cwd(), "cmd"));

if (violations > 0) {
  console.error(`\nFAILED: Found ${violations} code hygiene violations!`);
  process.exit(1);
}

console.log("PASS: 100% clean code verified across TypeScript and Go (0 emojis, 0 empty catches, 0 'as any', 0 <em> tags, 0 unannotated blank discards).");
