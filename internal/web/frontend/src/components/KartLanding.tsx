import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  BatteryMedium,
  ChevronRight,
  CircleAlert,
  Info,
  LoaderCircle,
  Moon,
  Plus,
  Sun,
  Wifi,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Badge } from "@/components/reui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { KartDetailDialog } from "@/components/KartDetailDialog";
import { KomitakeBrand } from "@/components/KomitakeBrand";
import { LanguageSwitcherButton } from "@/components/LanguageSwitcher";
import { MountainDotField } from "@/components/MountainDotField";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Item, ItemGroup } from "@/components/ui/item";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { Kart } from "@/lib/api";
import { formatKartLabel } from "@/lib/dashboard";

type KartLandingProps = {
  variant?: "page" | "panel";
  unavailableSlug?: string;
  loading: boolean;
  devices?: Kart[];
  onSelect?: (id: string) => void;
  onPair?: () => void;
  onReturn: () => void;
};

export function KartLanding({
  variant = "panel",
  unavailableSlug,
  loading,
  devices = [],
  onSelect,
  onPair,
  onReturn,
}: KartLandingProps) {
  const { t } = useTranslation();
  const [mounted, setMounted] = useState(false);
  const [detail, setDetail] = useState<Kart | null>(null);
  const { resolvedTheme, setTheme } = useTheme();
  useEffect(() => setMounted(true), []);
  const lightTheme = mounted && resolvedTheme === "light";
  const pageVariant = variant === "page";

  if (!pageVariant && loading) {
    return (
      <Empty className="m-3 min-h-0 flex-1 border md:m-4">
        <EmptyHeader>
          <EmptyMedia variant="icon"><LoaderCircle className="animate-spin" /></EmptyMedia>
          <EmptyTitle>{t("landing.connectingTitle")}</EmptyTitle>
          <EmptyDescription>{t("landing.connectingDescription")}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }

  if (unavailableSlug) {
    return (
      <Empty className="m-3 min-h-0 flex-1 border md:m-4">
        <EmptyHeader>
          <EmptyMedia variant="icon"><CircleAlert /></EmptyMedia>
          <EmptyTitle>{t("landing.kartUnavailableTitle")}</EmptyTitle>
          <EmptyDescription>
            {t("landing.kartUnavailableDescription", { slug: unavailableSlug })}
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button variant="outline" onClick={onReturn}>{t("landing.returnToSelection")}</Button>
        </EmptyContent>
      </Empty>
    );
  }

  return (
    <section
      data-slot="landing-background"
      className="relative isolate flex min-h-0 flex-1 items-center justify-start overflow-hidden bg-transparent p-4 sm:p-6 lg:p-10"
    >
      <div className="pointer-events-none absolute inset-0" data-slot="landing-background-content">
        <MountainDotField />
      </div>
      <div className="absolute top-4 right-4 z-20 flex gap-2 sm:top-6 sm:right-6 lg:top-10 lg:right-10">
        <LanguageSwitcherButton />
        <Button
          variant="outline"
          size="icon"
          onClick={() => setTheme(lightTheme ? "dark" : "light")}
          aria-label={lightTheme ? t("landing.useDarkTheme") : t("landing.useLightTheme")}
          disabled={!mounted}
        >
          {lightTheme ? <Moon /> : <Sun />}
        </Button>
      </div>
      <div className="relative z-10 h-[78%] min-h-80 w-full lg:max-h-[44rem]">
        <div className="flex h-full w-full flex-col-reverse items-center justify-center gap-4 md:flex-row md:items-stretch md:justify-start md:gap-8">
          <Card
            data-slot="landing-card"
            className="relative z-10 flex min-h-80 w-full max-w-lg flex-col rounded-3xl py-0 shadow-2xl md:h-full"
          >
            <CardContent
              className="flex min-h-80 flex-1 flex-col gap-4 p-6 md:min-h-0"
              data-slot="landing-card-content"
            >
              <div className="shrink-0 space-y-1">
                <h2 className="font-heading text-lg font-semibold tracking-tight">
                  {t("landing.connectedKarts")}
                </h2>
                <p className="text-sm text-muted-foreground">
                  {t("landing.selectKartDescription")}
                </p>
              </div>

              <div className="flex min-h-0 flex-1 flex-col">
                {loading ? (
                  <Empty className="min-h-0 flex-1 border-0">
                    <EmptyHeader>
                      <EmptyMedia variant="icon">
                        <LoaderCircle className="animate-spin" />
                      </EmptyMedia>
                      <EmptyTitle>{t("landing.connectingTitle")}</EmptyTitle>
                      <EmptyDescription>
                        {t("landing.connectingDescription")}
                      </EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                ) : devices.length === 0 ? (
                  <Empty className="min-h-0 flex-1 border-0">
                    <EmptyHeader>
                      <EmptyMedia variant="icon"><Wifi /></EmptyMedia>
                      <EmptyTitle>{t("landing.noKartsTitle")}</EmptyTitle>
                      <EmptyDescription>{t("landing.noKartsDescription")}</EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                ) : (
                  <ScrollArea className="-mx-1 h-full min-h-0 flex-1 px-1">
                    <ItemGroup className="gap-1">
                      {devices.map((kart) => {
                        const kartLabel = formatKartLabel(kart);
                        return (
                          <Item
                            key={kart.ident}
                            variant="outline"
                            size="xs"
                            className="flex-nowrap gap-1 p-1"
                          >
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-11 min-w-0 flex-1 justify-start gap-2 px-2 font-normal"
                              onClick={() => onSelect?.(kart.ident)}
                            >
                              <Wifi data-icon="inline-start" />
                              <span className="flex min-w-0 flex-1 flex-col items-start">
                                <span className="w-full truncate text-left text-sm font-medium">
                                  {kartLabel}
                                </span>
                                <span className="w-full truncate text-left font-mono text-xs text-muted-foreground">
                                  {kart.address || kart.ident}
                                </span>
                              </span>
                              <ChevronRight data-icon="inline-end" className="text-muted-foreground" />
                            </Button>
                            <div className="flex shrink-0 items-center gap-1">
                              {kart.battery !== undefined && (
                                <Badge
                                  variant="outline"
                                  size="sm"
                                  aria-label={t("kartPicker.batteryBars", { level: kart.battery })}
                                >
                                  <BatteryMedium />{kart.battery}/4
                                </Badge>
                              )}
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                aria-label={t("kartPicker.detailsFor", { label: kartLabel })}
                                onClick={() => setDetail(kart)}
                              >
                                <Info />
                              </Button>
                            </div>
                          </Item>
                        );
                      })}
                    </ItemGroup>
                  </ScrollArea>
                )}
              </div>

              <Button
                variant="default"
                size="lg"
                className="w-full shrink-0"
                onClick={onPair}
                disabled={!onPair}
              >
                <Plus data-icon="inline-start" />
                {t("landing.pairKart")}
              </Button>
            </CardContent>
          </Card>
          <div className="hidden h-full flex-col justify-end md:flex md:py-4 md:gap-4">
            <h1
              data-slot="landing-title"
              className="font-heading text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl w-fit bg-radial-[ellipse_farthest-corner_at_center] from-white/70 dark:from-black/70 to-transparent"
            >
              <KomitakeBrand variant="hero" />
            </h1>
            <p
              data-slot="landing-subtitle"
              className="max-w-xl font-heading text-pretty leading-relaxed text-lg text-muted-foreground sm:text-xl lg:max-w-2xl lg:text-2xl bg-radial-[ellipse_farthest-corner_at_center] from-white/70 dark:from-black/70 to-transparent"
            >
              {t("app.tagline")}
            </p>
          </div>
        </div>
      </div>
      <KartDetailDialog
        kart={detail}
        open={Boolean(detail)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setDetail(null);
        }}
      />
    </section>
  );
}
