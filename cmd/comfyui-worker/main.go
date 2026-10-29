package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"magpie-comfyui/internal/comfy"
)

func run() error {
	config := flag.String("config", "", "ComfyUI configuration JSON")
	server := flag.String("server", "", "ComfyUI URL; defaults to http://127.0.0.1:8188 without a configuration")
	tokenEnv := flag.String("token-env", "", "environment variable containing a Bearer token")
	name := flag.String("name", "", "connection name")
	flag.Parse()
	if flag.NArg() != 1 {
		return errors.New("usage: comfyui-worker [-config FILE] [-server URL] [-token-env NAME] catalog|inspect|request")
	}
	c, graph, err := comfy.Connection(*server, *tokenEnv, *name, *config)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch flag.Arg(0) {
	case "catalog":
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"id": c.Model, "name": c.Name, "base_url": c.BaseURL})
	case "inspect":
		inventory, err := comfy.Inspect(ctx, c, graph)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(inventory)
	case "request":
		b, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
		if err != nil || len(b) > 1<<20 {
			return errors.New("request is unreadable or exceeds 1 MiB")
		}
		var req comfy.Request
		if err := json.Unmarshal(b, &req); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(comfy.Handle(ctx, c, graph, req, os.Stderr))
	default:
		return errors.New("unknown command; use catalog, inspect or request")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
