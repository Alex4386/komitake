import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

type KomitakeBrandProps = {
  className?: string;
  variant?: "plain" | "hero";
};

export function KomitakeBrand({ className, variant = "plain" }: KomitakeBrandProps) {
  const { i18n, t } = useTranslation();
  const japanese = i18n.language.startsWith("ja");

  if (japanese) {
    return (
      <ruby
        className={cn(
          variant === "hero" && "landing-title-ruby",
          className,
        )}
      >
        {t("app.titleKanji")}
        <rt>{t("app.titleRuby")}</rt>
      </ruby>
    );
  }

  return <span className={className}>{t("app.title")}</span>;
}
