<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "../../lib/utils";
  import { alertVariants, type AlertProps } from "./index";

  let {
    variant = "default" as AlertProps["variant"],
    class: className,
    children,
    ...restProps
  }: AlertProps & {
    class?: string;
    children?: Snippet;
    [key: string]: any;
  } = $props();
</script>

<div
  data-slot="alert"
  data-variant={variant}
  class={cn(alertVariants({ variant }), className)}
  {...restProps}
>
  {@render children?.()}
</div>
