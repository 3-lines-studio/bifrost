package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// PartSize is how many bytes each part of an upload holds. The demo keeps it
// small enough to see several parts of a picture; S3 asks for 8 MiB at least.
const PartSize = 1 << 20

// MaxUpload is the largest file the demo takes.
const MaxUpload = 64 << 20

// ErrNoUpload is what an operation on an upload the server does not have
// answers.
var ErrNoUpload = errors.New("store: no such upload")

// Part is one part of an upload the server has: its number, from 1, the ETag of
// its bytes, and how many they are.
type Part struct {
	Number int    `json:"number"`
	ETag   string `json:"etag"`
	Size   int64  `json:"size"`
}

// Upload is a file on its way to the server.
type Upload struct {
	ID       string
	Name     string
	Type     string
	Size     int64
	PartSize int64
	Parts    map[int]Part

	// data are the bytes of the parts, by number.
	data map[int][]byte
}

// Media is a file the server joined and keeps, as a share link names it.
type Media struct {
	ID   string
	Name string
	Type string
	Size int64
	Data []byte
}

var uploads = struct {
	sync.Mutex
	byID  map[string]*Upload
	media map[string]*Media
	order []string
}{byID: map[string]*Upload{}, media: map[string]*Media{}}

// keptMedia is how many joined files the demo keeps: the oldest go.
const keptMedia = 8

// StartUpload begins an upload of size bytes and answers it.
func StartUpload(name, contentType string, size int64) (Upload, error) {
	if size <= 0 || size > MaxUpload {
		return Upload{}, fmt.Errorf("store: a file of %d bytes is not one this server takes", size)
	}
	id, err := newID()
	if err != nil {
		return Upload{}, err
	}
	upload := &Upload{ID: id, Name: name, Type: contentType, Size: size, PartSize: PartSize, Parts: map[int]Part{}, data: map[int][]byte{}}
	uploads.Lock()
	uploads.byID[id] = upload
	uploads.Unlock()
	return *upload, nil
}

// UploadByID returns the upload the id names.
func UploadByID(id string) (Upload, bool) {
	uploads.Lock()
	defer uploads.Unlock()
	upload, ok := uploads.byID[id]
	if !ok {
		return Upload{}, false
	}
	return *upload, true
}

// PutPart keeps one part of an upload and answers what the server has of it.
func PutPart(id string, number int, data []byte) (Part, error) {
	uploads.Lock()
	defer uploads.Unlock()
	upload, ok := uploads.byID[id]
	if !ok {
		return Part{}, ErrNoUpload
	}
	count := int((upload.Size + upload.PartSize - 1) / upload.PartSize)
	if number < 1 || number > count {
		return Part{}, fmt.Errorf("store: part %d of %d parts", number, count)
	}
	start := int64(number-1) * upload.PartSize
	expected := min(upload.PartSize, upload.Size-start)
	if int64(len(data)) != expected {
		return Part{}, fmt.Errorf("store: part %d holds %d bytes of %d", number, len(data), expected)
	}
	sum := sha256.Sum256(data)
	part := Part{Number: number, ETag: hex.EncodeToString(sum[:8]), Size: int64(len(data))}
	upload.Parts[number] = part
	upload.data[number] = append([]byte(nil), data...)
	return part, nil
}

// CompleteUpload joins the parts of an upload, as the client says it has them,
// and answers the file.
func CompleteUpload(id string, parts []Part) (Media, error) {
	uploads.Lock()
	defer uploads.Unlock()
	upload, ok := uploads.byID[id]
	if !ok {
		return Media{}, ErrNoUpload
	}
	count := int((upload.Size + upload.PartSize - 1) / upload.PartSize)
	if len(parts) != count {
		return Media{}, fmt.Errorf("store: %d parts of %d", len(parts), count)
	}
	sort.Slice(parts, func(a, b int) bool { return parts[a].Number < parts[b].Number })
	for index, part := range parts {
		held, ok := upload.Parts[part.Number]
		if !ok {
			return Media{}, fmt.Errorf("store: part %d is missing", part.Number)
		}
		if part.Number != index+1 || held.ETag != part.ETag || held.Size != part.Size {
			return Media{}, fmt.Errorf("store: part %d is not the one the server has", part.Number)
		}
	}
	data := make([]byte, 0, upload.Size)
	for number := 1; number <= count; number++ {
		data = append(data, upload.data[number]...)
	}
	delete(uploads.byID, id)
	media := &Media{ID: id, Name: upload.Name, Type: upload.Type, Size: upload.Size, Data: data}
	uploads.media[id] = media
	uploads.order = append(uploads.order, id)
	if len(uploads.order) > keptMedia {
		oldest := uploads.order[0]
		uploads.order = uploads.order[1:]
		delete(uploads.media, oldest)
	}
	return *media, nil
}

// AbortUpload drops an upload and its parts.
func AbortUpload(id string) {
	uploads.Lock()
	defer uploads.Unlock()
	delete(uploads.byID, id)
}

// MediaByID returns the file the id names.
func MediaByID(id string) (Media, bool) {
	uploads.Lock()
	defer uploads.Unlock()
	media, ok := uploads.media[id]
	if !ok {
		return Media{}, false
	}
	return *media, true
}

// newID returns an id nobody guesses: a share link names a file with it.
func newID() (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
