import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { KartLanding } from "@/components/KartLanding";
import { PairDialog } from "@/components/PairDialog";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useRealtime } from "@/hooks/useRealtime";
import { brandDocumentTitle } from "@/lib/brand";
import { kartRouteSlug } from "@/lib/dashboard";

export function LandingPage() {
  const navigate = useNavigate();
  const { status, devices, conn } = useRealtime();
  const [pairOpen, setPairOpen] = useState(false);

  useEffect(() => {
    document.title = brandDocumentTitle();
  }, []);

  const selectKart = (kartId: string) => {
    const kart = devices.find((candidate) => candidate.ident === kartId);
    if (!kart) return;
    navigate(`/karts/${encodeURIComponent(kartRouteSlug(kart))}`);
  };

  return (
    <TooltipProvider>
      <div className="theme flex h-svh bg-background text-foreground">
        <KartLanding
          variant="page"
          loading={conn !== "open"}
          devices={devices}
          onSelect={selectKart}
          onPair={() => setPairOpen(true)}
          onReturn={() => undefined}
        />
        <PairDialog open={pairOpen} onOpenChange={setPairOpen} status={status} />
        <Toaster />
      </div>
    </TooltipProvider>
  );
}
