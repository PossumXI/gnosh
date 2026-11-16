package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// llamaNoMmproj is what a llama.cpp server started without its vision part
// answers a request with an image in it.
const llamaNoMmproj = `{"error":{"code":500,"message":"image input is not supported - hint: if this is unexpected, you may need to provide the mmproj","type":"server_error"}}`

// refusingImages is a server that refuses any request with an image in it
// as status and body say, and answers the others.
func refusingImages(status int, body string) func(*fake, http.ResponseWriter, *http.Request) {
	return func(f *fake, w http.ResponseWriter, r *http.Request) {
		if f.last().Body != nil && bytes.Contains(f.last().Body, []byte("image_url")) {
			answering(status, nil, body)(f, w, r)
			return
		}
		f.play(w, r, okStream)
	}
}

// readAPicture is a conversation in which a tool read a picture.
func readAPicture() *core.Request {
	return &core.Request{Model: "m", Messages: []core.Message{
		core.UserText("look at the page"),
		{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse("call_1", "Read", json.RawMessage(`{"file_path":"shot.png"}`))}},
		{Role: core.RoleUser, Blocks: []core.Block{
			{Type: core.BlockToolResult, ToolUseID: "call_1", Content: []core.Block{
				core.Text("[shot.png: a PNG image, 1 KB.]"),
				{Type: core.BlockImage, MediaType: "image/png", Data: []byte("\x89PNG\r\n\x1a\nfake")},
			}},
		}},
	}}
}

// A server that cannot take images is sent the request again at once
// without them, each a line that says it was left out, and is never sent
// one again; the conversation keeps its pictures.
func TestImagesAServerRefusesAreLeftOut(t *testing.T) {
	for _, c := range []struct {
		name, typ string
		status    int
		body      string
	}{
		{"llama.cpp without its mmproj", TypeLlamaCpp, 500, llamaNoMmproj},
		{"a server whose model reads text alone", TypeOpenAI, 400, `{"error":{"message":"m is not a multimodal model","type":"BadRequestError","code":400}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t, refusingImages(c.status, c.body))
			p := f.provider(c.typ, nil)
			req := readAPicture()
			res := drain(p.Stream(context.Background(), req))
			wantOK(t, res)
			reqs := f.requests()
			if len(reqs) != 2 {
				t.Fatalf("%d requests, want the refused one and the one without its image", len(reqs))
			}
			if !bytes.Contains(reqs[0].Body, []byte("image_url")) || bytes.Contains(reqs[1].Body, []byte("image_url")) {
				t.Errorf("the first request should hold the image and the second not:\n%s\n%s", reqs[0].Body, reqs[1].Body)
			}
			if !bytes.Contains(reqs[1].Body, []byte("left out here: the model in use cannot take images")) {
				t.Errorf("the second request does not say the image was left out:\n%s", reqs[1].Body)
			}
			if req.Messages[2].Blocks[0].Content[1].Type != core.BlockImage {
				t.Errorf("the conversation lost its picture")
			}
			// From now on the image is left out from the start.
			wantOK(t, drain(p.Stream(context.Background(), readAPicture())))
			if reqs := f.requests(); len(reqs) != 3 || bytes.Contains(reqs[2].Body, []byte("image_url")) {
				t.Errorf("after the refusal: %d requests, the last with an image: %v", len(reqs), len(reqs) == 3 && bytes.Contains(reqs[2].Body, []byte("image_url")))
			}
		})
	}
}

// Any other failure is the caller's, as before: the request is not sent
// again here, with an image or without one.
func TestOtherFailuresAreNotSentAgain(t *testing.T) {
	f := newFake(t, answering(500, nil, `{"error":{"code":500,"message":"the KV cache is full","type":"server_error"}}`))
	p := f.provider(TypeLlamaCpp, nil)
	res := drain(p.Stream(context.Background(), readAPicture()))
	wantFailure(t, res.err, core.ErrServer)
	if n := len(f.requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}

	// A refusal of images for a request that holds none is not one.
	g := newFake(t, answering(500, nil, llamaNoMmproj))
	q := g.provider(TypeLlamaCpp, nil)
	wantFailure(t, ask(q).err, core.ErrServer)
	if n := len(g.requests()); n != 1 {
		t.Errorf("%d requests for a request with no image, want 1", n)
	}
}

// withoutImages leaves the request it is given as it was, and takes out
// images in messages and in tools' results alike.
func TestWithoutImages(t *testing.T) {
	req := readAPicture()
	req.Messages = append(req.Messages, core.Message{Role: core.RoleUser, Blocks: []core.Block{
		core.Text("and this one"), {Type: core.BlockImage, MediaType: "image/jpeg", Data: []byte("jpeg")},
	}})
	out := withoutImages(req)
	if hasImages(out) || !hasImages(req) {
		t.Fatalf("hasImages: copy %v, original %v; want false, true", hasImages(out), hasImages(req))
	}
	got := core.TextOf(out.Messages[2].Blocks[0].Content) + "|" + core.TextOf(out.Messages[3].Blocks)
	if strings.Count(got, imageLeftOut) != 2 || !strings.Contains(got, "and this one") || !strings.Contains(got, "a PNG image") {
		t.Errorf("the copy reads %q", got)
	}
	plain := &core.Request{Messages: []core.Message{core.UserText("hi")}}
	if withoutImages(plain) != plain {
		t.Errorf("a request with no image was copied")
	}
}
