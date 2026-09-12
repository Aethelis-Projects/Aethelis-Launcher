const fs = require("fs");
const path = require("path");

const emojiRegex = /[\u{1F300}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}]/u;
let violations = 0;

function checkTypeScriptFile(full, content) {
  // 1. Check for prohibited emoji characters
  if (emojiRegex.test(content)) {
    console.error(`[ANTI-SLOP VIOLATION] Prohibited emoji detected in ${full}`);
    violations++;
  }

  // 2. Check for empty catch blocks
  if (/catch\s*(\([^)]*\))?\s*\{\s*\}/.test(content)) {
    console.error(`[ANTI-SLOP VIOLATION] Empty catch block detected in ${full}`);
    violations++;
  }

  // 3. Check for 'as any' bypasses
  if (/\bas\s+any\b/.test(content)) {
    console.error(`[ANTI-SLOP VIOLATION] 'as any' type bypass detected in ${full}`);
    violations++;
  }

  // 4. Check for <em> emphasis slop
  if (/<em>/i.test(content)) {
    console.error(`[ANTI-SLOP VIOLATION] <em> tag detected in ${full}`);
    violations++;
  }
}

function checkGoFile(full, content) {
  // 1. Check for prohibited emoji characters
  if (emojiRegex.test(content)) {
    console.error(`[ANTI-SLOP VIOLATION] Prohibited emoji detected in ${full}`);
    violations++;
  }

  // 2. In non-test Go code, check for blank error discards without // slop:ok justification
  if (!full.endsWith("_test.go")) {
    const lines = content.split("\n");
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const prevLine = i > 0 ? lines[i - 1] : "";

      // Blank discards like `_ = ...` or `_, _ = ...`
      if (/^\s*(_\s*=|_\s*,\s*_\s*=)\s*/.test(line)) {
        // Exempt interface assertions: `var _ Interface = (*Impl)(nil)`
        if (!line.includes("var _") && !line.includes("// slop:ok") && !prevLine.includes("// slop:ok")) {
          console.error(
            `[ANTI-SLOP VIOLATION] Unhandled blank discard at ${full}:${i + 1}: ${line.trim()} (must handle error or annotate with '// slop:ok <reason>')`
          );
          violations++;
        }
      }

      // Empty error check blocks: if err != nil { }
      if (/if\s+err\s*!=\s*nil\s*\{\s*\}/.test(line)) {
        if (!line.includes("// slop:ok") && !prevLine.includes("// slop:ok")) {
          console.error(
            `[ANTI-SLOP VIOLATION] Empty error check at ${full}:${i + 1}: ${line.trim()} (must handle error or annotate with '// slop:ok <reason>')`
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

console.log("==> Running Anti-AI-Slop Pre-Flight Verification...");
console.log("--> Scanning frontend/src (TypeScript / React)...");
walk(path.join(process.cwd(), "frontend", "src"));

console.log("--> Scanning internal & cmd (Go)...");
walk(path.join(process.cwd(), "internal"));
walk(path.join(process.cwd(), "cmd"));

if (violations > 0) {
  console.error(`\nFAILED: Found ${violations} Anti-AI-Slop violations!`);
  process.exit(1);
}

console.log("PASS: 100% clean code verified across TypeScript and Go (0 emojis, 0 empty catches, 0 'as any', 0 <em> tags, 0 unannotated blank discards).");
