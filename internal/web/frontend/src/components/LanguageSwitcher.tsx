import { Languages } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { supportedLanguages, type SupportedLanguage } from "@/i18n";

export function LanguageSwitcher() {
  const { t, i18n } = useTranslation();
  const current = i18n.language.split("-")[0] as SupportedLanguage;

  const changeLanguage = (code: string) => {
    void i18n.changeLanguage(code as SupportedLanguage);
  };

  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger>
        <Languages />
        {t("topBar.language")}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-40">
        <DropdownMenuRadioGroup value={current} onValueChange={changeLanguage}>
          {supportedLanguages.map(({ code, label }) => (
            <DropdownMenuRadioItem key={code} value={code}>
              {label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

export function LanguageSwitcherButton({ className }: { className?: string }) {
  const { t, i18n } = useTranslation();
  const current = i18n.language.split("-")[0] as SupportedLanguage;

  const changeLanguage = (code: string) => {
    void i18n.changeLanguage(code as SupportedLanguage);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="icon" className={className} aria-label={t("topBar.language")}>
          <Languages />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        <DropdownMenuRadioGroup value={current} onValueChange={changeLanguage}>
          {supportedLanguages.map(({ code, label }) => (
            <DropdownMenuRadioItem key={code} value={code}>
              {label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
