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
  data-slot="alert-description"
  class={cn(
    "text-muted-foreground col-start-2 text-sm [&_p]:leading-relaxed",
    className
  )}
  {...restProps}
>
  {@render children?.()}
</div>
