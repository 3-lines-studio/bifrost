package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type knobConfig struct {
	Workers     string `json:"workers,omitempty"`
	GCThreshold string `json:"gc_threshold,omitempty"`
	GCInterval  string `json:"gc_interval,omitempty"`
}

func (k knobConfig) label() string {
	w, t, i := k.Workers, k.GCThreshold, k.GCInterval
	if w == "" {
		w = "default"
	}
	if t == "" {
		t = "default"
	}
	if i == "" {
		i = "default(25)"
	}
	return fmt.Sprintf("workers=%s gcThr=%s gcInt=%s", w, t, i)
}

func (k knobConfig) env() []string {
	keys := []string{"BIFROST_QUICKJS_WORKERS", "BIFROST_QUICKJS_GC_THRESHOLD", "BIFROST_QUICKJS_GC_INTERVAL"}
	out := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		inherited := false
		for _, key := range keys {
			if name == key {
				inherited = true
				break
			}
		}
		if !inherited {
			out = append(out, entry)
		}
	}
	if k.Workers != "" {
		out = append(out, "BIFROST_QUICKJS_WORKERS="+k.Workers)
	}
	if k.GCThreshold != "" {
		out = append(out, "BIFROST_QUICKJS_GC_THRESHOLD="+k.GCThreshold)
	}
	if k.GCInterval != "" {
		out = append(out, "BIFROST_QUICKJS_GC_INTERVAL="+k.GCInterval)
	}
	return out
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

type serverProc struct {
	cmd     *exec.Cmd
	port    int
	logs    bytes.Buffer
	rssMax  atomic.Int64
	rssStop chan struct{}
	rssDone chan struct{}
}

func sampleRSS(pid int) (int64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				return kb, err == nil
			}
		}
	}
	return 0, false
}

func (s *serverProc) watchRSS() {
	s.rssStop = make(chan struct{})
	s.rssDone = make(chan struct{})
	go func() {
		defer close(s.rssDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.rssStop:
				return
			case <-ticker.C:
				if s.cmd.Process == nil {
					continue
				}
				if kb, ok := sampleRSS(s.cmd.Process.Pid); ok {
					for {
						current := s.rssMax.Load()
						if kb <= current || s.rssMax.CompareAndSwap(current, kb) {
							break
						}
					}
				}
			}
		}
	}()
}

func startServer(bin string, port int, cfg knobConfig) (*serverProc, error) {
	sp := &serverProc{port: port}
	sp.cmd = exec.Command(bin, "-port", strconv.Itoa(port))
	sp.cmd.Env = cfg.env()
	sp.cmd.Stdout = &sp.logs
	sp.cmd.Stderr = &sp.logs
	if err := sp.cmd.Start(); err != nil {
		return nil, err
	}
	sp.watchRSS()
	base := fmt.Sprintf("http://localhost:%d/healthz", port)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return sp, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	sp.stop()
	return nil, fmt.Errorf("server did not become ready; logs:\n%s", sp.logs.String())
}

func (s *serverProc) stop() {
	if s.rssStop != nil {
		close(s.rssStop)
		<-s.rssDone
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
	done := make(chan struct{})
	go func() {
		_ = s.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()
		<-done
	}
}
