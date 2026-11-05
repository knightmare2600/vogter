package mapterm

import "strings"

// Mode is how cells are drawn.
type Mode int

// Drawing modes.
const (
	// ModeBraille shows every dot, eight to a cell. It is the sharpest.
	ModeBraille Mode = iota
	// ModeUnicode draws box-drawing and shade characters.
	ModeUnicode
	// ModeASCII uses only ASCII, for terminals without Unicode fonts.
	ModeASCII
)

// Modes in the order a toggle cycles them.
var Modes = []Mode{ModeBraille, ModeUnicode, ModeASCII}

func (m Mode) String() string {
	return [...]string{"braille", "unicode", "ascii"}[m]
}

// ParseMode reads a mode name, falling back to braille.
func ParseMode(s string) (Mode, bool) {
	for _, m := range Modes {
		if strings.EqualFold(s, m.String()) {
			return m, true
		}
	}
	return ModeBraille, false
}

// Cell is one terminal cell of a finished map.
type Cell struct {
	R     rune
	Class Class
	// Label marks text, which takes the label colour.
	Label bool
}

// Frame is a finished map, ready to colour.
type Frame struct {
	Cols, Rows int
	Cells      []Cell
}

// At returns a cell.
func (f *Frame) At(col, row int) *Cell { return &f.Cells[row*f.Cols+col] }

// Compose turns a canvas into characters in the given mode.
func Compose(c *Canvas, mode Mode) *Frame {
	f := &Frame{Cols: c.Cols, Rows: c.Rows, Cells: make([]Cell, c.Cols*c.Rows)}
	for row := range c.Rows {
		for col := range c.Cols {
			dots, class := c.Cell(col, row)
			r := ' '
			if dots != 0 {
				switch mode {
				case ModeBraille:
					r = Braille(dots)
				default:
					r = symbol(dots, class, mode == ModeASCII)
				}
			}
			f.Cells[row*c.Cols+col] = Cell{R: r, Class: class}
		}
	}
	return f
}

// Direction of the dots in a cell.
type direction int

const (
	dirDot direction = iota
	dirHorizontal
	dirVertical
	dirRising
	dirFalling
)

// shape reads which way a line runs through a cell from the dots lit in it.
func shape(dots uint8) direction {
	var xs, ys []int
	for y := range CellH {
		for x := range CellW {
			if dots&bit[y][x] != 0 {
				xs, ys = append(xs, x), append(ys, y)
			}
		}
	}
	if len(xs) <= 1 {
		return dirDot
	}
	minX, maxX, minY, maxY := 9, -1, 9, -1
	for i := range xs {
		minX, maxX = min(minX, xs[i]), max(maxX, xs[i])
		minY, maxY = min(minY, ys[i]), max(maxY, ys[i])
	}
	switch {
	case maxY-minY <= 1 && maxX > minX && len(xs) <= 4 && (maxY == minY || len(xs) >= 3):
		return dirHorizontal
	case maxX == minX:
		return dirVertical
	}
	// The sign of the covariance gives the slope; y grows downwards.
	var mx, my float64
	for i := range xs {
		mx += float64(xs[i])
		my += float64(ys[i])
	}
	mx, my = mx/float64(len(xs)), my/float64(len(xs))
	cov := 0.0
	for i := range xs {
		cov += (float64(xs[i]) - mx) * (float64(ys[i]) - my)
	}
	switch {
	case cov > 0:
		return dirFalling
	case cov < 0:
		return dirRising
	}
	return dirHorizontal
}

// glyphs per class: dot, horizontal, vertical, rising, falling.
type glyphs [5]rune

var unicodeGlyphs = map[Class]glyphs{
	ClassMotorway:  {'═', '═', '║', '╱', '╲'},
	ClassMajorRoad: {'·', '─', '│', '╱', '╲'},
	ClassMinorRoad: {'·', '·', '·', '·', '·'},
	ClassRail:      {'╪', '╪', '╫', '╱', '╲'},
	ClassBoundary:  {'┄', '┄', '┆', '╱', '╲'},
	ClassWaterway:  {'~', '~', '│', '╱', '╲'},
	ClassBuilding:  {'▒', '▒', '▒', '▒', '▒'},
	ClassPark:      {'░', '░', '░', '░', '░'},
	ClassWater:     {'≈', '≈', '≈', '≈', '≈'},
	ClassLand:      {'·', '·', '·', '·', '·'},
}

var asciiGlyphs = map[Class]glyphs{
	ClassMotorway:  {'=', '=', 'H', '/', '\\'},
	ClassMajorRoad: {'.', '-', '|', '/', '\\'},
	ClassMinorRoad: {'.', '.', '.', '.', '.'},
	ClassRail:      {'+', '+', '+', '/', '\\'},
	ClassBoundary:  {':', ':', ':', ':', ':'},
	ClassWaterway:  {'~', '~', '|', '/', '\\'},
	ClassBuilding:  {'#', '#', '#', '#', '#'},
	ClassPark:      {'"', '"', '"', '"', '"'},
	ClassWater:     {'~', '~', '~', '~', '~'},
	ClassLand:      {'.', '.', '.', '.', '.'},
}

func symbol(dots uint8, class Class, ascii bool) rune {
	table := unicodeGlyphs
	if ascii {
		table = asciiGlyphs
	}
	g, ok := table[class]
	if !ok {
		if ascii {
			return '.'
		}
		return '·'
	}
	if class.Fills() {
		return g[0]
	}
	return g[shape(dots)]
}
