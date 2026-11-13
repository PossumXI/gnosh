package transcript

// This file is the slide: what leaves the view, without leaving a frame,
// when the view is over its budget. It follows spec/gnosh/frames.md
// section 8. A slide is planned here and then recorded as a frame entry
// (op "slide"), so that every later view cuts the same place.

// planSlides picks, applies and returns the cuts of a slide that frees at
// least need tokens, or as much as can go. It cuts nothing when the
// session stands in no frame.
//
// What goes, in order, until enough is freed:
//
//	(a) the rounds before the first frame, from the summary (or the start)
//	    to the bottom frame's push;
//	(b) each frame that has a frame above it, from the bottom: its whole
//	    body, from its push round to the next frame's push;
//	(c) the oldest rounds of the frame the session stands in.
//
// Every cut starts after a round that stays (the first round of the
// region, or the frame's push round) and ends before a reply with a tool
// call, so no call is parted from its result and each open frame keeps the
// round that entered the next. Where the cut may end anywhere, (a) and
// (c), it is the shortest that frees the need and at least half of the
// region; if none does, the longest. The prompt the model is answering is
// never cut: a region that holds it is cut before it, and in (a) after it
// as well. The walk stops at the first frame whose push the model no
// longer has.
func (c *conv) planSlides(st *derived, need int, opts ViewOptions) []cut {
	if need <= 0 || len(st.stack) == 0 {
		return nil
	}
	keep := max(opts.KeepRounds, 1)
	prompt := c.promptRow(opts.Prompt)

	var push []int // the push rows of the frames, bottom up, while the model has them
	for _, f := range st.stack {
		if f.row < 0 || !c.present(f.row) {
			break
		}
		push = append(push, f.row)
	}
	if len(push) == 0 {
		return nil
	}

	var out []cut
	// slide cuts from after anchor's round to before until, and reports
	// whether enough has now been freed.
	slide := func(frame string, anchor, until int) bool {
		if until < 0 || c.entries[c.rows[anchor].ent].UUID == "" || c.entries[c.rows[until].ent].UUID == "" {
			return false
		}
		ct := cut{kind: OpSlide, frame: frame, ent: -1, pushEnt: -1, at: len(c.rows), noteRow: -1, turn: len(c.rows), live: true}
		c.bound(&ct, anchor, until)
		if ct.from >= ct.to || c.span(ct.from, ct.to) == 0 {
			return false
		}
		c.apply(&ct)
		out = append(out, ct)
		if frame != "" {
			st.slid[frame] = true
		}
		need -= ct.tokens
		return need <= 0
	}

	// (a) The rounds before the first frame.
	bottom := push[0]
	if prompt >= c.start && prompt < bottom {
		if a := c.firstRound(c.start, prompt-1); a >= 0 {
			if slide("", a, c.partial(a, prompt-1, prompt, need, 1)) {
				return out
			}
		}
		if a := c.firstRound(prompt+1, bottom); a >= 0 {
			if slide("", a, c.partial(a, bottom, bottom, need, 1)) {
				return out
			}
		}
	} else if a := c.firstRound(c.start, bottom); a >= 0 {
		if slide("", a, c.partial(a, bottom, bottom, need, 1)) {
			return out
		}
	}

	// (b) Each frame with a frame above it, from the bottom.
	for i := 0; i+1 < len(push); i++ {
		a, next := push[i], push[i+1]
		until := next
		if prompt > a && prompt < next {
			until = c.partial(a, prompt-1, prompt, need, 1)
		}
		if slide(st.stack[i].id, a, until) {
			return out
		}
	}

	// (c) The frame the session stands in.
	if len(push) == len(st.stack) {
		a := push[len(push)-1]
		last, end, k := len(c.rows)-1, len(c.rows), keep
		if prompt > a {
			last, end, k = prompt-1, prompt, 1
		}
		slide(st.stack[len(push)-1].id, a, c.partial(a, last, end, need, k))
	}
	return out
}

// promptRow returns the prompt row that came from the entry uuid, or -1.
func (c *conv) promptRow(uuid string) int {
	if uuid == "" {
		return -1
	}
	for r := len(c.rows) - 1; r >= 0; r-- {
		if c.rows[r].kind == rowPrompt && c.entries[c.rows[r].ent].UUID == uuid {
			return r
		}
	}
	return -1
}

// round reports whether row r is a reply with a tool call that the model
// still has: the start of a round.
func (c *conv) round(r int) bool {
	return c.present(r) && c.rows[r].kind == rowAssistant && c.rows[r].calls > 0
}

// firstRound returns the first round in rows [lo, hi], or -1.
func (c *conv) firstRound(lo, hi int) int {
	for r := max(lo, c.start); r <= hi && r < len(c.rows); r++ {
		if c.round(r) {
			return r
		}
	}
	return -1
}

// partial picks where a cut that starts after anchor's round should end.
// The cut may end before any round that starts in rows [from, last], less
// the newest keep-1 of them. It ends at the first that frees at least need
// tokens and at least half of what the rows up to end hold; at the last
// when none frees that much. It returns -1 when the cut would free
// nothing.
func (c *conv) partial(anchor, last, end, need, keep int) int {
	from := c.lastAnswer(anchor) + 1
	var ends []int
	for r := from; r <= last && r < len(c.rows); r++ {
		if c.round(r) {
			ends = append(ends, r)
		}
	}
	if len(ends) < keep {
		return -1
	}
	ends = ends[:len(ends)-(keep-1)]
	target := max(need, c.span(from, end)/2)
	until, freed, r := -1, 0, from
	for _, e := range ends {
		for ; r < e; r++ {
			if c.present(r) {
				freed += c.rowTokens(r)
			}
		}
		until = e
		if freed >= target {
			break
		}
	}
	if freed == 0 {
		return -1
	}
	return until
}

// slideEntry makes the frame entry that records a planned slide.
func (c *conv) slideEntry(ct *cut) Entry {
	return Entry{
		Type:   TypeFrame,
		Op:     OpSlide,
		Frame:  ct.frame,
		Anchor: c.entries[c.rows[ct.anchor].ent].UUID,
		Until:  c.entries[c.rows[ct.until].ent].UUID,
	}
}
