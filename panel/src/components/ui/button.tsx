import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md border text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 focus-visible:ring-offset-white disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        // Nut chinh
        default: "border-blue-600 bg-blue-600 text-white hover:border-blue-700 hover:bg-blue-700",
        primary: "border-blue-600 bg-blue-600 text-white hover:border-blue-700 hover:bg-blue-700",
        // Nut phu
        secondary: "border-gray-300 bg-white text-gray-700 hover:bg-gray-50",
        outline: "border-gray-300 bg-white text-gray-700 hover:bg-gray-50",
        // Nut nguy hiem
        destructive: "border-red-600 bg-red-600 text-white hover:border-red-700 hover:bg-red-700",
        danger: "border-red-600 bg-red-600 text-white hover:border-red-700 hover:bg-red-700",
        "danger-outline": "border-red-300 bg-white text-red-700 hover:bg-red-50",
        // Nut mo
        ghost: "border-transparent bg-transparent text-gray-600 hover:bg-gray-100 hover:text-gray-900",
        link: "border-transparent bg-transparent text-blue-700 underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-3.5 py-2",
        md: "h-9 px-3.5 py-2",
        sm: "h-8 px-3 text-xs",
        lg: "h-10 px-5",
        icon: "h-9 w-9 p-0",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button"
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = "Button"

export { Button, buttonVariants }
