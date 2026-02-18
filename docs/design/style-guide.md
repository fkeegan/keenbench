# KeenBench UI Style Guide

## Status
Draft v1.0

## Design Philosophy
The visual language draws from Notion's clean, minimal aesthetic: generous whitespace, subtle shadows, and a focus on content over chrome. The key difference is a **warm blush undertone** that replaces Notion's cooler grays—creating a welcoming, approachable feel without sacrificing professionalism.

The warmth should be **barely perceptible**. Users shouldn't think "this app is pink." They should think "this app feels friendly."

---

## Target Environment
- **Platform**: Desktop only (macOS, Windows, Linux)
- **Resolution range**: 1080p (1920×1080) to 4K (3840×2160)
- **Scaling**: Design at 1x, ensure clarity at 2x DPI
- **No mobile or tablet considerations**

---

## Color Palette

### Foundation Colors

```
Background (Primary)     #FDFCFB    Very subtle warm white
Background (Secondary)   #F9F7F5    Slightly warmer for panels/sidebars
Background (Elevated)    #FFFFFF    Pure white for cards and modals
Background (Hover)       #F5F2EF    Warm hover state
Background (Selected)    #EDE8E4    Warm selection state
```

### Surface Colors

```
Surface (Subtle)         #FAF8F6    For input fields, code blocks
Surface (Muted)          #F3F0ED    Disabled states, dividers
Surface (Overlay)        rgba(253, 252, 251, 0.95)  Overlay backgrounds
```

### Text Colors

```
Text (Primary)           #1F1F1F    Near-black, high contrast
Text (Secondary)         #6B6560    Warm medium gray
Text (Tertiary)          #9C9590    Warm light gray, placeholders
Text (Disabled)          #C5C0BB    Disabled text
Text (Inverse)           #FDFCFB    Light text on dark backgrounds
```

### Border Colors

```
Border (Subtle)          #EBE7E3    Default borders
Border (Default)         #DDD8D3    Input borders, dividers
Border (Strong)          #C5C0BB    Emphasized borders
Border (Focus)           #8B8580    Focus rings (with shadow)
```

### Brand / Accent Colors

```
Accent (Primary)         #5B7FC2    Muted blue, primary actions
Accent (Primary Hover)   #4A6AAF    Hover state
Accent (Primary Active)  #3D5A9A    Active/pressed state
Accent (Secondary)       #7B9FD4    Secondary buttons, links
```

### Semantic Colors

```
Success (Background)     #F0F7F0    Light green background
Success (Border)         #A8D4A8    Green border
Success (Text)           #2E7D32    Green text

Warning (Background)     #FFF8E6    Light amber background
Warning (Border)         #F5D88C    Amber border
Warning (Text)           #B8860B    Amber text

Error (Background)       #FDF2F2    Light red background (warm)
Error (Border)           #F5B0AC    Red border (warm)
Error (Text)             #C53030    Red text

Info (Background)        #F0F5FA    Light blue background
Info (Border)            #A3C4E8    Blue border
Info (Text)              #1A5490    Blue text
```

### Special Purpose

```
Draft (Indicator)        #E8B86D    Warm gold for draft status
Published (Indicator)    #6BAF8D    Soft green for published status
Diff (Added)             #DCEDC8    Light green for additions
Diff (Removed)           #FFCDD2    Warm pink for deletions
Clutter (Low)            #A8D4A8    Green meter fill
Clutter (Medium)         #F5D88C    Amber meter fill
Clutter (High)           #F5B0AC    Warm red meter fill
```

---

## Typography

### Font Stack

```
Primary:     Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif
Monospace:   "JetBrains Mono", "Fira Code", Consolas, "Liberation Mono", monospace
```

Inter is preferred for its excellent legibility at all sizes and its OpenType features. Fall back to system fonts if Inter is unavailable.

### Type Scale

Base font size: **14px** at 1080p, scales proportionally with display density.

