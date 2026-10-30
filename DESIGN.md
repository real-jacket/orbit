# Design System: orbit

## 1. Visual Theme & Atmosphere

A quiet instrument panel, not a dashboard. The surface is warm charcoal, the type is small and certain, and one
terracotta accent carries every action. Nothing glows, nothing floats: separation comes from hairline borders and
generous vertical rhythm, so the numbers read as measurements rather than decoration.

Dials: **Variance 5 / Motion 3 / Density 6**. This is Operate mode (someone finishing a task), so scanability,
consistency and truthful states outrank expression. Density is moderate, but expression is limited to typography,
one accent, and the honest presentation of data.

## 2. Colors — 调色板与用途

Neutral base (dark, the shipped theme):

- **Kiln Charcoal** `#111110` — window background
- **Cooled Ash** `#171715` — sidebar surface
- **Panel Ink** `#1b1a18` — panel and card fill
- **Raised Ink** `#21201d` — row surface, inputs that must read as raised
- **Sunken Ink** `#141412` — track fills, inputs
- **Whisper Line** `rgba(255,250,240,0.075)` — the only structural divider; 1px
- **Hairline Strong** `rgba(255,250,240,0.15)` — hover borders, modal edges
- **Chalk Ink** `#f4f2ed` — primary text
- **Warm Steel** `#a8a49b` — secondary text (AA on Panel Ink)
- **Muted Stone** `#8a867d` — meta text only, never body copy

Brand and semantics (one accent, four signals):

- **Terracotta** `#d97757` — the single accent: primary action, focus ring, active nav
- **Ink on Terracotta** `#1a1613` — text placed on the accent (white on terracotta fails AA at 11.5px)
- **Recoverable Green** `#58b78f` — safe to clean, healthy values
- **Review Amber** `#e7b35c` — needs confirmation, partial scans
- **Refusal Rose** `#ee8290` — failures, destructive confirmation
- **Data Blue** `#5b8def` — measured throughput / utilisation only

Banned in this product: purple accents (protection is communicated with a lock glyph and muted text, not a colour),
pure `#000000` / `#ffffff` fills, neon or outer glows, gradient text.

## 3. Typography — 排版规则

- **Display / page headline** — SF Pro (system), 20px / 1.35, 700, tracking `-0.02em`. One line; never wraps to three.
- **Section title** — 12.5px / 1.3, 700. Sits in the panel header, sentence case, no tracking tricks.
- **Row title** — 13px / 1.35, 600. Truncates with an ellipsis; the full value lives in the tooltip.
- **Body** — 12px / 1.5, 400, max 72ch (this is app copy, not an article).
- **Meta / unit labels** — 10.5px / 1.4, 500, `--text-muted`; never smaller than 10px.
- **Numbers** — SF Mono with `font-variant-numeric: tabular-nums`, always. Percentages, sizes and counts must not
  shift the layout when they change.
- Banned: Inter and other webfont substitutes (this is a native macOS app; SF is the correct voice), serif anywhere,
  letter-spacing above `0.06em`, all-caps labels longer than three words.

## 4. Components — 组件规范

- **Buttons** — 32px tall, 12px horizontal padding, radius 8, 11.5px/600. Primary: terracotta fill with **ink** text.
  Secondary: raised ink with a hairline border. Ghost: text only, appears on hover of its row. Every button shows
  hover, active (`translateY(0.5px)`), disabled (45% opacity, no pointer), and a 2px accent focus ring.
- **Panels** — Panel Ink fill, 1px Whisper Line, radius 12, **no shadow**. Elevation is declared once: borders for
  surfaces, shadow only for things that float (modal, tray, tooltip).
- **Rows** — 12px / 16px padding, hairline divider, hover tint, selected = accent tint plus a 1px inset accent ring.
  No coloured side bars.
- **Summary header** — 40px flat tinted icon tile (no gloss, no inner highlight), 20px headline, one 12px
  description, one right-aligned 26px stat without a decorative track. No chip rows, no repeated labels.
- **Inputs** — Label above, control below, helper text below that, 8px gap. Sunken Ink fill, hairline border,
  accent focus ring.
- **Loaders** — Skeleton rows that match the final row geometry (same heights, same paddings). No spinners.
- **Empty states** — Icon, one sentence naming what is missing, one action that fills it. Centred in the panel.
- **Modals** — Backdrop `rgba(0,0,0,0.55)` with 8px blur; sheet radius 14, shadow only here, header 12/18, footer 12/18.
- **Toasts** — Bottom-right, radius 10, raised ink, hairline border, 4s default, one line.
- **Scrollbars** — 9px, transparent track, stone thumb that darkens on hover. Selection uses accent at 28%.

## 5. Layout Principles

- Grid, never flex percentage math. Master-detail is `minmax(0,1.48fr) minmax(320px,1fr)` with a 12px gap.
- Spacing scale: 4 / 8 / 12 / 16 / 24 / 32. Anything else is a bug.
- Sidebar 236px, window content padding 14px 18px 18px, section gap 14px.
- A panel's header, body and footer share the same left/right inset (16px) so text columns line up across panels.
- Two panes that sit side by side share one height; long lists scroll inside their own card, never push the layout.
- Below 1100px the master-detail collapses to a single column and below 960px the sidebar narrows to 188px. No horizontal scroll.
- Nothing overlaps: no absolutely positioned labels over content except badges inside a tile's own padding box.

## 6. Motion & Interaction

- Easing `cubic-bezier(0.16, 1, 0.3, 1)`, 120ms for state, 180ms for movement. Only `transform`, `opacity`,
  `background-color`, `border-color` animate. `transition: all` is banned.
- One authored moment per surface: the panel/hero settles in on module switch (6px rise, 180ms). Nothing loops except
  the skeleton shimmer while a scan actually runs.
- Every interactive element is keyboard reachable, shows a visible focus ring, and answers `Enter` with its primary
  action.
- `prefers-reduced-motion: reduce` removes the rise and the shimmer; state changes become instant.

## 7. Anti-Patterns (Banned)

- No emoji or decorative glyphs standing in for icons; icons are drawn SVG in one stroke weight (the existing
  `get_skeuo_svg` set), used at 16/20/28px only.
- No glossy skeuomorphic tiles, inner highlights, or bevels. Depth is a hairline and one shadow scale.
- No coloured left borders on cards or rows (selection is a tint plus an inset ring).
- No card inside a card. If a panel needs grouping, use a hairline divider and space.
- No chip/badge rows in the page header; status is one line of text.
- No fake numbers, no fabricated percentages, no "全部通过" summaries without data behind them.
- No purple, no neon, no gradient text, no `#000`, no `#fff` fills.
- No dual "start scan" buttons (one intent, one label, in one place).
- No marketing verbs (提升 / 畅享 / 一键智能) and no hero copy longer than two lines.

## 8. Interaction Acceptance

- Selection is separate from inspection: selecting a row does not authorize deleting it.
- Confirmation lists exact paths and freezes the submitted set; background scans cannot expand it.
- Empty scans, absent data and failed scans are different states. A whitelist is not proof of why a list is empty.
- Native maintenance needs a correlated command receipt; stale diagnostics cannot animate into a new result.
- Light-mode action ink is `#fcfaf7` on `#a74426`; dark-mode action ink stays `#1a1613`.
- Search is contextual; unsupported modules disable it explicitly.
