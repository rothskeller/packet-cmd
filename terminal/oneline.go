package terminal

import (
	"errors"
	"strings"
)

const invalid = 99999

func (e *editor) onelineMode() (modefunc, EditResult, error) {
	var (
		fieldWidth int
		entryx     int
		verbatim   bool
	)
	e.term.clearToEOS()
	defer func() {
		e.term.move(0, 0)
		e.term.clearToEOS()
	}()
	entryx = e.labelWidth + 2
	fieldWidth = e.fieldWidth
	if fieldWidth == 0 || entryx+fieldWidth >= e.term.width {
		fieldWidth = e.term.width - entryx - 1
	}
	for {
		// Draw the label, entry area, and hint.
		var buf = newScreenBuf(e.term.width - 1)
		buf.writeAt(0, 0, colorLabel, e.field.Label)
		buf.fill(entryx, fieldWidth, 0, colorEntry)
		if len(e.value) < fieldWidth && e.field.EditHint != "" &&
			entryx+fieldWidth+len(e.field.EditHint)+2 < e.term.width {
			buf.writeAt(entryx+fieldWidth+2, 0, colorHint, e.field.EditHint)
		}
		// Write the value with selection.
		pre, sel, post := splitOnSelect(e.value, e.sels, e.sele)
		buf.writeAt(entryx, 0, colorEntry, pre)
		buf.write(colorSelected, sel)
		buf.write(colorEntry, post)
		// Update the screen with the buffer.
		e.term.paintBuf(buf)
		// Move the cursor to the proper spot.
		e.term.move(entryx+e.cursor, 0)
		// Get a key and handle it.
		switch key := e.term.readKey(); key {
		case 0:
			return nil, 0, errors.New("error reading stdin")
		case 0x01, keyHome: // Ctrl-A
			e.sels, e.sele, e.cursor = 0, 0, 0
		case keyShiftHome:
			e.sels, e.cursor = 0, 0
		case 0x02, keyLeft: // Ctrl-B
			if e.cursor > 0 {
				e.cursor--
			}
			e.sels, e.sele = e.cursor, e.cursor
		case keyShiftLeft:
			if e.sels != e.sele && e.sele == e.cursor {
				e.sele, e.cursor = e.sele-1, e.cursor-1
			} else if e.cursor > 0 {
				e.sels, e.cursor = e.sels-1, e.cursor-1
			}
		case keyCtrlLeft:
			e.cursor = prevword(e.value, e.cursor)
			e.sels, e.sele = e.cursor, e.cursor
		case keyCtrlShiftLeft:
			if e.sels != e.sele && e.sele == e.cursor {
				e.cursor = prevword(e.value, e.cursor)
				e.sele = e.cursor
				if e.sele < e.sels {
					e.sels = e.sele
				}
			} else {
				e.cursor = prevword(e.value, e.cursor)
				e.sels = e.cursor
			}
		case 0x03: // Ctrl-C
			return nil, 0, errors.New("interrupted")
		case 0x04, keyDelete: // Ctrl-D
			if len(e.value) > e.cursor {
				e.value = e.value[:e.cursor] + e.value[e.cursor+1:]
			}
			e.sels, e.sele = e.cursor, e.cursor
		case 0x05, keyEnd: // Ctrl-E, End
			e.cursor = len(e.value)
			e.sels, e.sele = e.cursor, e.cursor
		case keyShiftEnd:
			e.cursor = len(e.value)
			e.sele = e.cursor
		case 0x06, keyRight: // Ctrl-F
			if e.cursor < len(e.value) {
				e.cursor++
			}
			e.sels, e.sele = e.cursor, e.cursor
		case keyShiftRight:
			if e.sels != e.sele && e.sels == e.cursor {
				e.sels, e.cursor = e.sels+1, e.cursor+1
			} else if e.cursor < len(e.value) {
				e.sele, e.cursor = e.sele+1, e.cursor+1
			}
		case keyCtrlRight:
			e.cursor = nextword(e.value, e.cursor)
			e.sels, e.sele = e.cursor, e.cursor
		case keyCtrlShiftRight:
			if e.sels != e.sele && e.sels == e.cursor {
				e.cursor = nextword(e.value, e.cursor)
				e.sels = e.cursor
				if e.sels < e.sele {
					e.sele = e.sels
				}
			} else {
				e.cursor = nextword(e.value, e.cursor)
				e.sele = e.cursor
			}
		case 0x08, 0x7f: // Backspace
			if e.sels != e.sele {
				e.value = e.value[:e.sels] + e.value[e.sele:]
				e.cursor = e.sels
			} else if e.cursor > 0 {
				e.value = e.value[:e.cursor-1] + e.value[e.cursor:]
				e.cursor--
			}
			e.sels, e.sele = e.cursor, e.cursor
		case 0x09: // Tab
			return nil, ResultNext, nil
		case keyBackTab:
			return nil, ResultPrevious, nil
		case 0x0A, 0x0D: // Enter
			if verbatim || (e.cursor != 0 && e.field.Multiline) {
				// Add a literal newline and switch to multiline mode.
				e.value = e.value[:e.sels] + "\n" + e.value[e.sele:]
				e.sels++
				e.cursor, e.sele = e.sels, e.sels
				return e.multilineMode, 0, nil
			}
			return nil, ResultNext, nil
		case 0x0B: // Ctrl-K
			e.value = e.value[:e.cursor]
			e.sels, e.sele = e.cursor, e.cursor
		case 0x0C: // Ctrl-L
			return e.onelineMode, 0, nil
		case 0x15: // Ctrl-U
			e.value = ""
			e.sels, e.sele, e.cursor = 0, 0, 0
		case 0x16: // Ctrl-V
			verbatim = true
			continue
		case 0x1B:
			return nil, ResultDone, nil
		case keyF1:
			e.showHelp()
			return e.onelineMode, 0, nil
		default:
			if key >= 0x20 && key <= 0x7e { // Printable character
				e.value = e.value[:e.sels] + string(key) + e.value[e.sele:]
				e.cursor = e.sels + 1
				e.sels, e.sele = e.cursor, e.cursor
				if e.sels == len(e.value) {
					if auto := autocomplete(e.value, e.choices); auto != "" {
						e.value, e.sele = auto, len(auto)
					}
				}
				if entryx+len(e.value) >= e.term.width {
					// No room for value, switch to multiline mode.
					return e.multilineMode, 0, nil
				}
			}
		}
		verbatim = false
	}
}

func autocomplete(s string, choices []string) (match string) {
	for _, c := range choices {
		if len(c) > len(s) && strings.EqualFold(s, c[:len(s)]) {
			if match == "" {
				match = c
			} else {
				match = c[:diffindex(c, match)]
			}
		}
	}
	return match
}
func prevword(s string, cur int) int {
	for cur > 0 && (s[cur] == ' ' || s[cur] == '\n') {
		cur--
	}
	for cur > 0 && s[cur-1] != ' ' && s[cur] != '\n' {
		cur--
	}
	return cur
}
func nextword(s string, cur int) int {
	l := len(s)
	for cur < l && (s[cur] == ' ' || s[cur] == '\n') {
		cur++
	}
	for cur < l && s[cur] != ' ' && s[cur] != '\n' {
		cur++
	}
	return cur
}
func diffindex(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) > len(b) {
		return len(b)
	}
	if len(b) > len(a) {
		return len(a)
	}
	return invalid
}