| Token          | Size   | Weight | Line Height | Use Case                          |
|----------------|--------|--------|-------------|-----------------------------------|
| `display`      | 32px   | 600    | 1.2         | Hero headings, empty states       |
| `heading-1`    | 24px   | 600    | 1.3         | Page titles, modal headers        |
| `heading-2`    | 20px   | 600    | 1.35        | Section headings                  |
| `heading-3`    | 16px   | 600    | 1.4         | Card headers, subsections         |
| `body`         | 14px   | 400    | 1.5         | Primary content                   |
| `body-medium`  | 14px   | 500    | 1.5         | Emphasized body text              |
| `small`        | 13px   | 400    | 1.45        | Secondary info, timestamps        |
| `caption`      | 12px   | 400    | 1.4         | Labels, hints, metadata           |
| `overline`     | 11px   | 600    | 1.3         | Category labels, all-caps badges  |
| `code`         | 13px   | 400    | 1.5         | Inline code, file names           |
| `code-block`   | 13px   | 400    | 1.6         | Code blocks, diffs                |

### Letter Spacing

```
Headings:     -0.01em (slight tightening)
Body:          0em (default)
Overline:      0.05em (loose tracking for all-caps)
Code:          0em
```

---

## Spacing System

Use a **4px base unit**. All spacing should be multiples of 4.

| Token   | Value  | Use Case                                      |
|---------|--------|-----------------------------------------------|
| `xs`    | 4px    | Icon padding, tight gaps                      |
| `sm`    | 8px    | Inline element gaps, compact lists            |
| `md`    | 12px   | Default component padding                     |
| `base`  | 16px   | Standard gaps, section margins                |
| `lg`    | 24px   | Card padding, larger component gaps           |
| `xl`    | 32px   | Section separators                            |
| `2xl`   | 48px   | Major layout divisions                        |
| `3xl`   | 64px   | Page margins at large viewport sizes          |

### Layout Margins

| Viewport Width  | Page Margin | Content Max Width |
|-----------------|-------------|-------------------|
| 1080p (1920px)  | 24px        | 960px             |
| 1440p (2560px)  | 32px        | 1080px            |
| 4K (3840px)     | 48px        | 1200px            |

Content should remain centered with a max-width to preserve readability. Sidebars and panels are fixed-width or proportional, not content.

---

## Border Radius

| Token      | Value | Use Case                                |
|------------|-------|-----------------------------------------|
| `none`     | 0px   | Sharp edges where needed                |
| `sm`       | 4px   | Small elements (badges, chips)          |
| `md`       | 6px   | Buttons, inputs, small cards            |
| `lg`       | 8px   | Cards, panels, modals                   |
| `xl`       | 12px  | Large panels, onboarding cards          |
| `full`     | 9999px| Pills, avatars, progress bars           |

---

## Shadows

Shadows use warm undertones (achieved by using a slightly warm shadow color rather than pure black).

```
Shadow Color Base:  rgba(100, 90, 80, alpha)
```

| Token       | Definition                                              | Use Case                    |
|-------------|---------------------------------------------------------|-----------------------------|
| `none`      | none                                                    | Flat elements               |
| `sm`        | 0 1px 2px rgba(100, 90, 80, 0.05)                       | Subtle lift                 |
| `md`        | 0 2px 4px rgba(100, 90, 80, 0.08)                       | Cards, raised elements      |
| `lg`        | 0 4px 12px rgba(100, 90, 80, 0.1)                       | Dropdowns, popovers         |
| `xl`        | 0 8px 24px rgba(100, 90, 80, 0.12)                      | Modals, dialogs             |
| `focus`     | 0 0 0 3px rgba(91, 127, 194, 0.25)                      | Focus rings                 |

---

## Component Specifications

### Buttons

#### Primary Button
- Background: `Accent (Primary)`
- Text: `#FFFFFF`
- Border: none
- Border radius: `md` (6px)
- Padding: 8px 16px
- Font: `body-medium`
- Shadow: `sm`
- Hover: `Accent (Primary Hover)`, shadow `md`
- Active: `Accent (Primary Active)`, shadow `none`
- Disabled: opacity 0.5, cursor not-allowed

#### Secondary Button
- Background: transparent
- Text: `Accent (Primary)`
- Border: 1px solid `Border (Default)`
- All other specs same as primary
- Hover: Background `Background (Hover)`

#### Ghost Button
- Background: transparent
- Text: `Text (Secondary)`
- Border: none
- Hover: Background `Background (Hover)`, Text `Text (Primary)`

