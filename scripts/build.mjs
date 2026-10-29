import { spawnSync } from "node:child_process"
import { mkdirSync } from "node:fs"
import { dirname } from "node:path"
import { fileURLToPath } from "node:url"
import { platforms, workerPath } from "../platforms.js"

const args = process.argv.slice(2)
if (args.length && (args.length !== 1 || args[0] !== "--all")) {
  throw new Error("Usage: node scripts/build.mjs [--all]")
}
const targets = args.length ? Object.keys(platforms) : [`${process.platform}-${process.arch}`]
for (const target of targets) {
  const binary = workerPath(...target.split("-"))
  const [GOOS, GOARCH] = platforms[target]
  mkdirSync(dirname(binary), { recursive: true })
  const result = spawnSync("go", ["build", "-buildvcs=false", "-trimpath", "-o", binary, "./cmd/comfyui-worker"], {
    cwd: fileURLToPath(new URL("..", import.meta.url)),
    env: { ...process.env, CGO_ENABLED: "0", GOOS, GOARCH },
    stdio: "inherit",
  })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`Go build failed for ${target} (${result.signal || result.status})`)
}
