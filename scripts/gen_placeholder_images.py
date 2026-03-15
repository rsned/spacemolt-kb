#!/usr/bin/env python3
"""Generate placeholder PNG images for items missing thumbnails."""

import math
import os
import sqlite3
import sys

from PIL import Image, ImageDraw, ImageFont

SIZE = 210
BG = (18, 22, 30)          # dark background matching site theme
BORDER_COLOR = (60, 75, 95)
TEXT_COLOR = (140, 160, 185)
ACCENT = (80, 170, 220)    # cyan accent ring


def human_name(item_id: str) -> str:
    """Convert item_id to human-readable name."""
    return item_id.replace("_", " ").title()


def wrap_text(text: str, max_chars: int) -> list[str]:
    """Word-wrap text to max_chars per line."""
    words = text.split()
    lines: list[str] = []
    line = ""
    for w in words:
        test = f"{line} {w}".strip()
        if len(test) <= max_chars:
            line = test
        else:
            if line:
                lines.append(line)
            line = w
    if line:
        lines.append(line)
    return lines


def generate_placeholder(item_id: str, out_path: str) -> None:
    img = Image.new("RGB", (SIZE, SIZE), BG)
    draw = ImageDraw.Draw(img)

    # Subtle border
    draw.rectangle([0, 0, SIZE - 1, SIZE - 1], outline=BORDER_COLOR, width=1)

    # Accent diamond in center
    cx, cy = SIZE // 2, SIZE // 2 - 10
    r = 28
    diamond = [(cx, cy - r), (cx + r, cy), (cx, cy + r), (cx - r, cy)]
    draw.polygon(diamond, outline=ACCENT, fill=None)
    inner_r = 18
    inner = [(cx, cy - inner_r), (cx + inner_r, cy), (cx, cy + inner_r), (cx - inner_r, cy)]
    draw.polygon(inner, outline=(*ACCENT[:3], 80), fill=None)

    # Question mark
    try:
        qfont = ImageFont.truetype("/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf", 22)
    except OSError:
        qfont = ImageFont.load_default()
    bbox = draw.textbbox((0, 0), "?", font=qfont)
    tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
    draw.text((cx - tw // 2, cy - th // 2 - 2), "?", fill=ACCENT, font=qfont)

    # Item name at bottom
    name = human_name(item_id)
    try:
        font = ImageFont.truetype("/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf", 11)
    except OSError:
        font = ImageFont.load_default()

    lines = wrap_text(name, 18)
    line_height = 14
    total_h = len(lines) * line_height
    y_start = SIZE - total_h - 12
    for i, line in enumerate(lines):
        bbox = draw.textbbox((0, 0), line, font=font)
        tw = bbox[2] - bbox[0]
        draw.text((cx - tw // 2, y_start + i * line_height), line, fill=TEXT_COLOR, font=font)

    img.save(out_path)


def main():
    db_path = sys.argv[1] if len(sys.argv) > 1 else "../../spacemolt-crafting-server/database/crafting.db"
    img_dir = sys.argv[2] if len(sys.argv) > 2 else "kb/items/images"

    conn = sqlite3.connect(db_path)
    item_ids = [row[0] for row in conn.execute("SELECT id FROM items ORDER BY id")]
    conn.close()

    generated = 0
    for item_id in item_ids:
        img_path = os.path.join(img_dir, f"{item_id}.png")
        if os.path.exists(img_path):
            continue
        generate_placeholder(item_id, img_path)
        generated += 1

    print(f"Generated {generated} placeholder images in {img_dir}/")


if __name__ == "__main__":
    main()
