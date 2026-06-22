<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "../../lib/utils";
  import { buttonVariants, type ButtonProps } from "./index";

  let {
    variant = "default" as ButtonProps["variant"],
    size = "default" as ButtonProps["size"],
    class: className,
    href,
    children,
    ref = $bindable(null),
    ...restProps
  }: ButtonProps & {
    class?: string;
    href?: string;
    children?: Snippet;
    ref?: HTMLElement | null;
    [key: string]: any;
  } = $props();
</script>

{#if href}
  <a
    data-slot="button"
    data-variant={variant}
    data-size={size}
    class={cn(buttonVariants({ variant, size }), className)}
    {href}
    bind:this={ref}
    {...restProps}
  >
    {@render children?.()}
  </a>
{:else}
  <button
    data-slot="button"
    data-variant={variant}
    data-size={size}
    class={cn(buttonVariants({ variant, size }), className)}
    bind:this={ref}
    {...restProps}
  >
    {@render children?.()}
  </button>
{/if}
