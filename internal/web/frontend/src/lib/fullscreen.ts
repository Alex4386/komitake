import i18n from "@/i18n";

type FullscreenDocument = Document & {
  webkitFullscreenEnabled?: boolean;
  webkitFullscreenElement?: Element | null;
  webkitExitFullscreen?: () => Promise<void> | void;
};

type FullscreenElement = HTMLElement & {
  webkitRequestFullscreen?: () => Promise<void> | void;
};

export function isFullscreenSupported(): boolean {
  if (typeof document === "undefined") return false;
  const doc = document as FullscreenDocument;
  return Boolean(doc.fullscreenEnabled || doc.webkitFullscreenEnabled);
}

export function getFullscreenElement(): Element | null {
  const doc = document as FullscreenDocument;
  return doc.fullscreenElement ?? doc.webkitFullscreenElement ?? null;
}

export async function requestElementFullscreen(element: HTMLElement): Promise<void> {
  const target = element as FullscreenElement;
  if (typeof target.requestFullscreen === "function") {
    await target.requestFullscreen();
    return;
  }
  if (typeof target.webkitRequestFullscreen === "function") {
    await target.webkitRequestFullscreen();
    return;
  }
  throw new Error(i18n.t("errors.fullscreenNotSupported"));
}

export async function exitElementFullscreen(): Promise<void> {
  const doc = document as FullscreenDocument;
  if (doc.fullscreenElement && typeof doc.exitFullscreen === "function") {
    await doc.exitFullscreen();
    return;
  }
  if (doc.webkitFullscreenElement && typeof doc.webkitExitFullscreen === "function") {
    await doc.webkitExitFullscreen();
    return;
  }
}

export function subscribeFullscreenChange(onChange: () => void): () => void {
  document.addEventListener("fullscreenchange", onChange);
  document.addEventListener("webkitfullscreenchange", onChange);
  return () => {
    document.removeEventListener("fullscreenchange", onChange);
    document.removeEventListener("webkitfullscreenchange", onChange);
  };
}
