import { cva, type VariantProps } from "class-variance-authority";

export { default as Spinner } from "./spinner.svelte";

export const spinnerVariants = cva(
  "animate-spin rounded-full border-2 border-current border-t-transparent",
  {
    variants: {
      size: {
        sm: "size-4",
        default: "size-5",
        lg: "size-8",
      },
    },
    defaultVariants: {
      size: "default",
    },
  }
);

export type SpinnerProps = VariantProps<typeof spinnerVariants>;
