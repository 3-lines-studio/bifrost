// Package upload sends a file to a server in parts, as S3 and the stores that
// copy it take them: the server signs a URL per part, the app sends the parts
// at once, and the server joins them.
//
// The server is a Signer: an app implements Start, Sign, Complete and Abort on
// top of its own API, and this package does the rest. A file smaller than a
// part goes in one request, signed by Start alone.
//
//	result, err := upload.Run(ctx, file, signer, upload.Options{
//		OnProgress: func(p upload.Progress) {
//			window.Update(func() { screen.show(p.Sent, p.Total) })
//		},
//	})
//
// Run loads and sends on goroutines of its own, and OnProgress runs on them, so
// it is called from a goroutine, never from the one that draws.
package upload

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/3-lines-studio/bifrost/mygo/client"
)

// errStaleUpload is what sending the parts of a session the server forgot
// answers, and what makes Run start a new one.
var errStaleUpload = errors.New("upload: the server forgot the upload")

// File is what Run sends: a file the app opened, read in ranges.
type File interface {
	Name() string
	Size() int64
	ContentType() string
	Blob(start, end int64) client.Blob
}

// Info describes the file to the server, before it sends a byte of it.
type Info struct {
	Name string
	Size int64
	Type string
}

// Plan is what the server answers when an upload starts: the parts it expects,
// or one request for the whole file.
type Plan struct {
	// UploadID names the upload, and its parts. Empty means the file goes in
	// one request, to URL.
	UploadID string
	URL      string
	// PartSize is how many bytes each part holds, when the server wants a size
	// of its own; 0 for the one Options chooses.
	PartSize int64
}

// Part is one part of an upload the server has: its number, from 1, the ETag
// its answer gave, and how many bytes it holds.
type Part struct {
	Number int
	ETag   string
	Size   int64
}

// Signer is the API of the server that takes the upload.
type Signer interface {
	// Start begins an upload of file and answers how it takes it.
	Start(ctx context.Context, file Info) (Plan, error)
	// Sign returns the URL each part is sent to.
	Sign(ctx context.Context, uploadID string, numbers []int) (map[int]string, error)
	// Complete joins the parts the upload holds.
	Complete(ctx context.Context, uploadID string, parts []Part) error
	// Abort drops the parts of an upload nobody finishes, as the ones of one
	// the person canceled.
	Abort(ctx context.Context, uploadID string) error
}

// Progress is how much of a file went out.
type Progress struct {
	// Sent is how many bytes the server received, and Total how many the file
	// holds.
	Sent, Total int64
	// Done is how many parts the server has, and Parts how many it expects.
	Done, Parts int
}

// Retry says how Run sends a part again. Attempts counts the sends of a part,
// the first one included; zero is four. Base and Max bound the wait between
// them, which doubles: 300ms up to 10s when zero.
type Retry struct {
	Attempts int
	Base     time.Duration
	Max      time.Duration
}

// Options say how Run sends a file.
type Options struct {
	// Key names the upload in the Store, and is the name, a colon and the size
	// of the file when empty.
	Key string
	// PartSize is how many bytes each part holds: the server's choice when it
	// made one, this one when it did not, and the one for the size of the file
	// when both are 0.
	PartSize int64
	// Concurrency is how many parts Run sends at once; 6 when zero.
	Concurrency int
	Retry       Retry
	// Store keeps the parts the server already has, so a Run that failed, or a
	// page that reloaded, sends the rest and not the whole file again. Nil
	// sends no resume.
	Store Store
	// OnProgress reports the bytes that arrive at the server. It runs on the
	// goroutines that send the parts.
	OnProgress func(Progress)
	// Send makes each request, as a test does. Nil uses client.Do.
	Send func(ctx context.Context, request client.Request) (client.Response, error)
}

// Result is what a finished upload leaves.
type Result struct {
	UploadID string
	Parts    []Part
	// ETag is the one of the whole file, when it went in one request.
	ETag string
}

// Run sends file with the signer, in parts or in one request, and returns when
// the server has all of it.
func Run(ctx context.Context, file File, signer Signer, options Options) (Result, error) {
	if file == nil {
		return Result{}, errors.New("upload: no file to send")
	}
	if signer == nil {
		return Result{}, errors.New("upload: no signer to ask")
	}
	options = options.withDefault(file)
	state, resuming := options.load(file)
	plan, err := options.start(ctx, file, signer, state, resuming)
	if err != nil {
		return Result{}, err
	}
	if plan.UploadID == "" {
		return options.sendWhole(ctx, file, plan)
	}
	result, err := options.sendParts(ctx, file, signer, plan, state)
	if !errors.Is(err, errStaleUpload) {
		return result, err
	}
	options.Store.Delete(options.Key)
	plan, err = options.start(ctx, file, signer, State{}, false)
	if err != nil {
		return Result{}, err
	}
	if plan.UploadID == "" {
		return options.sendWhole(ctx, file, plan)
	}
	return options.sendParts(ctx, file, signer, plan, State{})
}

