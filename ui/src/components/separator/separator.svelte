<script lang="ts">
  import { cn } from "../../lib/utils";

  let {
    orientation = "horizontal",
    class: className,
    decorative = true,
    ...restProps
  }: {
    orientation?: "horizontal" | "vertical";
    class?: string;
    decorative?: boolean;
    [key: string]: any;
  } = $props();
</script>

<div
  role={decorative ? "none" : "separator"}
  aria-orientation={decorative ? undefined : orientation}
  data-slot="separator"
  data-orientation={orientation}
  class={cn(
    "bg-border shrink-0 data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-px",
    className
  )}
  {...restProps}
></div>
