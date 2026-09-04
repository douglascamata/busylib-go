# busylib-go

A Go port of [busylib-ts](https://github.com/busy-app/busylib-ts), the library
for talking to the [BUSY Bar](https://busy.app/).

Some features are also ported from the [busylib-py](https://github.com/busy-app/busylib-py)
library (for instance, the image and audio converters).

> [!IMPORTANT]
> **This is an unofficial project.** Built and maintained by
> [Douglas Camata](https://github.com/douglascamata). This is **not** an official
> Flipper Devices / BUSY product, and it is not affiliated with, endorsed by, or
> supported by them. The original busylib-ts library, the BUSY Bar API and
> protobuf schemas, and the "BUSY Bar" trademark belong to Flipper Devices.
> For the real hardware and the official libraries, visit
> **[busy.app](https://busy.app)** and
> **[github.com/busy-app](https://github.com/busy-app)**.

Four packages:

- **`busybar`**: a typed client for the BUSY Bar [HTTP API](https://docs.busy.app/bar/dev/http-api).
- **`statestream`**: real-time device state over WebSocket, decoded from protobuf.
- **`frame`**: pixel-format helpers for display frames (BGR, L4, L8, RLE, deflate) and an `image.RGBA` bridge.
- **`media`**: image resizing and PNG encoding, plus audio conversion through FFmpeg.

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
Concurrent calls share an in-flight version fetch. Each waiting caller can
cancel independently. A failed fetch is shared by its waiters; a later call
can try again.

### Display frames

`DisplayScreenFrameGet` returns the device's raw bytes or RGBA. Turn RGBA into
a standard image with `frame.ToImage`:

```go
rgba, err := bar.DisplayScreenFrameGet(ctx, frame.Front, busybar.FrameRGBA)
w, h := frame.Dimensions(frame.Front)
png.Encode(file, frame.ToImage(rgba, w, h))
```

## Image and audio conversion

Import `github.com/douglascamata/busylib-go/media`. Each converter takes file
bytes (for example, from `os.ReadFile`) and returns an `Asset` with `Name` and
`Data`. Use that name both for upload and for display or playback.
`AssetsUpload` and `StorageWrite` send bytes unchanged; conversion is explicit.

### Images

`ConvertImage` follows busylib-py's image recipe. It reads PNG, JPEG, BMP,
TIFF, WebP and static GIF and produces PNG. The zero-value `ImageOptions`
selects the front display (72 × 16), with scaling and center cropping enabled.
Use `Display: frame.Back` for the back display (160 × 80).

When either source dimension exceeds the display, it scales with Lanczos
using the larger width/height ratio. This covers the display before cropping.
The center crop runs only when both resulting dimensions fit the target.
Small images stay small; an image with only one small dimension can grow.
`NoScale` and `NoCrop` disable these steps independently.

Animation is rejected. EXIF orientation and color-profile processing are not
applied. PNG encoding and pixel rounding can differ from Pillow, but the sizing
rules match Python. Image conversion does not require FFmpeg.

```go
asset, err := media.ConvertImage(
	"photo.jpg", 
	imageBytes, 
	media.ImageOptions{Display: frame.Back},
)
if err != nil {
    log.Fatal(err)
}
if err := bar.AssetsUpload(ctx, busybar.AssetsUploadParams{
    ApplicationName: "demo", File: asset.Name, Data: asset.Data,
}); err != nil {
    log.Fatal(err)
}
if err := bar.DisplayDraw(ctx, busybar.DisplayDrawParams{
    ApplicationName: "demo",
    Elements: []busybar.Element{
        busybar.ImageElement{
            ElementBase: busybar.ElementBase{ID: "photo", Display: busybar.DisplayBack},
            Path:        asset.Name,
        },
    },
}); err != nil {
    log.Fatal(err)
}
```

### Audio

**Install FFmpeg and make sure `ffmpeg` is on `PATH`.** The
[ffmpeg-go](https://github.com/u2takey/ffmpeg-go) wrapper does not install it.
For example, use `brew install ffmpeg` on macOS or `sudo apt install ffmpeg`
on Debian/Ubuntu. Check the installation with `ffmpeg -version`.

`ConvertAudio` uses FFmpeg's default audio stream selection, mixes to mono, and resamples
it to 44.1 kHz. Supported inputs depend on your FFmpeg build; the tests cover
WAV, MP3, OGG, AAC, M4A and FLAC. The context cancels conversion. Temporary
input files are removed when the call returns.

The result is **raw signed 16-bit little-endian PCM with a `.wav` name**,
matching [busylib-py](https://github.com/busy-app/busylib-py/blob/main/src/busylib/converter/audio.py).
Despite that extension, there is no WAV header. Volume is not normalized.
Direct `ConvertAudio` calls with `.raw` or `.pcm` input keep the bytes unchanged
and change the extension to `.wav`, without running FFmpeg. Such input must
already use the device's PCM format.

```go
asset, err := media.ConvertAudio(ctx, "alert.mp3", audioBytes)
if err != nil {
    log.Fatal(err)
}
if err := bar.AssetsUpload(ctx, busybar.AssetsUploadParams{
    ApplicationName: "demo", File: asset.Name, Data: asset.Data,
}); err != nil {
    log.Fatal(err)
}
if err := bar.AudioPlay(ctx, busybar.AudioPlayParams{
    ApplicationName: "demo", Path: asset.Name,
}); err != nil {
    log.Fatal(err)
}
```

### Select a converter by file extension

```go
media.ConvertForStorage(ctx, filename, data)
```

The snippet follows Python's dispatcher:

- JPG, JPEG, PNG, BMP, TIF, TIFF and WebP use default image options.
- MP3, OGG, AAC, M4A, FLAC and WAV use the audio converter.
- GIF, MOV, MP4, MKV, AVI and WebM return `media.ErrUnsupported`.
- Unknown extensions, including RAW and PCM, keep their name and bytes.

Extension matching ignores case. Known formats return an error if conversion
fails. To convert a static GIF or select the back display, call `ConvertImage`
directly. Use `errors.Is(err, media.ErrUnsupported)` to identify unsupported
conversions. Image and audio decoders can return other errors for unsupported
encodings within those file formats.

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
`DataStale` when an open connection stops sending messages for `DataTimeout`.
It is `DataNone` while reconnecting. Callbacks run one at a time on the `Run`
goroutine. Cancellation can race with a callback that is about to start. No
callback fires after `Run` returns.
Token and subscription commands are sent in order. Callbacks can call these
methods. Each remote connection must authenticate within `ConnectTimeout`,
including connections opened after a drop.

Stream errors keep their underlying cause. Use `errors.Is` or `errors.As`
to inspect transport, token-provider, and decoding errors.

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
- `Status().Data` is `DataNone` while reconnecting. busylib-ts keeps its
  data timer across reconnects and reports `STALE` instead.
- Remote authentication has a deadline on every connection. The TypeScript
  connection timer applies to startup.

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
script clones the revision in `scripts/emulator-revision` into `.cache/` on
first run. Set `BUSYBAR_EMULATOR_DIR` to reuse a clean checkout of that revision.
Existing checkouts are never reset by the script.

CI installs FFmpeg and runs the unit tests, lint checks, and pinned emulator
smoke tests. Locally, tests that execute FFmpeg skip when it is not installed.
Run `go test -race -v ./media` with FFmpeg installed to check the converted
PNG pixels and PCM samples, including all six audio input formats above. Python/Pillow image references
can be regenerated with `python3 media/testdata/generate.py` (requires Pillow).

For an upstream review, use the `review-upstream` skill or run the script directly
with Python 3 and Git:

```bash
python3 scripts/review-upstream.py ts --from REVIEWED_SHA --to TARGET_REF --output .cache/upstream-review/ts.md
```

Replace the placeholders with a previously reviewed commit and the target commit,
branch, or tag. Sources are `ts`, `protobuf`, and `firmware`. The report includes
resolved revisions, source links, all changed files, and a focused diff. This is
an on-demand review aid; it does not certify compatibility or update any pins.

## License

This port is released under the [MIT License](LICENSE).

It is a derivative work of [busylib-ts](https://github.com/busy-app/busylib-ts),
© Flipper FZCO., MIT licensed; the original license text is included in
[LICENSE](LICENSE). The protobuf schemas in `proto/` come from
[bsb-protobuf](https://github.com/flipperdevices/bsb-protobuf), © BUSY App, MIT
licensed; see [proto/LICENSE.md](proto/LICENSE.md).

"BUSY Bar" is a trademark of Flipper Devices. This project is unaffiliated and
unofficial.
