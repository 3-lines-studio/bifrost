//go:build verify

package qr

import (
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// TestAgainstQREncode compares the codes Encode makes with the ones qrencode
// makes: the codewords they place, module by module, and how often they choose
// the same mask. It runs where qrencode is installed:
//
//	go test -tags verify ./qr -run TestAgainstQREncode
func TestAgainstQREncode(t *testing.T) {
	levels := map[Level]string{Low: "L", Medium: "M", Quartile: "Q", High: "H"}
	texts := []string{
		"https://picsel.app/e/abc123",
		"https://picsel.app/user/foto-2024/order/file/01J8Z3K4M5N6P7Q8R9S0T1U2V3",
		"0123456789",
		"HOLA MUNDO",
		strings.Repeat("picsel ", 40),
		strings.Repeat("a", 1000),
		"https://picsel.app/blog/como-vender-fotos-deportivas-sin-perder-calidad?utm_source=whatsapp&utm_medium=share",
		"ñandú, ação, 日本語",
	}
	sameMask, cases := 0, 0
	for _, level := range []Level{Low, Medium, Quartile, High} {
		for _, text := range texts {
			code, err := Encode(text, level)
			if err != nil {
				t.Fatalf("%q at %v: %v", text, level, err)
			}
			reference := qrencode(t, text, levels[level], code.Version())
			mask := formatMask(reference)
			mine, err := encodeData([]byte(text), code.Version(), level)
			if err != nil {
				t.Fatal(err)
			}
			theirs := readCodewords(reference, mask, code.Version())
			words := specs[code.Version()-1].words
			for i := 0; i < words; i++ {
				if mine[i] != theirs[i] {
					t.Fatalf("%q at %v version %d: codeword %d is %02x, qrencode says %02x", text, level, code.Version(), i, mine[i], theirs[i])
				}
			}
			cases++
			if mask == code.mask {
				sameMask++
			}
		}
	}
	t.Logf("%d of %d codes chose the mask qrencode chose", sameMask, cases)
}

// formatMask reads the mask of a code from its format information.
func formatMask(reference [][]bool) int {
	bits := 0
	take := func(i, row, column int) {
		if reference[row][column] {
			bits |= 1 << i
		}
	}
	for i := 0; i <= 5; i++ {
		take(i, i, 8)
	}
	take(6, 7, 8)
	take(7, 8, 8)
	take(8, 8, 7)
	for i := 9; i < 15; i++ {
		take(i, 8, 14-i)
	}
	return ((bits ^ 0x5412) >> 10) & 7
}

// readCodewords walks the zigzag of a code and returns the codewords placed
// there, unmasked with mask.
func readCodewords(reference [][]bool, mask int, version int) []byte {
	s := newSymbol(len(reference))
	s.patterns(version)
	bits := []bool{}
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
				if s.fixed(row, column) {
					continue
				}
				dark := reference[row][column]
				if maskAt(mask, row, column) {
					dark = !dark
				}
				bits = append(bits, dark)
			}
		}
	}
	out := make([]byte, len(bits)/8)
	for i, bit := range bits[:len(out)*8] {
		if bit {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

// qrencode returns the modules of the code qrencode makes of text at level and
// version.
func qrencode(t *testing.T, text, level string, version int) [][]bool {
	t.Helper()
	if _, err := exec.LookPath("qrencode"); err != nil {
		t.Skip("qrencode is not installed")
	}
	path := t.TempDir() + "/code.png"
	command := exec.Command("qrencode", "-o", path, "-l", level, "-v", strconv.Itoa(version), "--strict-version", "-8", "-s", "1", "-m", "0", text)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("qrencode: %v: %s", err, output)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	bounds := image.Bounds()
	modules := make([][]bool, bounds.Dy())
	for row := range modules {
		modules[row] = make([]bool, bounds.Dx())
		for column := range modules[row] {
			red, _, _, _ := image.At(bounds.Min.X+column, bounds.Min.Y+row).RGBA()
			modules[row][column] = red < 0x8000
		}
	}
	if len(modules) != 4*version+17 {
		t.Fatalf("qrencode made %d modules, version %d is %d", len(modules), version, 4*version+17)
	}
	return modules
}

// TestDecodes renders the codes Encode makes and has zbar read them back, as a
// phone reads a code off a screen. It runs where zbarimg is installed:
//
//	go test -tags verify ./qr -run TestDecodes
func TestDecodes(t *testing.T) {
	if _, err := exec.LookPath("zbarimg"); err != nil {
		t.Skip("zbarimg is not installed")
	}
	texts := []string{
		"https://picsel.app/e/abc123",
		"0123456789",
		"https://picsel.app/blog/como-vender-fotos-deportivas?utm_source=whatsapp&utm_medium=share",
		"ñandú, ação, 日本語",
		strings.Repeat("picsel ", 60),
	}
	for _, level := range []Level{Low, Medium, Quartile, High} {
		for _, text := range texts {
			code, err := Encode(text, level)
			if err != nil {
				t.Fatalf("%q: %v", text, err)
			}
			picture, err := code.PNG(4)
			if err != nil {
				t.Fatal(err)
			}
			path := t.TempDir() + "/code.png"
			if err := os.WriteFile(path, picture, 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command("zbarimg", "--raw", "-q", path).Output()
			if err != nil {
				t.Fatalf("%q at %v version %d: zbar: %v", text, level, code.Version(), err)
			}
			if got := strings.TrimSuffix(string(output), "\n"); got != text {
				t.Fatalf("%q at %v: zbar read %q", text, level, got)
			}
		}
	}
}
