package lsp

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Sync tells the server what the file at path holds now: it opens it the
// first time, and after that sends its whole text as a change (and a
// save, to a server that asked for saves) when the text is not what the
// server was last told. It returns the count of publishes before it, for
// Settle, and whether anything was sent.
func (c *Client) Sync(ctx context.Context, path, languageID, text string) (since uint64, sent bool, err error) {
	uri := FileURI(path)
	c.mu.Lock()
	since = c.seq
	d := c.docs[uri]
	c.mu.Unlock()
	switch {
	case d == nil:
		err = c.notify(ctx, "textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{"uri": uri, "languageId": languageID, "version": 1, "text": text}})
		if err == nil {
			c.mu.Lock()
			c.docs[uri] = &document{version: 1, text: text}
			c.mu.Unlock()
		}
		return since, err == nil, err
	case d.text == text:
		return since, false, nil
	}
	c.mu.Lock()
	version := d.version + 1
	c.mu.Unlock()
	err = c.notify(ctx, "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]string{{"text": text}},
	})
	if err != nil {
		return since, false, err
	}
	c.mu.Lock()
	d.version, d.text = version, text
	c.mu.Unlock()
	if save, withText := c.saves(); save {
		p := map[string]any{"textDocument": map[string]string{"uri": uri}}
		if withText {
			p["text"] = text
		}
		err = c.notify(ctx, "textDocument/didSave", p)
	}
	return since, true, err
}

// saves reports whether the server asked to be told of saves, and to be
// sent the text with them (textDocumentSync.save).
func (c *Client) saves() (save, withText bool) {
	c.mu.Lock()
	raw := c.caps["textDocumentSync"]
	c.mu.Unlock()
	var sync struct {
		Save json.RawMessage `json:"save"`
	}
	if json.Unmarshal(raw, &sync) != nil || len(sync.Save) == 0 {
		return false, false
	}
	s := strings.TrimSpace(string(sync.Save))
	if s == "true" {
		return true, false
	}
	var opts struct {
		IncludeText bool `json:"includeText"`
	}
	if json.Unmarshal(sync.Save, &opts) != nil {
		return false, false
	}
	return true, opts.IncludeText
}

// Forget forgets a file, telling the server so.
func (c *Client) Forget(ctx context.Context, path string) error {
	uri := FileURI(path)
	c.mu.Lock()
	_, open := c.docs[uri]
	delete(c.docs, uri)
	c.mu.Unlock()
	if !open {
		return nil
	}
	return c.notify(ctx, "textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": uri}})
}

// Settle waits until the server has published the diagnostics of the
// file at path since the count of publishes since (Sync's), and then for
// a moment of quiet with no more publishes of any file, as a change in
// one file shows up in others; or until ctx ends. It reports whether the
// file's diagnostics came.
func (c *Client) Settle(ctx context.Context, path string, since uint64, quiet time.Duration) bool {
	uri := FileURI(path)
	for {
		c.mu.Lock()
		got := c.diags[uri].seq > since
		changed := c.changed
		c.mu.Unlock()
		if got {
			break
		}
		select {
		case <-changed:
		case <-c.done:
			return false
		case <-ctx.Done():
			return false
		}
	}
	for {
		c.mu.Lock()
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-changed:
		case <-time.After(quiet):
			return true
		case <-c.done:
			return true
		case <-ctx.Done():
			return true
		}
	}
}

// Diagnostics is every file's diagnostics as the server last published
// them, by path; files with none are left out.
func (c *Client) Diagnostics() map[string][]Diagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string][]Diagnostic{}
	for uri, p := range c.diags {
		if path := URIPath(uri); path != "" && len(p.diags) > 0 {
			out[path] = append([]Diagnostic(nil), p.diags...)
		}
	}
	return out
}

// Text is what the server was last told the file at path holds, and
// whether it was told of it.
func (c *Client) Text(path string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.docs[FileURI(path)]
	if d == nil {
		return "", false
	}
	return d.text, true
}
