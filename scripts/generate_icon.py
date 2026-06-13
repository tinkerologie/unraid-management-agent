#!/usr/bin/env python3
"""Generate the plugin icons (icon.png + meta/plugin/images/*) from one design.

Community Applications requires the template <Icon> to be a PNG of at least
256x256 with transparency (https://github.com/mstrhakr/plugin-docs,
docs/distribution/community-applications.md); the webGUI .page icon renders
small (48-64px), so every size is downscaled from a 4x supersampled master to
stay crisp.

Usage: python3 scripts/generate_icon.py
"""

from pathlib import Path

from PIL import Image, ImageDraw

REPO = Path(__file__).resolve().parent.parent

# Design canvas is 512; draw at 4x and downscale for anti-aliasing.
SIZE = 512
SS = 4
S = SIZE * SS

# Palette
BG_TOP = (42, 59, 77, 255)  # slate gradient top
BG_BOTTOM = (19, 27, 36, 255)  # slate gradient bottom
SLAT = (236, 241, 245, 255)  # server slat face
VENT = (170, 184, 196, 255)  # slat vent slots
LED_GREEN = (63, 208, 109, 255)
ORANGE = (255, 140, 47, 255)  # Unraid brand orange


def px(v: float) -> int:
    return round(v * SS)


def rounded_square_gradient() -> Image.Image:
    """Dark vertical gradient clipped to a rounded square with transparent corners."""
    gradient = Image.new("RGBA", (S, S))
    for y in range(S):
        t = y / (S - 1)
        row = tuple(round(BG_TOP[i] + (BG_BOTTOM[i] - BG_TOP[i]) * t) for i in range(4))
        gradient.paste(row, (0, y, S, y + 1))

    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=px(116), fill=255)
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.paste(gradient, (0, 0), mask)
    return out


def draw_icon() -> Image.Image:
    img = rounded_square_gradient()
    d = ImageDraw.Draw(img)

    # Server stack: three slats, bottom-left weighted to leave room for the arcs.
    slat_x0, slat_x1 = px(96), px(372)
    slat_h, gap = px(64), px(17)
    y = px(190)
    led_colors = [LED_GREEN, LED_GREEN, ORANGE]
    for led in led_colors:
        d.rounded_rectangle([slat_x0, y, slat_x1, y + slat_h], radius=px(16), fill=SLAT)
        # status LED
        cx, cy, r = slat_x0 + px(38), y + slat_h // 2, px(13)
        d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=led)
        # two vent slots on the right
        for i in range(2):
            vx1 = slat_x1 - px(34) - i * px(46)
            vx0 = vx1 - px(26)
            vy0 = y + slat_h // 2 - px(6)
            d.rounded_rectangle([vx0, vy0, vx1, vy0 + px(12)], radius=px(6), fill=VENT)
        y += slat_h + gap

    # Telemetry arcs (top-right): dot + two arcs sweeping the top-right quadrant.
    acx, acy = px(380), px(132)
    dot_r = px(15)
    d.ellipse([acx - dot_r, acy - dot_r, acx + dot_r, acy + dot_r], fill=ORANGE)
    for radius in (px(46), px(78)):
        d.arc(
            [acx - radius, acy - radius, acx + radius, acy + radius],
            start=-80,
            end=-10,
            fill=ORANGE,
            width=px(17),
        )

    return img


def main() -> None:
    master = draw_icon()
    outputs = {
        REPO / "icon.png": 512,
        REPO / "meta/plugin/images/unraid-management-agent.png": 64,
        REPO / "meta/plugin/images/unraid-management-agent-48.png": 48,
        REPO / "meta/plugin/images/unraid-management-agent-128.png": 128,
    }
    for path, size in outputs.items():
        master.resize((size, size), Image.LANCZOS).save(path, optimize=True)
        print(f"wrote {path.relative_to(REPO)} ({size}x{size})")


if __name__ == "__main__":
    main()
