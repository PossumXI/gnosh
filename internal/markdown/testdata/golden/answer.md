The bug is in `flush`: it writes the header **before** checking whether the buffer is empty.

## What happens

1. `Write` appends to the buffer.
2. `flush` is called from two places:
   - the timer, every 50 ms
   - `Close`
3. On an empty buffer the header goes out alone, and the peer sees a truncated frame.

## The fix

```go
func (w *writer) flush() error {
	if w.buf.Len() == 0 {
		return nil // nothing to send, and no header either
	}
	return w.send(w.header(), w.buf.Bytes())
}
```

| Case | Before | After |
|------|--------|-------|
| empty buffer | header only | nothing |
| one byte | header + byte | header + byte |

> **Note:** the test in `writer_test.go` only covered the second row.

See <https://example.com/issues/42> for the report.
