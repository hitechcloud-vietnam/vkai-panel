import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2",
  {
    variants: {
      variant: {
        default: "border-brand-200 bg-brand-50 text-brand-700",
        brand: "border-brand-200 bg-brand-50 text-brand-700",
        accent: "border-accent-200 bg-accent-50 text-accent-700",
        info: "border-sky-200 bg-sky-50 text-sky-700",
        success: "border-emerald-200 bg-emerald-50 text-emerald-700",
        warning: "border-amber-200 bg-amber-50 text-amber-700",
        danger: "border-red-200 bg-red-50 text-red-700",
        destructive: "border-red-200 bg-red-50 text-red-700",
        neutral: "border-gray-200 bg-gray-100 text-gray-700",
        secondary: "border-gray-200 bg-gray-100 text-gray-700",
        outline: "border-gray-300 bg-white text-gray-700",
      },
    },
    defaultVariants: {
      variant: "neutral",
    },
  }
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  )
}

export { Badge, badgeVariants }
