import { execFile } from "node:child_process"
import { isAbsolute } from "node:path"
import { workerPath } from "./platforms.js"

const baseURL = "http://comfyui.invalid/v1"
const binary = workerPath()

// Magpie loads JS plugins; the short-lived Go worker owns the ComfyUI protocol.
function worker(source, command, request, signal) {
  return new Promise((resolve, reject) => {
    signal?.throwIfAborted()
    const connection = typeof source === "string" ? { config: source } : source
    const args = []
    for (const [key, flag] of Object.entries({ config: "-config", server: "-server", token_env: "-token-env", name: "-name" })) {
      if (connection[key]) args.push(flag, connection[key])
    }
    const child = execFile(binary, [...args, command], {
      signal, maxBuffer: 128 * 1024 * 1024, windowsHide: true,
    }, (error, stdout, stderr) => {
      if (error) return reject(new Error(stderr.trim() || error.message, { cause: error }))
      try { resolve(JSON.parse(stdout)) } catch { reject(new Error("ComfyUI worker returned invalid JSON")) }
    })
    child.stdin.on("error", () => {}) // execFile reports early exit or failed spawn.
    child.stdin.end(request === undefined ? "" : JSON.stringify(request))
  })
}

function savedConnection(key) {
  return key.startsWith("{") ? JSON.parse(key) : key
}

function serverURL(value) {
  const url = new URL(value || "http://127.0.0.1:8188")
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error("Use an HTTP(S) server URL without credentials, query or fragment")
  }
  return url.href.replace(/\/$/, "")
}

export const ComfyUIPlugin = async () => ({
  config: async (config) => {
    config.provider ??= {}
    config.provider.comfyui = {
      name: "ComfyUI", npm: "@ai-sdk/openai-compatible", api: baseURL, models: {},
    }
  },
  provider: {
    id: "comfyui",
    async models(_provider, { auth }) {
      if (!auth?.key) return {}
      const source = savedConnection(auth.key)
      const result = await worker(source, typeof source === "string" ? "catalog" : "inspect")
      const model = typeof source === "string" ? result : result.workflow
      if (!model) return {}
      if (model.issues?.length) throw new Error(model.issues.join("; "))
      return {
        [model.id]: {
          id: model.id, name: model.name, providerID: "comfyui",
          api: { id: model.id, url: baseURL, npm: "@ai-sdk/openai-compatible" },
          limit: { context: 0, output: 0 },
          cost: { input: 0, output: 0 },
          capabilities: {
            input: { text: true }, output: { image: true },
            toolcall: false, reasoning: false, temperature: false,
          },
        },
      }
    },
  },
  auth: {
    provider: "comfyui",
    methods: [{
      type: "api", label: "ComfyUI workflow configuration", keyless: true,
      prompts: [{
        type: "text", key: "config", message: "Absolute path to your ComfyUI configuration JSON",
        validate: (value) => isAbsolute(value || "") ? undefined : "Use an absolute file path",
      }],
      async authorize(inputs) {
        if (!isAbsolute(inputs?.config || "")) throw new Error("Use an absolute configuration path")
        await worker(inputs.config, "catalog")
        return { type: "success", key: inputs.config }
      },
    }, {
      type: "api", label: "Connect ComfyUI server and discover resources", keyless: true,
      prompts: [
        {
          type: "text", key: "server", message: "ComfyUI server URL (blank: http://127.0.0.1:8188)",
          validate(value) { try { serverURL(value); return undefined } catch { return "Use an HTTP(S) server URL" } },
        },
        { type: "text", key: "name", message: "Connection name (optional)" },
        {
          type: "text", key: "token_env", message: "Bearer token environment variable name (optional; not the token itself)",
          validate: (value) => !value || /^[A-Za-z_][A-Za-z0-9_]*$/.test(value) ? undefined : "Use an environment variable name",
        },
        {
          type: "text", key: "config", message: "Existing workflow configuration path (optional; blank connects for resource inspection only)",
          validate: (value) => !value || isAbsolute(value) ? undefined : "Use an absolute file path",
        },
      ],
      async authorize(inputs = {}) {
        if (inputs.config && !isAbsolute(inputs.config)) throw new Error("Use an absolute configuration path")
        if (inputs.token_env && !/^[A-Za-z_][A-Za-z0-9_]*$/.test(inputs.token_env)) throw new Error("Use an environment variable name")
        const source = { server: serverURL(inputs.server), name: inputs.name || "", token_env: inputs.token_env || "", config: inputs.config || "" }
        const inventory = await worker(source, "inspect")
        if (inventory.workflow?.issues.length) throw new Error(inventory.workflow.issues.join("; "))
        source.server = inventory.base_url
        source.name = inventory.name
        return {
          type: "success", key: JSON.stringify(source),
          // Magpie displays API account identity through metadata.email.
          metadata: { email: `${source.name} (${source.server})${source.config ? " · " + source.config : ""}` },
        }
      },
    }],
    async loader(getAuth) {
      return {
        apiKey: "comfyui-local", baseURL,
        async fetch(url, init = {}) {
          const { key } = await getAuth()
          if (!key) throw new Error("Connect a ComfyUI workflow configuration first")
          const result = await worker(savedConnection(key), "request", {
            method: init.method || "POST", path: new URL(url).pathname,
            body: typeof init.body === "string" ? init.body : "",
          }, init.signal)
          return new Response(JSON.stringify(result.body), {
            status: result.status, headers: { "Content-Type": "application/json" },
          })
        },
      }
    },
  },
})
