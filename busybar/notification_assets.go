package busybar

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"strings"
)

// NotificationAsset names either an upload inside NotificationOptions.ApplicationName
// (Path), or a shared firmware asset (StockPath). Set exactly one field.
type NotificationAsset struct {
	Path      string
	StockPath string
}

// NotificationIcon includes the dimensions needed to place text beside an icon.
// Fill these from known image dimensions, or use Client.NotificationIconGet.
type NotificationIcon struct {
	Asset         NotificationAsset
	Width, Height int
}

func (a NotificationAsset) validate(directory string) error {
	if (a.Path == "") == (a.StockPath == "") {
		return errors.New("notification: set exactly one of asset Path and StockPath")
	}
	if a.StockPath != "" {
		prefix := "shared/" + directory + "/"
		name, ok := strings.CutPrefix(a.StockPath, prefix)
		// Firmware resolves only the basename in the shared directory. Other
		// directories can select a different file than the caller requested.
		if !ok || name == "" || strings.Contains(name, "/") {
			return fmt.Errorf("notification: stock asset must use %s<file>", prefix)
		}
	}
	return nil
}

func (i NotificationIcon) validate() error {
	if err := i.Asset.validate("images"); err != nil {
		return err
	}
	// The front panel is 72x16. Keep the two-pixel gap and at least one
	// pixel for text; an image that fills the panel cannot be a text icon.
	if i.Width < 1 || i.Width > 69 || i.Height < 1 || i.Height > 16 {
		return fmt.Errorf("notification: icon %dx%d must fit within 69x16 to leave room for text", i.Width, i.Height)
	}
	return nil
}

// NotificationIconGet reads PNG or firmware .image dimensions from the device.
// Use the same applicationName when sending an uploaded icon with Notify.
// Empty applicationName defaults to busylib. Callers can reuse the returned icon
// until the asset changes; this method does not keep a cache or list directories.
func (c *Client) NotificationIconGet(ctx context.Context, applicationName string, asset NotificationAsset) (*NotificationIcon, error) {
	if err := asset.validate("images"); err != nil {
		return nil, err
	}
	path := "/ext/apps_assets/" + asset.StockPath
	if asset.Path != "" {
		if applicationName == "" {
			applicationName = "busylib"
		}
		path = "/ext/user_assets/" + applicationName + "/" + asset.Path
	}
	data, err := c.StorageRead(ctx, path)
	if err != nil {
		return nil, err
	}
	icon := &NotificationIcon{Asset: asset}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("notification: read PNG dimensions: %w", err)
		}
		icon.Width, icon.Height = config.Width, config.Height
	case len(data) >= 12 && data[0] == 0x19:
		// LVGL v9's 12-byte image header: magic, format, flags, then
		// little-endian uint16 width, height, stride and reserved bytes.
		icon.Width = int(binary.LittleEndian.Uint16(data[4:6]))
		icon.Height = int(binary.LittleEndian.Uint16(data[6:8]))
	default:
		return nil, errors.New("notification: icon needs a PNG or LVGL v9 image header")
	}
	if err := icon.validate(); err != nil {
		return nil, err
	}
	return icon, nil
}