#### Destructive Button
- Same structure as Primary
- Background: `Error (Text)` → `#C53030`
- Hover: Darken 10%

### Inputs

#### Text Input
- Background: `Surface (Subtle)`
- Border: 1px solid `Border (Default)`
- Border radius: `md` (6px)
- Padding: 10px 12px
- Font: `body`
- Text: `Text (Primary)`
- Placeholder: `Text (Tertiary)`
- Focus: Border `Accent (Primary)`, Shadow `focus`
- Error: Border `Error (Border)`, Background `Error (Background)`

#### Text Area
- Same as Text Input
- Min height: 80px
- Resize: vertical only

### Cards

- Background: `Background (Elevated)`
- Border: 1px solid `Border (Subtle)`
- Border radius: `lg` (8px)
- Padding: `lg` (24px)
- Shadow: `md`
- Hover (when interactive): Shadow `lg`, border `Border (Default)`

### Modals / Dialogs

- Background: `Background (Elevated)`
- Border radius: `lg` (8px)
- Shadow: `xl`
- Overlay: `rgba(31, 31, 31, 0.4)` with blur (8px)
- Max width: 560px (standard), 720px (wide), 400px (narrow)
- Padding: `lg` (24px)
- Header: `heading-2`, bottom border `Border (Subtle)`, padding-bottom `md`
- Footer: top border `Border (Subtle)`, padding-top `md`, flex-end alignment

### Sidebar / Navigation Panel

- Background: `Background (Secondary)`
- Width: 240px (collapsible to 64px icon-only)
- Border-right: 1px solid `Border (Subtle)`
- Navigation items:
  - Padding: 8px 12px
  - Border radius: `md` (6px)
  - Hover: Background `Background (Hover)`
  - Selected: Background `Background (Selected)`, text `Accent (Primary)`

---

## Component: Chat Interface (Workshop)

The Workshop chat follows Notion's clean conversation style.

### Message Container
- Max width: 720px (centered)
- Padding: `lg` between messages

### User Message
- Align: right
- Background: `Accent (Primary)` at 10% opacity → `rgba(91, 127, 194, 0.1)`
- Border radius: `lg` (8px)
- Padding: 12px 16px
- Text: `Text (Primary)`
- Max width: 80% of container

### AI Message
- Align: left
- Background: `Background (Elevated)`
- Border: 1px solid `Border (Subtle)`
- Border radius: `lg` (8px)
- Padding: 12px 16px
- Text: `Text (Primary)`
- Max width: 80% of container

### Message Actions (hover)
- Small icon buttons (`Ghost Button` style)
- Appear on hover with fade-in (150ms)
- Position: top-right of message
- Icons: Copy, Undo, Regenerate

### Streaming Indicator
- Pulsing dot using `Accent (Primary)` with 50% opacity
- Animation: opacity 0.3 → 1.0, 800ms ease-in-out, infinite

### Input Area
- Sticky to bottom
- Background: `Background (Primary)`
- Top border: 1px solid `Border (Subtle)`
- Padding: `base` (16px)
- Input: Full-width Text Area, max 5 lines before scroll
- Send button: Primary Button, right-aligned

---

## Component: File List (Workbench)

### File Item
- Background: transparent
- Padding: 10px 12px
- Border radius: `md` (6px)
- Hover: Background `Background (Hover)`
- Selected: Background `Background (Selected)`, border-left 3px `Accent (Primary)`

### File Icon
- Size: 20px × 20px
- Color: `Text (Tertiary)` (use file-type specific icons)

### File Name
- Font: `body`
- Color: `Text (Primary)`
- Truncate with ellipsis if too long

### File Meta (type, status)
- Use compact badges for file extension and status (`Read-only`, `Opaque`)
- Font: `caption`
- Color: `Text (Tertiary)`
- Do not show file size text in Workbench rows by default

### Unsupported File Indicator
- Italic file name
- Badge: "Binary" in `overline` style, background `Surface (Muted)`

---

## Component: Diff View (Review)

