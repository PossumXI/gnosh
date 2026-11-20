package fstools

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // (GIF pictures are decoded to be shrunk.)
	"image/jpeg"
	"image/png"
)

// This file shrinks a picture for Read, as Claude Code's Read resizes and
// recompresses a large image to fit the model's limits (its tools
// reference, "Read tool behavior"). A PNG, JPEG or GIF longer than
// imageEdge on a side, or larger than Read hands on, is scaled down to fit
// imageEdge: a model is shown it at about that size anyway, and a local
// model's count of tokens for a picture grows with its pixels. It stays a
// PNG where that is small enough, and becomes a JPEG otherwise. WebP, which
// Go does not decode, goes on as it is, or is refused when too large.

const (
	// imageEdge is the longest side of a picture Read hands on.
	imageEdge = 1568
	// imageReadMost is the largest picture file Read takes in to shrink.
	imageReadMost = 64 << 20
	// jpegQuality is the quality of a picture kept as JPEG.
	jpegQuality = 85
)

// imagePixelsMost is the most pixels Read decodes to shrink a picture
// (about 200 MB held as RGBA): a larger one is refused rather than held. A
// variable, for tests.
var imagePixelsMost = 50_000_000

// shrunk is a picture made smaller.
type shrunk struct {
	data          []byte
	media         string
	width, height int // its size now
	fromW, fromH  int // its size before
}

// shrinkImage scales a PNG, JPEG or GIF down to fit imageEdge and limit
// bytes. ok is false when it need not be shrunk, as it fits; err says why
// it cannot be.
func shrinkImage(data []byte, media string, limit int64) (s shrunk, ok bool, err error) {
	if media == "image/webp" {
		return s, false, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return s, false, fmt.Errorf("it cannot be decoded (%v)", err)
	}
	long := max(cfg.Width, cfg.Height)
	if long <= imageEdge && int64(len(data)) <= limit {
		return s, false, nil
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > int64(imagePixelsMost) {
		return s, false, fmt.Errorf("it is %dx%d, more pixels than are shrunk here", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return s, false, fmt.Errorf("it cannot be decoded (%v)", err)
	}
	w, h := cfg.Width, cfg.Height
	if long > imageEdge {
		w, h = max(1, w*imageEdge/long), max(1, h*imageEdge/long)
	}
	small := boxScale(src, w, h)
	s = shrunk{width: w, height: h, fromW: cfg.Width, fromH: cfg.Height}
	var b bytes.Buffer
	if media != "image/jpeg" {
		s.media = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, small)
	}
	if media == "image/jpeg" || (err == nil && int64(b.Len()) > limit) {
		// (JPEG keeps no transparency: what shows through is white.)
		b.Reset()
		s.media = "image/jpeg"
		err = jpeg.Encode(&b, onWhite(small), &jpeg.Options{Quality: jpegQuality})
	}
	if err != nil {
		return s, false, err
	}
	if int64(b.Len()) > limit {
		return s, false, errors.New("shrunk, it is still too large")
	}
	s.data = b.Bytes()
	return s, true, nil
}

// boxScale scales src to w by h, each pixel the average of the pixels it
// covers, weighted by how opaque they are (so that a clear pixel lends no
// colour to its neighbour's).
func boxScale(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	n, ok := src.(*image.NRGBA)
	if !ok {
		n = image.NewNRGBA(b)
		draw.Draw(n, b, src, b.Min, draw.Src)
	}
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for dy := 0; dy < h; dy++ {
		y0 := dy * sh / h
		y1 := max(y0+1, (dy+1)*sh/h)
		for dx := 0; dx < w; dx++ {
			x0 := dx * sw / w
			x1 := max(x0+1, (dx+1)*sw/w)
			var r, g, bl, a uint64
			for y := y0; y < y1; y++ {
				off := n.PixOffset(b.Min.X+x0, b.Min.Y+y)
				for x := x0; x < x1; x++ {
					pa := uint64(n.Pix[off+3])
					r += uint64(n.Pix[off]) * pa
					g += uint64(n.Pix[off+1]) * pa
					bl += uint64(n.Pix[off+2]) * pa
					a += pa
					off += 4
				}
			}
			o := dst.PixOffset(dx, dy)
			if a > 0 {
				dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2] = uint8(r/a), uint8(g/a), uint8(bl/a)
			}
			dst.Pix[o+3] = uint8(a / uint64((y1-y0)*(x1-x0)))
		}
	}
	return dst
}

// onWhite is img over a white ground.
func onWhite(img *image.NRGBA) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.White, image.Point{}, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}
