import { cva, type VariantProps } from "class-variance-authority";

export { default as Alert } from "./alert.svelte";
export { default as AlertTitle } from "./alert-title.svelte";
export { default as AlertDescription } from "./alert-description.svelte";

export const alertVariants = cva(
  "relative w-full rounded-lg border px-4 gap-3 text-sm flex items-start [&>svg]:size-4 [&>svg]:text-foreground",
  {
    variants: {
      variant: {
        default: "bg-card text-card-foreground [&>svg]:text-current",
        destructive: "text-destructive [&>svg]:text-current",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export type AlertProps = VariantProps<typeof alertVariants>;
