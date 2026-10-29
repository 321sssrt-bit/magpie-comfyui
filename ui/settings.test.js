import { test } from "node:test"
import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import { Script } from "node:vm"

test("settings preserve connection data, support add/edit/delete, and reject overwrites", async () => {
  const element = () => ({ textContent: "", disabled: false, hidden: false, focus() {} })
  const container = {
    children: [],
    append(row) { this.children.push(row) },
    replaceChildren() { this.children = [] },
  }
  const makeRow = () => {
    const inputs = Object.fromEntries(["id", "server", "token_env", "user"].map((name) => [name, {
      value: "", error: "", focus() {}, reportValidity() {},
      setCustomValidity(error) { this.error = error },
      checkValidity() { return !this.error && (!["id", "server"].includes(name) || !!this.value) },
    }]))
    const parts = Object.fromEntries(["strong", ".address", ".auth", ".remove"].map((name) => [name, element()]))
    const labels = Object.fromEntries(["id", "server", "auth", "token_env", "user", "remove"].map((name) => [name, element()]))
    const row = {
      inputs,
      querySelector(selector) {
        const attr = selector.match(/^\[data-(field|label)="(\w+)"\]$/)
        return attr ? (attr[1] === "field" ? inputs : labels)[attr[2]] : parts[selector]
      },
      querySelectorAll() { return Object.values(inputs) },
      remove() { container.children = container.children.filter((item) => item !== row) },
    }
    return row
  }
  const nodes = Object.fromEntries(["editor", "heading", "hint", "empty", "add", "reset", "save", "status"].map((id) => [id, element()]))
  nodes.connections = container
  nodes.connection = { content: { firstElementChild: { cloneNode: makeRow } } }
  let stored = {
    unrelated: "keep",
    connections: {
      one: { server: "http://one:8188", token_env: "ONE_TOKEN", user: "alice", extra: "keep" },
      two: { server: "http://two:8188" },
    },
  }
  let writes = 0, fail = false
  const magpie = {
    ready: Promise.resolve(), theme: { lang: "en" },
    options: {
      get: async () => structuredClone(stored),
      set: async (value) => {
        if (fail) throw new Error("Save failed")
        writes++
        stored = JSON.parse(JSON.stringify(value))
        return structuredClone(stored)
      },
    },
  }
  const source = await readFile(new URL("./settings.js", import.meta.url), "utf8")
  await new Script(source).runInNewContext({ document: { getElementById: (id) => nodes[id] }, magpie, URL })
  const first = container.children[0]
  first.inputs.id.value = "renamed"
  first.inputs.server.value = "http://changed:8188"
  first.oninput()
  container.children[1].querySelector(".remove").onclick()
  nodes.add.onclick()
  container.children[1].inputs.id.value = "three"
  container.children[1].inputs.server.value = "http://three:8188"
  stored.unrelated = "changed elsewhere"
  fail = true
  await nodes.save.onclick()
  assert.equal(writes, 0)
  assert.equal(nodes.status.textContent, "Save failed")
  assert.equal(container.children[0].inputs.id.value, "renamed")
  fail = false
  await nodes.save.onclick()
  assert.equal(writes, 1)
  assert.deepEqual(stored, {
    unrelated: "changed elsewhere",
    connections: {
      renamed: { server: "http://changed:8188", token_env: "ONE_TOKEN", user: "alice", extra: "keep" },
      three: { server: "http://three:8188" },
    },
  })
  nodes.add.onclick()
  const duplicate = container.children[2]
  duplicate.inputs.id.value = "renamed"
  duplicate.inputs.server.value = "http://another:8188"
  await nodes.save.onclick()
  assert.equal(writes, 1)
  assert.match(duplicate.inputs.id.error, /unique/)
  nodes.reset.onclick()
  assert.equal(container.children.length, 2)
  stored.connections.remote = { server: "http://remote:8188" }
  container.children[0].inputs.server.value = "http://local:8188"
  container.children[0].oninput()
  await nodes.save.onclick()
  assert.equal(writes, 1)
  assert.match(nodes.status.textContent, /changed elsewhere/)
})
