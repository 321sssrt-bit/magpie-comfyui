import { fileURLToPath } from "node:url"

// Keys use Node/Bun names; values are the corresponding Go build targets.
export const platforms = {
  "linux-arm64": ["linux", "arm64"],
  "win32-x64": ["windows", "amd64"],
  "darwin-arm64": ["darwin", "arm64"],
}

export function workerPath(platform = process.platform, arch = process.arch) {
  const target = `${platform}-${arch}`
  if (!Object.hasOwn(platforms, target)) {
    throw new Error(`ComfyUI has no prebuilt worker for ${target}. Supported: ${Object.keys(platforms).join(", ")}`)
  }
  const name = platform === "win32" ? "comfyui-worker.exe" : "comfyui-worker"
  return fileURLToPath(new URL(`./bin/${target}/${name}`, import.meta.url))
}