### File Change Header
- Background: `Surface (Subtle)`
- Padding: 8px 12px
- Border: 1px solid `Border (Subtle)`
- Border radius: `lg` top corners only
- File name: `body-medium`
- Change badge: Added (green), Modified (amber), Deleted (red)

### Line Diff (Text Files)
- Font: `code-block`
- Line numbers: `Text (Tertiary)`, width 48px, right-aligned
- Added line: Background `Diff (Added)`, left border 3px `Success (Border)`
- Removed line: Background `Diff (Removed)`, left border 3px `Error (Border)`
- Context line: Background transparent

### Side-by-Side Preview (Non-Text)
- Two panels, equal width
- Label: "Before" / "After" in `overline` style
- Border: 1px solid `Border (Subtle)`
- Background: checkerboard pattern for transparent images
- Zoom controls: icon buttons at top-right of each panel

### AI Summary (for non-text changes)
- Background: `Info (Background)`
- Border: 1px solid `Info (Border)`
- Border radius: `md` (6px)
- Padding: 12px 16px
- Icon: Info icon in `Info (Text)` color
- Text: `small`, `Info (Text)` color

---

## Component: Clutter Bar

### Container
- Height: 8px
- Background: `Surface (Muted)`
- Border radius: `full`
- Overflow: hidden

### Fill
- Border radius: `full`
- Transition: width 300ms ease-out
- Colors based on level:
  - 0–50%: `Clutter (Low)` → green
  - 51–75%: `Clutter (Medium)` → amber
  - 76–100%: `Clutter (High)` → warm red

### Warning Text (when high)
- Appears below bar
- Font: `caption`
- Color: `Warning (Text)`
- Text: "Workbench is cluttered — performance may be degraded"

---

## Component: Status Indicators

### Draft Status Badge
- Background: `Draft (Indicator)` at 15% opacity
- Text: `Draft (Indicator)` darkened
- Border: 1px solid `Draft (Indicator)`
- Border radius: `sm` (4px)
- Padding: 2px 8px
- Font: `caption`, weight 500

### Published Status Badge
- Background: `Published (Indicator)` at 15% opacity
- Text: `Published (Indicator)` darkened
- Border: 1px solid `Published (Indicator)`
- Other specs same as Draft

### Model Selector
- Style: Ghost Button with dropdown chevron
- Current model shown as text + provider icon
- Dropdown: standard dropdown panel styling

### Workbench Chrome Icons
- Checkpoints action: icon-only button in top-right app bar (`history` metaphor)
- Settings action: icon-only gear button anchored bottom-left in the file panel
- Icon buttons include tooltips and minimum 28px hit targets
- Keep icon color subtle (`Text (Secondary)`) with warm hover background

---

## Component: Progress & Loading

### Spinner
- Size: 20px (small), 32px (medium), 48px (large)
- Color: `Accent (Primary)`
- Style: Simple rotating ring (2px stroke)
- Animation: 800ms linear infinite

### Progress Bar
- Height: 6px
- Background: `Surface (Muted)`
- Fill: `Accent (Primary)`
- Border radius: `full`
- Animation: smooth width transition (200ms)

### Skeleton Loading
- Background: linear gradient animation
- Base: `Surface (Muted)`
- Shimmer: `Background (Hover)` → `Surface (Muted)`
- Animation: 1.5s ease-in-out infinite
- Border radius: Match component being loaded

---

## Icons

### Icon System
- Use a consistent icon set (recommend: Phosphor Icons or Lucide)
- Default size: 20px
- Default stroke: 1.5px
- Color: Inherit from parent (usually `Text (Secondary)`)

### Icon Sizing
| Size   | Pixels | Use Case                              |
|--------|--------|---------------------------------------|
| `xs`   | 14px   | Inline with small text                |
| `sm`   | 16px   | Inline with body text                 |
| `md`   | 20px   | Buttons, navigation items             |
| `lg`   | 24px   | Section headers, emphasis             |
| `xl`   | 32px   | Empty states, feature highlights      |

---

## Motion & Animation

### Principles
- Motion should be **subtle and purposeful**
- Avoid motion for motion's sake
- Use motion to:
  - Provide feedback (hover, click)
  - Guide attention (new content appearing)
  - Maintain context (transitions between states)

