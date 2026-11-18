// Package qr encodes text as a QR code, as the codes a camera reads on a
// screen, a ticket or a poster are: the text of a share link, a download code,
// a WhatsApp number.
//
//	code, err := qr.Encode("https://picsel.app/e/abc123", qr.Medium)
//
// A code draws in a MyGo view with Element, and becomes a file with PNG or
// SVG.
package qr

import (
	"errors"
	"fmt"
	"slices"
)

// ErrTooLong is what Encode answers for a text no QR code holds at its level.
var ErrTooLong = errors.New("qr: the text is too long for a QR code")

// Level is how much of a code a reader can lose and still read it: Low loses
// 7% of it, Medium 15%, Quartile 25% and High 30%.
type Level uint8

const (
	Low Level = iota
	Medium
	Quartile
	High
)

// levelBits are the bits of the level in the format information of a code.
var levelBits = [4]int{1, 0, 3, 2}

// Code is a QR code: the modules it is made of, and how to draw them.
type Code struct {
	level   Level
	version int
	size    int
	mask    int
	modules []bool
	picture picture
}

// Encode returns the smallest code of text at level.
func Encode(text string, level Level) (*Code, error) {
	if level > High {
		return nil, fmt.Errorf("qr: unknown level %d", level)
	}
	data := []byte(text)
	version := 0
	for candidate := 1; candidate <= 40; candidate++ {
		capacity := dataCodewords(candidate, level) * 8
		needed := 4 + countBits(candidate) + len(data)*8
		if needed <= capacity {
			version = candidate
			break
		}
	}
	if version == 0 {
		return nil, ErrTooLong
	}
	codewords, err := encodeData(data, version, level)
	if err != nil {
		return nil, err
	}
	return draw(codewords, version, level)
}

// Level is the level the code was made at.
func (c *Code) Level() Level { return c.level }

// Version is the version of the code, from 1 to 40.
func (c *Code) Version() int { return c.version }

// Size is how many modules the code is a side, the quiet zone aside.
func (c *Code) Size() int { return c.size }

// Dark reports whether the module in row and column is dark, as a code reads
// it. Row and column are 0 at the top left.
func (c *Code) Dark(row, column int) bool {
	if row < 0 || column < 0 || row >= c.size || column >= c.size {
		return false
	}
	return c.modules[row*c.size+column]
}

// countBits is how many bits the length of the text takes at version.
func countBits(version int) int {
	if version <= 9 {
		return 8
	}
	return 16
}

// dataCodewords is how many codewords of text a version holds at level.
func dataCodewords(version int, level Level) int {
	specification := specs[version-1]
	return specification.words - specification.ec[level]
}

// encodeData returns the codewords of text, with the error correction ones
// interleaved, as the modules of a code hold them.
func encodeData(data []byte, version int, level Level) ([]byte, error) {
	capacity := dataCodewords(version, level)
	buffer := bitBuffer{}
	buffer.put(0b0100, 4)
	buffer.put(len(data), countBits(version))
	for _, b := range data {
		buffer.put(int(b), 8)
	}
	if buffer.len() > capacity*8 {
		return nil, ErrTooLong
	}
	buffer.put(0, min(4, capacity*8-buffer.len()))
	buffer.put(0, (8-buffer.len()%8)%8)
	for pad := 0; buffer.len() < capacity*8; pad++ {
		if pad%2 == 0 {
			buffer.put(0xEC, 8)
		} else {
			buffer.put(0x11, 8)
		}
	}

	specification := specs[version-1]
	blocks := blocksFor(specification, level)
	// The text splits into blocks: the longer ones last, as a store writes a
	// code.
	remainder := capacity % blocks.count()
	short := capacity / blocks.count()
	at := 0
	encoded := make([]block, 0, blocks.count())
	for i := 0; i < blocks.count(); i++ {
		length := short
		if i >= blocks.count()-remainder {
			length++
		}
		text := buffer.bytes()[at : at+length]
		at += length
		encoded = append(encoded, block{data: text, ec: reedSolomon(text, blocks.ecPerBlock)})
	}
	return interleave(encoded, blocks.ecPerBlock), nil
}

