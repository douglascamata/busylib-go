package frame

import (
	"bytes"
	"compress/zlib"
	"testing"
)

func TestL4ToRGBA(t *testing.T) {
	// 0xF0: low nibble 0 -> black, high nibble F -> white. Third pixel is cut by width*height.
	got := L4ToRGBA([]byte{0xF0, 0x11}, 3, 1)
	want := []byte{0, 0, 0, 255, 255, 255, 255, 255, 17, 17, 17, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestL8ToRGBA(t *testing.T) {
	got := L8ToRGBA([]byte{7, 200}, 2, 1)
	want := []byte{7, 7, 7, 255, 200, 200, 200, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBGRToRGBA(t *testing.T) {
	// One full pixel (B=1,G=2,R=3) and one truncated pixel that must stay zero.
	got := BGRToRGBA([]byte{1, 2, 3, 9}, 2, 1)
	want := []byte{3, 2, 1, 255, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDecompressRLE(t *testing.T) {
	// literal of 2 blocks (3 bytes each), then repeat one block 3 times, then a zero-count opcode.
	src := []byte{0x82, 1, 2, 3, 4, 5, 6, 0x03, 7, 8, 9, 0x00}
	got := DecompressRLE(src, 3)
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 7, 8, 9, 7, 8, 9}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestInflate(t *testing.T) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write([]byte("hello frame"))
	w.Close()
	got, err := Inflate(buf.Bytes())
	if err != nil || string(got) != "hello frame" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestToImage(t *testing.T) {
	w, h := Dimensions(Front)
	img := ToImage(make([]byte, w*h*4), w, h)
	if img.Bounds().Dx() != 72 || img.Bounds().Dy() != 16 || img.Stride != 72*4 {
		t.Fatalf("unexpected image geometry %+v", img.Rect)
	}
}

func TestToImagePanicsOnShortBuffer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a short buffer")
		}
	}()
	w, h := Dimensions(Front)
	ToImage(make([]byte, 1), w, h)
}
