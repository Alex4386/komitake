import { useState } from "react";
import { BatteryCharging, BatteryMedium, Gamepad2, Wifi } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
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
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import { api, type Kart } from "@/lib/api";

type Props = {
  kart: Kart | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function KartDetailDialog({ kart, open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const [shuttingDown, setShuttingDown] = useState(false);
  const deviceDetails = kart
    ? [
        { label: t("kartDetail.id"), value: kart.ident },
        { label: t("kartDetail.type"), value: kart.kind },
        { label: t("kartDetail.address"), value: kart.address || "—" },
        { label: t("kartDetail.macAddress"), value: kart.mac_address || "—" },
      ]
    : [];

  const onShutdown = async () => {
    if (!kart || shuttingDown) return;
    setShuttingDown(true);
    try {
      await api.shutdownKart(kart.ident);
      toast.success(t("kartDetail.shutdownToast", { ident: kart.serial || kart.ident }));
      onOpenChange(false);
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setShuttingDown(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-mono">
            {kart?.serial || t("kartDetail.title")}
          </DialogTitle>
          <DialogDescription>
            {t("kartDetail.description")}
          </DialogDescription>
        </DialogHeader>
        {kart && (
          <div className="flex flex-col gap-4">
            <Table>
              <TableBody>
                {deviceDetails.map((detail) => (
                  <TableRow key={detail.label}>
                    <TableCell className="text-muted-foreground">
                      {detail.label}
                    </TableCell>
                    <TableCell className="max-w-64 truncate text-right font-mono text-xs">
                      {detail.value}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <section aria-labelledby="kart-live-status-title">
              <h3 id="kart-live-status-title" className="mb-2 text-sm font-medium">
                {t("kartDetail.currentStatus")}
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {kart.battery !== undefined && (
                  <Badge variant="outline" size="sm">
                    <BatteryMedium />{kart.battery}/4
                  </Badge>
                )}
                {kart.cable_connected !== undefined && (
                  <Badge
                    variant={kart.cable_connected ? "success-light" : "outline"}
                    size="sm"
                  >
                    <BatteryCharging />
                    {kart.cable_connected ? t("kartDetail.cableConnected") : t("kartDetail.cableDisconnected")}
                  </Badge>
                )}
                <Badge variant={kart.drive_armed ? "info-light" : "outline"} size="sm">
                  <Gamepad2 />{kart.drive_armed ? t("kartDetail.acceptingInput") : t("kartDetail.idle")}
                </Badge>
                {kart.signal_dbm !== undefined && (
                  <Badge variant="outline" size="sm">
                    <Wifi />{kart.signal_dbm} dBm
                  </Badge>
                )}
              </div>
            </section>
          </div>
        )}
        <DialogFooter className="sm:justify-between">
          <Button
            variant="destructive"
            disabled={!kart || shuttingDown}
            onClick={() => void onShutdown()}
          >
            {shuttingDown ? t("kartDetail.shuttingDown") : t("kartDetail.shutdown")}
          </Button>
          <DialogClose asChild><Button variant="outline">{t("common.close")}</Button></DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
