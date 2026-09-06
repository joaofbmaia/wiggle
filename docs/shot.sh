#!/bin/sh
# Regenerates docs/wiggle.svg and examples/**/*.svg with freeze
# (github.com/charmbracelet/freeze).
#
# Stock freeze drops the bold attribute and embeds only JetBrains Mono
# Regular. Set FREEZE to a build with the font-weight line in ansi.go
# enabled; embold.py then embeds JetBrains Mono Bold so bold spans keep the
# same advance widths instead of falling back to a system font.
set -e
cd "$(dirname "$0")/.."
FREEZE=${FREEZE:-freeze}
go build -o /tmp/wiggle-shot ./cmd/wiggle

shot() { # shot <json5> <svg> [freeze args...]
	in=$1; out=$2; shift 2
	CLICOLOR_FORCE=1 /tmp/wiggle-shot -p "$in" > /tmp/wiggle-shot.ansi
	"$FREEZE" /tmp/wiggle-shot.ansi --language ansi --theme charm --font.size 14 \
		--padding 24,32 --margin 0 --border.radius 10 --background "#1e1e2e" \
		--output "$out" "$@" >/dev/null
	python3 docs/embold.py "$out" docs/fonts/JetBrainsMono-Bold.woff2
}

# JetBrains Mono is embedded in every SVG: freeze lays text and fills out
# from its metrics, so any other font misaligns them.
shot examples/showcase.json5 docs/wiggle.svg --window --font.family "JetBrains Mono"
for f in examples/*.json5 examples/tutorial/*.json5; do
	shot "$f" "${f%.json5}.svg" --font.family "JetBrains Mono"
done
