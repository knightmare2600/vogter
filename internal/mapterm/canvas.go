package mapterm

// Canvas is a grid of dots grouped into terminal cells of 2×4 dots, the shape
// of a braille character. Each cell keeps one style, the most important one
// drawn into it, as a terminal cell can show only one colour.
type Canvas struct {
	// Cols and Rows are the size in terminal cells.
	Cols, Rows int
	dots       []uint8
	style      []Class
}

// Class is what a dot belongs to. A higher class wins a shared cell.
type Class uint8

// Cell sizes in dots.
const (
	CellW = 2
	CellH = 4
)

// NewCanvas makes a canvas of cols×rows cells.
func NewCanvas(cols, rows int) *Canvas {
	cols, rows = max(cols, 0), max(rows, 0)
	return &Canvas{Cols: cols, Rows: rows, dots: make([]uint8, cols*rows), style: make([]Class, cols*rows)}
}

// W and H are the size in dots.
func (c *Canvas) W() int { return c.Cols * CellW }
func (c *Canvas) H() int { return c.Rows * CellH }

// bit numbers dots in the order braille assigns them.
var bit = [CellH][CellW]uint8{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// Set lights one dot. Dots outside the canvas are ignored.
func (c *Canvas) Set(x, y int, class Class) {
	if x < 0 || y < 0 || x >= c.W() || y >= c.H() {
		return
	}
	i := (y/CellH)*c.Cols + x/CellW
	c.dots[i] |= bit[y%CellH][x%CellW]
	c.style[i] = max(c.style[i], class)
}

// Cell returns a cell's dot pattern, as the low byte of its braille rune,
// and its class.
func (c *Canvas) Cell(col, row int) (uint8, Class) {
	i := row*c.Cols + col
	return c.dots[i], c.style[i]
}

// Braille is the braille character for a dot pattern.
func Braille(pattern uint8) rune { return 0x2800 + rune(pattern) }
