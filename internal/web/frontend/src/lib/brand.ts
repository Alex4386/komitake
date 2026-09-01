export const BRAND_NAME = "Komitake";

export function brandDocumentTitle(suffix?: string): string {
  return suffix ? `${BRAND_NAME} · ${suffix}` : BRAND_NAME;
}
