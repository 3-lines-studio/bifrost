package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type navMetrics struct {
	TTFB     float64 `json:"ttfb_ms"`
	DCL      float64 `json:"dcl_ms"`
	Load     float64 `json:"load_ms"`
	LCP      float64 `json:"lcp_ms"`
	RenderMS float64 `json:"render_ms"`
	Transfer int64   `json:"transfer_bytes"`
	DocBytes int64   `json:"doc_bytes"`
}

func (m navMetrics) valid() bool { return m.TTFB > 0 }

const navTimingJS = `() => new Promise((resolve) => {
	const nav = performance.getEntriesByType("navigation")[0] || {};
	const f = (v) => (v > 0 ? v : null);
	const out = {
		ttfb_ms: f(nav.responseStart - nav.requestStart),
		dcl_ms: f(nav.domContentLoadedEventEnd - nav.requestStart),
		load_ms: f(nav.loadEventEnd - nav.requestStart),
		transfer_bytes: nav.transferSize ?? 0,
		doc_bytes: nav.decodedBodySize ?? 0,
		lcp_ms: null,
	};
	const obs = new PerformanceObserver((list) => {
		const entries = list.getEntries();
		obs.disconnect();
		if (entries.length) out.lcp_ms = entries[entries.length - 1].startTime;
		resolve(out);
	});
	obs.observe({ type: "largest-contentful-paint", buffered: true });
	setTimeout(() => { obs.disconnect(); resolve(out); }, 1500);
})`

type throttleSpec struct {
	latency float64
	down    float64
	up      float64
}

var fast4G = throttleSpec{
	latency: 150,
	down:    1.16 * 1024 * 1024,
	up:      0.40 * 1024 * 1024,
}

type harness struct {
	browser *rod.Browser
	baseURL string
}

func newHarness(browserBin string, port int, headed bool) (*harness, error) {
	l := launcher.New().
		Bin(browserBin).
		Headless(!headed).
		Set("disable-dev-shm-usage", "true")
	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	return &harness{browser: browser, baseURL: fmt.Sprintf("http://localhost:%d", port)}, nil
}

func (h *harness) close() {
	_ = h.browser.Close()
}

func (h *harness) newPage() (*rod.Page, error) {
	incog, err := h.browser.Incognito()
	if err != nil {
		return nil, err
	}
	page, err := incog.Page(proto.TargetCreateTarget{})
	if err != nil {
		_ = incog.Close()
		return nil, err
	}
	return page, nil
}

func measureOnce(p *rod.Page, url string, throttle *throttleSpec) (navMetrics, error) {
	page := p.Timeout(60 * time.Second)
	if throttle != nil {
		if err := (proto.NetworkEmulateNetworkConditions{
			Offline:            false,
			Latency:            throttle.latency,
			DownloadThroughput: throttle.down,
			UploadThroughput:   throttle.up,
		}).Call(page); err != nil {
			return navMetrics{}, fmt.Errorf("emulate network: %w", err)
		}
	}
	captured := make(chan float64, 1)
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		return navMetrics{}, fmt.Errorf("enable network: %w", err)
	}
	go page.EachEvent(func(e *proto.NetworkResponseReceived) bool {
		if e.Type != proto.NetworkResourceTypeDocument {
			return false
		}
		for key, value := range e.Response.Headers {
			if strings.EqualFold(key, "X-Bifrost-Render-Ms") {
				ms, _ := strconv.ParseFloat(value.Str(), 64)
				select {
				case captured <- ms:
				default:
				}
				break
			}
		}
		return true
	})()

	wait := page.WaitNavigation(proto.PageLifecycleEventNameLoad)
	navErr := page.Navigate(url)
	wait()
	if navErr != nil {
		return navMetrics{}, navErr
	}

	obj, err := page.Eval(navTimingJS)
	if err != nil {
		return navMetrics{}, fmt.Errorf("read nav timing: %w", err)
	}
	var m navMetrics
	if err := json.Unmarshal([]byte(obj.Value.JSON("", "")), &m); err != nil {
		return navMetrics{}, fmt.Errorf("decode nav timing: %w", err)
	}
	select {
	case m.RenderMS = <-captured:
	case <-time.After(time.Second):
	}
	return m, nil
}

type scenario struct {
	name        string
	samples     int
	concurrency int
	rounds      int
	throttle    *throttleSpec
	urlPath     string
	reuse       bool
}

func (h *harness) runScenario(s scenario) ([]navMetrics, error) {
	var out []navMetrics
	url := h.baseURL + s.urlPath

	if s.concurrency == 1 {
		if s.reuse {
			page, err := h.newPage()
			if err != nil {
				return out, err
			}
			defer page.Close()
			for range s.samples {
				m, err := measureOnce(page, url, s.throttle)
				if err != nil {
					return out, err
				}
				out = append(out, m)
			}
			return out, nil
		}
		for range s.samples {
			page, err := h.newPage()
			if err != nil {
				return out, err
			}
			m, err := measureOnce(page, url, s.throttle)
			_ = page.Close()
			if err != nil {
				return out, err
			}
			out = append(out, m)
		}
		return out, nil
	}

	incog, err := h.browser.Incognito()
	if err != nil {
		return out, err
	}
	defer incog.Close()
	pages := make([]*rod.Page, s.concurrency)
	for i := range pages {
		pages[i], err = incog.Page(proto.TargetCreateTarget{})
		if err != nil {
			return out, err
		}
	}
	for range s.rounds {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var firstErr error
		for _, p := range pages {
			wg.Add(1)
			go func(p *rod.Page) {
				defer wg.Done()
				m, err := measureOnce(p, url, s.throttle)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					return
				}
				out = append(out, m)
			}(p)
		}
		wg.Wait()
		if firstErr != nil {
			return out, firstErr
		}
	}
	return out, nil
}

type stats struct {
	P50  float64
	P95  float64
	Mean float64
	N    int
}

func column(samples []navMetrics, pick func(navMetrics) float64) []float64 {
	var values []float64
	for _, m := range samples {
		if !m.valid() {
			continue
		}
		if v := pick(m); v > 0 {
			values = append(values, v)
		}
	}
	return values
}

func aggregate(samples []navMetrics, pick func(navMetrics) float64) stats {
	values := column(samples, pick)
	if len(values) == 0 {
		return stats{}
	}
	sort.Float64s(values)
	p := func(percent float64) float64 {
		idx := int(math.Ceil(percent/100*float64(len(values)))) - 1
		if idx < 0 {
			idx = 0
		}
		return values[idx]
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return stats{P50: p(50), P95: p(95), Mean: sum / float64(len(values)), N: len(values)}
}

func (s stats) fmt() string {
	if s.N == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f/%.0f", s.P50, s.P95)
}
