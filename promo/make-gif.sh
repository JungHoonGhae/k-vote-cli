#!/bin/sh
# mp4 → README 히어로 GIF (palette 2-pass). GitHub 인라인 재생은 GIF만 되므로.
set -e
SRC=../docs/k-vote-cli-promo.mp4
OUT=../docs/k-vote-cli-promo.gif
PALETTE=$(mktemp -t kvote-palette).png

ffmpeg -y -i "$SRC" -vf "fps=12,scale=880:-1:flags=lanczos,palettegen=stats_mode=diff" "$PALETTE"
ffmpeg -y -i "$SRC" -i "$PALETTE" \
  -lavfi "fps=12,scale=880:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
  "$OUT"
rm -f "$PALETTE"
ls -la "$OUT"
