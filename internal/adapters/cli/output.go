package cli

import (
	"fmt"
	"os"
)

type Output struct {
	enableColors bool
}

func NewOutput() *Output {
	return &Output{
		enableColors: isTerminal(),
	}
}

func (o *Output) DisableColors() {
	o.enableColors = false
}

func (o *Output) Green(text string) string {
	if !o.enableColors {
		return text
	}
	return "\033[32m" + text + "\033[0m"
}

func (o *Output) Yellow(text string) string {
	if !o.enableColors {
		return text
	}
	return "\033[33m" + text + "\033[0m"
}

func (o *Output) Red(text string) string {
	if !o.enableColors {
		return text
	}
	return "\033[31m" + text + "\033[0m"
}

func (o *Output) Gray(text string) string {
	if !o.enableColors {
		return text
	}
	return "\033[90m" + text + "\033[0m"
}

func (o *Output) PrintHeader(msg string) {
	fmt.Println(msg)
	fmt.Println()
}

func (o *Output) PrintStep(emoji, msg string, args ...any) {
	fmt.Printf("  "+msg+"\n", args...)
}

func (o *Output) PrintSuccess(msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	fmt.Printf("  "+o.Green("✓ ")+"%s\n", formatted)
}

func (o *Output) PrintWarning(msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	fmt.Printf("  "+o.Yellow("⚠ ")+"%s\n", formatted)
}

func (o *Output) PrintError(msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	fmt.Fprintf(os.Stderr, "  "+o.Red("✗ ")+"%s\n", formatted)
}

func (o *Output) PrintFile(path string) {
	fmt.Printf("    %s\n", path)
}

func (o *Output) PrintDone(msg string) {
	fmt.Println(msg)
}

func isTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}
