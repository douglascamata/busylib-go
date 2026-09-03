package busybar

import "encoding/json"

// Element is one item of a DisplayDraw request: TextElement, ImageElement,
// AnimationElement, CountdownElement or RectangleElement.
type Element interface {
	elementType() string
}

type ElementDisplay string

const (
	DisplayFront ElementDisplay = "front"
	DisplayBack  ElementDisplay = "back"
)

type Align string

const (
	AlignTopLeft     Align = "top_left"
	AlignTopMid      Align = "top_mid"
	AlignTopRight    Align = "top_right"
	AlignMidLeft     Align = "mid_left"
	AlignCenter      Align = "center"
	AlignMidRight    Align = "mid_right"
	AlignBottomLeft  Align = "bottom_left"
	AlignBottomMid   Align = "bottom_mid"
	AlignBottomRight Align = "bottom_right"
)

type Font string

const (
	FontTiny       Font = "tiny"
	FontSmall      Font = "small"
	FontNormal     Font = "normal"
	FontCondensed  Font = "condensed"
	FontBold       Font = "bold"
	FontLarge      Font = "large"
	FontExtraLarge Font = "extra_large"
	FontGlobal     Font = "global"
)

// ElementBase holds the fields shared by every element. Colors are #RRGGBBAA.
type ElementBase struct {
	ID string `json:"id"`
	// Timeout in seconds; 0 means no timeout. Mutually exclusive with DisplayUntil.
	Timeout int `json:"timeout,omitempty"`
	// DisplayUntil is a Unix timestamp in seconds, as a string.
	DisplayUntil string         `json:"display_until,omitempty"`
	X            int            `json:"x"`
	Y            int            `json:"y"`
	Display      ElementDisplay `json:"display,omitempty"`
	Align        Align          `json:"align,omitempty"`
}

type TextElement struct {
	ElementBase
	Text  string `json:"text"`
	Font  Font   `json:"font,omitempty"`
	Color string `json:"color,omitempty"`
	Width int    `json:"width,omitempty"`
	// ScrollRate is in pixels per minute; the delays are in milliseconds.
	ScrollRate        int `json:"scroll_rate,omitempty"`
	ScrollStartDelay  int `json:"scroll_start_delay,omitempty"`
	ScrollRepeatDelay int `json:"scroll_repeat_delay,omitempty"`
}

// ImageElement shows an uploaded asset (Path) or a stock image (StockPath).
type ImageElement struct {
	ElementBase
	Path      string `json:"path,omitempty"`
	StockPath string `json:"stock_path,omitempty"`
	// Opacity is 0-100; nil means the device default of 100.
	Opacity *int `json:"opacity,omitempty"`
}

type AnimationElement struct {
	ElementBase
	Path             string `json:"path,omitempty"`
	StockPath        string `json:"stock_path,omitempty"`
	Loop             bool   `json:"loop,omitempty"`
	AwaitPreviousEnd bool   `json:"await_previous_end,omitempty"`
	// Section names the part of the animation to play; "default" is the whole animation.
	Section string `json:"section,omitempty"`
	// Opacity is 0-100; nil means the device default of 100.
	Opacity *int `json:"opacity,omitempty"`
}

type CountdownDirection string

const (
	CountdownTimeLeft  CountdownDirection = "time_left"
	CountdownTimeSince CountdownDirection = "time_since"
)

type ShowHours string

const (
	ShowHoursWhenNonZero ShowHours = "when_non_zero"
	ShowHoursAlways      ShowHours = "always"
)

type CountdownElement struct {
	ElementBase
	// Timestamp is a Unix UTC timestamp in seconds, as a string.
	Timestamp string             `json:"timestamp"`
	Color     string             `json:"color,omitempty"`
	Direction CountdownDirection `json:"direction,omitempty"`
	ShowHours ShowHours          `json:"show_hours,omitempty"`
}

type Fill string

const (
	FillNone      Fill = "none"
	FillSolid     Fill = "solid"
	FillGradientH Fill = "gradient_h"
	FillGradientV Fill = "gradient_v"
)

type RectangleElement struct {
	ElementBase
	Width  int  `json:"width"`
	Height int  `json:"height"`
	Radius int  `json:"radius,omitempty"`
	Fill   Fill `json:"fill,omitempty"`
	// FillColors holds one color for solid fill, two for gradients.
	FillColors []string `json:"fill_colors,omitempty"`
	// BorderWidth in pixels; nil means the device default of 1.
	BorderWidth *int   `json:"border_width,omitempty"`
	BorderColor string `json:"border_color,omitempty"`
}

func (TextElement) elementType() string      { return "text" }
func (ImageElement) elementType() string     { return "image" }
func (AnimationElement) elementType() string { return "animation" }
func (CountdownElement) elementType() string { return "countdown" }
func (RectangleElement) elementType() string { return "rectangle" }

func (e TextElement) MarshalJSON() ([]byte, error) {
	type plain TextElement
	return withType(e, plain(e))
}

func (e ImageElement) MarshalJSON() ([]byte, error) {
	type plain ImageElement
	return withType(e, plain(e))
}

func (e AnimationElement) MarshalJSON() ([]byte, error) {
	type plain AnimationElement
	return withType(e, plain(e))
}

func (e CountdownElement) MarshalJSON() ([]byte, error) {
	type plain CountdownElement
	return withType(e, plain(e))
}

func (e RectangleElement) MarshalJSON() ([]byte, error) {
	type plain RectangleElement
	return withType(e, plain(e))
}

// withType marshals v and prepends the "type" discriminator. v always has at
// least the id, x and y fields, so the object is never empty.
func withType(e Element, v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append([]byte(`{"type":"`+e.elementType()+`",`), b[1:]...), nil
}
