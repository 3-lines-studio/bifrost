# bifrost-ui — Follow-Up Plan

## Current State (as of last session)

### What's done

**Library at `ui/` (source files, excluding node_modules):**
```
ui/
  package.json                      # bifrost-ui, deps: cva, clsx, tailwind-merge, peer: svelte
  tsconfig.json
  README.md
  bun.lock
  src/
    index.ts                        # barrel export — all 12 components
    lib/
      utils.ts                      # cn() = twMerge(clsx(inputs))
      utils.test.ts                 # 4 tests for cn()
    styles/
      globals.css                   # COSS UI tokens ported: @theme inline, :root, .dark, keyframes, @layer base, @utility container
    components/
      button/    button.svelte + index.ts   # 7 variants × 10 sizes, href prop → <a>, exact COSS cva
      badge/     badge.svelte + index.ts    # 8 variants × 3 sizes, exact COSS cva
      card/      6× .svelte + index.ts      # Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter
      alert/     3× .svelte + index.ts      # Alert, AlertTitle, AlertDescription + alertVariants
      separator/ separator.svelte + index.ts
      skeleton/  skeleton.svelte + index.ts # animate-skeleton gradient
      spinner/   spinner.svelte + index.ts  # CSS-only, 3 sizes
      label/     label.svelte + index.ts    # native <label>
      kbd/       kbd.svelte + index.ts      # native <kbd>
      avatar/    avatar.svelte + avatar-image.svelte + avatar-fallback.svelte + avatar-context.svelte.ts + index.ts
      input/     input.svelte + index.ts    # $bindable value, native <input>
      textarea/  textarea.svelte + index.ts # $bindable value, native <textarea>
```

**Showcase at `showcase/` (project root, NOT under ui/):**
```
showcase/
  go.mod          # module bifrost-ui-showcase, replace bifrost => ..
  main.go         # bifrost.New(BifrostFS, Page("/{$}", "./pages/showcase.svelte")), http.ListenAndServe(":8080")
  package.json    # svelte, tailwindcss, bun-plugin-tailwind, clsx, tailwind-merge
  tsconfig.json
  style.css       # @import "tailwindcss"; @source "../ui/src/**/*.{ts,svelte}"; @import "../ui/src/styles/globals.css";
  .gitignore      # .bifrost/* + !.bifrost/.gitkeep
  .bifrost/.gitkeep
  pages/
    showcase.svelte  # imports from "../../ui/src/index" — wait, actually from "../ui/src/index" (showcase is at root)
```

**Build verified:** `bifrost build ./main.go` from showcase/ succeeds. Produces 73KB compiled CSS. `go vet` passes.

### Known issues / deviations from plan

1. **Showcase location:** Was planned for `ui/showcase/` but ended up at project root `showcase/`. The `go.mod` replace directive points to `..` (one level up) which is correct from root level. The `style.css` paths reference `../ui/src/`. This works but should be documented — if we move it under `ui/`, the replace directive and style.css paths need updating.

2. **globals.css does NOT import tailwindcss.** The `@import "tailwindcss"` line was removed during the build-fix phase because Bun couldn't resolve `tailwindcss` from the library directory (it's only in showcase's node_modules). The consuming app's `style.css` handles `@import "tailwindcss"` + `@import "../ui/src/styles/globals.css"`. This is the correct pattern for a library — the library provides tokens, the app provides the Tailwind runtime.

3. **Skeleton keyframes diverge slightly from COSS UI.** The original uses `background-position: -200% 0` for the `to` keyframe; our version uses `background-position: -100% 50%`. The original uses `to { background-position: -200% 0; }` — ours has `0% { background-position: 100% 50%; } 100% { background-position: -100% 50%; }`. This may cause a different animation direction. Minor cosmetic issue.

4. **Toast keyframes diverge from COSS UI.** The original toast keyframes animate `scale` and `translate` for bounce/shake effects. Our version animates `opacity` and `transform: translateX`. These will be wrong when we implement Toast in Phase 3. Need to re-sync before implementing Toast.

5. **`@source` in globals.css was removed.** The original had `@source "../../../apps/**/*.{ts,tsx}";` and `@source "../**/*.{ts,tsx}";`. We removed it because the library shouldn't dictate source scanning. The showcase's `style.css` has `@source "../ui/src/**/*.{ts,svelte}";` instead. This is correct but consumers need to add their own `@source` directive pointing to the library.

6. **No `global.d.ts` in showcase.** The reviewer recommended adding `declare module "*.css";` but it may not have been created. Check and add if missing.

7. **`prefers-reduced-motion` media query** was added with a broader set of rules than the original (which only targeted toast animations). Ours disables all animations/transitions. This is arguably better but diverges from source.

---

## Phase 1 Cleanup (do first, ~30 min)

### Task 1.1: Fix skeleton keyframes
- **File:** `ui/src/styles/globals.css`
- **Change:** Replace the skeleton keyframe with the exact COSS UI version:
  ```css
  @keyframes skeleton {
    to {
      background-position: -200% 0;
    }
  }
  ```

