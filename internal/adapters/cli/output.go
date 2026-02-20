package cli

import (
	"fmt"
	"os"
)

type Output struct{}

func NewOutput() *Output {
	return &Output{}
}

func (o *Output) PrintHeader(msg string) {
	fmt.Println(msg)
	fmt.Println()
}

func (o *Output) PrintStep(emoji, msg string, args ...any) {
	fmt.Printf("  "+msg+"\n", args...)
}

func (o *Output) PrintSuccess(msg string, args ...any) {
	fmt.Printf("  "+msg+"\n", args...)
}

func (o *Output) PrintWarning(msg string, args ...any) {
	fmt.Printf("  "+msg+"\n", args...)
}

func (o *Output) PrintError(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "  "+msg+"\n", args...)
}

func (o *Output) PrintFile(path string) {
	fmt.Printf("    %s\n", path)
}

func (o *Output) PrintDone(msg string) {
	fmt.Println(msg)
}
