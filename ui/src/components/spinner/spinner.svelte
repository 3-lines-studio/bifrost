<script lang="ts">
  import { cn } from "../../lib/utils";
  import { spinnerVariants, type SpinnerProps } from "./index";

  let {
    size = "default" as SpinnerProps["size"],
    class: className,
    ...restProps
  }: SpinnerProps & {
    class?: string;
    [key: string]: any;
  } = $props();
</script>

<span
  data-slot="spinner"
  aria-hidden="true"
  class={cn(spinnerVariants({ size }), className)}
  {...restProps}
></span>
