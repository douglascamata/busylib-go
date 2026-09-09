// Package notification lays out messages on the 72×16 front display.
// Layouts and stock assets follow busylib-py 2.1.0 (MIT, Flipper Devices).
package notification

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/douglascamata/busylib-go/busybar"
)

const (
	PriorityDefault = 50
	// PriorityInterrupt places a notification above a running Busy session.
	PriorityInterrupt = 91
)

// Options controls a one- or two-line notification. Colors use #RRGGBBAA.
type Options struct {
	Line2 string
	// Icon is check, error, info, low_battery, clock, hourglass, start, or setup.
	Icon string
	// Font defaults to small. Large and extra_large support one line only.
	Font       busybar.Font
	Color      string
	Line2Color string
	// BackgroundColor requires device API 24.3.0 or newer.
	BackgroundColor string
	// Duration is in seconds. Zero leaves the notification until cleared.
	Duration int
	// Priority defaults to 50; valid explicit values are 1–100.
	Priority int
	// ApplicationName defaults to busylib; use it with Client.DisplayClear.
	ApplicationName string
	// Sound is event, reminder, or volume. Only Notify plays it.
	Sound string
}

type stockIcon struct {
	path  string
	width int
}

var icons = map[string]stockIcon{
	"check":       {"shared/images/checkmark_front_8x8.image", 8},
	"error":       {"shared/images/error_front_8x8.image", 8},
	"info":        {"shared/images/info_front_8x8.image", 8},
	"low_battery": {"shared/images/low_battery_front_8x8.image", 8},
	"clock":       {"shared/images/clock_5x5.image", 5},
	"hourglass":   {"shared/images/hourglass_5x5.image", 5},
	"start":       {"shared/images/start_11x11.image", 11},
	"setup":       {"shared/images/setup_11x11.image", 11},
}

var sounds = map[string]string{
	"event":    "shared/sounds/calendar_event_starts.snd",
	"reminder": "shared/sounds/calendar_reminder_ends.snd",
	"volume":   "shared/sounds/volume_change.snd",
}

type fontLayout struct {
	singleY, topY, bottomY int
	twoLines               bool
}

// These offsets were calibrated on hardware by busylib-py. Some glyph boxes
// intentionally extend past the anchor: normal/bold use -1 and 17 for two lines.
var fonts = map[busybar.Font]fontLayout{
	busybar.FontTiny:       {8, 1, 15, true},
	busybar.FontSmall:      {7, 0, 16, true},
	busybar.FontNormal:     {7, -1, 17, true},
	busybar.FontCondensed:  {7, -1, 17, true},
	busybar.FontBold:       {7, -1, 17, true},
	busybar.FontLarge:      {7, 0, 0, false},
	busybar.FontExtraLarge: {8, 0, 0, false},
}

// Build returns display elements without contacting a device. Callers can edit
// the returned elements or send them with Client.DisplayDraw. Long lines use
// Python's approximate character budget, not exact glyph measurement.
func Build(text string, opts Options) (busybar.DisplayDrawParams, error) {
	if opts.Font == "" {
		opts.Font = busybar.FontSmall
	}
	layout, ok := fonts[opts.Font]
	if !ok {
		return busybar.DisplayDrawParams{}, fmt.Errorf("notification: unsupported font %q", opts.Font)
	}
	if opts.Line2 != "" && !layout.twoLines {
		return busybar.DisplayDrawParams{}, fmt.Errorf("notification: font %q does not fit two lines", opts.Font)
	}
	if opts.Duration < 0 {
		return busybar.DisplayDrawParams{}, fmt.Errorf("notification: duration must not be negative")
	}
	if opts.Priority == 0 {
		opts.Priority = PriorityDefault
	}
	if opts.Priority < 1 || opts.Priority > 100 {
		return busybar.DisplayDrawParams{}, fmt.Errorf("notification: priority must be between 1 and 100")
	}
	if opts.ApplicationName == "" {
		opts.ApplicationName = "busylib"
	}
	params := busybar.DisplayDrawParams{ApplicationName: opts.ApplicationName, Priority: opts.Priority}
	if opts.BackgroundColor != "" {
		params.Elements = append(params.Elements, busybar.RectangleElement{
			ElementBase: busybar.ElementBase{ID: "0", Display: busybar.DisplayFront, Timeout: opts.Duration},
			Width:       72, Height: 16, Fill: busybar.FillSolid, FillColors: []string{opts.BackgroundColor}, BorderWidth: new(0),
		})
	}
	x := 2
	if opts.Icon != "" {
		icon, ok := icons[opts.Icon]
		if !ok {
			return busybar.DisplayDrawParams{}, fmt.Errorf("notification: unknown icon %q", opts.Icon)
		}
		x = icon.width + 2
		params.Elements = append(params.Elements, busybar.ImageElement{
			ElementBase: busybar.ElementBase{ID: "10", Display: busybar.DisplayFront, Y: 8, Align: busybar.AlignMidLeft, Timeout: opts.Duration},
			StockPath:   icon.path,
		})
	}
	type textLine struct {
		id, text, color string
		y               int
		align           busybar.Align
	}
	lines := []textLine{
		{"11", text, opts.Color, layout.singleY, busybar.AlignMidLeft},
	}
	if opts.Line2 != "" {
		lines[0].y, lines[0].align = layout.topY, busybar.AlignTopLeft
		lines = append(lines, textLine{"12", opts.Line2, opts.Line2Color, layout.bottomY, busybar.AlignBottomLeft})
	}
	for _, line := range lines {
		element := busybar.TextElement{
			ElementBase: busybar.ElementBase{ID: line.id, X: x, Y: line.y, Display: busybar.DisplayFront, Align: line.align, Timeout: opts.Duration},
			Text:        line.text, Font: opts.Font, Color: line.color,
		}
		available := 72 - x
		if utf8.RuneCountInString(line.text) > max(1, 12*available/72) {
			element.Width, element.ScrollRate = available, 1200
		}
		params.Elements = append(params.Elements, element)
	}
	return params, nil
}

// Notify draws the message, then plays the optional sound with the same
// application name. If drawing fails, sound is not played. A sound error can
// occur after a successful draw; these are two separate device requests.
func Notify(ctx context.Context, client *busybar.Client, text string, opts Options) error {
	var sound string
	if opts.Sound != "" {
		var ok bool
		sound, ok = sounds[opts.Sound]
		if !ok {
			return fmt.Errorf("notification: unknown sound %q", opts.Sound)
		}
	}
	params, err := Build(text, opts)
	if err != nil {
		return err
	}
	if opts.BackgroundColor != "" {
		version := client.APISemver()
		if version == "" {
			info, err := client.SystemVersionGet(ctx)
			if err != nil {
				return err
			}
			version = info.APISemver
		}
		var major, minor, patch int
		if _, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch); err != nil {
			return fmt.Errorf("notification: invalid device API version %q: %w", version, err)
		}
		if major < 24 || (major == 24 && minor < 3) {
			return fmt.Errorf("notification: background color requires API 24.3.0; device reports %s", version)
		}
	}
	if err := client.DisplayDraw(ctx, params); err != nil {
		return err
	}
	if sound != "" {
		return client.AudioPlay(ctx, busybar.AudioPlayParams{ApplicationName: params.ApplicationName, StockPath: sound})
	}
	return nil
}
