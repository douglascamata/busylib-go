package busybar

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageAppendAndOverwrite(t *testing.T) {
	d := newFakeDevice(t)
	file := filepath.Join(t.TempDir(), "events.log")
	const devicePath = "/ext/events & status.log"
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Query().Get("path") != devicePath {
			t.Errorf("path = %q", r.URL.Query().Get("path"))
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/storage/read" {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Error(err)
			}
			w.Write(data)
			return true
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/storage/write" || r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected write: %s %s, %s", r.Method, r.URL, r.Header.Get("Content-Type"))
		}
		flags := os.O_WRONLY | os.O_CREATE
		switch r.URL.Query().Get("append") {
		case "1":
			flags |= os.O_APPEND
		case "":
			flags |= os.O_TRUNC
		default:
			t.Errorf("invalid append parameter %q", r.URL.Query().Get("append"))
		}
		f, err := os.OpenFile(file, flags, 0600)
		if err != nil {
			t.Error(err)
			http.Error(w, err.Error(), 500)
			return true
		}
		_, err = io.Copy(f, r.Body)
		f.Close()
		if err != nil {
			t.Error(err)
		}
		writeJSON(w, 200, map[string]string{"result": "OK"})
		return true
	}
	c := d.client(t, Config{})
	ctx := context.Background()
	for _, step := range []struct {
		append     bool
		data, want []byte
	}{
		{true, []byte("first\n"), []byte("first\n")},
		{true, []byte("second\n"), []byte("first\nsecond\n")},
		{true, nil, []byte("first\nsecond\n")},
		{false, []byte("reset"), []byte("reset")},
		{true, []byte{0, 255}, []byte{'r', 'e', 's', 'e', 't', 0, 255}},
	} {
		write := c.StorageWrite
		if step.append {
			write = c.StorageAppend
		}
		if err := write(ctx, devicePath, step.data); err != nil {
			t.Fatal(err)
		}
		got, err := c.StorageRead(ctx, devicePath)
		if err != nil || !bytes.Equal(got, step.want) {
			t.Fatalf("read = %q, %v; want %q", got, err, step.want)
		}
	}
}
