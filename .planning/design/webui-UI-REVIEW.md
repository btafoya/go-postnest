# UI Review — PostNest Webmail (Frontend)

> Retroactive 6-pillar visual audit. Scope: `web/src/` React frontend (9 components + Tailwind design system).
> No GSD phase — audited against `.planning/design/DESIGN.md` contract + abstract 6-pillar standards.
> Date: 2026-05-17 · Scale: 1 (poor) → 4 (excellent)

---

## Overall: 17 / 24

| Pillar | Score |
|--------|-------|
| Copywriting | 3/4 |
| Visuals | 3/4 |
| Color | 3/4 |
| Typography | 3/4 |
| Spacing | 3/4 |
| Experience Design | 2/4 |

Solid Gmail-clone execution. Design system disciplined, palette coherent. Experience layer drags: error UX inconsistent (`alert()` vs inline), no toast component (DESIGN.md §10 lists `Toast.jsx` — absent), accessibility gaps, modals not keyboard-trapped.

---

## 1. Copywriting — 3/4

**Strong:**
- Empty states have primary + secondary line (`Inbox` "No messages / Your inbox is empty", `Contacts` "Add your first contact to get started"). Good hierarchy.
- Search-aware empty state: `Inbox.jsx:149` swaps copy when `searchQuery` set ("Try a different search"). Context-sensitive — above baseline.
- Button verbs concrete: "Sign in", "Compose", "New Contact", "New Event".
- Batch failure message specific: `Inbox.jsx:75` "{N} of {M} message(s) failed to {action}" — counts + action, not generic.

**Gaps:**
- "User management coming soon" / "Security settings coming soon" (`Admin.jsx:160,167`) — placeholder copy shipped to prod. Either build or hide the tab.
- Generic alerts: `Contacts.jsx:43` `alert('Failed to save contact')`, `Calendar.jsx:72` `alert('Failed to save event')` — no cause, no recovery hint. Loses server error message that other components surface.
- "Sign in to Webmail" (`Login.jsx:36`) — product is "PostNest"; "Webmail" generic mismatch with brand wordmark directly above.

**Fix:** Replace `alert()` copy with server `error.message`; remove or gate "coming soon" tabs; align Login subhead to brand.

---

## 2. Visuals — 3/4

**Strong:**
- Consistent iconography — `lucide-react` throughout, uniform `h-4 w-4` sizing. No icon-set mixing.
- Avatar pattern reused coherently (initial-in-circle, `bg-primary-100 text-primary-700`) across Layout, MessageView, Contacts.
- Loading states present everywhere (spinner: `border-2 border-primary-500 border-t-transparent animate-spin`) — consistent component, not ad-hoc.
- Card abstraction (`.card`) gives uniform elevation/border treatment.

**Gaps:**
- Spinner duplicated inline 5× (App, Inbox, MessageView, Contacts, Admin) instead of shared `<Spinner>` — visual consistency holds only by copy-paste discipline; will drift.
- No favicon/branding verification — `index.html` not audited for icon (out of component scope but flagged).
- Modal backdrop `bg-black/50` raw vs design tokens — only place bypassing `surface` palette. Minor.
- Star toggle uses `fill-yellow-400` — yellow not in Tailwind theme (`tailwind.config.js` defines only `primary`/`surface`). Off-palette accent.

**Fix:** Extract `<Spinner>`; add `accent`/`warning` colors to theme for star/notice rather than raw Tailwind yellows/ambers.

---

## 3. Color — 3/4

**Strong:**
- Disciplined two-ramp system: `primary` (Google blue) + `surface` (Google grey 50–900). Maps cleanly to Gmail reference. DESIGN.md intent honored.
- Semantic state colors consistent: red-50/red-700 errors (`Login`, `Compose`), amber-50/amber-800 notices (`Inbox`), green/red status dots (`Admin`).
- Unread emphasis via `bg-blue-50` + `font-semibold` (`Inbox.jsx:159`) — clear read/unread contrast.
- Focus rings defined in `.input-field`/`.btn-primary` (`focus:ring-2 focus:ring-primary-500`) — color used for a11y, not just decoration.

