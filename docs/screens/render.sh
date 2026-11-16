#!/bin/sh
# Draws every screen in src/ as a picture, in light and in dark mode, into
# png/light and png/dark, named <area>-<screen>.png. Needs Google Chrome.
# Run from anywhere: sh docs/screens/render.sh [only-this-area]
set -e
here=$(cd "$(dirname "$0")" && pwd)
chrome=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
[ -x "$chrome" ] || chrome=$(command -v google-chrome || command -v chromium || true)
[ -n "$chrome" ] || { echo "Google Chrome is needed" >&2; exit 1; }
mkdir -p "$here/png/light" "$here/png/dark"
# A profile of its own, thrown away at the end, so the drawing never opens a
# window in a Chrome that is already running.
profile=$(mktemp -d)
trap 'rm -rf "$profile"' EXIT
for page in "$here"/src/*/*.html; do
  area=$(basename "$(dirname "$page")")
  [ -n "$1" ] && [ "$1" != "$area" ] && continue
  name="$area-$(basename "$page" .html)"
  for mode in light dark; do
    "$chrome" --headless=new --user-data-dir="$profile" --disable-gpu --hide-scrollbars \
      --no-first-run --no-default-browser-check --disable-extensions \
      --window-size=390,844 --force-device-scale-factor=2 \
      --screenshot="$here/png/$mode/$name.png" "file://$page#$mode" >/dev/null 2>&1
  done
  echo "$name"
done
if python3 -c 'import PIL' 2>/dev/null; then
  python3 "$here/shrink.py"
else
  echo "Pillow is not installed, so the pictures keep their full weight" >&2
fi
