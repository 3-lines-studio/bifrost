<script lang="ts">
  import { cn } from "../../lib/utils";

  let {
    class: className,
    ...restProps
  }: {
    class?: string;
    [key: string]: any;
  } = $props();
</script>

<div
  data-slot="skeleton"
  class={cn(
    "animate-skeleton rounded-md [--skeleton-highlight:--alpha(var(--color-white)/64%)] [background:linear-gradient(120deg,transparent_40%,var(--skeleton-highlight),transparent_60%)_var(--color-muted)_0_0/200%_100%_fixed] dark:[--skeleton-highlight:--alpha(var(--color-white)/4%)]",
    className
  )}
  {...restProps}
></div>
