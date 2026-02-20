package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
	ColorBold   = "\033[1m"
)

const (
	EmojiCheck   = "✅"
	EmojiCross   = "❌"
	EmojiWarning = "⚠️"
	EmojiInfo    = "ℹ️"
	EmojiRocket  = "🚀"
	EmojiFolder  = "📁"
	EmojiFile    = "📝"
	EmojiZap     = "⚡"
	EmojiSearch  = "🔍"
	EmojiPackage = "📦"
	EmojiCopy    = "📂"
	EmojiSparkle = "✨"
	EmojiGear    = "⚙️"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Output struct{}

func NewOutput() *Output {
	return &Output{}
}

func (o *Output) PrintHeader(msg string) {
	fmt.Println()
	fmt.Printf("%s%s %s %s\n", ColorBold+ColorPurple, EmojiRocket, msg, ColorReset)
	fmt.Printf("%s%s%s\n", ColorGray, strings.Repeat("─", len(msg)+4), ColorReset)
}

func (o *Output) PrintStep(emoji, msg string, args ...any) {
	message := fmt.Sprintf(msg, args...)
	fmt.Printf("  %s %s\n", emoji, message)
}

func (o *Output) PrintSuccess(msg string, args ...any) {
	message := fmt.Sprintf(msg, args...)
	fmt.Printf("%s %s%s%s\n", ColorGreen+EmojiCheck+ColorReset, ColorGreen, message, ColorReset)
}

func (o *Output) PrintWarning(msg string, args ...any) {
	message := fmt.Sprintf(msg, args...)
	fmt.Printf("%s %s%s%s\n", ColorYellow+EmojiWarning+ColorReset, ColorYellow, message, ColorReset)
}

func (o *Output) PrintError(msg string, args ...any) {
	message := fmt.Sprintf(msg, args...)
	fmt.Fprintf(os.Stderr, "%s %s%s%s\n", ColorRed+EmojiCross+ColorReset, ColorRed, message, ColorReset)
}

func (o *Output) PrintFile(path string) {
	fmt.Printf("    %s %s%s%s\n", ColorGray+"│"+ColorReset, ColorCyan, path, ColorReset)
}

func (o *Output) PrintDone(msg string) {
	fmt.Printf("\n%s %s%s%s\n", ColorGreen+EmojiSparkle+ColorReset, ColorGreen+ColorBold, msg, ColorReset)
}

type Spinner struct {
	message string
	stop    chan bool
	wg      sync.WaitGroup
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		stop:    make(chan bool),
	}
}

func (s *Spinner) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		frameIdx := 0
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				frame := spinnerFrames[frameIdx%len(spinnerFrames)]
				fmt.Printf("\r%s %s %s", ColorCyan+frame+ColorReset, s.message, ColorGray+"..."+ColorReset)
				frameIdx++
			case <-s.stop:
				fmt.Print("\r" + strings.Repeat(" ", len(s.message)+10) + "\r")
				return
			}
		}
	}()
}

func (s *Spinner) Stop() {
	close(s.stop)
	s.wg.Wait()
}
