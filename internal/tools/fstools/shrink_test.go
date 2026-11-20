package fstools

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// picture makes a w by h picture: a smooth wash of colour, or noise that
// does not compress; half of it is clear when clear is set.
func picture(w, h int, clear, noisy bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewPCG(1, 2))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if clear && x < w/2 {
				a = 0
			}
			c := color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128, a}
			if noisy {
				v := rnd.Uint32()
				c = color.NRGBA{uint8(v), uint8(v >> 8), uint8(v >> 16), a}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func encoded(t *testing.T, img image.Image, kind string) []byte {
	t.Helper()
	var b bytes.Buffer
	var err error
	if kind == "jpeg" {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A picture longer than a model is shown, or larger than Read hands on,
// comes back shrunk to fit, as Claude Code's Read shrinks one; a small one
// comes back as it is; one of more pixels than are decoded here, that fits
// in bytes, goes as it is too.
func TestReadShrinksLargePictures(t *testing.T) {
	f := newFixture(t, Options{})
	big := f.file("big.png", string(encoded(t, picture(3000, 2000, true, false), "png")))
	res := f.run(f.read, in{"file_path": big})
	if res.IsError || len(res.Content) != 2 {
		t.Fatalf("failed: %s", core.TextOf(res.Content))
	}
	mustContain(t, res.Content[0].Text, "3000x2000, shown shrunk to 1568x1045 (PNG")
	got, _, err := image.Decode(bytes.NewReader(res.Content[1].Data))
	if err != nil || got.Bounds().Dx() != 1568 || got.Bounds().Dy() != 1045 {
		t.Fatalf("the picture sent: %v, %v", got.Bounds(), err)
	}
	if _, _, _, a := got.At(10, 10).RGBA(); a != 0 {
		t.Errorf("the clear half is no longer clear: alpha %d", a)
	}
	if res.Content[1].MediaType != "image/png" {
		t.Errorf("sent as %s", res.Content[1].MediaType)
	}

	photo := f.file("photo.jpg", string(encoded(t, picture(2000, 2000, false, false), "jpeg")))
	res = f.run(f.read, in{"file_path": photo})
	mustContain(t, core.TextOf(res.Content), "shown shrunk to 1568x1568 (JPEG")
	if res.Content[1].MediaType != "image/jpeg" {
		t.Errorf("a JPEG went as %s", res.Content[1].MediaType)
	}

	small := encoded(t, picture(100, 50, false, false), "png")
	p := f.file("small.png", string(small))
	res = f.run(f.read, in{"file_path": p})
	if !bytes.Equal(res.Content[1].Data, small) {
		t.Error("a small picture was changed")
	}

	defer func(n int) { imagePixelsMost = n }(imagePixelsMost)
	imagePixelsMost = 1000
	wide := encoded(t, picture(2000, 10, false, false), "png")
	q := filepath.Join(f.dir, "wide.png")
	if err := os.WriteFile(q, wide, 0o644); err != nil {
		t.Fatal(err)
	}
	res = f.run(f.read, in{"file_path": q})
	if res.IsError || !bytes.Equal(res.Content[1].Data, wide) {
		t.Errorf("a picture of too many pixels that fits was not sent as it is: %s", core.TextOf(res.Content))
	}
}

// Larger than Read hands on, and more pixels than are decoded here: it is
// refused, and says why.
func TestReadRefusesAPictureItCannotShrink(t *testing.T) {
	defer func(n int) { imagePixelsMost = n }(imagePixelsMost)
	imagePixelsMost = 1000
	f := newFixture(t, Options{ReadMaxImageBytes: 2000})
	p := f.file("wide.png", string(encoded(t, picture(3000, 40, false, true), "png")))
	f.fails(f.read, in{"file_path": p}, "could not be shrunk", "more pixels than are shrunk here", "smaller copy")
}