### Task 1.2: Fix toast keyframes
- **File:** `ui/src/styles/globals.css`
- **Change:** Replace toast keyframes with exact COSS UI versions:
  ```css
  @keyframes toast-success-odd {
    0% { scale: 1; }
    30% { scale: 1.025; }
    60% { scale: 0.99; }
    100% { scale: 1; }
  }
  @keyframes toast-success-even {
    0% { scale: 1; }
    30% { scale: 1.025; }
    60% { scale: 0.99; }
    100% { scale: 1; }
  }
  @keyframes toast-error-odd {
    0% { translate: 0 0; }
    25% { translate: -3px 0; }
    50% { translate: 3px 0; }
    75% { translate: -3px 0; }
    100% { translate: 0 0; }
  }
  @keyframes toast-error-even {
    0% { translate: 0 0; }
    25% { translate: -3px 0; }
    50% { translate: 3px 0; }
    75% { translate: -3px 0; }
    100% { translate: 0 0; }
  }
  ```

### Task 1.3: Fix reduced-motion media query
- **File:** `ui/src/styles/globals.css`
- **Change:** Replace the broad reduced-motion block with the COSS UI targeted version:
  ```css
  @media (prefers-reduced-motion: reduce) {
    :where(
        .animate-toast-success-odd,
        .animate-toast-success-even,
        .animate-toast-error-odd,
        .animate-toast-error-even
      ) {
      animation: none;
    }
  }
  ```

### Task 1.4: Add missing global.d.ts to showcase
- **File:** `showcase/global.d.ts`
- **Content:** `declare module "*.css";`

### Task 1.5: Rebuild showcase and verify
- Run `bifrost build ./main.go` from showcase/
- Confirm build succeeds with no errors

---

## Phase 2 — Semi-Interactive Components (vanilla Svelte, no Bits UI)

These components need behavior but can be done with native HTML + Svelte 5 runes.

### COSS UI Source References
Fetch from `https://raw.githubusercontent.com/cosscom/coss/main/packages/ui/src/components/{name}.tsx`:
- `accordion.tsx` (2.2KB)
- `collapsible.tsx` (1.1KB)
- `tabs.tsx` (check if exists in components list)
- `toggle.tsx`
- `toggle-group.tsx`
- `switch.tsx`
- `tooltip.tsx`
- `progress.tsx`
- `meter.tsx` (1.7KB)

### Task 2.1: Accordion
- Use native `<details>`/`<summary>` or Svelte state-based accordion
- COSS UI uses Base UI Accordion — we'll use Svelte state + `<details>` for simplicity
- Port the cva class strings for styling
- Sub-components: Accordion, AccordionItem, AccordionTrigger, AccordionContent
- Support `type="single" | "multiple"` for collapsible behavior

### Task 2.2: Collapsible
- Simplest interactive component — use `<details>`/`<summary>` or Svelte state
- Sub-components: Collapsible, CollapsibleTrigger, CollapsibleContent
- Port cva from COSS source

### Task 2.3: Toggle
- Two-state button with `pressed` data attribute
- Use `$state` for pressed state, `$bindable` for controlled mode
- Port toggleVariants cva

### Task 2.4: Toggle Group
- Group of toggles with shared state (single or multiple selection)
- Use Svelte context for group state
- Sub-components: ToggleGroup, ToggleGroupItem

### Task 2.5: Switch
- Toggle styled as a switch (on/off slider)
- Use native `<input type="checkbox">` styled with COSS classes, or custom
- `$bindable` checked state
- Port switch cva

### Task 2.6: Tabs
- Use Svelte state for active tab management
- Sub-components: Tabs, TabsList, TabsTrigger, TabsContent
- Port cva from COSS source
- Support `orientation="horizontal" | "vertical"`

### Task 2.7: Tooltip
- CSS-only or minimal JS: show on hover/focus, hide on blur/mouseleave
- Use Svelte action or simple state
- Sub-components: Tooltip, TooltipTrigger, TooltipContent
- Position: CSS `position: absolute` relative to trigger (simple top/bottom for Phase 2)

### Task 2.8: Progress
- Native `<progress>` element styled with COSS classes
- Port progress cva
- `$bindable` value prop

### Task 2.9: Meter
- Native `<meter>` element styled with COSS classes
- Port meter cva (1.7KB source — small)
- Props: value, min, max, low, high, optimum

### Task 2.10: Update showcase
- Add new components to showcase page
- Add interactive demos (accordion expand/collapse, toggle pressed, switch on/off, tab switching)

### Task 2.11: Update barrel export
- Add all Phase 2 components to `ui/src/index.ts`

---

## Phase 3 — Complex Components (Bits UI as primitive layer)

### Prerequisites
- Add `bits-ui` to `ui/package.json` dependencies
- Bits UI is the Svelte equivalent of Base UI/Radix — provides accessible, unstyled primitives
- Reference: https://bits-ui.com/

