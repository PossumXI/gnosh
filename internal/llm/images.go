package llm

import (
	"errors"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// This file is what happens to images for a server that cannot take them:
// a model run without its vision part (a llama.cpp server started without
// --mmproj) or a model that reads text alone. Such a server refuses the
// whole request, and since the conversation keeps a picture a tool read,
// it would refuse every request after it too. So the request is sent again
// with each image as a line of text that says it was left out, and the
// provider sends them that way from then on (provider.noImages).

// imageLeftOut stands in a request where an image was.
const imageLeftOut = "[An image was left out here: the model in use cannot take images.]"

// refusals are what servers say when a request holds an image they cannot
// take, in lower case: llama.cpp's server ("image input is not supported -
// hint: if this is unexpected, you may need to provide the mmproj"), and
// the words of servers that serve a model for text alone.
var refusals = []string{
	"image input is not supported",
	"images are not supported",
	"image inputs are not supported",
	"does not support image",
	"not a multimodal model",
	"multimodal input is not supported",
	"vision is not supported",
}

// LeavesOutImages reports whether the server has refused the images of a
// request for model, so that each is now sent to it as a line saying it
// was left out (core.ImageDropper).
func (p *provider) LeavesOutImages(model string) bool {
	_, ok := p.noImages.Load(model)
	return ok
}

// imagesRefused reports whether err is a server saying it cannot take
// the request's images.
func imagesRefused(err error) bool {
	var pe *core.ProviderError
	if !errors.As(err, &pe) || pe.Status < 400 {
		return false
	}
	said := strings.ToLower(pe.Message + "\n" + pe.Body)
	for _, r := range refusals {
		if strings.Contains(said, r) {
			return true
		}
	}
	return false
}

// hasImages reports whether a request holds an image, in a message or in
// a tool's result.
func hasImages(req *core.Request) bool {
	for _, m := range req.Messages {
		if blocksHaveImage(m.Blocks) {
			return true
		}
	}
	return false
}

func blocksHaveImage(bs []core.Block) bool {
	for _, b := range bs {
		if b.Type == core.BlockImage || b.Type == core.BlockToolResult && blocksHaveImage(b.Content) {
			return true
		}
	}
	return false
}

// withoutImages returns a copy of req with each image a line of text
// that says it was left out. req itself, the conversation's, is left as it
// was, so a model that can see is shown the pictures again.
func withoutImages(req *core.Request) *core.Request {
	if !hasImages(req) {
		return req
	}
	out := *req
	out.Messages = make([]core.Message, len(req.Messages))
	for i, m := range req.Messages {
		if blocksHaveImage(m.Blocks) {
			m.Blocks = replaceImages(m.Blocks)
		}
		out.Messages[i] = m
	}
	return &out
}

func replaceImages(bs []core.Block) []core.Block {
	out := make([]core.Block, len(bs))
	for i, b := range bs {
		switch {
		case b.Type == core.BlockImage:
			b = core.Text(imageLeftOut)
		case b.Type == core.BlockToolResult && blocksHaveImage(b.Content):
			b.Content = replaceImages(b.Content)
		}
		out[i] = b
	}
	return out
}
