package protocol

import "encoding/json"

const Schema uint32 = 3

type Spec struct {
	Schema uint32      `json:"schema"`
	Routes []RouteSpec `json:"routes"`
}

type RouteSpec struct {
	Pattern    string `json:"pattern"`
	View       string `json:"view"`
	Kind       string `json:"kind"`
	Navigation bool   `json:"navigation,omitempty"`
}

type DescribeResult struct {
	Spec       Spec          `json:"spec"`
	SpecHash   string        `json:"specHash"`
	SourceRoot string        `json:"sourceRoot"`
	Limits     RuntimeLimits `json:"limits"`
}

type RuntimeLimits struct {
	MaxPropsBytes int `json:"maxPropsBytes"`
	MaxHeadBytes  int `json:"maxHeadBytes"`
	MaxFrameBytes int `json:"maxFrameBytes"`
}

type GenerateResult struct {
	SpecHash string          `json:"specHash"`
	Limits   RuntimeLimits   `json:"limits"`
	Pages    []GeneratedPage `json:"pages"`
}

type GeneratedPage struct {
	Pattern  string             `json:"pattern"`
	Path     string             `json:"path"`
	Props    json.RawMessage    `json:"props"`
	Document DocumentAttributes `json:"document"`
}

type DocumentAttributes struct {
	Lang  string `json:"lang"`
	Class string `json:"class,omitempty"`
	Dir   string `json:"dir,omitempty"`
}

type Manifest struct {
	Schema             uint32        `json:"schema"`
	SpecHash           string        `json:"specHash"`
	BuildID            string        `json:"buildId"`
	Toolchain          Toolchain     `json:"toolchain"`
	Runtime            *FileRef      `json:"runtime,omitempty"`
	RuntimeCompression string        `json:"runtimeCompression,omitempty"`
	Views              []BuiltView   `json:"views"`
	Routes             []BuiltRoute  `json:"routes"`
	ClientFiles        []FileRef     `json:"clientFiles,omitempty"`
	Public             []PublicAsset `json:"public,omitempty"`
}

type PublicAsset struct {
	URL  string  `json:"url"`
	File FileRef `json:"file"`
}

type Toolchain struct {
	Bifrost string `json:"bifrost"`
	Bun     string `json:"bun"`
	Vite    string `json:"vite"`
	React   string `json:"react"`
}

type BuiltView struct {
	ID     string        `json:"id"`
	Source string        `json:"source"`
	Mode   string        `json:"mode"`
	Client AssetSet      `json:"client"`
	Server *ServerAssets `json:"server,omitempty"`
}

type ServerAssets struct {
	Entry   FileRef   `json:"entry"`
	Imports []FileRef `json:"imports,omitempty"`
}

type AssetSet struct {
	Entry   FileRef   `json:"entry"`
	Styles  []FileRef `json:"styles,omitempty"`
	Imports []FileRef `json:"imports,omitempty"`
}

type BuiltRoute struct {
	Pattern   string     `json:"pattern"`
	Kind      string     `json:"kind"`
	ViewID    string     `json:"viewId"`
	Documents []Document `json:"documents,omitempty"`
}

type Document struct {
	Path     string             `json:"path"`
	File     FileRef            `json:"file"`
	Props    json.RawMessage    `json:"props,omitempty"`
	Document DocumentAttributes `json:"document"`
}

type FileRef struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}
