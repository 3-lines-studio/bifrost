package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost"
)

func intParam(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return def
	}
	return value
}

func heavyLoader(rowsN, colsN, latencyMS int) bifrost.PropsLoader {
	return func(r *http.Request) (any, error) {
		rowsN = intParam(r, "rows", rowsN)
		colsN = intParam(r, "cols", colsN)
		latency := intParam(r, "latency", latencyMS)
		if latency > 0 {
			time.Sleep(time.Duration(latency) * time.Millisecond)
		}
		rows := make([]map[string]any, rowsN)
		for i := range rows {
			cells := make([]string, colsN)
			for j := range cells {
				cells[j] = fmt.Sprintf("cell-%d-%d", i, j)
			}
			rows[i] = map[string]any{"id": fmt.Sprintf("row-%05d", i), "cells": cells}
		}
		return map[string]any{
			"title": "Bifrost heavy bench page",
			"rows":  rows,
		}, nil
	}
}

func main() {
	port := flag.Int("port", 8080, "listen port")
	rows := flag.Int("rows", 2000, "default row count")
	cols := flag.Int("cols", 20, "default cell count per row")
	latency := flag.Int("latency", 50, "default loader latency in ms")
	flag.Parse()

	routes := []bifrost.Route{
		bifrost.Page("/heavy", "./pages/heavy.tsx", bifrost.WithLoader(heavyLoader(*rows, *cols, *latency))),
	}

	app, err := bifrost.New(bifrostFS, routes...)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}
	defer app.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", app.Handler())

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	log.Printf("bench server listening on :%d", *port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server stopped: %v", err)
	}
}
