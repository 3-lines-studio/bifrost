package main

import "runtime"

type GreetService struct{}

type Greeting struct {
	Message  string `json:"message"`
	Platform string `json:"platform"`
}

func (service *GreetService) Greet(name string) Greeting {
	if name == "" {
		name = "Bifrost"
	}
	return Greeting{
		Message:  "Hello, " + name + ".",
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
}
