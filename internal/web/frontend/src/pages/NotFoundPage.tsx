import { CircleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";

export function NotFoundPage() {
  const { t } = useTranslation();

  return (
    <div className="theme flex h-svh bg-background text-foreground">
      <Empty className="m-4 min-h-0 flex-1 border">
        <EmptyHeader>
          <EmptyMedia variant="icon"><CircleAlert /></EmptyMedia>
          <EmptyTitle>{t("notFound.title")}</EmptyTitle>
          <EmptyDescription>{t("notFound.description")}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button variant="outline" asChild><Link to="/">{t("notFound.returnHome")}</Link></Button>
        </EmptyContent>
      </Empty>
    </div>
  );
}
