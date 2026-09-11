# Design System & DNA Specification: Nord Launcher

/* Hallmark · genre: modern-minimal · theme: Nordic Dark-Tech · 8-state discipline · zero-slop */

## 1. Aesthetic Direction & Design Read
- **Product**: Next-generation Minecraft Launcher for players, modpack creators, and server administrators.
- **Aesthetic Family**: Sleek Nordic Precision / Tactical Dark-Tech.
- **Tone**: Utilitarian, fast, high-contrast, tactile, zero marketing fluff, zero AI-generated clichés.
- **Design Dials**:
  - `DESIGN_VARIANCE: 6` (balanced structure with asymmetrical instance cockpit).
  - `MOTION_INTENSITY: 4` (crisp spring micro-motion 100-200ms; strictly user-action triggered).
  - `VISUAL_DENSITY: 8` (dense desktop ergonomics: 8px/12px/16px spacing scale, high information density).

## 2. Strict Anti-AI-Slop Blacklist
1. **NO AI-Purple / Neon Glows**: Banned purple mesh backgrounds, blurred neon blobs, and glowing borders.
2. **NO Cliché 3-Card Rows**: Banned equal 3-column feature cards with centered icons.
3. **NO Fake Drawn Chrome**: Banned fake browser dots, mock traffic lights, and simulated URL bars.
4. **NO Emoji Icons**: Interface icons must exclusively use the Lucide SVG icon family with `stroke-width: 1.5px`.
5. **NO Italic Headers**: Display headings are always Roman (`font-style: normal`). No mid-sentence italic emphasis.
6. **NO Eyebrow Spam**: Maximum one small uppercase category tag per screen, only when functionally required.
7. **NO Serif Fonts**: Banned Fraunces, Instrument Serif, or decorative serifs in the launcher UI.
8. **NO Fabricated Metrics**: No fake statistics, counts, or unverified claims.
9. **NO Apology Copy**: Error states must provide exact technical reason and an actionable recovery button.

## 3. Typography
- **UI & Display**: `Geist Sans`, `Satoshi`, or system `Segoe UI Variable / Inter`.
- **Monospace (Logs, Hashes, JVM flags, RAM meters)**: `JetBrains Mono` with `font-variant-numeric: tabular-nums`.
- **Scale**:
  - Display Title: `text-2xl font-semibold tracking-tight`
  - Section Header: `text-lg font-medium`
  - Body Text: `text-sm leading-relaxed text-zinc-300`
  - Metadata / Pills: `text-xs font-mono font-medium`

## 4. Color Palette (Locked Tokens)
```css
:root {
  --bg-app: #09090b;             /* zinc-950 base */
  --bg-surface: #121215;         /* elevated surface */
  --bg-card: #18181b;            /* interactive card surface */
  --bg-card-hover: #222226;      /* card hover state */
  --border-subtle: rgba(255, 255, 255, 0.08);
  --border-focus: #00d4b2;       /* Nordic Cyan */
  
  --text-primary: #f4f4f5;       /* zinc-100 */
  --text-secondary: #a1a1aa;     /* zinc-400 */
  --text-muted: #71717a;         /* zinc-500 */
  
  --accent-cyan: #00d4b2;        /* primary action / active state */
  --accent-cyan-hover: #00bfa0;
  --accent-amber: #ffb224;       /* warning / staging status */
  --accent-rose: #f43f5e;        /* error state */
  --accent-emerald: #10b981;     /* success state */
}
```

## 5. Mandatory 8-State Interactive Discipline
Every interactive component (Buttons, Cards, Inputs, Toggles) must provide styling for:
1. `default`: rest state with calibrated contrast.
2. `hover`: subtle brightness lift (+3-5%) without geometry shift.
3. `:focus-visible`: clear 2px outline `ring-2 ring-[#00d4b2]` with 2px offset.
4. `:active`: tactile compression (`transform: scale(0.98)`).
5. `disabled`: `opacity-40 cursor-not-allowed`.
6. `loading`: spinner/pulse maintaining exact container dimensions (zero layout shift).
7. `error`: border and text tinted `--accent-rose` with clear inline error message.
8. `success`: brief completion feedback with `--accent-emerald`.