### COSS UI Source References
Fetch from `https://raw.githubusercontent.com/cosscom/coss/main/packages/ui/src/components/{name}.tsx`:
- `dialog.tsx` (6.5KB)
- `alert-dialog.tsx` (5KB)
- `popover.tsx` (4.6KB)
- `select.tsx`
- `combobox.tsx` (15.3KB)
- `command.tsx` (7.6KB)
- `menu.tsx` (12.4KB)
- `context-menu.tsx` (12.4KB)
- `sheet.tsx`
- `drawer.tsx` (23.9KB — largest component)
- `slider.tsx`
- `calendar.tsx` (5.3KB)
- `date-picker.tsx`
- `toast.tsx`
- `checkbox.tsx`
- `checkbox-group.tsx` (462B — tiny)
- `radio-group.tsx`
- `number-field.tsx` (5.8KB)
- `otp-field.tsx` (2.7KB)
- `scroll-area.tsx`
- `autocomplete.tsx` (10.6KB)

### Implementation order (by complexity, simplest first):
1. **Checkbox** — Bits UI Checkbox, port cva
2. **Radio Group** — Bits UI RadioGroup, port cva
3. **Dialog** — Bits UI Dialog, port cva
4. **Alert Dialog** — Bits UI AlertDialog, port cva
5. **Popover** — Bits UI Popover, port cva
6. **Select** — Bits UI Select, port cva
7. **Slider** — Bits UI Slider, port cva
8. **Scroll Area** — Bits UI ScrollArea, port cva
9. **Toast** — Bits UI Toast, port cva (uses the toast keyframes)
10. **Menu** — Bits UI Menu, port cva
11. **Context Menu** — Bits UI ContextMenu, port cva
12. **Sheet** — Bits UI Sheet/Dialog, port cva
13. **Combobox** — Bits UI Combobox, port cva (complex — autocomplete + select)
14. **Command** — Bits UI Command, port cva (command palette)
15. **Autocomplete** — Bits UI Autocomplete, port cva
16. **Number Field** — Bits UI NumberField, port cva
17. **OTP Field** — Bits UI OTPField, port cva
18. **Calendar** — Bits UI Calendar, port cva
19. **Date Picker** — Combines Calendar + Popover
20. **Drawer** — Bits UI Drawer (most complex — swipe gestures, snap points)

### For each component:
1. Fetch the COSS UI `.tsx` source
2. Identify the Bits UI equivalent (check https://bits-ui.com/docs/components/)
3. Port the cva class strings verbatim
4. Create Svelte wrapper using Bits UI primitive + COSS classes
5. Export from component index.ts + barrel
6. Add to showcase

---

## Architecture Notes for New Sessions

### Key decisions made during brainstorming:
1. **Behavior layer:** Vanilla HTML + Svelte for simple, Bits UI for complex
2. **Components are thin wrappers** — minimal JS, maximum HTML
3. **Design tokens ported verbatim** from COSS UI `globals.css`
4. **cva strings ported verbatim** from COSS UI component `.tsx` files
5. **Package name:** `bifrost-ui`
6. **Button polymorphism:** `href` prop renders `<a>` instead of `<button>`
7. **Spinner:** CSS-only (animated border)
8. **Avatar:** Svelte context-based image/fallback swap (no Radix)
9. **Input/Textarea:** `$bindable()` for `bind:value`
10. **`cn()` util:** clsx + tailwind-merge (required for class override merging)

### Svelte 5 component pattern:
```svelte
<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "../../lib/utils";

  let {
    class: className,
    children,
    ...restProps
  }: {
    class?: string;
    children?: Snippet;
    [key: string]: any;
  } = $props();
</script>

<element class={cn("base-classes", className)} {...restProps}>
  {@render children?.()}
</element>
```

For variant components, cva is in `index.ts`:
```ts
// index.ts
import { cva, type VariantProps } from "class-variance-authority";
export { default as Component } from "./component.svelte";
export const componentVariants = cva("base", { variants: { ... } });
export type ComponentProps = VariantProps<typeof componentVariants>;
```

```svelte
<!-- component.svelte -->
<script lang="ts">
  import { componentVariants, type ComponentProps } from "./index";
  import { cn } from "../../lib/utils";
  let { variant, size, class: className, children, ...rest } = $props();
</script>
```

### Files that use Svelte runes in .ts files:
- Any file using `$state()`, `$props()`, `$derived()`, etc. MUST have `.svelte.ts` extension
- Example: `avatar-context.svelte.ts`

### Build verification:
```bash
# From project root
cd showcase
/home/berti/Code/bifrost/bifrost build ./main.go
# Should produce .bifrost/ with dist/, ssr/, manifest.json

# Go vet
go vet ./...

# Type check (if svelte-check available)
cd ui && bunx svelte-check --tsconfig ./tsconfig.json
```

### COSS UI source locations:
- Design tokens: `packages/ui/src/styles/globals.css`
- Components: `packages/ui/src/components/{name}.tsx`
- Utils: `packages/ui/src/lib/utils.ts`
- Raw URL pattern: `https://raw.githubusercontent.com/cosscom/coss/main/packages/ui/src/components/{name}.tsx`
