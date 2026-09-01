import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Globe, Lock, Plug, SlidersHorizontal, Video } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { api, type ServiceSettings } from "@/lib/api";
import { brandDocumentTitle } from "@/lib/brand";
import { cn } from "@/lib/utils";

type SettingsSection = "web" | "video" | "tls" | "socket";

type LocationState = {
  from?: string;
};

const fieldClass =
  "h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30";

function argsToText(args?: string[]): string {
  return (args ?? []).join("\n");
}

function textToArgs(text: string): string[] {
  return text.split("\n").map((line) => line.trim()).filter(Boolean);
}

function SettingsRow({
  label,
  hint,
  children,
}: {
  label: React.ReactNode;
  hint?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="grid gap-2 border-b py-3 last:border-b-0 md:grid-cols-[minmax(0,10rem)_minmax(0,1fr)] md:items-center md:gap-6">
      <div>
        <div className="text-sm font-medium">{label}</div>
        {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  );
}

export function SettingsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as LocationState | null)?.from;
  const [section, setSection] = useState<SettingsSection>("web");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [webBind, setWebBind] = useState("");
  const [tlsEnabled, setTlsEnabled] = useState(false);
  const [tlsCertFile, setTlsCertFile] = useState("");
  const [tlsKeyFile, setTlsKeyFile] = useState("");
  const [socketBind, setSocketBind] = useState("");
  const [socketChmod, setSocketChmod] = useState("");
  const [videoHwaccel, setVideoHwaccel] = useState("auto");
  const [videoFFmpegPath, setVideoFFmpegPath] = useState("");
  const [videoFFmpegProfile, setVideoFFmpegProfile] = useState("");
  const [videoFFmpegArgsInput, setVideoFFmpegArgsInput] = useState("");
  const [videoFFmpegArgsOutput, setVideoFFmpegArgsOutput] = useState("");
  const [metadata, setMetadata] = useState<ServiceSettings | null>(null);

  const backTarget = from && from !== "/settings" ? from : "/";
  const backLabel = from?.startsWith("/karts/")
    ? t("settings.backToDashboard")
    : t("settings.backToHome");

  const navItems = useMemo(
    () => [
      { id: "web" as const, label: t("settings.navWeb"), icon: Globe },
      { id: "video" as const, label: t("settings.navVideo"), icon: Video },
      { id: "tls" as const, label: t("settings.navTls"), icon: Lock },
      { id: "socket" as const, label: t("settings.navSocket"), icon: Plug },
    ],
    [t],
  );

  useEffect(() => {
    document.title = brandDocumentTitle(t("settings.pageTitle"));
  }, [t]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      try {
        const settings = await api.getSettings();
        if (cancelled) return;
        setMetadata(settings);
        setWebBind(settings.web?.bind ?? "");
        setTlsEnabled(settings.web?.tls?.enabled ?? false);
        setTlsCertFile(settings.web?.tls?.cert_file ?? "");
        setTlsKeyFile(settings.web?.tls?.key_file ?? "");
        setSocketBind(settings.socket?.bind ?? "");
        setSocketChmod(settings.socket?.chmod ?? "");
        setVideoHwaccel(settings.video?.hwaccel || settings.defaults.video?.hwaccel || "auto");
        setVideoFFmpegPath(settings.video?.ffmpeg_path ?? "");
        setVideoFFmpegProfile(settings.video?.ffmpeg_profile ?? "");
        setVideoFFmpegArgsInput(argsToText(settings.video?.ffmpeg_args?.input));
        setVideoFFmpegArgsOutput(argsToText(settings.video?.ffmpeg_args?.output));
      } catch (error) {
        toast.error((error as Error).message);
        navigate(backTarget);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [backTarget, navigate]);

  async function save() {
    setBusy(true);
    try {
      const settings = await api.putSettings({
        web: {
          bind: webBind.trim(),
          tls: {
            enabled: tlsEnabled,
            cert_file: tlsCertFile.trim(),
            key_file: tlsKeyFile.trim(),
          },
        },
        socket: { bind: socketBind.trim(), chmod: socketChmod.trim() },
        video: {
          hwaccel: videoHwaccel,
          ffmpeg_path: videoFFmpegPath.trim(),
          ffmpeg_profile: videoFFmpegProfile,
          ffmpeg_args: {
            input: textToArgs(videoFFmpegArgsInput),
            output: textToArgs(videoFFmpegArgsOutput),
          },
        },
      });
      setMetadata(settings);
      toast.success(t("settings.saved"));
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const configPath = metadata?.config_path ?? t("settings.configJson");
  const sectionMeta = {
    web: t("settings.sectionWebTitle"),
    video: t("settings.sectionVideoTitle"),
    tls: t("settings.sectionTlsTitle"),
    socket: t("settings.sectionSocketTitle"),
  }[section];

  return (
    <TooltipProvider>
      <div className="theme min-h-svh bg-background text-foreground">
        <header className="sticky top-0 z-10 flex items-center gap-3 border-b bg-background px-4 py-3 md:px-6">
          <Button variant="ghost" size="sm" className="-ml-2 gap-1.5" asChild>
            <Link to={backTarget} replace={false}>
              <ArrowLeft />
              {backLabel}
            </Link>
          </Button>
        </header>

        <div className="mx-auto flex w-full max-w-5xl flex-col lg:flex-row">
          <aside className="border-b lg:w-52 lg:shrink-0 lg:border-r lg:border-b-0">
            <div className="px-4 py-4 md:px-5">
              <div className="flex items-center gap-2 text-sm font-semibold">
                <SlidersHorizontal className="size-4 text-muted-foreground" />
                {t("settings.pageTitle")}
              </div>
            </div>
            <Separator className="hidden lg:block" />
            <nav className="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-col lg:gap-0.5 lg:px-2 lg:py-2">
              {navItems.map(({ id, label, icon: Icon }) => (
                <Button
                  key={id}
                  variant={section === id ? "secondary" : "ghost"}
                  size="sm"
                  className={cn(
                    "shrink-0 justify-start lg:w-full",
                    section === id && "font-medium",
                  )}
                  onClick={() => setSection(id)}
                >
                  <Icon data-icon="inline-start" />
                  {label}
                </Button>
              ))}
            </nav>
          </aside>

          <main className="flex-1 px-4 py-5 md:px-8 md:py-6">
            <h1 className="font-heading mb-4 text-xl font-semibold tracking-tight">{sectionMeta}</h1>

            {loading ? (
              <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
                <Spinner />
                {t("common.loading")}
              </div>
            ) : (
              <form
                className="rounded-xl border bg-card"
                onSubmit={(event) => {
                  event.preventDefault();
                  void save();
                }}
              >
                <div className="border-b px-4 py-2 text-xs text-muted-foreground md:px-5">
                  {t("settings.savedTo", { path: configPath })}
                </div>
                <div className="px-4 md:px-5">
                  {section === "web" && (
                    <SettingsRow label={t("settings.webBindLabel")} hint={t("settings.webBindHint")}>
                      <Input
                        id="web-bind"
                        value={webBind}
                        placeholder={metadata?.defaults.web.bind || "127.0.0.1:8080"}
                        onChange={(event) => setWebBind(event.target.value)}
                        disabled={busy}
                        autoComplete="off"
                        spellCheck={false}
                      />
                    </SettingsRow>
                  )}

                  {section === "video" && (
                    <>
                      <SettingsRow label={t("settings.videoHwaccelLabel")} hint={t("settings.videoHwaccelHint")}>
                        <Select
                          value={videoHwaccel}
                          onValueChange={setVideoHwaccel}
                          disabled={busy}
                        >
                          <SelectTrigger id="video-hwaccel" className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              <SelectItem value="auto">{t("settings.videoHwaccelAuto")}</SelectItem>
                              <SelectItem value="none">{t("settings.videoHwaccelNone")}</SelectItem>
                              <SelectItem value="vaapi">{t("settings.videoHwaccelVaapi")}</SelectItem>
                              <SelectItem value="nvenc">{t("settings.videoHwaccelNvenc")}</SelectItem>
                              <SelectItem value="qsv">{t("settings.videoHwaccelQsv")}</SelectItem>
                              <SelectItem value="custom">{t("settings.videoHwaccelCustom")}</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </SettingsRow>
                      {videoHwaccel !== "none" && (
                        <>
                      <SettingsRow label={t("settings.videoFFmpegPathLabel")} hint={t("settings.videoFFmpegPathHint")}>
                        <Input
                          id="video-ffmpeg-path"
                          value={videoFFmpegPath}
                          placeholder="ffmpeg"
                          onChange={(event) => setVideoFFmpegPath(event.target.value)}
                          disabled={busy}
                          autoComplete="off"
                          spellCheck={false}
                        />
                      </SettingsRow>
                      <SettingsRow label={t("settings.videoFFmpegProfileLabel")} hint={t("settings.videoFFmpegProfileHint")}>
                        <Select
                          value={videoFFmpegProfile || "default"}
                          onValueChange={(value) => setVideoFFmpegProfile(value === "default" ? "" : value)}
                          disabled={busy}
                        >
                          <SelectTrigger id="video-ffmpeg-profile" className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              <SelectItem value="default">{t("settings.videoFFmpegProfileDefault")}</SelectItem>
                              <SelectItem value="realtime">{t("settings.videoFFmpegProfileRealtime")}</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </SettingsRow>
                      {videoHwaccel === "custom" && (
                        <>
                          <SettingsRow label={t("settings.videoFFmpegArgsInputLabel")} hint={t("settings.videoFFmpegArgsInputHint")}>
                            <textarea
                              id="video-ffmpeg-args-input"
                              className={cn(fieldClass, "min-h-24 resize-y py-2 font-mono text-xs")}
                              value={videoFFmpegArgsInput}
                              onChange={(event) => setVideoFFmpegArgsInput(event.target.value)}
                              disabled={busy}
                              spellCheck={false}
                            />
                          </SettingsRow>
                          <SettingsRow label={t("settings.videoFFmpegArgsOutputLabel")} hint={t("settings.videoFFmpegArgsOutputHint")}>
                            <textarea
                              id="video-ffmpeg-args-output"
                              className={cn(fieldClass, "min-h-24 resize-y py-2 font-mono text-xs")}
                              value={videoFFmpegArgsOutput}
                              onChange={(event) => setVideoFFmpegArgsOutput(event.target.value)}
                              disabled={busy}
                              spellCheck={false}
                            />
                          </SettingsRow>
                        </>
                      )}
                        </>
                      )}
                    </>
                  )}

                  {section === "tls" && (
                    <>
                      <SettingsRow label={t("settings.tlsEnabledLabel")} hint={t("settings.tlsEnabledHint")}>
                        <Switch
                          checked={tlsEnabled}
                          onCheckedChange={setTlsEnabled}
                          disabled={busy}
                          aria-label={t("settings.tlsEnabledLabel")}
                        />
                      </SettingsRow>
                      <SettingsRow label={t("settings.tlsCertLabel")} hint={t("settings.tlsCertHint")}>
                        <Input
                          id="tls-cert-file"
                          value={tlsCertFile}
                          placeholder="/etc/komitake/web.crt"
                          onChange={(event) => setTlsCertFile(event.target.value)}
                          disabled={busy}
                          autoComplete="off"
                          spellCheck={false}
                        />
                      </SettingsRow>
                      <SettingsRow label={t("settings.tlsKeyLabel")} hint={t("settings.tlsKeyHint")}>
                        <Input
                          id="tls-key-file"
                          type="password"
                          value={tlsKeyFile}
                          placeholder="/etc/komitake/web.key"
                          onChange={(event) => setTlsKeyFile(event.target.value)}
                          disabled={busy}
                          autoComplete="off"
                          spellCheck={false}
                        />
                      </SettingsRow>
                    </>
                  )}

                  {section === "socket" && (
                    <>
                      <SettingsRow label={t("settings.socketBindLabel")} hint={t("settings.socketBindHint")}>
                        <Input
                          id="socket-bind"
                          value={socketBind}
                          placeholder={metadata?.defaults.socket.bind || "unix:/run/komitake.sock"}
                          onChange={(event) => setSocketBind(event.target.value)}
                          disabled={busy}
                          autoComplete="off"
                          spellCheck={false}
                        />
                      </SettingsRow>
                      <SettingsRow label={t("settings.socketChmodLabel")} hint={t("settings.socketChmodHint")}>
                        <Input
                          id="socket-chmod"
                          value={socketChmod}
                          placeholder={metadata?.defaults.socket.chmod || "0600"}
                          onChange={(event) => setSocketChmod(event.target.value)}
                          disabled={busy}
                          autoComplete="off"
                          spellCheck={false}
                        />
                      </SettingsRow>
                    </>
                  )}
                </div>

                <div className="flex justify-end gap-2 border-t px-4 py-3 md:px-5">
                  <Button type="button" variant="outline" disabled={busy} onClick={() => navigate(backTarget)}>
                    {t("common.cancel")}
                  </Button>
                  <Button type="submit" disabled={busy || loading}>
                    {busy && <Spinner data-icon="inline-start" />}
                    {t("settings.save")}
                  </Button>
                </div>
              </form>
            )}
          </main>
        </div>
        <Toaster />
      </div>
    </TooltipProvider>
  );
}
