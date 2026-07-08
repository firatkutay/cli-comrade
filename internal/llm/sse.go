package llm

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// errSSEDone signals that scanSSE's caller-supplied fn detected the
// openai_compat "data: [DONE]" sentinel. scanSSE treats it as a normal,
// non-error stop (returning nil to its own caller) rather than a stream
// failure — callers detect it purely by returning it from fn, never by
// inspecting scanSSE's return value themselves.
var errSSEDone = errors.New("llm: sse [DONE] sentinel")

// sseEvent is one "data:" line from a Server-Sent Events stream, along
// with any "event:" line that immediately preceded it in the same
// blank-line-delimited frame. Anthropic sends both an "event:" line and a
// matching "type" field inside the JSON payload; openai_compat and Google
// send bare "data:" lines with no "event:" line at all. Every connector in
// this package dispatches purely on the JSON payload's own type field, so
// Event is carried for completeness but not required by any of them.
type sseEvent struct {
	Event string
	Data  string
}

// scanSSE reads Server-Sent Events frames from r, calling fn once per
// "data:" line. Comment lines (leading ':'), "id:"/"retry:" fields, and
// blank frame separators are consumed and ignored. It returns fn's first
// non-nil error, except errSSEDone, which is swallowed (scanSSE returns
// nil), or the underlying scanner's own error, or nil at a clean EOF.
func scanSSE(r io.Reader, fn func(sseEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var evt sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			evt = sseEvent{}
		case strings.HasPrefix(line, ":"):
			// comment / heartbeat line — ignored
		case strings.HasPrefix(line, "event:"):
			evt.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			evt.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if err := fn(evt); err != nil {
				if errors.Is(err, errSSEDone) {
					return nil
				}
				return err
			}
		default:
			// id:, retry:, or an unrecognized field — ignored
		}
	}
	return scanner.Err()
}
