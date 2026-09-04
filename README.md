# busylib-go

A Go port of [busylib-ts](https://github.com/busy-app/busylib-ts), the library
for talking to the [BUSY Bar](https://busy.app/).

> [!IMPORTANT]
> **This is an unofficial project.** Built and maintained by
> [Douglas Camata](https://github.com/douglascamata). This is **not** an official
> Flipper Devices / BUSY product, and it is not affiliated with, endorsed by, or
> supported by them. The original busylib-ts library, the BUSY Bar API and
> protobuf schemas, and the "BUSY Bar" trademark belong to Flipper Devices.
> For the real hardware and the official libraries, visit
> **[busy.app](https://busy.app)** and
> **[github.com/busy-app](https://github.com/busy-app)**.

Three packages:

- **`busybar`**: a typed client for the BUSY Bar [HTTP API](https://docs.busy.app/bar/dev/http-api).
- **`statestream`**: real-time device state over WebSocket, decoded from protobuf.
- **`frame`**: pixel-format helpers for display frames (BGR, L4, L8, RLE, deflate) and an `image.RGBA` bridge.

```bash
go get github.com/douglascamata/busylib-go
```

## HTTP API

```go
bar, err := busybar.New(busybar.Config{Addr: "10.0.4.20"})
if err != nil {
    log.Fatal(err)
}

status, err := bar.SystemStatusGet(ctx)

err = bar.DisplayDraw(ctx, busybar.DisplayDrawParams{
    ApplicationName: "hello",
    Elements: []busybar.Element{
        busybar.TextElement{
            ElementBase: busybar.ElementBase{ID: "t", X: 36, Y: 8, Align: busybar.AlignCenter},
            Text:        "Hello",
            Font:        busybar.FontBold,
        },
    },
})
```

Method names follow the TypeScript library: `SystemStatusGet`, `DisplayDraw`,
`StorageWrite`, `WifiConnect`, and so on. Every method takes a `context.Context`.

### Connection

`Addr` accepts an IP, a host name, or a full URL. Without a scheme, `http://`
is used, except for the BUSY proxy (`api.busy.app`), which defaults to
`https://` and requires a `Token`.

```go
busybar.New(busybar.Config{})                                   // http://10.0.4.20
busybar.New(busybar.Config{Addr: "192.168.13.37", HTTPAccessPassword: "1234"})
busybar.New(busybar.Config{Addr: "api.busy.app", Token: token}) // remote proxy
```

`SetToken` and `SetHTTPAccessPassword` change credentials at runtime.

### Timeouts and errors

`Config.Timeout` (default 3s) applies to a call only when its context has no
deadline. Use `context.WithTimeout` to override it per call.

- A 4xx/5xx response returns a `*busybar.HTTPError` with `StatusCode`, `Body`
  and a parsed `Message`.
- A timeout returns `context.DeadlineExceeded`; a cancelled context returns
  `context.Canceled`.
- Network failures return the underlying `net/http` error.

The client fetches `/version` once, sends the result as `X-API-Sem-Ver` on
every request, and refreshes it and retries once when the device answers 405.

### Display frames

`DisplayScreenFrameGet` returns the device's raw bytes or RGBA. Turn RGBA into
a standard image with `frame.ToImage`:

```go
rgba, err := bar.DisplayScreenFrameGet(ctx, frame.Front, busybar.FrameRGBA)
w, h := frame.Dimensions(frame.Front)
png.Encode(file, frame.ToImage(rgba, w, h))
```

## StateStream

```go
stream, err := statestream.NewLocal(statestream.Options{Addr: "10.0.4.20"})

runCtx, cancel := context.WithCancel(ctx)
done := make(chan error, 1)
go func() {
    done <- stream.Run(runCtx, statestream.Callbacks{
        Ready: func() { fmt.Println("connected") },
        Data: func(st *statestream.State) {
            for _, u := range st.Updates {
                switch u.Kind {
                case statestream.KindInput:
                    fmt.Println(u.GetInput())
                case statestream.KindFrame:
                    img := u.Frame.Image() // already RGBA
                }
            }
        },
        Status: func(st statestream.Status) { fmt.Println(st.Main, st.Connection, st.Data) },
        Error:  func(e *statestream.Error) { fmt.Println(e.Code, e.Message) },
    })
}()

// Cancel the stream and wait for Run to return.
cancel()
err = <-done
```

`Run` blocks for the stream lifetime. Cancel its context to stop it. The caller
starts a goroutine when it needs other work to continue. `Ready` fires once
per `Run` when the stream is connected (and, for remote streams,
authenticated). `Run` reconnects on its own, up to `MaxReconnectAttempts`. It
returns `CodeReconnectFailed` when it gives up. `Status().Data` flips to
`DataStale` when no message arrives for `DataTimeout`, even across a
reconnect. Callbacks run one at a time on the `Run` goroutine, and a callback
may cancel the context; after that only a final `Status` with `Main ==
Stopped` is reported.

Each `Update` embeds the decoded protobuf message, so `u.GetPower()`,
`u.GetWifi()` and friends are available. The `statestream.BatteryStatus`,
`WifiSecurity`, `BleStatus`, ... functions map protobuf enums to the string
values the HTTP API uses.

`statestream.NewRemote` connects to the BUSY cloud with a `Token`, an optional
`TokenProvider` for refreshes, and `Subscribe(guid)` per device.

## Differences from busylib-ts

- Requests take a `context.Context` instead of `timeout`/`signal` options.
- Mutating calls return only `error`; the `{"result":"OK"}` body is dropped.
- `ScreenRenderer` (WebGL) has no Go counterpart. `frame.ToImage` gives you an
  `image.RGBA` to render however you like.
- `StateStream.Run` blocks. The caller owns its goroutine and cancellation.

## Development

```bash
make build           # compile every package
make test            # unit tests with the race detector
make smoke-test      # boots busybar-emulator and runs the smoke tests
make lint            # gofmt, go vet, golangci-lint
make generate-proto  # regenerate statestream/pb from proto/
make update-protos   # fetch the latest bsb-protobuf schemas, then regenerate
```

The smoke tests drive the HTTP client and the WebSocket stream against
[busybar-emulator](https://github.com/maxswinkels/busybar-emulator). The
script clones it into `.cache/` on first run; set `BUSYBAR_EMULATOR_DIR` to
reuse a checkout.

## License

This port is released under the [MIT License](LICENSE).

It is a derivative work of [busylib-ts](https://github.com/busy-app/busylib-ts),
© Flipper FZCO., MIT licensed; the original license text is included in
[LICENSE](LICENSE). The protobuf schemas in `proto/` come from
[bsb-protobuf](https://github.com/flipperdevices/bsb-protobuf), © BUSY App, MIT
licensed; see [proto/LICENSE.md](proto/LICENSE.md).

"BUSY Bar" is a trademark of Flipper Devices. This project is unaffiliated and
unofficial.
