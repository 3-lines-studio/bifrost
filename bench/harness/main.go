package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"text/tabwriter"
	"time"
)

func benchDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

func ensureBuilt() error {
	dir := benchDir()
	if _, err := os.Stat(filepath.Join(dir, "bin", "bifrost-cli")); err != nil {
		repoRoot := filepath.Dir(dir)
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "bin", "bifrost-cli"), "./cmd/bifrost")
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build bifrost cli: %w\n%s", err, out)
		}
	}
	// Assets are rebuilt on every run: the page fixture or the library may
	// have changed since the last build, and stale bundles silently
	// invalidate measurements.
	cmd := exec.Command("./bin/bifrost-cli", "build", "./main.go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build bench assets: %w\n%s", err, out)
	}
	buildServer := exec.Command("go", "build", "-o", "bin/server", ".")
	buildServer.Dir = dir
	if out, err := buildServer.CombinedOutput(); err != nil {
		return fmt.Errorf("build bench server: %w\n%s", err, out)
	}
	return nil
}

func sweepConfigs() []knobConfig {
	var out []knobConfig
	out = append(out, knobConfig{})
	for _, w := range []string{"1", "2", "4", "6", "8", "12", "16"} {
		out = append(out, knobConfig{Workers: w})
	}
	for _, t := range []int64{4 << 20, 8 << 20, 16 << 20, 32 << 20, 64 << 20} {
		// Threshold rows must pin interval to 0: cadence mode is the default
		// and ignores the threshold below the safety floor.
		out = append(out, knobConfig{GCThreshold: strconv.FormatInt(t, 10), GCInterval: "0"})
	}
	for _, i := range []string{"0", "1", "5", "10", "25", "50"} {
		out = append(out, knobConfig{GCInterval: i})
	}
	return out
}

type configResult struct {
	Config    knobConfig              `json:"config"`
	Scenarios map[string][]navMetrics `json:"scenarios"`
	RSSMaxKB  int64                   `json:"rss_max_kb"`
}

type runResult struct {
	Generated string         `json:"generated"`
	CPU       string         `json:"cpu"`
	Browser   string         `json:"browser"`
	Rows      int            `json:"rows"`
	Cols      int            `json:"cols"`
	LatencyMS int            `json:"loader_latency_ms"`
	Configs   []configResult `json:"configs"`
}

