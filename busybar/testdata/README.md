# Notification icon fixtures

`notification-icon.png` is an opaque orange 16×10 image. Its `.image` counterpart
was produced with the BUSY firmware 1.2.4 converter recipe: `LVGLImage.from_png`
with `cf=ColorFormat.RGB888`,
`adjust_stride(align=1)`, and `to_bin`, followed by renaming `.bin` to `.image`.

The converter is [LVGLImage.py at the firmware's pinned LVGL revision](https://github.com/flipperdevices/lvgl/blob/90a24ece10155b33f0a701138fbcef59d68f1b81/scripts/LVGLImage.py).
It needs `pypng` and `lz4` to regenerate the binary. Neither is needed for Go tests.
These fixtures provide an independent encoder for icon metadata checks.
