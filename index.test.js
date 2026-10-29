import { test } from "node:test"
import assert from "node:assert/strict"
import { fileURLToPath } from "node:url"
import { createServer } from "node:http"
import { readFile } from "node:fs/promises"
import { ComfyUIPlugin } from "./index.js"

const config = fileURLToPath(new URL("./examples/config.json", import.meta.url))

test("real Go worker registers an image provider and validates configuration", async () => {
  const plugin = await ComfyUIPlugin()
  const cfg = {}
  await plugin.config(cfg)
  assert.equal(cfg.provider.comfyui.npm, "@ai-sdk/openai-compatible")
  const auth = await plugin.auth.methods[0].authorize({ config })
  assert.equal(auth.key, config)
  const models = await plugin.provider.models({}, { auth })
  assert.equal(models.sdxl.capabilities.output.image, true)
  assert.equal(models.sdxl.api.id, "sdxl")
  await assert.rejects(plugin.auth.methods[0].authorize({ config: "/missing/config.json" }))
  await assert.rejects(plugin.auth.methods[0].authorize({ config: "relative.json" }))
})

test("unsupported operations return structured errors without contacting ComfyUI", async () => {
  const plugin = await ComfyUIPlugin()
  const loader = await plugin.auth.loader(async () => ({ key: config }))
  for (const path of ["/v1/chat/completions", "/v1/images/edits"]) {
    const res = await loader.fetch("http://comfyui.invalid" + path, { method: "POST", body: "{}" })
    assert.equal(res.status, 400)
    assert.match((await res.json()).error.message, /only POST/)
  }
  const res = await loader.fetch("http://comfyui.invalid/v1/images/generations", {
    method: "POST", body: '{"model":"sdxl","prompt":"x","quality":"high"}',
  })
  assert.equal(res.status, 400)
})

test("an already cancelled request never starts generation", async () => {
  const plugin = await ComfyUIPlugin()
  const loader = await plugin.auth.loader(async () => ({ key: config }))
  const signal = AbortSignal.abort()
  await assert.rejects(loader.fetch("http://comfyui.invalid/v1/images/generations", {
    method: "POST", body: '{"model":"sdxl","prompt":"x"}', signal,
  }))
})

test("server login checks live resources, keeps instances separate, and requires a workflow for generation", async (t) => {
  const graph = JSON.parse(await readFile(new URL("./examples/sdxl.json", import.meta.url), "utf8"))
  const nodes = Object.fromEntries(Object.values(graph).map(node => [node.class_type, { input: { required: {} } }]))
  nodes.CheckpointLoaderSimple.input.required.ckpt_name = [["sd_xl_base_1.0.safetensors"]]
  const replies = {
    "/system_stats": { system: { comfyui_version: "0.36.0" } },
    "/object_info": nodes,
    "/models": ["checkpoints", "loras"],
    "/models/checkpoints": ["sd_xl_base_1.0.safetensors"],
    "/models/loras": ["style.safetensors"],
    "/queue": { queue_running: [], queue_pending: [] },
  }
  let requests = 0
  const server = createServer((req, res) => {
    requests++
    assert.equal(req.method, "GET")
    assert.ok(Object.hasOwn(replies, req.url))
    res.setHeader("Content-Type", "application/json")
    res.end(JSON.stringify(replies[req.url]))
  })
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve))
  t.after(() => new Promise(resolve => server.close(resolve)))
  const url = `http://127.0.0.1:${server.address().port}`
  const plugin = await ComfyUIPlugin()
  const method = plugin.auth.methods[1]
  const auth = await method.authorize({ server: url, name: "GPU A" })
  assert.ok(requests > 0)
  assert.equal(JSON.parse(auth.key).server, url)
  assert.match(auth.metadata.email, /GPU A/)
  assert.deepEqual(await plugin.provider.models({}, { auth }), {})
  const loader = await plugin.auth.loader(async () => auth)
  const before = requests
  const response = await loader.fetch("http://comfyui.invalid/v1/images/generations", {
    method: "POST", body: '{"model":"sdxl","prompt":"audit"}',
  })
  assert.equal(response.status, 404)
  assert.match((await response.json()).error.message, /model not available.*workflow configuration/)
  assert.equal(requests, before)
  const configured = await method.authorize({ server: url, name: "GPU B", config })
  assert.notEqual(configured.key, auth.key)
  assert.equal((await plugin.provider.models({}, { auth: configured })).sdxl.name, "GPU B")
  nodes.CheckpointLoaderSimple.input.required.ckpt_name = [[]]
  await assert.rejects(method.authorize({ server: url, config }), /not available/)
  replies["/system_stats"] = {}
  await assert.rejects(method.authorize({ server: url }), /ComfyUI version/)
})
