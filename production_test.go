package bifrost

import (
	"context"
	"errors"
	"testing"
	"time"
)

type readinessRenderer struct{ err error }

func (*readinessRenderer) Render(context.Context, renderRequest, renderSink) error { return nil }
func (*readinessRenderer) Close(context.Context) error                             { return nil }
func (r *readinessRenderer) Ready(context.Context) error                           { return r.err }

func TestAppReady(t *testing.T) {
	want := errors.New("not ready")
	app := &App{runtime: &runtimeState{renderer: &readinessRenderer{err: want}}}
	if err := app.Ready(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Ready error = %v", err)
	}
	app.runtime.renderer = nil
	if err := app.Ready(context.Background()); err != nil {
		t.Fatalf("static app readiness = %v", err)
	}
	if err := (&App{}).Ready(context.Background()); err == nil {
		t.Fatal("app without runtime reported ready")
	}
}

func TestRendererQueueHookReportsOverload(t *testing.T) {
	var event QueueEvent
	renderer := &productionRenderer{
		admission: make(chan struct{}),
		idle:      make(chan *rendererWorker),
		queueHooks: []QueueHook{func(_ context.Context, received QueueEvent) {
			event = received
		}},
	}
	err := renderer.Render(context.Background(), renderRequest{Pattern: "/busy"}, nil)
	if !errors.Is(err, ErrRendererBusy) || !errors.Is(event.Err, ErrRendererBusy) || event.Pattern != "/busy" {
		t.Fatalf("render error = %v, event = %+v", err, event)
	}
}

func TestRendererCloseWaitsForActiveWork(t *testing.T) {
	renderer := &productionRenderer{}
	renderer.active.Add(1)
	done := make(chan error, 1)
	go func() { done <- renderer.Close(context.Background()) }()
	select {
	case err := <-done:
		t.Fatalf("Close returned before drain: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	renderer.active.Done()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after drain")
	}
}

func TestRendererRestartRateLimit(t *testing.T) {
	worker := &rendererWorker{}
	now := time.Unix(1_000, 0)
	for attempt := range 5 {
		if !worker.allowRestart(now.Add(time.Duration(attempt)*time.Second), nil) {
			t.Fatalf("attempt %d was rejected", attempt+1)
		}
	}
	if worker.allowRestart(now.Add(5*time.Second), nil) {
		t.Fatal("sixth restart in one minute was allowed")
	}
	if !worker.allowRestart(now.Add(61*time.Second), nil) {
		t.Fatal("restart limit did not reset after one minute")
	}
}
