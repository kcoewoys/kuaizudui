# Design System: fudai-v2 mobile activity hub

## 1. Visual Theme & Atmosphere

A calm, trustworthy mobile utility with a soft blue-white canvas and precise green action states. Density is balanced for repeated daily use; hierarchy comes from restrained borders, small surface shifts, and a single strong action color. Layout follows the supplied Stitch mobile screens at a 390 px design width.

## 2. Color Palette & Roles

- **Cloud Canvas** (`#F6F9FF`) — app and modal backdrop.
- **Pure Surface** (`#FFFFFF`) — cards, controls, and elevated containers.
- **Deep Ink** (`#0D1D29`) — primary text.
- **Slate Note** (`#5C6C7A`) — secondary text and metadata.
- **Whisper Border** (`#E1E5E8`) — 1 px structural lines.
- **Queue Blue** (`#EBF5FF`) — informational stat surface.
- **Reward Mint** (`#E3FCEF`) — positive stat surface.
- **Action Green** (`#006E2A`) — primary CTA, active state, success, and focus ring.
- **Pressed Green** (`#00531E`) — pressed and high-contrast green state.

Green is the only interactive accent. Activity illustrations may use contained red surfaces for category recognition, never for controls.

## 3. Typography Rules

- **Display and UI:** The native system sans stack, with Noto Sans SC as a Chinese fallback, avoids a render-blocking external font request.
- **Mobile title:** 18 px / 700.
- **Card title:** 16 px / 700.
- **Body:** 14 px / 1.55, maximum readable line length 65 characters.
- **Caption and metadata:** 12 px / 1.45.
- **Admin operational text:** 12 px minimum for navigation labels, controls, table values, metadata, status, validation, and record labels; hierarchy comes from weight and spacing rather than sub-12 px type.
- **Numbers:** tabular lining figures.
- **Banned:** Inter, generic serif type, decorative display fonts, and oversized marketing headlines.

## 4. Component Stylings

- **Primary buttons:** deep green fill, white label, 12 px radius, minimum 44 px touch height, 1 px downward press feedback.
- **Secondary buttons:** white surface, whisper border, deep ink label, no glow.
- **Cards:** white, 14–16 px radius, 1 px border, subtle blue-tinted shadow. Elevation only separates actionable groups.
- **Inputs:** label or explanatory text above; 12 px radius; green focus ring; validation below the field.
- **Image uploads:** a full-width dashed selector with a mint file icon, filename, accepted formats, size limit, immediate local preview, and explicit upload/cancel/remove actions. Never expose storage URLs as an administrator input.
- **Status chips:** pill shape, mint fill, green label, concise status text.
- **Modals:** bottom sheets on mobile, with an opaque white content surface, dimmed backdrop, safe-area padding, and no desktop-specific alternate layout.
- **Loaders:** layout-matched skeletons only; no circular spinners.
- **Admin gate:** non-dismissible, full-viewport verification gate with a bottom sheet sized for one-handed phone use. It asks only for a mobile number, exposes no close or bypass action, and never reveals the configured administrator number.
- **Admin navigation:** four labeled operational destinations—users and points, exchange codes, content configuration, and user feedback. Icons support the text labels but never replace them; the verified state and exit actions remain explicit.
- **Admin records:** label-preserving vertical record groups at every width; every value keeps its data label, and copy/recharge actions retain a minimum 44 px target.

## 5. Layout Principles

- Mobile-first single column, 390 px reference width, 16 px page gutters, 12 px vertical rhythm.
- The consumer app fills at least `100dvh`; desktop presentation centers the mobile surface without changing the interaction model.
- The administrator route is phone-only: a centered mobile application shell with a compact safe-area header, a single-column action-first work stream, and a fixed four-item labeled bottom navigation. The points section opens directly on recharge and recent records, without a user directory, search, or pagination.
- Admin data is always rendered as stacked record groups with visible field labels. Operational controls retain a minimum 44 px target, inputs use 16 px text to avoid mobile browser zoom, and no horizontal scrolling is required from 320 px upward.
- Primary actions use the full available width when they conclude a task.
- No horizontal scrolling or overlapping content.
- Shared activity logic uses the same visual component for buy-food, cash-turntable, cash-monopoly, and daily-cash variants.

## 6. Motion & Interaction

- Use weighty ease-out motion approximating `stiffness: 100, damping: 20`.
- Animate only `transform` and `opacity`.
- Cards enter with a subtle cascade; buttons depress by 1 px; active queue dots pulse gently.
- Honor `prefers-reduced-motion` and disable decorative movement when requested.

## 7. Anti-Patterns (Banned)

- No emojis as interface icons; use consistent SVG symbols.
- No pure black, neon glows, gradient text, glassmorphism, or saturated purple controls.
- No generic three-column feature rows, floating labels, custom cursors, or overlapping content.
- No filler prompts or decorative scroll arrows.
- No state hidden only by color; pair color with text or an icon.