func main() {
	defaultBrowser := os.Getenv("BIFROST_BENCH_BROWSER")
	if defaultBrowser == "" {
		defaultBrowser = "/usr/bin/chromium"
	}
	sweep := flag.Bool("sweep", false, "run the knob sweep")
	throttled := flag.Bool("throttled", true, "spot-check fast-4g latency on baseline and best config")
	browserFlag := flag.String("browser", defaultBrowser, "chromium binary path")
	rows := flag.Int("rows", 2000, "bench page row count")
	cols := flag.Int("cols", 20, "bench page columns")
	latency := flag.Int("latency", 50, "loader latency ms")
	samples := flag.Int("samples", 5, "solo scenario samples")
	burstConcurrency := flag.Int("burst", 8, "burst concurrency")
	burstRounds := flag.Int("rounds", 2, "burst rounds")
	workers := flag.String("workers", "", "BIFROST_QUICKJS_WORKERS for the single config")
	gcThreshold := flag.Int64("gc-threshold", 0, "BIFROST_QUICKJS_GC_THRESHOLD bytes (0 = default)")
	gcInterval := flag.Int("gc-interval", 0, "BIFROST_QUICKJS_GC_INTERVAL renders (0 = threshold mode)")
	soak := flag.Int("soak", 0, "run only a soak of N sequential renders on one page (default GC decision scenario)")
	sweepSoak := flag.Int("sweep-soak", 30, "soak samples per config in sweep mode (0 = off)")
	headed := flag.Bool("headed", false, "run the browser headed (needs a display)")
	flag.Parse()

	if err := ensureBuilt(); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}

	port, err := freePort()
	if err != nil {
		fmt.Fprintln(os.Stderr, "port:", err)
		os.Exit(1)
	}
	browserBin := *browserFlag

	configs := []knobConfig{{}}
	if *sweep {
		configs = sweepConfigs()
	} else {
		set := map[string]bool{}
		flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if set["workers"] || set["gc-threshold"] || set["gc-interval"] {
			cfg := knobConfig{}
			if set["workers"] {
				cfg.Workers = *workers
			}
			if set["gc-interval"] {
				cfg.GCInterval = strconv.Itoa(*gcInterval)
			}
			if set["gc-threshold"] {
				cfg.GCThreshold = strconv.FormatInt(*gcThreshold, 10)
				// The threshold only takes effect in threshold mode.
				if !set["gc-interval"] {
					cfg.GCInterval = "0"
				}
			}
			configs = []knobConfig{cfg}
		}
	}

	results := runResult{
		Generated: time.Now().Format(time.RFC3339),
		CPU:       runtime.GOARCH + " " + runtime.GOOS,
		Browser:   browserBin,
		Rows:      *rows,
		Cols:      *cols,
		LatencyMS: *latency,
	}

	serverBin := filepath.Join(benchDir(), "bin", "server")

	runOne := func(cfg knobConfig) (configResult, error) {
		sp, err := startServer(serverBin, port, cfg)
		if err != nil {
			return configResult{}, err
		}
		defer sp.stop()
		h, err := newHarness(browserBin, port, *headed)
		if err != nil {
			return configResult{}, err
		}
		defer h.close()

		cr := configResult{Config: cfg, Scenarios: map[string][]navMetrics{}}
		scenarios := []scenario{
			{name: "solo", samples: *samples, concurrency: 1, urlPath: "/heavy"},
			{name: "burst", concurrency: *burstConcurrency, rounds: *burstRounds, urlPath: "/heavy"},
		}
		if *soak > 0 {
			scenarios = []scenario{
				{name: "soak", samples: *soak, concurrency: 1, reuse: true, urlPath: "/heavy"},
			}
		} else if *sweep && *sweepSoak > 0 {
			scenarios = append(scenarios, scenario{
				name: "soak", samples: *sweepSoak, concurrency: 1, reuse: true, urlPath: "/heavy",
			})
		}
		for _, s := range scenarios {
			metrics, err := h.runScenario(s)
			if err != nil {
				return configResult{}, fmt.Errorf("scenario %s: %w", s.name, err)
			}
			cr.Scenarios[s.name] = metrics
		}
		cr.RSSMaxKB = sp.rssMax.Load()
		return cr, nil
	}

	fmt.Printf("bifrost browser bench  cpu=%s browser=%s rows=%d cols=%d latency=%dms\n",
		runtime.GOARCH, browserBin, *rows, *cols, *latency)
	fmt.Println()

	tab := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tab, "config\t solo.ttfb p50/p95\t solo.render p50/p95\t burst.ttfb p50/p95\t burst.render p50/p95\t soak.render p50/p95\t rss(MB)\t")

	for _, cfg := range configs {
		cr, err := runOne(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config %s: %v\n", cfg.label(), err)
			continue
		}
		results.Configs = append(results.Configs, cr)
		solo := aggregate(cr.Scenarios["solo"], func(m navMetrics) float64 { return m.TTFB })
		soloRender := aggregate(cr.Scenarios["solo"], func(m navMetrics) float64 { return m.RenderMS })
		burst := aggregate(cr.Scenarios["burst"], func(m navMetrics) float64 { return m.TTFB })
		burstRender := aggregate(cr.Scenarios["burst"], func(m navMetrics) float64 { return m.RenderMS })
		if *soak > 0 {
			soakStats := aggregate(cr.Scenarios["soak"], func(m navMetrics) float64 { return m.RenderMS })
			soakTTFB := aggregate(cr.Scenarios["soak"], func(m navMetrics) float64 { return m.TTFB })
			fmt.Printf("%s: soak render p50/p95 %s ttfb p50/p95 %s rss %d MB\n",
				cfg.label(), soakStats.fmt(), soakTTFB.fmt(), cr.RSSMaxKB/1024)
			continue
		}
		fmt.Fprintf(tab, "%s\t %s\t %s\t %s\t %s\t %s\t %d\t\n",
			cfg.label(), solo.fmt(), soloRender.fmt(), burst.fmt(), burstRender.fmt(),
			aggregate(cr.Scenarios["soak"], func(m navMetrics) float64 { return m.RenderMS }).fmt(),
			cr.RSSMaxKB/1024)
	}
	tab.Flush()

	if *throttled && len(results.Configs) >= 1 {
		bestIdx := pickBestIndex(results.Configs)
		indexes := []int{0}
		if bestIdx != 0 {
			indexes = append(indexes, bestIdx)
		}
		fmt.Println()
		fmt.Println("throttled (fast 4g: 150ms rtt, 9.3mbps): ttfb/load p50/p95")
		for _, i := range indexes {
			cfg := results.Configs[i].Config
			sp, err := startServer(serverBin, port, cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "config %s: %v\n", cfg.label(), err)
				continue
			}
			h, err := newHarness(browserBin, port, *headed)
			if err != nil {
				sp.stop()
				fmt.Fprintf(os.Stderr, "config %s: %v\n", cfg.label(), err)
				continue
			}
			metrics, err := h.runScenario(scenario{
				name:        "throttled",
				samples:     3,
				concurrency: 1,
				throttle:    &fast4G,
				urlPath:     "/heavy",
			})
			h.close()
			sp.stop()
			if err != nil {
				fmt.Fprintf(os.Stderr, "config %s throttled: %v\n", cfg.label(), err)
				continue
			}
			results.Configs[i].Scenarios["throttled"] = metrics
			if sp.rssMax.Load() > results.Configs[i].RSSMaxKB {
				results.Configs[i].RSSMaxKB = sp.rssMax.Load()
			}
			ttfb := aggregate(metrics, func(m navMetrics) float64 { return m.TTFB })
			load := aggregate(metrics, func(m navMetrics) float64 { return m.Load })
			fmt.Printf("%s: ttfb %s load %s\n", cfg.label(), ttfb.fmt(), load.fmt())
		}
	}

	fmt.Println()
	if len(results.Configs) > 0 {
		best := pickBestIndex(results.Configs)
		fmt.Printf("recommended: %s (lowest burst p95 ttfb)\n", results.Configs[best].Config.label())
	}

	resultsDir := filepath.Join(benchDir(), "..", "results")
	if err := os.MkdirAll(resultsDir, 0o755); err == nil {
		path := filepath.Join(resultsDir, "browser-"+time.Now().Format("20060102-150405")+".json")
		data, err := json.MarshalIndent(results, "", "  ")
		if err == nil {
			if err := os.WriteFile(path, data, 0o644); err == nil {
				fmt.Printf("results written to %s\n", path)
			}
		}
	}
}

func pickBestIndex(configs []configResult) int {
	rank := func(cr configResult) (float64, float64) {
		burst := aggregate(cr.Scenarios["burst"], func(m navMetrics) float64 { return m.TTFB })
		solo := aggregate(cr.Scenarios["solo"], func(m navMetrics) float64 { return m.TTFB })
		return burst.P95, solo.P95
	}
	best := 0
	bestBurst, bestSolo := rank(configs[0])
	for i, cr := range configs[1:] {
		b, s := rank(cr)
		if b < bestBurst || (b == bestBurst && s < bestSolo) {
			best, bestBurst, bestSolo = i+1, b, s
		}
	}
	return best
}