**Gaps:**
- Off-theme colors leak: `yellow-400` (stars), `amber-50/800` (notice), `red-50/700` (errors), `green-500/red-500` (status) — all raw Tailwind, none in `tailwind.config.js`. Theme defines 2 ramps, code uses ~6. Token system incomplete.
- `bg-blue-50` unread (`Inbox.jsx:159`) ≠ `primary-50` (`#e8f0fe`) — two different blues for related meaning. Should be `primary-50`.
- No dark mode — acceptable for v1, but no `dark:` infra at all.

**Fix:** Promote semantic colors (`success`, `warning`, `danger`, `star`) into theme; replace `bg-blue-50` with `bg-primary-50`.

---

## 4. Typography — 3/4

**Strong:**
- Single typeface (Roboto, `index.css:7`) — matches Gmail/Material intent. Sane system fallback stack.
- Size scale restrained: `text-xs`/`text-sm`/`text-lg`/`text-xl`/`text-2xl` only. No arbitrary sizes except `text-[10px]` calendar chips (defensible — dense grid).
- Weight semantics consistent: `font-semibold` headings/unread, `font-medium` labels/active nav, default body. Reads as deliberate hierarchy.
- `truncate` + `min-w-0` applied correctly on flex children (Inbox sender/subject, Layout user) — no overflow blowout.

**Gaps:**
- Antialiasing forced (`-webkit-font-smoothing: antialiased`, `index.css:8`) — common but flattens weight on some displays; cosmetic preference, not a bug.
- `text-[10px]` calendar event chips (`Calendar.jsx:161`) below 12px — borderline legibility, especially the `+N more`.
- No line-height tuning anywhere — relies on Tailwind defaults. Message body (`prose`) inherits Typography plugin defaults but `@tailwindcss/typography` not in `plugins: []` (`tailwind.config.js:32`). `prose` class is a **no-op** — message body + editor have no readable measure/leading.

**Fix:** `prose` is dead — either add `@tailwindcss/typography` plugin or stop using `prose`/`prose max-w-none` (MessageView.jsx:120, RichEditor.jsx:48). Currently misleading.

---

## 5. Spacing — 3/4

**Strong:**
- Consistent rhythm: `px-4 py-2` toolbars, `px-4 py-3` headers, `p-4`/`p-6`/`p-8` content. Reads as a scale, not random.
- Gap utilities uniform (`gap-2`/`gap-3`/`gap-4`) — flex spacing systematic.
- Inbox row grid `grid-cols-[200px_1fr_80px] gap-4` — deliberate column rhythm, sender/subject/date alignment holds.
- Section dividers via `border-b border-surface-200` consistent — visual grouping without spacing guesswork.

**Gaps:**
- Fixed `200px` sender column (`Inbox.jsx:175`) — not responsive; long names truncate hard on narrow viewports, wastes space on wide.
- No responsive breakpoints in core mail flow. `md:`/`lg:` only in Contacts/Admin grids. Inbox/MessageView/Compose are desktop-fixed — sidebar `w-64` + fixed grid will break < 768px. DESIGN.md silent on mobile; still a gap for "production".
- Calendar cells `min-h-[100px]` arbitrary — fine, but event overflow (`slice(0,3)`) loses data silently beyond visual `+N more`.
- Compose `max-w-3xl mx-auto` but parent already padded `px-6` — double horizontal constraint, slightly cramped on mid widths.

**Fix:** Make Inbox sender column responsive (`minmax`); add mobile breakpoints to mail flow or document desktop-only.

---

## 6. Experience Design — 2/4

**Strong:**
- Autosave with debounce (`Compose.jsx:64`, 3s) + "Saved {time}" feedback. Real product behavior.
- Optimistic-ish flows: star toggle in MessageView updates local state immediately (`MessageView.jsx:34`).
- SSE live refresh (`Inbox.jsx:49` `message:new`) — inbox updates without poll.
- Drag-drop attachments (`Compose.jsx:109`) — expected webmail affordance, present.
- Sandboxed iframe HTML render (`MessageView.jsx:113` `sandbox=""` + DOMPurify) — security UX done right, matches DESIGN.md §1.4.
- Destructive confirm on delete (`Contacts.jsx:58`, `Calendar.jsx:76` `confirm()`).

