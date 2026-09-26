# busylib-go

A Go port of [busylib-ts](https://github.com/busy-app/busylib-ts), the library
for talking to the [BUSY Bar](https://busy.app/).

Some features are also ported from the [busylib-py](https://github.com/busy-app/busylib-py)
library (including discovery and the image and audio converters).

> [!IMPORTANT]
> **This is an unofficial project.** Built and maintained by
> [Douglas Camata](https://github.com/douglascamata). This is **not** an official
> Flipper Devices / BUSY product, and it is not affiliated with, endorsed by, or
> supported by them. The original busylib-ts library, the BUSY Bar API and
> protobuf schemas, and the "BUSY Bar" trademark belong to Flipper Devices.
> For the real hardware and the official libraries, visit
> **[busy.app](https://busy.app)** and
> **[github.com/busy-app](https://github.com/busy-app)**.

Five packages:

- **`busybar`**: a typed client for the BUSY Bar [HTTP API](https://docs.busy.app/bar/dev/http-api), including notification layouts.
- **`statestream`**: real-time device state over WebSocket, decoded from protobuf.
- **`frame`**: pixel-format helpers for display frames (BGR, L4, L8, RLE, deflate) and an `image.RGBA` bridge.
- **`media`**: image resizing and PNG encoding, plus audio conversion through FFmpeg.
- **`discovery`**: find BUSY Bar devices over mDNS, including USB and Wi-Fi addresses.

```bash
go get github.com/douglascamata/busylib-go
```

## Quick start

Connect to a bar over USB (the default address, `10.0.4.20`) and show a
notification:

```go
package main

import (
	"context"
	"log"

	"github.com/douglascamata/busylib-go/busybar"
)

func main() {
	bar, err := busybar.New(busybar.Config{Addr: "10.0.4.20"})
	if err != nil {
		log.Fatal(err)
	}
	err = bar.Notify(context.Background(), "Hello from Go", busybar.NotificationOptions{
		Icon:     "check",
		Sound:    "event",
		Duration: 10, // seconds
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

Use `discovery.Discover` to find bars on the network instead of hard-coding an
address.

See [FEATURES.md](FEATURES.md) for discovery, the full HTTP API, timers,
notifications with custom assets, image and audio conversion, the real-time
state stream, and differences from busylib-ts.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development, changelog, and release
instructions.

## License

This port is released under the [MIT License](LICENSE).

It is a derivative work of [busylib-ts](https://github.com/busy-app/busylib-ts),
© Flipper FZCO., MIT licensed; the original license text is included in
[LICENSE](LICENSE). The protobuf schemas in `proto/` come from
[bsb-protobuf](https://github.com/flipperdevices/bsb-protobuf), © BUSY App, MIT
licensed; see [proto/LICENSE.md](proto/LICENSE.md).

"BUSY Bar" is a trademark of Flipper Devices. This project is unaffiliated and
unofficial.
