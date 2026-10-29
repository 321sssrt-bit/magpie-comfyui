# magpie-comfyui-link

[中文](https://unpkg.com/magpie-comfyui-link@0.2.0/README.md) · [MIT](https://unpkg.com/magpie-comfyui-link@0.2.0/LICENSE)

![magpie-comfyui-link](https://unpkg.com/magpie-comfyui-link@0.2.0/assets/readme-hero.png)

A Magpie tool plugin for ComfyUI. Let AI agents discover models and nodes, submit workflows, track tasks and retrieve generated images. Supports multiple independent ComfyUI connections.

Configure and distribute ComfyUI tools through Magpie so different agents can use the same capabilities, with fewer connection and authentication settings to maintain in each agent.

## Use cases

- **Share ComfyUI across agents**: maintain connections in Magpie and connect each agent to its tool endpoint to discover resources, submit tasks and retrieve images.
- **Operate multiple ComfyUI servers**: select a server by connection name, use its models and nodes, and keep status queries and image retrieval on that same server.
- **Generate images from natural-language requests**: the agent inspects available resources, authors and submits a workflow, then retrieves the image. For example:

  > Inspect the models and nodes on studio, generate a portrait by a window, and retrieve the image when it is ready.

## Install

Install the plugin through Magpie:

```sh
magpie plugin add magpie-comfyui-link
```

## Configure

Select **Settings** in Magpie's sidebar, open the **Plugins** tab at the top, and use the **ComfyUI connections** panel to add, edit or delete connections. Click **Save** to apply changes. Each connection has a name and a ComfyUI URL. Expand **Authentication (optional)** when needed.

You can also set connections through Magpie's plugin options:

```sh
magpie plugin options magpie-comfyui-link '{
  "connections": {
    "studio": { "server": "https://comfy-a.example.com" },
    "render": { "server": "https://comfy-b.example.com" }
  }
}'
```

`studio` and `render` are custom connection names. Replace the example addresses with your ComfyUI servers. A single server needs one entry. When updating options, retain any connections you still use.

Each connection accepts these fields:

| Field | Description |
| --- | --- |
| `server` | Required. ComfyUI HTTP(S) server address. |
| `token_env` | Optional. Name of an environment variable containing a Bearer token in the Magpie process. |
| `user` | Optional. ComfyUI multi-user ID. |

## Use

Agents call the tools below through Magpie's shared plugin-tools MCP endpoint. Agents already connected to that endpoint do not need to add another MCP server for this plugin.

| Tool | Function |
| --- | --- |
| `resources` | Discover connections, model files and node definitions; read system stats and the queue. |
| `submit` | Submit a native ComfyUI API workflow and receive a prompt ID. |
| `history` | Read task status and output files. |
| `view` | Retrieve an image as its MIME type and original base64 bytes. |

The agent uses resource information to author a workflow, submits it, checks the task and retrieves the image. Every call for a task uses the same `connection`: a name from the configuration. ComfyUI executes the workflow.

If submission is not confirmed, inspect the original server's queue and history before retrying. The plugin currently limits each response to 16 MiB and rejects oversized responses. This is a plugin transport limit, not a ComfyUI image-generation limit.
