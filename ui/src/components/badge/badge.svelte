<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "../../lib/utils";
  import { badgeVariants, type BadgeProps } from "./index";

  let {
    variant = "default" as BadgeProps["variant"],
    size = "default" as BadgeProps["size"],
    class: className,
    children,
    ...restProps
  }: BadgeProps & {
    class?: string;
    children?: Snippet;
    [key: string]: any;
  } = $props();
</script>

<span
  data-slot="badge"
  data-variant={variant}
  data-size={size}
  class={cn(badgeVariants({ variant, size }), className)}
  {...restProps}
>
  {@render children?.()}
</span>
