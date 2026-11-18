package qr

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVersionAndSize(t *testing.T) {
	cases := []struct {
		text    string
		level   Level
		version int
		size    int
	}{
		{"0123456789", Low, 1, 21},
		{"0123456789", Medium, 1, 21},
		{"https://picsel.app/e/abc123", Low, 2, 25},
		{"https://picsel.app/e/abc123", High, 4, 33},
		{strings.Repeat("a", 1000), Low, 22, 105},
		{strings.Repeat("a", 1000), High, 36, 161},
	}
	for _, c := range cases {
		code, err := Encode(c.text, c.level)
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		if code.Version() != c.version || code.Size() != c.size {
			t.Fatalf("%q at %v: version %d size %d, want %d and %d", c.text, c.level, code.Version(), code.Size(), c.version, c.size)
		}
		if code.Level() != c.level {
			t.Fatalf("level %v", code.Level())
		}
	}
}

func TestTooLong(t *testing.T) {
	if _, err := Encode(strings.Repeat("a", 3000), High); err != ErrTooLong {
		t.Fatalf("error %v", err)
	}
}

func TestUnknownLevel(t *testing.T) {
	if _, err := Encode("x", High+1); err == nil {
		t.Fatal("an unknown level was taken")
	}
}

func TestFinderPatterns(t *testing.T) {
	code, err := Encode("https://picsel.app", Medium)
	if err != nil {
		t.Fatal(err)
	}
	for _, corner := range [][2]int{{0, 0}, {0, code.Size() - 7}, {code.Size() - 7, 0}} {
		row, column := corner[0], corner[1]
		for dr := 0; dr < 7; dr++ {
			for dc := 0; dc < 7; dc++ {
				ring := dr == 0 || dr == 6 || dc == 0 || dc == 6
				core := dr >= 2 && dr <= 4 && dc >= 2 && dc <= 4
				if code.Dark(row+dr, column+dc) != (ring || core) {
					t.Fatalf("finder at %d,%d is not a finder", row, column)
				}
			}
		}
	}
	for i := 8; i < code.Size()-8; i++ {
		if code.Dark(6, i) != (i%2 == 0) || code.Dark(i, 6) != (i%2 == 0) {
			t.Fatalf("timing line broken at %d", i)
		}
	}
	if !code.Dark(code.Size()-8, 8) {
		t.Fatal("the dark module is light")
	}
	if code.Dark(-1, 0) || code.Dark(0, -1) || code.Dark(code.Size(), 0) {
		t.Fatal("a module outside the code is dark")
	}
}

func TestDarkModuleIsNotTheSameEveryTime(t *testing.T) {
	first, err := Encode("https://picsel.app/e/abc123", Medium)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode("https://picsel.app/e/abc124", Medium)
	if err != nil {
		t.Fatal(err)
	}
	same := true
	for row := 0; row < first.Size() && same; row++ {
		for column := 0; column < first.Size(); column++ {
			if first.Dark(row, column) != second.Dark(row, column) {
				same = false
				break
			}
		}
	}
	if same {
		t.Fatal("two texts made the same code")
	}
}

func TestUnicode(t *testing.T) {
	code, err := Encode("ñandú, ação, 日本語", Quartile)
	if err != nil {
		t.Fatal(err)
	}
	if code.Size() < 21 {
		t.Fatalf("size %d", code.Size())
	}
}

func TestPNG(t *testing.T) {
	code, err := Encode("https://picsel.app", Low)
	if err != nil {
		t.Fatal(err)
	}
	data, err := code.PNG(4)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	want := (code.Size() + 2*Quiet) * 4
	if picture.Bounds().Dx() != want || picture.Bounds().Dy() != want {
		t.Fatalf("png %v, want %d", picture.Bounds(), want)
	}
}

func TestSVG(t *testing.T) {
	code, err := Encode("https://picsel.app", Low)
	if err != nil {
		t.Fatal(err)
	}
	svg := code.SVG()
	side := code.Size() + 2*Quiet
	if !strings.Contains(svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 `+strconv.Itoa(side)+` `+strconv.Itoa(side)+`"`) {
		t.Fatalf("svg %q", svg[:80])
	}
	if strings.Count(svg, "M") != strings.Count(svg, "h1v1h-1z") {
		t.Fatal("svg path is malformed")
	}
}

func TestGolden(t *testing.T) {
	code, err := Encode("https://picsel.app/e/abc123", Medium)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "abc123.txt")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		out.WriteString("version " + strconv.Itoa(code.Version()) + "\n")
		for row := 0; row < code.Size(); row++ {
			for column := 0; column < code.Size(); column++ {
				if code.Dark(row, column) {
					out.WriteString("1")
				} else {
					out.WriteString("0")
				}
			}
			out.WriteString("\n")
		}
		if err := os.WriteFile(path, []byte(out.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	if lines[0] != "version "+strconv.Itoa(code.Version()) {
		t.Fatalf("golden says %q", lines[0])
	}
	for row := 0; row < code.Size(); row++ {
		for column := 0; column < code.Size(); column++ {
			dark := lines[row+1][column] == '1'
			if code.Dark(row, column) != dark {
				t.Fatalf("module %d,%d changed", row, column)
			}
		}
	}
}
