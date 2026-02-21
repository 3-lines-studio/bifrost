package cli

import (
	"fmt"
	"os"
	"time"
)

type BuildStep struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Success   bool
	Error     string
}

type cliOutputWithColors interface {
	Green(text string) string
	Yellow(text string) string
	Red(text string) string
	Gray(text string) string
}

type BuildError struct {
	Page    string
	Message string
	Details []string
}

type BuildReport struct {
	colors      cliOutputWithColors
	steps       []BuildStep
	warnings    []BuildError
	errors      []BuildError
	startTime   time.Time
	pageCount   int
	outputDir   string
	hasFailures bool
}

func NewBuildReport(colors cliOutputWithColors, outputDir string) *BuildReport {
	return &BuildReport{
		colors:    colors,
		steps:     make([]BuildStep, 0),
		warnings:  make([]BuildError, 0),
		errors:    make([]BuildError, 0),
		startTime: time.Now(),
		outputDir: outputDir,
	}
}

func (r *BuildReport) SetPageCount(count int) {
	r.pageCount = count
}

func (r *BuildReport) StartStep(name string) *BuildStep {
	step := BuildStep{
		Name:      name,
		StartTime: time.Now(),
	}
	r.steps = append(r.steps, step)
	return &r.steps[len(r.steps)-1]
}

func (r *BuildReport) EndStep(step *BuildStep, success bool, err string) {
	step.EndTime = time.Now()
	step.Success = success
	step.Error = err
	if !success {
		r.hasFailures = true
	}
}

func (r *BuildReport) AddWarning(page string, message string, details []string) {
	r.warnings = append(r.warnings, BuildError{
		Page:    page,
		Message: message,
		Details: details,
	})
}

func (r *BuildReport) AddError(page string, message string, details []string) {
	r.errors = append(r.errors, BuildError{
		Page:    page,
		Message: message,
		Details: details,
	})
	r.hasFailures = true
}

func (r *BuildReport) Render() {
	duration := time.Since(r.startTime)

	if len(r.errors) == 0 && len(r.warnings) == 0 {
		r.renderMinimal(duration)
	} else {
		r.renderVerbose(duration)
	}
}

func (r *BuildReport) renderMinimal(duration time.Duration) {
	fmt.Printf("  "+r.colors.Green("✓ ")+"%d pages found\n", r.pageCount)

	stepLines := make([]string, 0, len(r.steps))
	allSuccessful := true

	for _, step := range r.steps {
		if !step.Success {
			allSuccessful = false
			stepLines = append(stepLines, "  "+r.colors.Red("✗ ")+step.Name)
		}
	}

	if allSuccessful {
		fmt.Printf("  "+r.colors.Green("✓ ")+"Build complete in %s\n", formatDuration(duration))
	} else {
		fmt.Println()
		fmt.Println("Failed steps:")
		for _, line := range stepLines {
			fmt.Println(line)
		}
	}

	if r.outputDir != "" {
		fmt.Printf("\n  %s\n", r.colors.Gray("Output: "+r.outputDir))
	}
}

func (r *BuildReport) renderVerbose(duration time.Duration) {
	fmt.Printf("  %d pages found\n", r.pageCount)

	fmt.Println()
	for _, step := range r.steps {
		status := r.colors.Green("✓")
		if !step.Success {
			status = r.colors.Red("✗")
		}
		fmt.Printf("  %s %s\n", status, step.Name)
	}

	if len(r.errors) > 0 {
		fmt.Println()
		fmt.Fprintf(os.Stderr, "  "+r.colors.Red("✗ ")+"Errors (%d):\n", len(r.errors))
		r.renderErrors(r.errors)
	}

	if len(r.warnings) > 0 {
		fmt.Println()
		fmt.Printf("  "+r.colors.Yellow("⚠ ")+"Warnings (%d):\n", len(r.warnings))
		r.renderErrors(r.warnings)
	}

	fmt.Println()
	if len(r.errors) > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", r.colors.Red(fmt.Sprintf("Build failed after %s", formatDuration(duration))))
	} else {
		fmt.Printf("  "+r.colors.Green("✓ ")+"Build complete in %s\n", formatDuration(duration))
	}

	if r.outputDir != "" {
		fmt.Printf("\n  %s\n", r.colors.Gray("Output: "+r.outputDir))
	}
}

func (r *BuildReport) renderErrors(errors []BuildError) {
	for _, err := range errors {
		fmt.Printf("  %s %s\n", r.colors.Red("✗"), err.Page)
		fmt.Printf("    %s\n", err.Message)

		deduplicated := deduplicateStrings(err.Details)
		for _, detail := range deduplicated {
			fmt.Printf("      • %s\n", detail)
		}
	}
}

func (r *BuildReport) HasFailures() bool {
	return r.hasFailures
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.1fs", float64(d)/float64(time.Second))
}

func deduplicateStrings(items []string) []string {
	if len(items) <= 1 {
		return items
	}

	seen := make(map[string]int)
	for _, item := range items {
		seen[item]++
	}

	result := make([]string, 0, len(seen))
	for item, count := range seen {
		if count > 1 {
			result = append(result, fmt.Sprintf("%s (%d occurrences)", item, count))
		} else {
			result = append(result, item)
		}
	}

	return result
}
