import { test } from "node:test"
import assert from "node:assert/strict"
import { access, readFile } from "node:fs/promises"
import { constants } from "node:fs"
import { fileURLToPath } from "node:url"
import { workerPath } from "./platforms.js"

test("prebuilt workers match the selected operating system and CPU", async () => {
  const linux = await readFile(workerPath("linux", "arm64"))
  assert.equal(linux.subarray(0, 4).toString("hex"), "7f454c46")
  assert.equal(linux.readUInt16LE(18), 183) // ELF AArch64
  const windows = await readFile(workerPath("win32", "x64"))
  const pe = windows.readUInt32LE(0x3c)
  assert.equal(windows.subarray(pe, pe + 4).toString(), "PE\0\0")
  assert.equal(windows.readUInt16LE(pe + 4), 0x8664)
  const mac = await readFile(workerPath("darwin", "arm64"))
  assert.equal(mac.readUInt32LE(0), 0xfeedfacf)
  assert.equal(mac.readUInt32LE(4), 0x0100000c)
  await access(workerPath(), constants.X_OK)
  assert.equal(workerPath(), fileURLToPath(new URL(
    `./bin/${process.platform}-${process.arch}/comfyui-worker${process.platform === "win32" ? ".exe" : ""}`,
    import.meta.url,
  )))
})

test("unverified targets fail explicitly instead of falling back to another worker", () => {
  for (const [platform, arch] of [["linux", "x64"], ["darwin", "x64"], ["win32", "arm64"], ["freebsd", "arm64"]]) {
    assert.throws(() => workerPath(platform, arch), /no prebuilt worker.*Supported:/)
  }
})
