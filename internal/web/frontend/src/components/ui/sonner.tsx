"use client"

import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"
import { CircleCheckIcon, InfoIcon, TriangleAlertIcon, OctagonXIcon, Loader2Icon } from "lucide-react"

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      richColors
      icons={{
        success: (
          <CircleCheckIcon className="size-4" />
        ),
        info: (
          <InfoIcon className="size-4" />
        ),
        warning: (
          <TriangleAlertIcon className="size-4" />
        ),
        error: (
          <OctagonXIcon className="size-4" />
        ),
        loading: (
          <Loader2Icon className="size-4 animate-spin" />
        ),
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
          "--success-bg": "color-mix(in oklab, var(--success) 14%, var(--popover))",
          "--success-border": "color-mix(in oklab, var(--success) 35%, var(--border))",
          "--success-text": "var(--success-foreground)",
          "--info-bg": "color-mix(in oklab, var(--info) 14%, var(--popover))",
          "--info-border": "color-mix(in oklab, var(--info) 35%, var(--border))",
          "--info-text": "var(--info-foreground)",
          "--warning-bg": "color-mix(in oklab, var(--warning) 14%, var(--popover))",
          "--warning-border": "color-mix(in oklab, var(--warning) 35%, var(--border))",
          "--warning-text": "var(--warning-foreground)",
          "--error-bg": "color-mix(in oklab, var(--destructive) 14%, var(--popover))",
          "--error-border": "color-mix(in oklab, var(--destructive) 35%, var(--border))",
          "--error-text": "var(--destructive-foreground)",
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: "cn-toast",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
