<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "../../lib/utils";
  import { createAvatarContext } from "./avatar-context.svelte";

  let {
    class: className,
    children,
    ...restProps
  }: {
    class?: string;
    children?: Snippet;
    [key: string]: any;
  } = $props();

  createAvatarContext();
</script>

<span
  data-slot="avatar"
  class={cn("relative flex size-8 shrink-0 overflow-hidden rounded-full", className)}
  {...restProps}
>
  {@render children?.()}
</span>
