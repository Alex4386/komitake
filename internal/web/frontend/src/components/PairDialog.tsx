import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import QRCode from "qrcode";
import { Badge } from "@/components/reui/badge";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { api, base64ToBytes, type Pairing, type Status } from "@/lib/api";
import { toast } from "sonner";

type Props = { open: boolean; onOpenChange: (open: boolean) => void; status: Status | null };

export function PairDialog({ open, onOpenChange, status }: Props) {
  const { t } = useTranslation();
  const [pairing, setPairing] = useState<Pairing | null>(null);
  const [busy, setBusy] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const completedRef = useRef(false);
  const observedPairingRef = useRef(false);

  useEffect(() => {
    if (!open) {
      setPairing(null); setWaiting(false); setBusy(false); completedRef.current = false;
      observedPairingRef.current = false;
      return;
    }
    let cancelled = false;
    completedRef.current = false;
    observedPairingRef.current = false;
    void (async () => {
      setBusy(true);
      try {
        const nextPairing = await api.startPairing();
        if (!cancelled) { setPairing(nextPairing); setWaiting(true); }
      } catch (error) {
        toast.error((error as Error).message);
        if (!cancelled) onOpenChange(false);
      } finally {
        if (!cancelled) setBusy(false);
      }
    })();
    return () => { cancelled = true; };
  }, [open, onOpenChange]);

  useEffect(() => {
    if (!pairing || !canvasRef.current) return;
    QRCode.toCanvas(canvasRef.current, [{ data: base64ToBytes(pairing.qr_payload), mode: "byte" }], {
      version: 4, errorCorrectionLevel: "M", width: 400, margin: 2,
    }).catch((error) => toast.error(String(error)));
  }, [pairing]);

  useEffect(() => {
    if (open && waiting && status?.mode === "pairing") observedPairingRef.current = true;
    if (
      !open ||
      !waiting ||
      !observedPairingRef.current ||
      status?.mode !== "normal" ||
      completedRef.current
    ) return;
    completedRef.current = true;
    setWaiting(false);
    setPairing(null);
    onOpenChange(false);
    toast.success(t("pair.complete"));
  }, [open, waiting, status, onOpenChange, t]);

  async function stop() {
    setBusy(true);
    try {
      await api.stopPairing();
      completedRef.current = true;
      setWaiting(false); setPairing(null); onOpenChange(false);
      toast.message(t("pair.canceled"));
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function handleOpenChange(next: boolean) {
    if (!next && waiting && !completedRef.current) {
      completedRef.current = true;
      void api.stopPairing().catch(() => {});
      setWaiting(false); setPairing(null);
      toast.message(t("pair.canceled"));
    }
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("pair.title")}</DialogTitle>
          <DialogDescription>{t("pair.description")}</DialogDescription>
        </DialogHeader>
        {busy && !pairing ? (
          <Empty className="border-0 py-10"><EmptyHeader><EmptyMedia variant="icon"><Spinner /></EmptyMedia><EmptyTitle>{t("pair.startingTitle")}</EmptyTitle><EmptyDescription>{t("pair.startingDescription")}</EmptyDescription></EmptyHeader></Empty>
        ) : pairing ? (
          <div className="flex flex-col items-center gap-4">
            <div className="rounded-lg border bg-white p-3">
              <canvas
                ref={canvasRef}
                aria-label={t("pair.qrAria")}
                className="h-auto w-full max-w-[min(100%,400px)]"
              />
            </div>
            <p className="text-center text-sm text-muted-foreground">
              {t("common.network")} <span className="font-mono text-foreground">{pairing.ssid}</span>
              <span aria-hidden="true"> · </span>
              {t("common.channel")} <span className="font-mono text-foreground">{pairing.channel}</span>
            </p>
            {waiting && (
              <Badge variant="warning-light" size="lg" radius="full">
                <Spinner />{t("pair.waitingForKart")}
              </Badge>
            )}
          </div>
        ) : null}
        <DialogFooter><Button variant="outline" onClick={stop} disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{t("pair.cancelPairing")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
