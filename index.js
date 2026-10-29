const object = (value) => value !== null && typeof value === "object" && !Array.isArray(value)
const segment = (value) => {
  if (typeof value !== "string" || !value || value === "." || value === ".." || /[/\\\x00\r\n]/.test(value)) {
    throw new Error("Expected a single model folder, node class or prompt ID")
  }
  return encodeURIComponent(value)
}
const serverURL = (value) => {
  let url
  try { if (typeof value === "string") url = new URL(value) } catch {}
  if (!url || !["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error("Use an HTTP(S) server URL without credentials, query or fragment")
  }
  return url.href.replace(/\/+$/, "")
}

export const ComfyUIPlugin = async (_input, options = {}) => {
  const connections = options.connections ?? {}
  if (!object(connections)) throw new Error("Plugin options.connections must be an object keyed by connection ID")

  const request = async (connection, path, body, ctx, image = false) => {
    if (typeof connection !== "string" || !Object.hasOwn(connections, connection)) {
      throw new Error("Choose a connection ID from resources(resource=connections); configure connections with magpie plugin options")
    }
    const saved = connections[connection]
    if (!object(saved) || typeof saved.server !== "string") throw new Error("Missing server URL for connection " + connection)
    const url = serverURL(saved.server)
    const headers = new Headers()
    if (body !== undefined) headers.set("Content-Type", "application/json")
    if (saved.token_env) {
      if (typeof saved.token_env !== "string" || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(saved.token_env)) throw new Error("token_env must name an environment variable")
      const token = process.env[saved.token_env]
      if (!token) throw new Error("token_env is not set in the Magpie process")
      headers.set("Authorization", "Bearer " + token)
    }
    if (saved.user) {
      if (typeof saved.user !== "string") throw new Error("user must be a ComfyUI user ID")
      headers.set("Comfy-User", saved.user)
    }
    const timeout = AbortSignal.timeout(30_000)
    const signal = ctx.abort ? AbortSignal.any([ctx.abort, timeout]) : timeout
    signal.throwIfAborted()
    try {
      const response = await fetch(url + path, {
        method: body === undefined ? "GET" : "POST", headers,
        body: body === undefined ? undefined : JSON.stringify(body), redirect: "manual", signal,
      })
      if (response.status >= 300 && response.status < 400) {
        await response.body?.cancel()
        throw new Error("Redirect refused; configure the final server URL")
      }
      const chunks = []
      let size = 0
      if (response.body) for await (const chunk of response.body) {
        size += chunk.byteLength
        if (size > 16 * 1024 * 1024) throw new Error("ComfyUI response exceeds 16 MiB")
        chunks.push(chunk)
      }
      const data = Buffer.concat(chunks)
      if (!response.ok) throw new Error(`ComfyUI HTTP ${response.status}: ${data.toString("utf8", 0, 4096)}`)
      if (!image) return JSON.parse(data.toString("utf8"))
      const mime = (response.headers.get("Content-Type") || "").split(";")[0].trim()
      if (!mime.startsWith("image/")) throw new Error("ComfyUI /view did not return an image")
      return { mime_type: mime, base64: data.toString("base64") }
    } catch (error) {
      if (body !== undefined) throw new Error("Submission was not confirmed; check ComfyUI queue/history before retrying. " + error.message, { cause: error })
      throw error
    }
  }
  const tool = (description, properties, required, execute) => ({
    description,
    args: { type: "object", properties: {
      connection: { type: "string", description: "Configured connection ID. Keep the same ID for submit, history and view." },
      ...properties,
    }, required, additionalProperties: false },
    execute: async (args, ctx) => {
      ctx.abort?.throwIfAborted()
      const result = await execute(args, ctx)
      // Magpie transports tool results as one JSON line with a 32 MiB limit.
      if (Buffer.byteLength(JSON.stringify({ result: { text: JSON.stringify(result) } })) > 31 * 1024 * 1024) {
        throw new Error("ComfyUI result exceeds Magpie's tool message limit")
      }
      return result
    },
  })
  return { tool: {
    resources: tool("List configured connections, or read ComfyUI's live models, node definitions, system stats or queue. Use real files and object_info schemas to author a native API graph.", {
      resource: { type: "string", enum: ["connections", "models", "object_info", "system_stats", "queue"] },
      name: { type: "string", description: "Optional model folder or node class." },
    }, ["resource"], async ({ resource, connection, name }, ctx) => {
      if (resource === "connections") return Object.entries(connections).map(([id, value]) => {
        try { return { id, server: serverURL(value?.server) } } catch { return { id, server: null } }
      })
      if (!["models", "object_info", "system_stats", "queue"].includes(resource)) throw new Error("Unknown resource")
      if (name !== undefined && !["models", "object_info"].includes(resource)) throw new Error("name applies only to models or object_info")
      return request(connection, "/" + resource + (name === undefined ? "" : "/" + segment(name)), undefined, ctx)
    }),
    submit: tool("Submit a native ComfyUI API graph once and receive prompt_id. Poll history with the same connection. Never resubmit merely because a call timed out.", {
      prompt: { type: "object", description: "Native node-ID map of class_type and inputs, not editor canvas JSON." },
      partial_execution_targets: { type: "array", items: { type: "string" }, minItems: 1 },
    }, ["connection", "prompt"], async ({ connection, prompt, partial_execution_targets }, ctx) => {
      if (!object(prompt) || !Object.keys(prompt).length || Object.values(prompt).some((node) => !object(node) || typeof node.class_type !== "string" || !node.class_type || !object(node.inputs))) {
        throw new Error("prompt must be a native API graph; each node needs class_type and inputs")
      }
      const body = { prompt }
      if (partial_execution_targets !== undefined) {
        if (!Array.isArray(partial_execution_targets) || !partial_execution_targets.length || partial_execution_targets.some((id) => typeof id !== "string" || !Object.hasOwn(prompt, id))) {
          throw new Error("partial_execution_targets must contain existing node IDs")
        }
        body.partial_execution_targets = partial_execution_targets
      }
      return request(connection, "/prompt", body, ctx)
    }),
    history: tool("Read /history/{prompt_id} on the connection used to submit. An empty object means no history yet. Completed outputs contain image locations for view.", {
      prompt_id: { type: "string" },
    }, ["connection", "prompt_id"], async ({ connection, prompt_id }, ctx) => request(connection, "/history/" + segment(prompt_id), undefined, ctx)),
    view: tool("Retrieve an image using the exact filename, subfolder and type from history. Returns original mime_type and base64 bytes, without resizing or conversion.", {
      filename: { type: "string" }, subfolder: { type: "string" },
      type: { type: "string", enum: ["output", "temp", "input"], default: "output" },
    }, ["connection", "filename"], async ({ connection, filename, subfolder = "", type = "output" }, ctx) => {
      if (typeof filename !== "string" || !filename || typeof subfolder !== "string" || !["output", "temp", "input"].includes(type)) throw new Error("Use the filename, subfolder and type from history")
      return request(connection, "/view?" + new URLSearchParams({ filename, subfolder, type }), undefined, ctx, true)
    }),
  } }
}
