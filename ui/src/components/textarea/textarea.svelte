<script lang="ts">
  import { cn } from "../../lib/utils";

  let {
    value = $bindable(""),
    class: className,
    ref = $bindable(null),
    ...restProps
  }: {
    value?: string;
    class?: string;
    ref?: HTMLTextAreaElement | null;
    [key: string]: any;
  } = $props();
</script>

<textarea
  bind:this={ref}
  bind:value
  data-slot="textarea"
  class={cn(
    "border-input flex field-sizing-content min-h-16 w-full rounded-md border bg-transparent px-3 py-2 text-base shadow-xs outline-none transition-[color,box-shadow] selection:bg-primary selection:text-primary-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/32 focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
    className
  )}
  {...restProps}
></textarea>