// withDefault returns the options with the ones Run fills in.
func (o Options) withDefault(file File) Options {
	if o.Concurrency <= 0 {
		o.Concurrency = 6
	}
	o.Retry = o.Retry.withDefault()
	if o.Key == "" {
		o.Key = file.Name() + ":" + strconv.FormatInt(file.Size(), 10)
	}
	if o.Send == nil {
		o.Send = client.Do
	}
	return o
}

func (r Retry) withDefault() Retry {
	if r.Attempts <= 0 {
		r.Attempts = 4
	}
	if r.Base <= 0 {
		r.Base = 300 * time.Millisecond
	}
	if r.Max <= 0 {
		r.Max = 10 * time.Second
	}
	if r.Max < r.Base {
		r.Max = r.Base
	}
	return r
}

// wait is how long Run waits before sending a part again, after attempt
// failures.
func (r Retry) wait(attempt int) time.Duration {
	delay := r.Base
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= r.Max {
			return r.Max
		}
	}
	return min(delay, r.Max)
}

// load returns the state the store kept of the file, and whether it did.
func (o Options) load(file File) (State, bool) {
	if o.Store == nil {
		return State{}, false
	}
	state, ok := o.Store.Load(o.Key)
	if !ok || state.UploadID == "" || state.Size != file.Size() {
		return State{}, false
	}
	return state, true
}

// start asks the server to begin the upload, with the session the store kept
// when there is one.
func (o Options) start(ctx context.Context, file File, signer Signer, state State, resuming bool) (Plan, error) {
	if resuming {
		return Plan{UploadID: state.UploadID, PartSize: state.PartSize}, nil
	}
	plan, err := signer.Start(ctx, Info{Name: file.Name(), Size: file.Size(), Type: file.ContentType()})
	if err != nil {
		return Plan{}, err
	}
	if plan.PartSize <= 0 {
		plan.PartSize = o.PartSize
	}
	if plan.PartSize <= 0 {
		plan.PartSize = PartSize(file.Size())
	}
	if plan.UploadID != "" && partCount(file.Size(), plan.PartSize) > maxParts {
		return Plan{}, fmt.Errorf("upload: %d bytes in parts of %d is more than %d parts", file.Size(), plan.PartSize, maxParts)
	}
	return plan, nil
}

// sendWhole sends a file the server takes in one request.
func (o Options) sendWhole(ctx context.Context, file File, plan Plan) (Result, error) {
	if plan.URL == "" {
		return Result{}, errors.New("upload: the plan has neither an upload id nor a url")
	}
	progress := newProgress(file.Size(), file.Size(), 1)
	progress.notify = o.OnProgress
	progress.changed()
	request := client.Request{
		Method: "PUT",
		URL:    plan.URL,
		Blob:   file.Blob(0, file.Size()),
		OnUpload: func(sent int64) {
			progress.sending(1, sent)
		},
	}
	response, err := o.sendPart(ctx, request)
	if err != nil {
		progress.stop()
		return Result{}, err
	}
	progress.done(Part{Number: 1, Size: file.Size()})
	return Result{ETag: trimETag(header(response, "ETag"))}, nil
}

// sendParts sends the parts of the file the server is missing, and asks it to
// join them.
func (o Options) sendParts(ctx context.Context, file File, signer Signer, plan Plan, state State) (Result, error) {
	count := partCount(file.Size(), plan.PartSize)
	resuming := state.UploadID != "" && state.UploadID == plan.UploadID
	known := map[int]Part{}
	if resuming {
		for _, part := range state.Parts {
			known[part.Number] = part
		}
	}

	progress := newProgress(file.Size(), plan.PartSize, count)
	progress.notify = o.OnProgress
	for _, part := range known {
		progress.done(part)
	}
	progress.changed()

	missing := make([]int, 0, count)
	for number := 1; number <= count; number++ {
		if _, ok := known[number]; !ok {
			missing = append(missing, number)
		}
	}
	urls, err := o.sign(ctx, signer, plan.UploadID, missing)
	if err != nil {
		if resuming {
			return Result{}, errStaleUpload
		}
		return Result{}, err
	}

	var (
		mu   sync.Mutex
		held = maps.Clone(known)
	)
	save := func() {
		if o.Store == nil {
			return
		}
		o.Store.Save(o.Key, State{
			UploadID: plan.UploadID,
			PartSize: plan.PartSize,
			Size:     file.Size(),
			Parts:    slices.Collect(maps.Values(held)),
		})
	}
	mu.Lock()
	save()
	mu.Unlock()

	err = o.workers(ctx, missing, func(ctx context.Context, number int) error {
		start := int64(number-1) * plan.PartSize
		end := min(start+plan.PartSize, file.Size())
		part, err := o.sendNumber(ctx, file, number, urls[number], start, end, progress)
		if err != nil {
			return err
		}
		mu.Lock()
		held[number] = part
		save()
		mu.Unlock()
		progress.done(part)
		return nil
	})
	if err != nil {
		progress.stop()
		return Result{}, err
	}

	parts := slices.Collect(maps.Values(held))
	slices.SortFunc(parts, func(a, b Part) int { return a.Number - b.Number })
	if len(parts) != count {
		return Result{}, fmt.Errorf("upload: %d of %d parts were sent", len(parts), count)
	}
	if err := signer.Complete(ctx, plan.UploadID, parts); err != nil {
		return Result{}, err
	}
	if o.Store != nil {
		o.Store.Delete(o.Key)
	}
	return Result{UploadID: plan.UploadID, Parts: parts}, nil
}