// blocksFor returns how a version splits its text into blocks at level.
func blocksFor(specification specification, level Level) blocks {
	group := specification.blocks[level]
	ec := specification.ec[level]
	return blocks{first: group[0], second: group[1], ecPerBlock: ec / (group[0] + group[1])}
}

// blocks is how a version splits its text: two groups of blocks whose text
// codewords differ by one, and the error correction codewords of each block.
type blocks struct {
	first, second int
	ecPerBlock    int
}

func (b blocks) count() int { return b.first + b.second }

// block is one block of the text of a code, with its error correction
// codewords.
type block struct {
	data []byte
	ec   []byte
}

// interleave returns the codewords of the blocks as a code holds them: the
// n-th codeword of every block, then the n-th error correction codeword.
func interleave(blocks []block, ecPerBlock int) []byte {
	longest := 0
	for _, b := range blocks {
		longest = max(longest, len(b.data))
	}
	out := make([]byte, 0, longest*len(blocks)+ecPerBlock*len(blocks))
	for i := 0; i < longest; i++ {
		for _, b := range blocks {
			if i < len(b.data) {
				out = append(out, b.data[i])
			}
		}
	}
	for i := 0; i < ecPerBlock; i++ {
		for _, b := range blocks {
			out = append(out, b.ec[i])
		}
	}
	return out
}

// bitBuffer collects bits, the most significant first.
type bitBuffer struct {
	bits []bool
}

func (b *bitBuffer) put(value, count int) {
	for i := count - 1; i >= 0; i-- {
		b.bits = append(b.bits, value>>i&1 == 1)
	}
}

func (b *bitBuffer) len() int { return len(b.bits) }

