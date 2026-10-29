package comfy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"
)

type WorkflowInfo struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Issues []string `json:"issues"`
}

type Inventory struct {
	BaseURL  string              `json:"base_url"`
	Name     string              `json:"name"`
	Version  string              `json:"comfyui_version"`
	Checked  string              `json:"checked_at"`
	Models   map[string][]string `json:"models"`
	Nodes    []string            `json:"nodes"`
	Running  int                 `json:"running"`
	Pending  int                 `json:"pending"`
	Workflow *WorkflowInfo       `json:"workflow,omitempty"`
	Note     string              `json:"note"`
}

// Inspect reads the server's own lists. It never loads models or submits jobs.
func Inspect(ctx context.Context, c Config, graph Graph) (Inventory, error) {
	out := Inventory{BaseURL: c.BaseURL, Name: c.Name, Models: map[string][]string{}, Nodes: []string{},
		Note: "Installed filenames and node choices do not prove model/LoRA compatibility or successful execution."}
	api, err := newClient(c)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	read := func(path string, target any, limit int64) error {
		b, err := api.call(ctx, "GET", path, nil, limit)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, target); err != nil {
			return fmt.Errorf("%s: invalid ComfyUI response: %w", path, err)
		}
		return nil
	}
	var stats struct {
		System struct {
			Version string `json:"comfyui_version"`
		} `json:"system"`
	}
	if err := read("/system_stats", &stats, 1<<20); err != nil {
		return out, err
	}
	if stats.System.Version == "" {
		return out, errors.New("server did not identify its ComfyUI version")
	}
	out.Version = stats.System.Version
	var nodes map[string]struct {
		Input map[string]map[string]json.RawMessage `json:"input"`
	}
	if err := read("/object_info", &nodes, 32<<20); err != nil {
		return out, err
	}
	if len(nodes) == 0 {
		return out, errors.New("ComfyUI returned no node definitions")
	}
	for name := range nodes {
		out.Nodes = append(out.Nodes, name)
	}
	sort.Strings(out.Nodes)
	var folders []string
	if err := read("/models", &folders, 1<<20); err != nil {
		return out, err
	}
	if folders == nil {
		return out, errors.New("ComfyUI returned no model folder list")
	}
	for _, folder := range []string{"checkpoints", "diffusion_models", "unet", "text_encoders", "clip", "vae", "loras", "controlnet", "upscale_models", "embeddings"} {
		if !slices.Contains(folders, folder) {
			continue
		}
		var files []string
		if err := read("/models/"+folder, &files, 4<<20); err != nil {
			return out, err
		}
		if files == nil {
			return out, fmt.Errorf("/models/%s returned null, not a model list", folder)
		}
		out.Models[folder] = files
	}
	var queue struct {
		Running []json.RawMessage `json:"queue_running"`
		Pending []json.RawMessage `json:"queue_pending"`
	}
	if err := read("/queue", &queue, 8<<20); err != nil {
		return out, err
	}
	if queue.Running == nil || queue.Pending == nil {
		return out, errors.New("ComfyUI returned no queue state")
	}
	out.Running, out.Pending = len(queue.Running), len(queue.Pending)
	if len(graph) > 0 {
		out.Workflow = &WorkflowInfo{ID: c.Model, Name: c.Name, Issues: []string{}}
		for id, node := range graph {
			info, ok := nodes[node.ClassType]
			if !ok {
				out.Workflow.Issues = append(out.Workflow.Issues, fmt.Sprintf("node %s: %s is not installed", id, node.ClassType))
				continue
			}
			for field, value := range node.Inputs {
				text, ok := value.(string)
				if !ok {
					continue
				}
				definition := info.Input["required"][field]
				if definition == nil {
					definition = info.Input["optional"][field]
				}
				var spec []json.RawMessage
				var choices []string
				if json.Unmarshal(definition, &spec) != nil || len(spec) == 0 || json.Unmarshal(spec[0], &choices) != nil || choices == nil {
					continue
				}
				if !slices.Contains(choices, text) {
					out.Workflow.Issues = append(out.Workflow.Issues, fmt.Sprintf("node %s: %s.%s value %q is not available on this server", id, node.ClassType, field, text))
				}
			}
		}
		sort.Strings(out.Workflow.Issues)
	}
	out.Checked = time.Now().UTC().Format(time.RFC3339)
	return out, nil
}
