import type { Kart } from "@/lib/api";

export function formatKartLabel(kart: Kart): string {
  if (kart.serial) return kart.serial;
  return kart.ident.length > 12 ? `${kart.ident.slice(0, 12)}…` : kart.ident;
}

export function kartRouteSlug(kart: Kart): string {
  return kart.serial || kart.ident;
}

export function resolveKartSlug(slug: string, karts: Kart[]): Kart | null {
  const normalizedSlug = slug.toLowerCase();
  const matches = karts.filter((kart) => {
    return kart.ident.toLowerCase().startsWith(normalizedSlug)
      || kart.serial?.toLowerCase().startsWith(normalizedSlug);
  });
  return matches.length === 1 ? matches[0] : null;
}

export function readStoredLimit(key: string): number {
  const rawValue = localStorage.getItem(key);
  if (rawValue === null) return 1;
  const storedValue = Number(rawValue);
  if (!Number.isFinite(storedValue)) return 1;
  return Math.min(1, Math.max(0.1, storedValue));
}
