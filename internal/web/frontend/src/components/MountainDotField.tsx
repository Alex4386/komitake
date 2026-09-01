import { useEffect, useRef } from "react";
import { useTheme } from "next-themes";

const MAX_DEVICE_PIXEL_RATIO = 2;
const PARALLAX_X = 22;
const PARALLAX_Y = 12;
const KOMITAKE_LAYER_FACTOR = 1.18;
const MAX_DRIFT_X = 1.8;
const FRAME_INTERVAL_MILLISECONDS = 1000 / 30;

type MountainDot = {
  x: number;
  y: number;
  radius: number;
  opacity: number;
  phase: number;
  komitake: boolean;
};

type MountainScene = {
  width: number;
  height: number;
  devicePixelRatio: number;
  fujiColor: string;
  komitakeColor: string;
  dots: MountainDot[];
};

export function MountainDotField() {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const { resolvedTheme } = useTheme();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    let scene = buildMountainScene(canvas);
    let animationFrame = 0;
    let targetX = 0;
    let targetY = 0;
    let currentX = 0;
    let currentY = 0;
    let visible = !document.hidden;
    let lastRenderedAt = 0;

    const render = (timestamp: number) => {
      if (timestamp - lastRenderedAt < FRAME_INTERVAL_MILLISECONDS) {
        animationFrame = requestAnimationFrame(render);
        return;
      }
      lastRenderedAt = timestamp;
      currentX += (targetX - currentX) * 0.09;
      currentY += (targetY - currentY) * 0.055;
      drawMountainScene(canvas, scene, timestamp, currentX, currentY, reducedMotion.matches);
      if (visible && !reducedMotion.matches) animationFrame = requestAnimationFrame(render);
    };
    const restart = () => {
      cancelAnimationFrame(animationFrame);
      if (visible && !reducedMotion.matches) {
        animationFrame = requestAnimationFrame(render);
      } else {
        drawMountainScene(canvas, scene, 0, currentX, currentY, true);
      }
    };
    const rebuild = () => {
      scene = buildMountainScene(canvas);
      restart();
    };
    const onPointerMove = (event: PointerEvent) => {
      targetX = ((event.clientX / window.innerWidth) * 2 - 1) * PARALLAX_X;
      targetY = ((event.clientY / window.innerHeight) * 2 - 1) * PARALLAX_Y;
    };
    const onPointerLeave = () => {
      targetX = 0;
      targetY = 0;
    };
    const onDeviceOrientation = (event: DeviceOrientationEvent) => {
      if (event.gamma !== null) targetX = clamp(event.gamma / 30, -1, 1) * PARALLAX_X;
      if (event.beta !== null) targetY = clamp((event.beta - 45) / 45, -1, 1) * PARALLAX_Y;
    };
    const onVisibilityChange = () => {
      visible = !document.hidden;
      restart();
    };

    const resizeObserver = new ResizeObserver(rebuild);
    resizeObserver.observe(canvas);
    window.addEventListener("pointermove", onPointerMove, { passive: true });
    document.documentElement.addEventListener("pointerleave", onPointerLeave, { passive: true });
    window.addEventListener("deviceorientation", onDeviceOrientation, { passive: true });
    document.addEventListener("visibilitychange", onVisibilityChange);
    reducedMotion.addEventListener("change", restart);
    restart();

    return () => {
      cancelAnimationFrame(animationFrame);
      resizeObserver.disconnect();
      window.removeEventListener("pointermove", onPointerMove);
      document.documentElement.removeEventListener("pointerleave", onPointerLeave);
      window.removeEventListener("deviceorientation", onDeviceOrientation);
      document.removeEventListener("visibilitychange", onVisibilityChange);
      reducedMotion.removeEventListener("change", restart);
    };
  }, [resolvedTheme]);

  return <canvas ref={canvasRef} aria-hidden="true" className="size-full" />;
}

