import { test } from "node:test"
import assert from "node:assert/strict"
import { ComfyUIPlugin } from "./index.js"

const ctx = { abort: new AbortController().signal }

test("stock tool context keeps connections and native requests separate", async (t) => {
  const requests = []
  const png = Buffer.from("89504e470d0a1a0a", "hex")
  t.mock.method(globalThis, "fetch", async (url, init) => {
    requests.push({ url, init })
    return url.includes("/view?")
      ? new Response(png, { headers: { "Content-Type": "image/png" } })
      : Response.json({ prompt_id: "job" })
  })
  const previous = process.env.COMFY_TEST_TOKEN
  process.env.COMFY_TEST_TOKEN = "fixture"
  t.after(() => { if (previous === undefined) delete process.env.COMFY_TEST_TOKEN; else process.env.COMFY_TEST_TOKEN = previous })
  const plugin = await ComfyUIPlugin({}, { connections: {
    broken: { server: "not a URL" },
    one: { server: "http://first:8188" },
    two: { server: "http://second:8188/prefix", user: "second-user", token_env: "COMFY_TEST_TOKEN" },
  } })
  assert.deepEqual(Object.keys(plugin), ["tool"])
  assert.deepEqual(Object.keys(plugin.tool), ["resources", "submit", "history", "view"])
  const connections = await plugin.tool.resources.execute({ resource: "connections" }, ctx)
  assert.deepEqual(connections, [
    { id: "broken", server: null }, { id: "one", server: "http://first:8188" },
    { id: "two", server: "http://second:8188/prefix" },
  ])
  const prompt = { "7": { class_type: "SaveImage", inputs: { images: ["6", 0] } } }
  const receipt = await plugin.tool.submit.execute({ connection: "two", prompt, partial_execution_targets: ["7"] }, ctx)
  assert.equal(receipt.prompt_id, "job")
  assert.equal(requests[0].url, "http://second:8188/prefix/prompt")
  assert.equal(requests[0].init.headers.get("Comfy-User"), "second-user")
  assert.equal(requests[0].init.headers.get("Authorization"), "Bearer fixture")
  assert.deepEqual(JSON.parse(requests[0].init.body), { prompt, partial_execution_targets: ["7"] })
  await plugin.tool.history.execute({ connection: "two", prompt_id: "job" }, ctx)
  await plugin.tool.resources.execute({ connection: "one", resource: "object_info", name: "SaveImage" }, ctx)
  const result = await plugin.tool.view.execute({ connection: "two", filename: "one + two.png", subfolder: "a/b" }, ctx)
  assert.deepEqual(result, { mime_type: "image/png", base64: png.toString("base64") })
  assert.equal(new URL(requests[3].url).searchParams.get("filename"), "one + two.png")
  assert.equal(new URL(requests[3].url).searchParams.get("subfolder"), "a/b")
  assert.equal(requests[1].url, "http://second:8188/prefix/history/job")
  assert.equal(requests[2].url, "http://first:8188/object_info/SaveImage")
  assert.equal(requests[2].init.headers.has("Authorization"), false)
  for (const connection of [undefined, "missing", "broken", "toString"]) {
    await assert.rejects(plugin.tool.history.execute({ connection, prompt_id: "job" }, ctx))
  }
  await assert.rejects(plugin.tool.submit.execute({ connection: "one", prompt: { nodes: [] } }, ctx))
  await assert.rejects(plugin.tool.history.execute({ connection: "one", prompt_id: ".." }, ctx))
  await assert.rejects(plugin.tool.resources.execute({ connection: "one", resource: "interrupt" }, ctx))
  await assert.rejects(plugin.tool.history.execute({ connection: "one", prompt_id: "job" }, { abort: AbortSignal.abort() }))
  assert.equal(requests.length, 4)
})

test("redirects, uncertain submits and oversized replies stay within the plugin", async (t) => {
  const plugin = await ComfyUIPlugin({}, { connections: { one: { server: "http://first:8188" } } })
  let reply = () => new Response(null, { status: 307, headers: { Location: "http://other:8188" } })
  let calls = 0, cancelled = false
  t.mock.method(globalThis, "fetch", async (_url, init) => {
    calls++
    assert.equal(init.redirect, "manual")
    return reply()
  })
  await assert.rejects(plugin.tool.resources.execute({ connection: "one", resource: "models" }, ctx), /Redirect refused/)
  reply = () => { throw new Error("connection lost") }
  await assert.rejects(plugin.tool.submit.execute({ connection: "one", prompt: { "1": { class_type: "SaveImage", inputs: {} } } }, ctx), /check ComfyUI queue\/history before retrying/)
  assert.equal(calls, 2, "a failed submission is not retried")
  reply = () => new Response(new ReadableStream({
    pull(controller) { controller.enqueue(new Uint8Array(1024 * 1024)) },
    cancel() { cancelled = true },
  }), { headers: { "Content-Type": "image/png" } })
  await assert.rejects(plugin.tool.view.execute({ connection: "one", filename: "large.png" }, ctx), /exceeds 16 MiB/)
  assert.equal(cancelled, true)
  reply = () => Response.json('"'.repeat(8 * 1024 * 1024 - 2))
  await assert.rejects(plugin.tool.resources.execute({ connection: "one", resource: "models" }, ctx), /tool message limit/)
})
