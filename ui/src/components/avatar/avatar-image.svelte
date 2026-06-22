<script lang="ts">
  import { cn } from "../../lib/utils";
  import { getAvatarContext } from "./avatar-context.svelte";

  let {
    src,
    alt,
    class: className,
    ...restProps
  }: {
    src: string;
    alt?: string;
    class?: string;
    [key: string]: any;
  } = $props();

  const context = getAvatarContext();
</script>

{#if context?.status !== "error"}
  <img
    data-slot="avatar-image"
    {src}
    {alt}
    class={cn("aspect-square size-full object-cover", className)}
    onerror={() => context?.setStatus("error")}
    onload={() => context?.setStatus("loaded")}
    {...restProps}
  />
{/if}
