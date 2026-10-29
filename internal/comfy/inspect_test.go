package comfy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func inspector(t *testing.T, checkpoint string) (*httptest.Server, map[string]any) {
	t.Helper()
	_, graph := sample(t)
	nodes := map[string]any{}
	for _, node := range graph {
		nodes[node.ClassType] = map[string]any{"input": map[string]any{"required": map[string]any{}}}
	}
	nodes["CheckpointLoaderSimple"] = map[string]any{"input": map[string]any{"required": map[string]any{"ckpt_name": []any{[]string{checkpoint}}}}}
	nodes["LoraLoader"] = map[string]any{"input": map[string]any{"required": map[string]any{"lora_name": []any{[]string{"folder/style.safetensors"}}}}}
	responses := map[string]any{
		"/system_stats":       map[string]any{"system": map[string]string{"comfyui_version": "0.36.0"}},
		"/object_info":        nodes,
		"/models":             []string{"checkpoints", "loras", "custom_nodes"},
		"/models/checkpoints": []string{checkpoint},
		"/models/loras":       []string{"folder/style.safetensors"},
		"/queue":              map[string]any{"queue_running": []any{}, "queue_pending": []any{[]any{1, "job-already-waiting"}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("discovery made a mutation: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing server authentication")
		}
		if !strings.HasPrefix(r.URL.Path, "/prefix/") {
			t.Errorf("lost path prefix: %s", r.URL.Path)
		}
		response, ok := responses[strings.TrimPrefix(r.URL.Path, "/prefix")]
		if !ok {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	t.Setenv("COMFY_INSPECT_TOKEN", "fixture-token")
	return server, responses
}

func TestInspectServerResourcesAndWorkflow(t *testing.T) {
	server, _ := inspector(t, "sd_xl_base_1.0.safetensors")
	c, graph := sample(t)
	c.BaseURL, c.TokenEnv = server.URL+"/prefix", "COMFY_INSPECT_TOKEN"
	report, err := Inspect(context.Background(), c, graph)
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != "0.36.0" || report.Pending != 1 || report.Running != 0 || report.BaseURL != c.BaseURL || report.Checked == "" {
		t.Fatalf("wrong server state: %+v", report)
	}
	if !slices.Equal(report.Models["loras"], []string{"folder/style.safetensors"}) || !slices.Contains(report.Nodes, "LoraLoader") {
		t.Fatalf("lost server resources: %+v", report)
	}
	if report.Workflow == nil || report.Workflow.ID != c.Model || len(report.Workflow.Issues) != 0 {
		t.Fatalf("wrong workflow report: %+v", report.Workflow)
	}
	graph["8"] = Node{ClassType: "LoraLoader", Inputs: map[string]any{"lora_name": "not-installed.safetensors"}}
	report, err = Inspect(context.Background(), c, graph)
	if err != nil || len(report.Workflow.Issues) != 1 || !strings.Contains(report.Workflow.Issues[0], "not-installed.safetensors") {
		t.Fatalf("missing LoRA was not identified: %+v, %v", report.Workflow, err)
	}
}

func TestInspectSeparatesServers(t *testing.T) {
	a, _ := inspector(t, "a.safetensors")
	b, _ := inspector(t, "b.safetensors")
	for _, item := range []struct{ url, model string }{{a.URL, "a.safetensors"}, {b.URL, "b.safetensors"}} {
		c, _, err := Connection(item.url+"/prefix", "COMFY_INSPECT_TOKEN", "", "")
		if err != nil {
			t.Fatal(err)
		}
		report, err := Inspect(context.Background(), c, nil)
		if err != nil || !slices.Equal(report.Models["checkpoints"], []string{item.model}) || report.Workflow != nil {
			t.Fatalf("mixed inventory or invented workflow: %+v, %v", report, err)
		}
	}
}

func TestInspectUnavailableIsNotEmpty(t *testing.T) {
	for _, path := range []string{"/system_stats", "/object_info", "/models", "/models/checkpoints", "/queue"} {
		t.Run(path, func(t *testing.T) {
			server, responses := inspector(t, "test.safetensors")
			responses[path] = nil
			c, _, _ := Connection(server.URL+"/prefix", "COMFY_INSPECT_TOKEN", "", "")
			if _, err := Inspect(context.Background(), c, nil); err == nil {
				t.Fatalf("accepted a null response at %s as an empty inventory", path)
			}
		})
	}
}

func TestInspectMissingNodeAndCheckpoint(t *testing.T) {
	server, responses := inspector(t, "other.safetensors")
	delete(responses["/object_info"].(map[string]any), "SaveImage")
	c, graph := sample(t)
	c.BaseURL, c.TokenEnv = server.URL+"/prefix", "COMFY_INSPECT_TOKEN"
	report, err := Inspect(context.Background(), c, graph)
	if err != nil || len(report.Workflow.Issues) != 2 {
		t.Fatalf("missing node/checkpoint not identified: %+v, %v", report.Workflow, err)
	}
}

func TestConnectionDefaultsAndInvalidURLs(t *testing.T) {
	c, graph, err := Connection("", "", "", "")
	if err != nil || c.BaseURL != "http://127.0.0.1:8188" || graph != nil || c.Model != "" {
		t.Fatalf("wrong default connection: %+v, %v", c, err)
	}
	for _, url := range []string{"file:///tmp/model", "https://user@server", "http://server?token=x", "http://server#fragment", "http://:8188"} {
		if _, _, err := Connection(url, "", "", ""); err == nil {
			t.Fatalf("accepted invalid server URL %s", url)
		}
	}
}
