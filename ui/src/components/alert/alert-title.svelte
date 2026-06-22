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

<div
  data-slot="alert-title"
  class={cn(
    "col-start-2 line-clamp-1 min-h-4 font-medium tracking-tight",
    className
  )}
  {...restProps}
>
  {@render children?.()}
</div>
