const fs = require("fs");
const path = require("path");

const emojiRegex = /[\u{1F300}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}]/u;
let violations = 0;

function walk(dir) {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir)) {
    const full = path.join(dir, entry);
    const stat = fs.statSync(full);
    if (stat.isDirectory()) {
      walk(full);
    } else if (entry.endsWith(".ts") || entry.endsWith(".tsx")) {
      const content = fs.readFileSync(full, "utf8");
      
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
  }
}

console.log("==> Running Anti-AI-Slop Pre-Flight Verification on frontend/src...");
walk(path.join(process.cwd(), "frontend", "src"));

if (violations > 0) {
  console.error(`\nFAILED: Found ${violations} Anti-AI-Slop violations!`);
  process.exit(1);
}

console.log("PASS: 100% clean code verified (0 emojis, 0 empty catches, 0 'as any', 0 <em> tags).");
