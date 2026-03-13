package printing

// ============================================================================
// ESC/POS COMMAND CONSTANTS — Winpal WP230W compatible
// ============================================================================

// ESC/POS standard commands
var (
	// Initialization
	CmdInit  = []byte{0x1B, 0x40} // ESC @ — Initialize printer
	CmdReset = []byte{0x1B, 0x40}

	// Text formatting
	CmdBoldOn    = []byte{0x1B, 0x45, 0x01} // ESC E 1
	CmdBoldOff   = []byte{0x1B, 0x45, 0x00} // ESC E 0
	CmdUnderOn   = []byte{0x1B, 0x2D, 0x01} // ESC - 1
	CmdUnderOff  = []byte{0x1B, 0x2D, 0x00} // ESC - 0
	CmdDoubleH   = []byte{0x1B, 0x21, 0x10} // ESC ! — double height
	CmdDoubleW   = []byte{0x1B, 0x21, 0x20} // ESC ! — double width
	CmdDoubleHW  = []byte{0x1B, 0x21, 0x30} // ESC ! — double height + width
	CmdNormalSize = []byte{0x1B, 0x21, 0x00} // ESC ! — normal size

	// Alignment
	CmdAlignLeft   = []byte{0x1B, 0x61, 0x00} // ESC a 0
	CmdAlignCenter = []byte{0x1B, 0x61, 0x01} // ESC a 1
	CmdAlignRight  = []byte{0x1B, 0x61, 0x02} // ESC a 2

	// Line spacing
	CmdDefaultLineSpacing = []byte{0x1B, 0x32}       // ESC 2
	CmdSetLineSpacing     = []byte{0x1B, 0x33}       // ESC 3 n (followed by n byte)

	// Paper cutting
	CmdFullCut    = []byte{0x1D, 0x56, 0x00} // GS V 0 — full cut
	CmdPartialCut = []byte{0x1D, 0x56, 0x01} // GS V 1 — partial cut
	CmdFeedAndCut = []byte{0x1D, 0x56, 0x42, 0x03} // GS V B 3 — feed 3 lines + partial cut

	// Line feed
	CmdLF      = []byte{0x0A}             // Line feed
	CmdFeed3   = []byte{0x1B, 0x64, 0x03} // ESC d 3 — feed 3 lines
	CmdFeed5   = []byte{0x1B, 0x64, 0x05} // ESC d 5 — feed 5 lines

	// Cash drawer
	CmdOpenDrawer = []byte{0x1B, 0x70, 0x00, 0x19, 0xFA} // ESC p — open cash drawer
)

// PaperWidth constants
const (
	Paper58mm  = 58  // 32 characters per line
	Paper80mm  = 80  // 48 characters per line
)

// CharsPerLine returns the number of printable characters per line
// for the given paper width.
func CharsPerLine(paperWidth int) int {
	switch paperWidth {
	case Paper58mm:
		return 32
	case Paper80mm:
		return 48
	default:
		return 48 // default to 80mm
	}
}

// DividerLine returns a full-width dashed line.
func DividerLine(paperWidth int) string {
	chars := CharsPerLine(paperWidth)
	line := make([]byte, chars)
	for i := range line {
		line[i] = '-'
	}
	return string(line)
}

// DoubleDividerLine returns a full-width equals line.
func DoubleDividerLine(paperWidth int) string {
	chars := CharsPerLine(paperWidth)
	line := make([]byte, chars)
	for i := range line {
		line[i] = '='
	}
	return string(line)
}

// PadRight pads a string on the right to fill the given width.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + spaces(width-len(s))
}

// PadLeft pads a string on the left to fill the given width.
func PadLeft(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return spaces(width-len(s)) + s
}

// FormatTwoColumn formats a left-aligned and right-aligned string on one line.
func FormatTwoColumn(left, right string, lineWidth int) string {
	gap := lineWidth - len(left) - len(right)
	if gap < 1 {
		gap = 1
	}
	return left + spaces(gap) + right
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
