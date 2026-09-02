import { useState } from "react";
import { useTranslation } from "react-i18next";
import { BatteryMedium, Check, ChevronDown, Info, Plus, Wifi } from "lucide-react";
import { Badge } from "@/components/reui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty";
import { Item, ItemGroup } from "@/components/ui/item";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { KartDetailDialog } from "@/components/KartDetailDialog";
import type { Kart } from "@/lib/api";
import { formatKartLabel } from "@/lib/dashboard";

type KartPickerProps = {
  devices: Kart[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onPair: () => void;
};

export function KartPicker({ devices, selectedId, onSelect, onPair }: KartPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [detail, setDetail] = useState<Kart | null>(null);
  const selected = devices.find((device) => device.ident === selectedId) ?? null;
  const triggerLabel = selected
    ? formatKartLabel(selected)
    : t("kartPicker.selectKart");

  const selectKart = (kartId: string) => {
    onSelect(kartId);
    setOpen(false);
  };

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="default"
            className="w-44 justify-between font-normal sm:w-48"
            aria-label={selected ? t("kartPicker.selectedKart", { label: triggerLabel }) : t("kartPicker.selectKart")}
          >
            <span className="truncate">{triggerLabel}</span>
            <ChevronDown data-icon="inline-end" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-80 gap-0 p-0">
          <PopoverHeader className="px-3 py-2.5">
            <PopoverTitle>{t("landing.connectedKarts")}</PopoverTitle>
            <PopoverDescription>{t("landing.selectKartDescription")}</PopoverDescription>
          </PopoverHeader>
          <Separator />
          {devices.length === 0 ? (
            <EmptyState
              className="border-0 px-4 py-8"
              icon={<Wifi />}
              title={t("landing.noKartsTitle")}
              description={t("landing.noKartsDescription")}
            />
          ) : (
            <ScrollArea className="max-h-72">
              <ItemGroup className="gap-1 p-1.5">
                {devices.map((kart) => {
                  const active = kart.ident === selectedId;
                  const kartLabel = formatKartLabel(kart);
                  return (
                    <Item
                      key={kart.ident}
                      variant={active ? "muted" : "default"}
                      size="xs"
                      className="flex-nowrap gap-1 p-1"
                    >
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-10 min-w-0 flex-1 justify-start gap-2 px-2 font-normal"
                        onClick={() => selectKart(kart.ident)}
                        aria-current={active ? "true" : undefined}
                      >
                        <Wifi data-icon="inline-start" />
                        <span className="flex min-w-0 flex-1 flex-col items-start">
                          <span className="w-full truncate text-left text-sm font-medium">{kartLabel}</span>
                          <span className="w-full truncate text-left font-mono text-xs text-muted-foreground">
                            {kart.address || kart.ident}
                          </span>
                        </span>
                      </Button>
                      <div className="flex shrink-0 items-center gap-1">
                        {kart.battery !== undefined && (
                          <Badge variant="outline" size="sm" aria-label={t("kartPicker.batteryBars", { level: kart.battery })}>
                            <BatteryMedium />{kart.battery}/4
                          </Badge>
                        )}
                        {active && <Check className="text-primary" aria-hidden="true" />}
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={t("kartPicker.detailsFor", { label: kartLabel })}
                          onClick={() => {
                            setOpen(false);
                            setDetail(kart);
                          }}
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
          <Separator />
          <div className="p-1.5">
            <Button
              variant="ghost"
              size="default"
              className="w-full justify-start"
              onClick={() => {
                setOpen(false);
                onPair();
              }}
            >
              <Plus data-icon="inline-start" />
              {t("landing.pairKart")}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      <KartDetailDialog
        kart={detail}
        open={Boolean(detail)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setDetail(null);
        }}
      />
    </>
  );
}
