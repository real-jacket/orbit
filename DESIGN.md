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
- Pointer navigation into or out of a directory uses one 160ms map transition with an 8px horizontal offset;
  opening small-item details uses 120ms. Keyboard navigation stays immediate and rapid navigation cancels the previous transition.
  Inspector tabs (子项 / 大文件 / 折叠项) switch instantly; the map never resizes when a tab changes.
  Hidden windows present the final state; hiding a window, keyboard input, or enabling reduced motion cancels both map and detail movement. Nothing loops except
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
- AI cleanup names the detected tool and data type, separates measured/reviewable/retained bytes within the current tool and keyword scope, and includes recognized whitelist-excluded caches in tool discovery without counting unmeasured bytes. Category counts and exclusion evidence use that same scope. Filtering never silently changes the global selection: hidden selected paths show their count and size before review, and clearing display filters retains the selection. Tool filtering keeps its native keyboard focus as controls change; removal previews only explicitly selected cache paths or eligible old diagnostic files.
- AI diagnostic cleanup retains the last 30 days, newest file, `latest` target, active/open files and uncertain state. Credentials, config, history indexes, task checkpoints, worktrees and models remain read-only; old diagnostic files are never described as recoverable caches or preselected.
- Expired agent sessions are the only removable conversation history: whole Codex rollouts and Claude Code session units (transcript, `<uuid>/`, file history) untouched for 90 days, outside the newest 5 per tool or project, and not open. They are review tier in their own “过期会话” band, never preselected, say they cannot be rebuilt, go to the Trash with a restore entry, and are re-verified (age, ranking, open state) at move time. Everything else under session roots stays read-only.
- A tool whose data remains without a launcher or app is marked “已卸载” in the filter and band; the label never relaxes retention or exposes credentials.
- Uninstall lists launchd items matched by exact bundle id (or its dotted children) or an executable inside the bundle. User LaunchAgents are reviewable and explain that a loaded job stays loaded until logout; `/Library` agents and daemons are listed with “需要管理员权限，仅列出” and are not selectable.
- Last-used time comes only from Spotlight; without a record the row says “未记录” and sorts last. Sorting by size, last use or name keeps the selected app by id.
- A running app shows “运行中”, its bundle cannot be selected or moved, and “退出应用” only requests a normal quit; the state changes only after a rescan observes it.
- Empty scans, absent data and failed scans are different states. A whitelist is not proof of why a list is empty.
- Zero-allocation and unmeasured samples remain inspectable without drawn area; unmeasured sizes stay unknown, never 0 B.
- Analysis focus follows the real path and action across sorting and folding, not array or button indices.
- Micro tiles never gain area from padding or borders; readable names and actions stay in the details panel.
- Responsive focus recovery only repairs layout-hidden targets in the same scope; explicit new focus or background clicks take precedence.
- Search clearing preserves a still-present inspected path; small-item arrows update inspection as well as focus.
- Draft cleanup sizes use readable B/KB/MB/GB units; unmeasured paths stay unknown and are separate from measured totals.
- Micro map tiles leave Tab order; keyboard movement and directory return use the same path's readable details instead of an invisible focus target.
- Loaded directory focus prefers its map, then the inspector tab that holds the same path, before ancestor controls, including aggregate-only layers.
- Space analysis is map first: one summary card (back/forward, breadcrumb title, scoped stats, volume bar), then the
  treemap beside a 340px inspector (300px below 1100px) whose bottom aligns with the map. The inspector shows the
  selection with its actions, then tabs 子项 / 大文件 / 折叠项. An empty 折叠项 tab stays visible but `aria-disabled`.
  Activating the folded tile or focusing a hidden path switches to the tab that holds it. Tiles carry no action buttons.
- Small-item names use Backspace and Left to return to the parent just like directory rows; search-field editing remains native.
- The menu-bar overview shows CPU, native memory pressure, available storage and network rates; battery is conditional and detailed process actions stay in the main window.
- Menu-bar CPU uses a fixed 0–100% trend; network directions share a rate scale and name their real sample count. Memory segments are application, wired, compressed and remaining capacity, with cached files included in the remainder rather than counted twice. The quick-clean entry opens the current selection's frozen path review; optional file-cache release explains administrator authorization and possible reloading slowdown before submission, and never reports invented freed application memory.
- Runtime and permission guidance identifies the exact running bundle, separates readability from authorization, and never promises to grant Full Disk Access itself.
- Runtime permission rows distinguish readable, denied, unchecked, missing and unknown outcomes with text as well as color. Desktop, Documents and Downloads are probed only by an explicit check and retain their observation times; App Management is confirmed in Settings, and administrator authorization is per operation. Long guidance is expandable and the modal footer stays visible while the results scroll.
- Native maintenance needs a correlated command receipt; stale diagnostics cannot animate into a new result.
- Light-mode action ink is `#fcfaf7` on `#a74426`; dark-mode action ink stays `#1a1613`.
- Search is contextual; unsupported modules disable it explicitly.
- Space analysis searches the current tree level only, never implies a full-disk search, and keeps the active
  query visible within that level. Changing directories clears the old scope's filter; changing sort order keeps
  the inspected path. Below 960px the map stacks before the inspector at 360px or taller, and the summary
  stays compact so the disk picture remains in the first viewport.
- Folded space is inspectable through a bounded sample, not an inert “其它” tile or an empty-list message.
  A sample row names its exact path and size; remaining unreported children are sent to Finder instead of
  pretending the sample is complete.
- Space analysis uses one squarified geometry for both the current layer and bounded direct-child previews inside
  large directory tiles. Preview cells sit in an inset map below the parent title and above its totals; child labels
  appear only when the cell can carry a name and measured size. Preview cells are visual only; selection and
  drill-down keep one action model on the parent.
- Space analysis shows allocated/logical size, file count and newest modification separately. One “查看折叠项”
  intent exists per surface; the map, summary and tile must not sprout competing duplicate CTAs.
- Read restrictions are grouped by the directory a user would open in Finder. Every group keeps a path action and
  the recovery panel names the exact permission pane; it never reduces the fix to a generic “go to Settings” button.
- In the space-analysis context bar, reveal and copy are icon actions with accessible names; the cleanup action keeps
  its visible “待清理” label because its intent is not a native toolbar convention.
- Space-analysis inspection updates in place: single click or Space inspects, double click or Enter enters a directory.
  Map arrows move focus; list arrows inspect the focused row. Inner action buttons keep their native keyboard behavior,
  and an asynchronous child result restores focus in its own map or list. Empty directories retain a return action.
- Small-tile hover details are an overlay inside the map's own bottom edge, never reserve breadcrumb width or shift layout. Child previews
  use their inset area's actual aspect ratio, and small allocated sizes use B/KB/MB rather than rounding to zero GB.
- Sparse layers keep slim tiles with their real paths. Unreadable tiles and dense folded items share a normally sized,
  detail list in the inspector's 折叠项 tab, including exact paths, measured sizes and 28px navigation actions.
  Areas retain their measured proportions; selection and list scrolling survive collection changes and resizing.
- App and leftover icon plates stay 38px square in rows and 46px in the inspector. Text flexes independently;
  fallback plates have no shadow, and real bundle artwork keeps its own shape.
- Status trends show actual display samples with units and scales. Memory occupancy and native memory pressure remain
  separate; occupancy alone does not reduce the reference score. Opening the tray reuses the latest scan snapshot.
