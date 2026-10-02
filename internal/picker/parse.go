package picker

// key is what the list makes of the bytes of one key.
type key int

const (
	other key = iota // anything the list does nothing with
	up
	down
	enter
	esc
	interrupt // Ctrl+C
	kill      // Ctrl+X
)

// parse takes the first key off input: the key and its length in bytes, or a length of 0 where
// input only starts one, an Esc or a sequence that may still be arriving. Terminals send a key
// with Alt as Esc and the key, which does nothing (decision 14).
func parse(input []byte) (key, int) {
	switch input[0] {
	case '\r':
		return enter, 1
	case 0x03: // Ctrl+C in raw mode
		return interrupt, 1
	case 0x18: // Ctrl+X in raw mode
		return kill, 1
	case 0x1b:
		return parseEscape(input)
	}
	return other, 1
}

// parseEscape is parse for input that starts with Esc. The arrows are `ESC [ A` and `ESC [ B`, or
// `ESC O A` and `ESC O B` once a program has turned on application cursor keys: the list takes
// both, and sets neither mode itself.
func parseEscape(input []byte) (key, int) {
	if len(input) == 1 {
		return other, 0
	}
	switch input[1] {
	case '[':
		return parseSequence(input)
	case 'O':
		if len(input) == 2 {
			return other, 0
		}
		return arrow(input[2]), 3
	case 0x1b:
		return parseEscapes(input)
	}
	return other, 2
}

// parseEscapes is parse for input that starts with two Escs: the first stands alone, unless a
// sequence follows the second, as in Alt+Up (`ESC`, then `ESC [ A`).
func parseEscapes(input []byte) (key, int) {
	if len(input) == 2 {
		return other, 0
	}
	if input[2] != '[' && input[2] != 'O' {
		return esc, 1
	}
	if _, n := parse(input[1:]); n > 0 {
		return other, 1 + n
	}
	return other, 0
}

// parseSequence is parse for a control sequence: `ESC [`, parameter bytes, intermediate bytes,
// then a final byte.
func parseSequence(input []byte) (key, int) {
	i := 2
	for i < len(input) && input[i] >= 0x30 && input[i] <= 0x3f {
		i++
	}
	for i < len(input) && input[i] >= 0x20 && input[i] <= 0x2f {
		i++
	}
	if i == len(input) {
		return other, 0
	}
	if input[i] < 0x40 || input[i] > 0x7e {
		return other, i // not a sequence after all: what came before goes
	}
	// A modified arrow, such as Shift+Up (`ESC [ 1 ; 2 A`), does nothing.
	if params := string(input[2:i]); params == "" || params == "1" {
		return arrow(input[i]), i + 1
	}
	return other, i + 1
}

// arrow is the key a cursor key sequence ending in final is.
func arrow(final byte) key {
	switch final {
	case 'A':
		return up
	case 'B':
		return down
	}
	return other
}
