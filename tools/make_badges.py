#!/usr/bin/env python3
"""Generate the README custom badge SVGs (premium dark-glass pill style).

Single source of truth for the bespoke README badges. Style: a near-black pill
with a soft top-gloss gradient, rim highlight and drop shadow, a centered
uppercase gray label and a centered white value split by a faint divider.
(Mirrors the tossinvest-cli badge set for cross-repo consistency.)

Usage: python3 tools/make_badges.py
Output: docs/badges/*.svg
"""
from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "docs" / "badges"

PH, SP, RX = 50, 7, 8  # pill height, shadow padding, corner radius


def _lw(s: str) -> float:
    return len(s) * 8.0  # uppercase label, font 12 + letter-spacing 2


def _vw(s: str) -> float:
    return sum(12.2 if c.isupper() else (5.0 if c in " ·." else 10.1) for c in s)


_DEFS = '''
 <defs>
  <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
   <stop offset="0" stop-color="#1a1b1d"/><stop offset="0.55" stop-color="#101113"/><stop offset="1" stop-color="#0a0a0b"/>
  </linearGradient>
  <radialGradient id="gloss" cx="0.5" cy="0" r="0.85">
   <stop offset="0" stop-color="#ffffff" stop-opacity="0.10"/><stop offset="0.65" stop-color="#ffffff" stop-opacity="0"/>
  </radialGradient>
  <filter id="sh" x="-12%" y="-12%" width="124%" height="148%">
   <feDropShadow dx="0" dy="2.5" stdDeviation="3.5" flood-color="#000000" flood-opacity="0.5"/>
  </filter>
 </defs>'''


def _shell(pw: float, extra: str) -> str:
    VW, VH = int(pw + 2 * SP), PH + 2 * SP
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{VW}" height="{VH}" viewBox="0 0 {VW} {VH}">{_DEFS}\n'
        f' <g filter="url(#sh)"><rect x="{SP}" y="{SP}" width="{pw:.1f}" height="{PH}" rx="{RX}" fill="url(#bg)"/></g>\n'
        f' <rect x="{SP}" y="{SP}" width="{pw:.1f}" height="{PH}" rx="{RX}" fill="url(#gloss)"/>\n'
        f' <rect x="{SP + 0.5}" y="{SP + 0.5}" width="{pw - 1:.1f}" height="{PH - 1}" rx="{RX - 0.5:.1f}" fill="none" stroke="#ffffff" stroke-opacity="0.14" stroke-width="1"/>\n'
        f'{extra}\n</svg>\n'
    )


def _esc(s: str) -> str:
    return s.replace("&", "&amp;").replace("<", "&lt;").replace("·", "&#183;")


def badge(fn: str, label: str, value: str) -> None:
    lpad, vpad, minlabel = 22, 26, 92
    lreg = max(_lw(label) + 2 * lpad, minlabel)
    vreg = _vw(value) + 2 * vpad
    pw = lreg + vreg
    cy = SP + PH / 2
    divx = SP + lreg
    lcx, vcx = SP + lreg / 2, divx + vreg / 2
    extra = (
        f' <line x1="{divx:.1f}" y1="{SP + 15}" x2="{divx:.1f}" y2="{SP + PH - 15}" stroke="#ffffff" stroke-opacity="0.16" stroke-width="1.2"/>\n'
        f' <text x="{lcx:.1f}" y="{cy:.1f}" text-anchor="middle" dominant-baseline="central" font-family="ui-sans-serif,-apple-system,Segoe UI,Roboto,sans-serif" font-size="12" font-weight="500" letter-spacing="2" fill="#8b8d94">{_esc(label.upper())}</text>\n'
        f' <text x="{vcx:.1f}" y="{cy:.1f}" text-anchor="middle" dominant-baseline="central" font-family="ui-sans-serif,-apple-system,Segoe UI,Roboto,sans-serif" font-size="21" font-weight="600" fill="#fafafa">{_esc(value)}</text>'
    )
    (OUT / fn).write_text(_shell(pw, extra))
    print(f"  {fn}  {label} | {value}")


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    # 새 디자인으로 덮어쓰기 전, 구 flat 버전 정리
    for old in OUT.glob("*.svg"):
        old.unlink()

    badge("license.svg", "License", "MIT")
    badge("keyless-core.svg", "Core", "Keyless")
    badge("go.svg", "Built with", "Go 1.25+")
    badge("agents.svg", "Works with", "Claude · Codex · jq")
    badge("output.svg", "Output", "JSON · JSONL · Table")


if __name__ == "__main__":
    main()
