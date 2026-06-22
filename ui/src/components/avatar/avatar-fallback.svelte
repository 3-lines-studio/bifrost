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

<span
  data-slot="avatar-fallback"
  class={cn("absolute inset-0 flex size-full items-center justify-center rounded-full bg-muted", className)}
  {...restProps}
>
  {@render children?.()}
</span>