func (b *bitBuffer) bytes() []byte {
	out := make([]byte, (len(b.bits)+7)/8)
	for i, bit := range b.bits {
		if bit {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

// draw returns the code of the codewords: their modules, the patterns of the
// code and the mask that makes it read best.
func draw(codewords []byte, version int, level Level) (*Code, error) {
	specification := specs[version-1]
	symbol := newSymbol(specification.size)
	symbol.patterns(version)
	symbol.codewords(codewords, specification.remainder)

	best := -1
	bestPenalty := 0
	var bestModules []bool
	for mask := 0; mask < 8; mask++ {
		candidate := symbol.clone()
		candidate.mask(mask)
		candidate.format(level, mask)
		penalty := candidate.penalty()
		if best < 0 || penalty < bestPenalty {
			best, bestPenalty, bestModules = mask, penalty, candidate.dark
		}
	}
	return &Code{
		level:   level,
		version: version,
		size:    specification.size,
		mask:    best,
		modules: bestModules,
	}, nil
}

// symbol is a code as it is drawn: which modules are dark, and which belong to
// its patterns, which no codeword and no mask touches.
type symbol struct {
	size     int
	dark     []bool
	reserved []bool
}

func newSymbol(size int) *symbol {
	return &symbol{size: size, dark: make([]bool, size*size), reserved: make([]bool, size*size)}
}

func (s *symbol) clone() *symbol {
	return &symbol{size: s.size, dark: slices.Clone(s.dark), reserved: slices.Clone(s.reserved)}
}

func (s *symbol) set(row, column int, dark bool) {
	if row < 0 || column < 0 || row >= s.size || column >= s.size {
		return
	}
	s.dark[row*s.size+column] = dark
	s.reserved[row*s.size+column] = true
}

func (s *symbol) at(row, column int) bool {
	if row < 0 || column < 0 || row >= s.size || column >= s.size {
		return false
	}
	return s.dark[row*s.size+column]
}

func (s *symbol) fixed(row, column int) bool {
	if row < 0 || column < 0 || row >= s.size || column >= s.size {
		return true
	}
	return s.reserved[row*s.size+column]
}

// patterns draws the finders, the timing lines, the alignment patterns, the
// dark module and the version information.
func (s *symbol) patterns(version int) {
	s.finder(0, 0)
	s.finder(0, s.size-7)
	s.finder(s.size-7, 0)
	for i := 8; i < s.size-8; i++ {
		s.set(6, i, i%2 == 0)
		s.set(i, 6, i%2 == 0)
	}
	for _, row := range alignmentPositions(version, s.size) {
		for _, column := range alignmentPositions(version, s.size) {
			// The ones over a finder are the finder's.
			if (row == 6 && column == 6) || (row == 6 && column == s.size-7) || (row == s.size-7 && column == 6) {
				continue
			}
			s.alignment(row, column)
		}
	}
	s.set(s.size-8, 8, true)
	if version >= 7 {
		s.versionInfo(version)
	}
	s.reserveFormat()
}

// reserveFormat keeps the modules of the format information out of the
// codewords: the mask writes it once the mask itself is chosen.
func (s *symbol) reserveFormat() {
	for i := 0; i <= 5; i++ {
		s.set(i, 8, false)
	}
	s.set(7, 8, false)
	s.set(8, 8, false)
	s.set(8, 7, false)
	for i := 9; i < 15; i++ {
		s.set(8, 14-i, false)
	}
	for i := 0; i < 8; i++ {
		s.set(8, s.size-1-i, false)
	}
	for i := 8; i < 15; i++ {
		s.set(s.size-15+i, 8, false)
	}
}

// finder draws the finder pattern at row, column, with its separator.
func (s *symbol) finder(row, column int) {
	for dr := -1; dr <= 7; dr++ {
		for dc := -1; dc <= 7; dc++ {
			inside := dr >= 0 && dr <= 6 && dc >= 0 && dc <= 6
			ring := dr == 0 || dr == 6 || dc == 0 || dc == 6
			core := dr >= 2 && dr <= 4 && dc >= 2 && dc <= 4
			s.set(row+dr, column+dc, inside && (ring || core))
		}
	}
}

// alignment draws the alignment pattern at row, column.
func (s *symbol) alignment(row, column int) {
	for dr := -2; dr <= 2; dr++ {
		for dc := -2; dc <= 2; dc++ {
			s.set(row+dr, column+dc, max(abs(dr), abs(dc)) != 1)
		}
	}
}

// versionInfo writes the version information of a code of version 7 and up.
func (s *symbol) versionInfo(version int) {
	bits := versionBits(version)
	for i := 0; i < 18; i++ {
		dark := bits>>i&1 == 1
		a := s.size - 11 + i%3
		b := i / 3
		s.set(b, a, dark)
		s.set(a, b, dark)
	}
}

// codewords places the codeword bits in the zigzag of a code, and leaves the
// remainder bits light.
func (s *symbol) codewords(codewords []byte, remainder int) {
	bit := 0
	for right := s.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vert := 0; vert < s.size; vert++ {
			for j := 0; j < 2; j++ {
				column := right - j
				row := vert
				if upward {
					row = s.size - 1 - vert
				}
				if s.fixed(row, column) || bit >= len(codewords)*8 {
					continue
				}
				s.dark[row*s.size+column] = codewords[bit/8]>>(7-bit%8)&1 == 1
				bit++
			}
		}
	}
}

// mask applies the mask to the modules the patterns do not hold.
func (s *symbol) mask(mask int) {
	for row := 0; row < s.size; row++ {
		for column := 0; column < s.size; column++ {
			if s.fixed(row, column) {
				continue
			}
			if maskAt(mask, row, column) {
				s.dark[row*s.size+column] = !s.dark[row*s.size+column]
			}
		}
	}
}

// maskAt reports whether the mask turns the module in row and column.
func maskAt(mask, row, column int) bool {
	switch mask {
	case 0:
		return (row+column)%2 == 0
	case 1:
		return row%2 == 0
	case 2:
		return column%3 == 0
	case 3:
		return (row+column)%3 == 0
	case 4:
		return (row/2+column/3)%2 == 0
	case 5:
		return (row*column)%2+(row*column)%3 == 0
	case 6:
		return ((row*column)%2+(row*column)%3)%2 == 0
	case 7:
		return ((row+column)%2+(row*column)%3)%2 == 0
	}
	return false
}

// format writes the format information of the level and the mask.
func (s *symbol) format(level Level, mask int) {
	bits := formatBits(level, mask)
	for i := 0; i <= 5; i++ {
		s.set(i, 8, bits>>i&1 == 1)
	}
	s.set(7, 8, bits>>6&1 == 1)
	s.set(8, 8, bits>>7&1 == 1)
	s.set(8, 7, bits>>8&1 == 1)
	for i := 9; i < 15; i++ {
		s.set(8, 14-i, bits>>i&1 == 1)
	}
	for i := 0; i < 8; i++ {
		s.set(8, s.size-1-i, bits>>i&1 == 1)
	}
	for i := 8; i < 15; i++ {
		s.set(s.size-15+i, 8, bits>>i&1 == 1)
	}
	s.set(8, s.size-8, true)
}

// formatBits returns the 15 bits of the format information of a level and a
// mask, masked as the specification asks.
func formatBits(level Level, mask int) int {
	data := levelBits[level]<<3 | mask
	remainder := data
	for i := 0; i < 10; i++ {
		remainder = rem(remainder<<1, 0x537)
	}
	return (data<<10 | remainder) ^ 0x5412
}

// versionBits returns the 18 bits of the version information of a version.
func versionBits(version int) int {
	remainder := version
	for i := 0; i < 12; i++ {
		remainder = rem(remainder<<1, 0x1F25)
	}
	return version<<12 | remainder
}

// rem returns the remainder of the binary division of value by generator.
func rem(value, generator int) int {
	degree := 0
	for g := generator; g > 1; g >>= 1 {
		degree++
	}
	for bit := 31; bit >= degree; bit-- {
		if value&(1<<bit) != 0 {
			value ^= generator << (bit - degree)
		}
	}
	return value
}

// alignmentPositions returns the rows, and the columns, the alignment patterns
// of a version stand at.
func alignmentPositions(version, size int) []int {
	if version == 1 {
		return nil
	}
	count := version/7 + 2
	step := (version*4 + count*2 + 1) / (count*2 - 2) * 2
	if version == 32 {
		step = 26
	}
	positions := make([]int, count)
	positions[0] = 6
	for i, at := count-1, size-7; i >= 1; i, at = i-1, at-step {
		positions[i] = at
	}
	return positions
}

// penalty returns how badly the code reads, as the specification scores it:
// the mask with the lowest score is the one a code uses.
func (s *symbol) penalty() int {
	return s.penaltyRuns() + s.penaltyBlocks() + s.penaltyPatterns() + s.penaltyBalance()
}

// penaltyRuns scores runs of five same-colored modules and more.
func (s *symbol) penaltyRuns() int {
	score := 0
	for i := 0; i < s.size; i++ {
		for _, line := range [2]bool{true, false} {
			run := 1
			previous := lineAt(s, i, 0, line)
			for j := 1; j < s.size; j++ {
				dark := lineAt(s, i, j, line)
				if dark == previous {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
				previous = dark
			}
			if run >= 5 {
				score += 3 + run - 5
			}
		}
	}
	return score
}

// lineAt reads the module at position of row i of the code, or of its column
// when column is set.
func lineAt(s *symbol, i, position int, column bool) bool {
	if column {
		return s.at(position, i)
	}
	return s.at(i, position)
}

// penaltyBlocks scores every block of two by two same-colored modules.
func (s *symbol) penaltyBlocks() int {
	score := 0
	for row := 0; row < s.size-1; row++ {
		for column := 0; column < s.size-1; column++ {
			first := s.at(row, column)
			if s.at(row+1, column) == first && s.at(row, column+1) == first && s.at(row+1, column+1) == first {
				score += 3
			}
		}
	}
	return score
}

// penaltyPatterns scores the finder-like patterns that appear where none
// belongs.
func (s *symbol) penaltyPatterns() int {
	patterns := [2][11]bool{
		{true, false, true, true, true, false, true, false, false, false, false},
		{false, false, false, false, true, false, true, true, true, false, true},
	}
	score := 0
	for i := 0; i < s.size; i++ {
		for start := 0; start+11 <= s.size; start++ {
			for _, pattern := range patterns {
				row, column := true, true
				for k := 0; k < 11; k++ {
					if lineAt(s, i, start+k, false) != pattern[k] {
						row = false
					}
					if lineAt(s, i, start+k, true) != pattern[k] {
						column = false
					}
				}
				if row {
					score += 40
				}
				if column {
					score += 40
				}
			}
		}
	}
	return score
}

// penaltyBalance scores how far the dark modules are from half of the code.
func (s *symbol) penaltyBalance() int {
	dark := 0
	for _, module := range s.dark {
		if module {
			dark++
		}
	}
	total := s.size * s.size
	steps := (abs(dark*20-total*10)+total-1)/total - 1
	return steps * 10
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
