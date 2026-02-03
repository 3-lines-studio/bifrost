package bifrost

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
	EmojiCheck   = "✓"
	EmojiCross   = "✗"
	EmojiWarning = "⚠"
	EmojiInfo    = "ℹ"
	EmojiRocket  = "🚀"
	EmojiFolder  = "📁"
	EmojiFile    = "📝"
	EmojiZap     = "⚡"
	EmojiSearch  = "🔍"
	EmojiPackage = "📦"
	EmojiCopy    = "📂"
	EmojiSparkle = "✨"
	EmojiGear    = "⚙"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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

func PrintSuccess(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s%s%s\n", ColorGreen+EmojiCheck+ColorReset, ColorGreen, message, ColorReset)
}

func PrintError(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s %s%s%s\n", ColorRed+EmojiCross+ColorReset, ColorRed, message, ColorReset)
}

func PrintWarning(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s%s%s\n", ColorYellow+EmojiWarning+ColorReset, ColorYellow, message, ColorReset)
}

func PrintInfo(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", ColorBlue+EmojiInfo+ColorReset, message)
}

func PrintHeader(title string) {
	fmt.Println()
	fmt.Printf("%s%s %s %s\n", ColorBold+ColorPurple, EmojiRocket, title, ColorReset)
	fmt.Printf("%s%s%s\n", ColorGray, strings.Repeat("─", len(title)+4), ColorReset)
}

func PrintStep(emoji, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Printf("  %s %s\n", emoji, message)
}

func PrintDone(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Printf("\n%s %s%s%s\n", ColorGreen+EmojiSparkle+ColorReset, ColorGreen+ColorBold, message, ColorReset)
}

func PrintFile(path string) {
	fmt.Printf("    %s %s%s%s\n", ColorGray+"│"+ColorReset, ColorCyan, path, ColorReset)
}