**Gaps (this is where it bleeds):**
- **Inconsistent error UX**: `Login`/`Compose`/`Inbox` use inline styled banners; `Contacts`/`Calendar` use native `alert()`/`confirm()`. Two error paradigms in one app. DESIGN.md §10 explicitly lists `web/src/components/Toast.jsx` — **not implemented**. Design contract unmet.
- **No accessibility baseline**: icon-only buttons (Inbox toolbar, MessageView toolbar, star, pagination) have no `aria-label`. `title` on some, missing on most. Screen-reader unusable for core mail actions.
- **Modals not accessible**: Contacts/Calendar/event modals — no focus trap, no `Escape` close, no `role="dialog"`/`aria-modal`, no focus restore. Backdrop click doesn't dismiss. Keyboard users trapped behind modal.
- **No keyboard shortcuts** — DESIGN.md §6 AGENTS amendment lists "keyboard shortcuts" as unchanged feature. Absent. (`j/k`, `e`, `c` etc.)
- **Autosave failures silently swallowed** (`Compose.jsx:58` empty catch) — user believes draft saved, may not be. Only surfaced on explicit send. Data-loss risk with no signal.
- **Forward button is dead** (`MessageView.jsx:66`) — no `onClick`. Ships a non-functional control.
- **ReplyAll imported, never rendered** (`MessageView.jsx:4`) — incomplete feature surface.
- **No optimistic UI on Inbox actions** — every star/batch/delete triggers full `fetchMessages()` refetch. Janky on slow networks; selection lost, scroll resets.
- **Calendar field mismatch risk**: writes `start_time`/`end_time` (`Calendar.jsx:58`), reads `e.start_time || e.start` (`:47`) — defensive dual-read signals unsettled API contract; DESIGN.md §3.4 says verify-at-implement, unverified here.
- **No `confirm()` on MessageView delete** (`MessageView.jsx:27`) — Contacts/Calendar confirm destructive deletes, MessageView deletes immediately + navigates away. Inconsistent destructive-action safety.

**Fix priority:**
1. Build `Toast.jsx`, route all errors through it — kill `alert()`. (Design contract.)
2. Add `aria-label` to every icon-only button + `role="dialog"`/focus-trap/Esc to modals. (A11y floor.)
3. Wire or remove Forward + ReplyAll. (No dead controls in prod.)
4. Surface autosave failure (stale-draft indicator). (Data-loss signal.)
5. Confirm-guard MessageView delete. (Consistent destructive safety.)

---

## Top Fixes (ranked)

1. **`prose` is a no-op** — `@tailwindcss/typography` not registered; message body + compose editor have zero readable typography. Add plugin or remove class. (Typography, silent failure.)
2. **No `Toast.jsx`, `alert()` in Contacts/Calendar** — design contract §10 unmet, jarring native dialogs. (Experience.)
3. **Accessibility floor missing** — icon buttons unlabeled, modals not keyboard-accessible. Not production-grade. (Experience.)
4. **Dead/incomplete controls** — Forward no handler, ReplyAll unrendered. (Experience.)
5. **Off-theme color sprawl** — 6 color families, theme defines 2; `bg-blue-50` ≠ `primary-50` for unread. (Color.)
6. **Mail flow not responsive** — fixed `w-64` sidebar + `200px` grid col break sub-tablet. (Spacing.)

---

## Notes

- Architecture clean: consistent component shape, sane state, good separation. Audit is about *visual/UX polish*, not code structure — code structure is sound.
- Security UX (iframe sandbox, DOMPurify, double-submit CSRF per DESIGN.md) is above typical — credit where due.
- Biggest gap is the **experience layer finishing**: contract-specified Toast missing, a11y absent, two dead controls. These are the difference between "looks done" and "is done".
