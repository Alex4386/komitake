import { WifiOff } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/reui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import type { Status } from "@/lib/api";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  status: Status | null;
};

export function ApDialog({ open, onOpenChange, status }: Props) {
  const { t } = useTranslation();
  const wireless = status?.wireless;
  const hasSession = Boolean(wireless && (wireless.ssid || wireless.interface));
  const networkDetails = [
    { label: t("ap.ssid"), value: wireless?.ssid || "—" },
    { label: t("common.channel"), value: String(wireless?.channel ?? "—") },
    { label: t("ap.interface"), value: wireless?.interface || "—" },
    { label: t("ap.address"), value: wireless?.address || "—" },
    { label: t("ap.subnet"), value: wireless?.subnet || "—" },
  ];

  const modeLabel = status?.mode
    ? t(`ap.mode.${status.mode}`, { defaultValue: status.mode })
    : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="flex items-center justify-between gap-2 pr-8">
            <DialogTitle>{t("ap.title")}</DialogTitle>
            {status && modeLabel && (
              <Badge
                variant={
                  status.mode === "normal"
                    ? "success-light"
                    : status.mode === "pairing"
                      ? "warning-light"
                      : "outline"
                }
                size="sm"
              >
                {modeLabel}
              </Badge>
            )}
          </div>
          <DialogDescription>
            {t("ap.description")}
          </DialogDescription>
        </DialogHeader>
        {!hasSession ? (
          <Empty className="border-0 py-8">
            <EmptyHeader>
              <EmptyMedia variant="icon"><WifiOff /></EmptyMedia>
              <EmptyTitle>{t("ap.notRunningTitle")}</EmptyTitle>
              <EmptyDescription>
                {t("ap.notRunningDescription")}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <Table>
            <TableBody>
              {networkDetails.map((detail) => (
                <TableRow key={detail.label}>
                  <TableCell className="text-muted-foreground">{detail.label}</TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {detail.value}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <DialogFooter>
          <DialogClose asChild><Button variant="outline">{t("common.close")}</Button></DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
