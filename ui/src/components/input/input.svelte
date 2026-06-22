<script lang="ts">
  import { cn } from "../../lib/utils";

  let {
    value = $bindable(""),
    class: className,
    type = "text",
    ref = $bindable(null),
    ...restProps
  }: {
    value?: string;
    class?: string;
    type?: string;
    ref?: HTMLInputElement | null;
    [key: string]: any;
  } = $props();
</script>

<input
  bind:this={ref}
  bind:value
  data-slot="input"
  {type}
  class={cn(
    "border-input flex h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 text-base shadow-xs outline-none transition-[color,box-shadow] selection:bg-primary selection:text-primary-foreground file:text-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/32 focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
    className
  )}
  {...restProps}
/>
