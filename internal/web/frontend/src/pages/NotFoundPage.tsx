import { CircleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty";

export function NotFoundPage() {
  const { t } = useTranslation();

  return (
    <div className="theme flex h-dvh bg-background text-foreground">
      <EmptyState
        className="m-4 min-h-0 flex-1 border"
        icon={<CircleAlert />}
        title={t("notFound.title")}
        description={t("notFound.description")}
        action={<Button variant="outline" asChild><Link to="/">{t("notFound.returnHome")}</Link></Button>}
      />
    </div>
  );
}