// sign returns the URL of each part, asking for them in batches.
func (o Options) sign(ctx context.Context, signer Signer, uploadID string, numbers []int) (map[int]string, error) {
	urls := make(map[int]string, len(numbers))
	for batch := range slices.Chunk(numbers, signBatch) {
		signed, err := signer.Sign(ctx, uploadID, batch)
		if err != nil {
			return nil, err
		}
		maps.Copy(urls, signed)
	}
	for _, number := range numbers {
		if urls[number] == "" {
			return nil, fmt.Errorf("upload: the server signed no url for part %d", number)
		}
	}
	return urls, nil
}

// workers sends the numbers with as many goroutines as the options allow, and
// stops at the first error.
func (o Options) workers(ctx context.Context, numbers []int, send func(context.Context, int) error) error {
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	queue := make(chan int)
	var group sync.WaitGroup
	for range min(o.Concurrency, len(numbers)) {
		group.Add(1)
		go func() {
			defer group.Done()
			for number := range queue {
				if ctx.Err() != nil {
					return
				}
				if err := send(ctx, number); err != nil {
					stop(err)
					return
				}
			}
		}()
	}
	for _, number := range numbers {
		select {
		case queue <- number:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(queue)
	group.Wait()
	return context.Cause(ctx)
}

// sendNumber sends one part, and sends it again while the server or the network
// fails and the retries allow.
func (o Options) sendNumber(ctx context.Context, file File, number int, url string, start, end int64, progress *progress) (Part, error) {
	if url == "" {
		return Part{}, fmt.Errorf("upload: part %d has no url", number)
	}
	request := client.Request{
		Method: "PUT",
		URL:    url,
		Blob:   file.Blob(start, end),
		OnUpload: func(sent int64) {
			progress.sending(number, sent)
		},
	}
	for attempt := 1; ; attempt++ {
		response, err := o.Send(ctx, request)
		if err == nil && response.Status >= 200 && response.Status <= 299 {
			return Part{Number: number, ETag: trimETag(header(response, "ETag")), Size: end - start}, nil
		}
		if err == nil && !worthRetry(response.Status) {
			return Part{}, fmt.Errorf("upload: part %d answered %d", number, response.Status)
		}
		if ctx.Err() != nil {
			return Part{}, context.Cause(ctx)
		}
		if attempt >= o.Retry.Attempts {
			if err != nil {
				return Part{}, fmt.Errorf("upload: part %d: %w", number, err)
			}
			return Part{}, fmt.Errorf("upload: part %d answered %d", number, response.Status)
		}
		select {
		case <-time.After(o.Retry.wait(attempt)):
		case <-ctx.Done():
			return Part{}, context.Cause(ctx)
		}
		progress.retry(number)
	}
}

// sendPart sends one request, and sends it again while the server or the
// network fails and the retries allow.
func (o Options) sendPart(ctx context.Context, request client.Request) (client.Response, error) {
	for attempt := 1; ; attempt++ {
		response, err := o.Send(ctx, request)
		if err == nil && response.Status >= 200 && response.Status <= 299 {
			return response, nil
		}
		if err == nil && !worthRetry(response.Status) {
			return client.Response{}, fmt.Errorf("upload: the server answered %d", response.Status)
		}
		if ctx.Err() != nil {
			return client.Response{}, context.Cause(ctx)
		}
		if attempt >= o.Retry.Attempts {
			if err != nil {
				return client.Response{}, fmt.Errorf("upload: %w", err)
			}
			return client.Response{}, fmt.Errorf("upload: the server answered %d", response.Status)
		}
		select {
		case <-time.After(o.Retry.wait(attempt)):
		case <-ctx.Done():
			return client.Response{}, context.Cause(ctx)
		}
	}
}

// Abort drops the upload a Store kept of key, as the one of a file the person
// canceled.
func Abort(ctx context.Context, signer Signer, store Store, key string) error {
	if signer == nil {
		return errors.New("upload: no signer to ask")
	}
	if store == nil {
		return nil
	}
	state, ok := store.Load(key)
	if !ok || state.UploadID == "" {
		return nil
	}
	if err := signer.Abort(ctx, state.UploadID); err != nil {
		return err
	}
	return store.Delete(key)
}
