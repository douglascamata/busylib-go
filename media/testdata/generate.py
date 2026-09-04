"""Regenerate image references with Python 3 and Pillow: python3 media/testdata/generate.py.

The resize/crop recipe follows busylib-py (MIT, Copyright Flipper FZCO.):
https://github.com/busy-app/busylib-py/blob/main/src/busylib/converter/image.py
These fixtures were first generated with Pillow 11.3.0.
"""

import json
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).parent


def convert(image, display=0, no_scale=False, no_crop=False):
    width, height = (72, 16) if display == 0 else (160, 80)
    img_w, img_h = image.size
    if not no_scale and (img_w > width or img_h > height):
        scale_factor = max(width / img_w, height / img_h)
        new_size = (max(1, int(img_w * scale_factor)), max(1, int(img_h * scale_factor)))
        image = image.resize(new_size, Image.Resampling.LANCZOS)
    img_w, img_h = image.size
    if not no_crop and width <= img_w and height <= img_h:
        left, top = (img_w - width) // 2, (img_h - height) // 2
        image = image.crop((left, top, left + width, top + height))
    return image


cases = [
    ("default front", 200, 100, 0, False, False),
    ("default back", 200, 100, 1, False, False),
    ("small unchanged", 8, 8, 0, False, False),
    ("one small dimension", 200, 8, 0, False, False),
    ("thin", 1, 100, 0, False, False),
    ("scale only", 200, 100, 0, False, True),
    ("crop only", 200, 100, 0, True, False),
    ("crop cannot fit", 200, 8, 0, True, False),
    ("original", 200, 100, 0, True, True),
    ("fractional size", 137, 51, 0, False, True),
]
reference = []
for name, width, height, display, no_scale, no_crop in cases:
    converted = convert(Image.new("RGB", (width, height)), display, no_scale, no_crop)
    reference.append(dict(Name=name, Width=width, Height=height, Display=display,
                          NoScale=no_scale, NoCrop=no_crop,
                          WantWidth=converted.width, WantHeight=converted.height))
(ROOT / "reference.json").write_text(json.dumps(reference, indent=2) + "\n")

image = Image.new("RGBA", (137, 51))
image.putdata([(x * 255 // 136, y * 255 // 50, (x + y) % 256, 128 + x % 128)
               for y in range(51) for x in range(137)])
image.save(ROOT / "lanczos-input.png")
convert(image).save(ROOT / "lanczos-output.png")
red, blue = Image.new("RGBA", (8, 8), "red"), Image.new("RGBA", (8, 8), "blue")
red.save(ROOT / "still.webp", lossless=True)
for ext in ("png", "webp"):
    red.save(ROOT / f"animated.{ext}", save_all=True, append_images=[blue], duration=100, loop=0)
