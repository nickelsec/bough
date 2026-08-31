package pick

import "os"

// key is one meaningful press. Everything else is ignored.
type key int

const (
	keyNone key = iota
	keyUp
	keyDown
	keyHome
	keyEnd
	keyEnter
	keyCancel
)

// readKey waits for the next press.
//
// Arrow keys arrive as an escape sequence of three or more bytes, while escape
// on its own is a single byte. Reading into a buffer and inspecting what came
// back tells the two apart without waiting for a timeout.
func readKey(in *os.File) (key, error) {
	buf := make([]byte, 8)
	n, err := in.Read(buf)
	if err != nil {
		return keyNone, err
	}
	return classify(buf[:n]), nil
}

// classify turns the bytes a key press produced into a key.
func classify(b []byte) key {
	n := len(b)
	if n == 0 {
		return keyNone
	}

	if n == 1 {
		switch b[0] {
		case 13, 10: // enter
			return keyEnter
		case 27: // escape on its own
			return keyCancel
		case 3, 4, 'q': // ctrl-c, ctrl-d, q
			return keyCancel
		case 'k':
			return keyUp
		case 'j':
			return keyDown
		}
		return keyNone
	}

	// Escape sequences: ESC [ A and friends.
	if n >= 3 && b[0] == 27 && (b[1] == '[' || b[1] == 'O') {
		switch b[2] {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		case 'H':
			return keyHome
		case 'F':
			return keyEnd
		}
	}
	return keyNone
}
