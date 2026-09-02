import i18n from "@/i18n";

type FullscreenDocument = Document & {
  webkitFullscreenEnabled?: boolean;
  webkitFullscreenElement?: Element | null;
  webkitExitFullscreen?: () => Promise<void> | void;
};

type FullscreenElement = HTMLElement & {
  webkitRequestFullscreen?: () => Promise<void> | void;
};

// iOS Safari on iPhone has no element Fullscreen API; the only native fullscreen
// is on <video> via webkitEnterFullscreen. iPad and desktop expose the standard
// or webkit element APIs above.
type IOSVideoElement = HTMLVideoElement & {
  webkitEnterFullscreen?: () => void;
  webkitExitFullscreen?: () => void;
  webkitSupportsFullscreen?: boolean;
};

export function isFullscreenSupported(): boolean {
  if (typeof document === "undefined") return false;
  const doc = document as FullscreenDocument;
  return Boolean(doc.fullscreenEnabled || doc.webkitFullscreenEnabled);
}

// isVideoElementFullscreenSupported reports whether a <video> can go fullscreen
// natively (iPhone Safari path) when the element Fullscreen API is absent.
export function isVideoElementFullscreenSupported(): boolean {
  if (typeof HTMLVideoElement === "undefined") return false;
  return typeof (HTMLVideoElement.prototype as IOSVideoElement).webkitEnterFullscreen === "function";
}

// requestVideoElementFullscreen enters native fullscreen on a video element
// (iPhone Safari). Returns false when the element does not support it.
export function requestVideoElementFullscreen(video: HTMLVideoElement | null): boolean {
  if (!video) return false;
  const iosVideo = video as IOSVideoElement;
  if (typeof iosVideo.webkitEnterFullscreen === "function") {
    // webkitSupportsFullscreen is only true once metadata has loaded.
    if (iosVideo.webkitSupportsFullscreen === false) return false;
    iosVideo.webkitEnterFullscreen();
    return true;
  }
  return false;
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