function buildMountainScene(canvas: HTMLCanvasElement): MountainScene {
  const bounds = canvas.getBoundingClientRect();
  const width = Math.max(1, bounds.width);
  const height = Math.max(1, bounds.height);
  const devicePixelRatio = Math.min(window.devicePixelRatio || 1, MAX_DEVICE_PIXEL_RATIO);
  canvas.width = Math.round(width * devicePixelRatio);
  canvas.height = Math.round(height * devicePixelRatio);

  const styles = getComputedStyle(canvas);
  const fujiColor = styles.getPropertyValue("--landing-fuji-dot").trim() || "white";
  const komitakeColor = styles.getPropertyValue("--landing-komitake-dot").trim() || "#facc15";
  // Size the geological section from viewport height only. Width changes reveal or crop
  // the same proportional slopes rather than making the mountain taller.
  const mountainScale = height / 9;
  const areaBasedSpacing = Math.sqrt((width * height) / 9000);
  const spacing = clamp(Math.max(mountainScale * 0.095, areaBasedSpacing), 8, 24);
  // Extra columns on each side so parallax + wiggle never expose empty edges.
  const horizontalMargin = Math.ceil(PARALLAX_X * KOMITAKE_LAYER_FACTOR + MAX_DRIFT_X) + spacing;
  // Keep the shared basement below the viewport so parallax and wiggle never reveal
  // a transparent ground band at the bottom edge.
  const baseY = height + spacing * 2;
  const fujiCenterX = width * 0.68;
  const fujiHalfWidth = mountainScale * 8.6;
  const fujiHeight = mountainScale * 7.15;
  const fujiPeakY = baseY - fujiHeight;

  // GSJ drilling studies model Komitake as an analogous cone peaking near 2,300 m,
  // versus Fuji at 3,776 m, over a shared ~300 m basement: (2300-300)/(3776-300).
  const komitakeScale = 2000 / 3476;
  // This section is oriented south-to-north from left to right. Place the older
  // edifice's summit distinctly on Fuji's northern (right-hand) flank.
  const komitakeCenterX = fujiCenterX + fujiHalfWidth * 0.38;
  const komitakeHalfWidth = fujiHalfWidth * komitakeScale;
  const komitakeHeight = fujiHeight * komitakeScale;
  const komitakePeakY = baseY - komitakeHeight;
  const dots: MountainDot[] = [];

  for (let gridY = spacing / 2; gridY <= height + spacing; gridY += spacing) {
    for (let gridX = spacing / 2 - horizontalMargin; gridX <= width + horizontalMargin; gridX += spacing) {
      const fujiSurfaceY = mountainSurfaceY(gridX, fujiCenterX, fujiHalfWidth, fujiPeakY, baseY);
      const komitakeSurfaceY = mountainSurfaceY(
        gridX,
        komitakeCenterX,
        komitakeHalfWidth,
        komitakePeakY,
        baseY,
      );
      const insideFuji = fujiSurfaceY !== null && gridY >= fujiSurfaceY;
      const insideKomitake = komitakeSurfaceY !== null && gridY >= komitakeSurfaceY;
      if (!insideFuji && !insideKomitake) continue;
      const komitake = insideKomitake;
      const activeSurfaceY = komitake ? komitakeSurfaceY : fujiSurfaceY;
      if (activeSurfaceY === null) continue;
      const surfaceDistance = gridY - activeSurfaceY;
      const surfaceEmphasis = Math.max(0, 1 - surfaceDistance / (spacing * 2.5));
      const depthRatio = Math.min(1, surfaceDistance / Math.max(1, baseY - fujiPeakY));
      dots.push({
        x: gridX,
        y: gridY,
        radius: spacing * (komitake ? 0.2 : 0.17) + surfaceEmphasis * 0.45,
        opacity: komitake
          ? 0.58 + surfaceEmphasis * 0.38
          : 0.16 + surfaceEmphasis * 0.64 - depthRatio * 0.06,
        phase: gridX * 0.031 + gridY * 0.017,
        komitake,
      });
    }
  }

  return { width, height, devicePixelRatio, fujiColor, komitakeColor, dots };
}

function drawMountainScene(
  canvas: HTMLCanvasElement,
  scene: MountainScene,
  timestamp: number,
  parallaxX: number,
  parallaxY: number,
  staticFrame: boolean,
): void {
  const context = canvas.getContext("2d");
  if (!context) return;
  context.setTransform(scene.devicePixelRatio, 0, 0, scene.devicePixelRatio, 0, 0);
  context.clearRect(0, 0, scene.width, scene.height);
  const time = timestamp * 0.001;

  for (const dot of scene.dots) {
    const depth = 0.25 + dot.y / scene.height * 0.75;
    const layerFactor = dot.komitake ? KOMITAKE_LAYER_FACTOR : 0.72;
    const driftX = staticFrame ? 0 : Math.sin(time * 0.24 + dot.phase * 0.35) * (dot.komitake ? 1.8 : 1.15);
    const driftY = staticFrame ? 0 : Math.cos(time * 0.31 + dot.phase) * (dot.komitake ? 1.4 : 0.85);
    const wiggle = staticFrame ? 0 : Math.sin(time * 0.85 + dot.phase) * 0.65;
    const x = dot.x + parallaxX * depth * layerFactor + driftX;
    const y = dot.y + parallaxY * depth * layerFactor + driftY;

    context.globalAlpha = clamp(dot.opacity + wiggle * 0.045, 0.08, 1);
    context.fillStyle = dot.komitake ? scene.komitakeColor : scene.fujiColor;
    context.beginPath();
    context.arc(x, y, Math.max(0.4, dot.radius + wiggle * 0.12), 0, Math.PI * 2);
    context.fill();
  }
  context.globalAlpha = 1;
}

function mountainSurfaceY(
  x: number,
  centerX: number,
  halfWidth: number,
  peakY: number,
  baseY: number,
): number | null {
  const offset = Math.abs(x - centerX);
  if (offset > halfWidth) return null;
  // Truncated crater rim (~500-800 m on Fuji; apply the same silhouette to Komitake).
  const flatHalfWidth = halfWidth * 0.1;
  if (offset <= flatHalfWidth) return peakY;
  const slopeSpan = halfWidth - flatHalfWidth;
  const slopeProgress = ((offset - flatHalfWidth) / slopeSpan) ** 0.78;
  return peakY + (baseY - peakY) * slopeProgress;
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}
