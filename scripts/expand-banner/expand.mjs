#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import opentype from "opentype.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "../..");
const fontDir = path.join(repoRoot, "docs/fonts");
const srcPath = path.join(repoRoot, "docs/banner.svg");
const dstPath = path.join(repoRoot, "docs/banner.expanded.svg");

const STYLES = {
  title: { fontSize: 72, weight: "bold", letterSpacing: -0.03 },
  ruby: { fontSize: 22, weight: "medium", letterSpacing: 0.04 },
  desc: { fontSize: 22, weight: "regular", letterSpacing: 0 },
};

function pretendardCandidates(weightName) {
  const envKey = `KOMITAKE_BANNER_FONT_${weightName.toUpperCase()}`;
  const fileName = `PretendardJP-${weightName}.otf`;
  const home = process.env.HOME ?? "";
  return [
    process.env[envKey],
    path.join(fontDir, fileName),
    `/usr/share/fonts/opentype/pretendard/${fileName}`,
    `/usr/share/fonts/truetype/pretendard/${fileName}`,
    home ? path.join(home, ".local/share/fonts", fileName) : null,
    `/Library/Fonts/${fileName}`,
    path.join(home, "Library/Fonts", fileName),
  ];
}

const FONT_CANDIDATES = {
  bold: pretendardCandidates("Bold"),
  medium: pretendardCandidates("Medium"),
  regular: pretendardCandidates("Regular"),
};

function firstExisting(paths) {
  for (const candidate of paths) {
    if (!candidate) continue;
    if (fs.existsSync(candidate)) return candidate;
  }
  return null;
}

function loadFont(weight) {
  const fontPath = firstExisting(FONT_CANDIDATES[weight]);
  if (!fontPath) {
    throw new Error(
      `Pretendard JP ${weight} not found; run ./scripts/expand-banner.sh to download fonts into docs/fonts/`,
    );
  }
  return { font: opentype.loadSync(fontPath), fontPath };
}

function textToPath(font, text, x, y, fontSize, letterSpacingEm = 0) {
  const parts = [];
  let cursorX = x;
  const tracking = letterSpacingEm * fontSize;
  const scale = fontSize / font.unitsPerEm;
  for (const char of text) {
    const glyph = font.charToGlyph(char);
    parts.push(glyph.getPath(cursorX, y, fontSize).toPathData(2));
    cursorX += glyph.advanceWidth * scale + tracking;
  }
  return parts.join(" ");
}

function parseCopyGroup(copyInner) {
  const items = [];
  const textRegex = /<text class="([^"]+)" x="([^"]+)" y="([^"]+)">([\s\S]*?)<\/text>/g;
  for (const match of copyInner.matchAll(textRegex)) {
    const [, className, x, y, inner] = match;
    const tspans = [...inner.matchAll(/<tspan x="([^"]+)" dy="([^"]+)">([^<]*)<\/tspan>/g)];
    if (tspans.length > 0) {
      let currentY = Number(y);
      for (const [, tspanX, dy, text] of tspans) {
        currentY += Number(dy);
        items.push({ className, x: Number(tspanX), y: currentY, text });
      }
      continue;
    }
    const plain = inner.replace(/\s+/g, " ").trim();
    items.push({ className, x: Number(x), y: Number(y), text: plain });
  }
  return items;
}

function expandCopyGroup(copyInner) {
  const paths = [];
  for (const item of parseCopyGroup(copyInner)) {
    const style = STYLES[item.className];
    if (!style) {
      throw new Error(`unknown text class ${item.className}`);
    }
    const { font, fontPath } = loadFont(style.weight);
    const d = textToPath(font, item.text, item.x, item.y, style.fontSize, style.letterSpacing);
    paths.push(`    <path class="${item.className}" d="${d}"/>`);
    process.stderr.write(`expanded ${item.className}: ${path.basename(fontPath)}\n`);
  }
  return `<g id="copy" class="copy">\n${paths.join("\n")}\n  </g>`;
}

function stripFontCSS(css) {
  return css
    .replace(/\s*font-family:[^;]+;/g, "")
    .replace(/\s*font-size:[^;]+;/g, "")
    .replace(/\s*font-weight:[^;]+;/g, "")
    .replace(/\s*letter-spacing:[^;]+;/g, "");
}

function expandBanner(svg) {
  const copyMatch = svg.match(/<g[^>]*class="copy"[^>]*>([\s\S]*?)<\/g>/);
  if (!copyMatch) {
    throw new Error('banner.svg is missing <g class="copy">');
  }
  const expandedCopy = expandCopyGroup(copyMatch[1]);
  let out = svg.replace(/<g[^>]*class="copy"[^>]*>[\s\S]*?<\/g>/, expandedCopy);
  out = out.replace(
    /<style type="text\/css"><!\[CDATA\[([\s\S]*?)\]\]><\/style>/,
    (_, css) => `<style type="text/css"><![CDATA[${stripFontCSS(css)}]]></style>`,
  );
  return out;
}

function main() {
  const svg = fs.readFileSync(srcPath, "utf8");
  const expanded = expandBanner(svg);
  fs.writeFileSync(dstPath, expanded);
  process.stdout.write(`wrote ${dstPath}\n`);
}

main();
