---
name: MQVision
description: Utility gas-meter monitoring dashboard — precise dials, quiet teal status
colors:
  bg: "#f7fbfa"
  surface: "#ffffff"
  fg: "#141b24"
  muted: "#454e58"
  border: "#d7e0de"
  border-strong: "#bac7c4"
  primary: "#158374"
  primary-ink: "#00463c"
  accent: "#158374"
  accent-soft: "#daf6ef"
  danger: "#c53637"
  danger-soft: "#ffe7e4"
  chart: "#037465"
  image-well: "#ecf4f2"
  skeleton: "#e1eae8"
  focus: "#158374"
typography:
  display:
    fontFamily: "Fira Code, ui-monospace, monospace"
    fontSize: "2.75rem"
    fontWeight: 700
    lineHeight: 1.05
    letterSpacing: "-0.025em"
  title:
    fontFamily: "Fira Sans, IBM Plex Sans, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  body:
    fontFamily: "Fira Sans, IBM Plex Sans, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "normal"
  label:
    fontFamily: "Fira Sans, IBM Plex Sans, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: "normal"
  mono:
    fontFamily: "Fira Code, ui-monospace, monospace"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: "normal"
rounded:
  sm: "4px"
  md: "8px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "12px"
  lg: "16px"
  xl: "24px"
  2xl: "32px"
  3xl: "48px"
components:
  status-ok:
    backgroundColor: "{colors.accent-soft}"
    textColor: "{colors.fg}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    height: "44px"
  status-bad:
    backgroundColor: "{colors.danger-soft}"
    textColor: "{colors.danger}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    height: "44px"
  button-refresh:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.fg}"
    rounded: "{rounded.sm}"
    padding: "0 12px"
    height: "44px"
  button-refresh-hover:
    backgroundColor: "{colors.accent-soft}"
    textColor: "{colors.fg}"
    rounded: "{rounded.sm}"
    padding: "0 12px"
    height: "44px"
  chart-shell:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.fg}"
    rounded: "{rounded.md}"
    padding: "12px 8px"
---

## Overview

**North star:** Utility-closet tablet at noon — precise meter dials, quiet teal status.

MQVision’s web UI is a single-purpose product surface: glance at the latest gas-meter reading, confirm the camera proof, and scan a 7-day trend. Design is **restrained**: near-pure surfaces, one teal accent for live/healthy state, danger red only for faults. Density and tabular numbers beat decoration. The tool should disappear into the monitoring task.

Canonical tokens live as OKLCH CSS variables in `web/src/index.css`. Hex values in this frontmatter are sRGB approximations for tooling that requires hex.

## Colors

| Role | OKLCH (canonical) | Use |
|------|-------------------|-----|
| bg | `oklch(0.985 0.004 180)` | Page ground |
| surface | `oklch(1 0 0)` | Panels, controls |
| fg | `oklch(0.22 0.02 250)` | Primary text |
| muted | `oklch(0.42 0.02 250)` | Labels, secondary |
| primary / accent | `oklch(0.55 0.095 180)` | Brand + healthy status |
| primary-ink | `oklch(0.35 0.07 180)` | Logo / title |
| danger | `oklch(0.55 0.18 25)` | Faults, alerts |
| chart | `oklch(0.5 0.09 180)` | Trend stroke |

Accent ≤10% of the surface. Do not tint the page background warmly; mood lives in teal + typography.

## Typography

- **UI / labels:** Fira Sans (system fallbacks). Fixed rem scale — no fluid `clamp` headings.
- **Data / times / readings:** Fira Code with `font-variant-numeric: tabular-nums`.
- Scale: 0.75 / 0.875 / 1 / 1.125 / 1.5 / 2.75 rem.
- Section labels are sentence-case Korean at `text-sm` weight 600 — never uppercase tracked eyebrows.

## Elevation

Flat product UI. Separation via 1px borders (`--border`) and spacing, not shadows. Chart/status shells use a single border + `--radius` (8px). No glassmorphism, no wide drop shadows.

## Components

- **Topbar:** brand + status chips + refresh clock; wraps on narrow viewports.
- **Status chip:** min 44×44; text states `정상` / `이상` / `확인 중`; fault detail inline (not `title`-only).
- **Reading band:** value + meta list beside camera frame; divider, not card grid.
- **History:** area chart (decorative to AT) + screen-reader summary + `<details>` data table.
- **Refresh:** bordered button, `aria-busy` while loading.
- **Skeleton:** shimmer placeholders; respect `prefers-reduced-motion`.

## Do's and Don'ts

**Do**
- Prefer partial data (`Promise.allSettled`) over blanking the whole dashboard.
- Keep touch targets ≥44px; focus rings via `:focus-visible`.
- Use teal only for healthy/live cues; danger for faults.
- Ship empty states that explain what will appear next.

**Don't**
- Don’t use identical bordered card grids or uppercase section kickers.
- Don’t put error causes only in native `title` tooltips.
- Don’t pair border + soft multi-layer shadows on chrome.
- Don’t invent a second accent for decoration; stay restrained.
