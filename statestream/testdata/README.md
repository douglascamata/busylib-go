# Firmware frame fixture

`back-l4.rle` was generated with the official BUSY Bar firmware 1.2.3
[RLE encoder](https://github.com/busy-app/busybar-firmware/blob/2cd7ec8abf8479ba3398241e99d291ec24f2a96f/lib/toolbox/rle_encode.c).
The [screen producer](https://github.com/busy-app/busybar-firmware/blob/2cd7ec8abf8479ba3398241e99d291ec24f2a96f/applications/services/state_publisher/screen_streamer.c#L234)
uses a block size of 2 for the back display.

Input: a full 160 × 80 L4 image, represented by 3,200 copies of `21 43`.
Output: 78 bytes. Each input group represents grayscale pixels 17, 34, 51, 68.
The test compares the complete RGBA image with those original pixels.