### Timing
| Token      | Duration | Easing              | Use Case                      |
|------------|----------|---------------------|-------------------------------|
| `instant`  | 0ms      | —                   | Immediate state changes       |
| `fast`     | 100ms    | ease-out            | Hover states, micro-feedback  |
| `normal`   | 200ms    | ease-out            | Most transitions              |
| `slow`     | 300ms    | ease-in-out         | Panel slides, modals          |
| `slower`   | 500ms    | ease-in-out         | Page transitions              |

### Standard Transitions
- Hover: `background-color 100ms ease-out`
- Focus: `box-shadow 150ms ease-out`
- Modal open: Fade in 200ms + scale from 0.95
- Sidebar expand: Width 300ms ease-in-out
- Dropdown: Fade in 150ms + translate-y from -8px

---

## Responsive Behavior

### Breakpoints
Since this is desktop-only, breakpoints are based on common desktop resolutions:

| Name        | Min Width | Target                     |
|-------------|-----------|----------------------------|
| `compact`   | 1280px    | Laptops, smaller monitors  |
| `standard`  | 1920px    | 1080p monitors             |
| `large`     | 2560px    | 1440p monitors             |
| `xlarge`    | 3840px    | 4K monitors                |

### Scaling Strategy
- Use logical pixels; let the OS handle DPI scaling
- Font sizes remain constant in logical pixels
- Spacing scales slightly at larger breakpoints
- Content max-width increases at larger breakpoints
- Sidebar width remains fixed; content area expands

### Panel Widths

| Component         | Compact  | Standard | Large    | XLarge   |
|-------------------|----------|----------|----------|----------|
| Sidebar           | 200px    | 240px    | 260px    | 280px    |
| Content Max       | 800px    | 960px    | 1080px   | 1200px   |
| Chat Max          | 640px    | 720px    | 800px    | 880px    |
| Modal (standard)  | 480px    | 560px    | 600px    | 640px    |

---

## Accessibility

### Color Contrast
All text/background combinations must meet WCAG 2.1 AA requirements:
- Normal text: minimum 4.5:1 contrast ratio
- Large text (18px+ or 14px+ bold): minimum 3:1 contrast ratio
- UI components: minimum 3:1 contrast ratio

The warm color palette has been checked for compliance:
- `Text (Primary)` on `Background (Primary)`: 15.8:1 ✓
- `Text (Secondary)` on `Background (Primary)`: 5.2:1 ✓
- `Text (Tertiary)` on `Background (Primary)`: 3.5:1 (use only for non-essential)
- `Accent (Primary)` on `Background (Elevated)`: 4.6:1 ✓

### Focus Indicators
- All interactive elements must have visible focus states
- Focus ring: `Shadow (focus)` — 3px ring in accent color
- Never remove focus outlines without providing an alternative

### Keyboard Navigation
- Tab order follows visual order
- All actions accessible via keyboard
- Escape closes modals/dropdowns
- Arrow keys navigate within lists

### Screen Reader Considerations
- Use semantic HTML elements
- Provide aria-labels for icon-only buttons
- Status updates use aria-live regions
- Progress indicators announce changes

---

## Dark Mode (Future Consideration)

The current palette is light-mode only. If dark mode is added later:
- Invert the background scale (dark backgrounds, light text)
- Keep the warm undertone (use warm grays, not blue-grays)
- Semantic colors need adjusted brightness (softer in dark mode)
- Test all contrast ratios again

---

## Implementation Notes

### Flutter Specifics
- Define colors as `const Color` values in a theme file
- Use `ThemeData` extension for custom tokens
- Typography styles as `TextStyle` constants
- Consider using a design token generator for consistency

### Recommended Theme Structure
```
lib/
  theme/
    colors.dart          // All color definitions
    typography.dart      // Text styles
    spacing.dart         // Spacing constants
    shadows.dart         // Box shadows
    borders.dart         // Border radii, widths
    theme.dart           // Combined ThemeData
    components/          // Component-specific themes
      button_theme.dart
      input_theme.dart
      card_theme.dart
      ...
```

### Asset Management
- Icons: SVG preferred, with PNG fallback at 1x and 2x
- Store icon set in `assets/icons/`
- Use Flutter's `SvgPicture` or icon font approach
