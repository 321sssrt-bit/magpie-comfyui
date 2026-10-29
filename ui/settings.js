(async () => {
  const $ = (id) => document.getElementById(id)
  const rows = new Map()
  const object = (value) => value !== null && typeof value === "object" && !Array.isArray(value)
  const field = (row, name) => row.querySelector(`[data-field="${name}"]`)
  let options, baseline
  const dirty = (changed) => {
    $("save").disabled = $("reset").disabled = !changed
    $("status").textContent = changed ? t("有未保存的更改", "Unsaved changes") : ""
  }
  await magpie.ready
  const t = (zh, en) => magpie.theme.lang?.startsWith("zh") ? zh : en
  const labels = {
    id: t("连接名称", "Connection name"), server: t("ComfyUI 地址", "ComfyUI URL"),
    auth: t("认证（可选）", "Authentication (optional)"),
    token_env: t("令牌环境变量名", "Token environment variable name"),
    user: t("ComfyUI 用户 ID", "ComfyUI user ID"), remove: t("删除连接", "Delete connection"),
  }
  $("heading").textContent = t("ComfyUI 连接", "ComfyUI connections")
  $("hint").textContent = t("Agent 按连接名称选择 ComfyUI。更改在保存后生效。", "Agents select ComfyUI by connection name. Changes take effect when saved.")
  $("empty").textContent = t("尚未添加连接。", "No connections added yet.")
  $("add").textContent = t("添加连接", "Add connection")
  $("reset").textContent = t("取消更改", "Discard changes")
  $("save").textContent = t("保存", "Save")

  const add = (id = "", saved = {}, open = true) => {
    const row = $("connection").content.firstElementChild.cloneNode(true)
    row.open = open
    for (const [name, text] of Object.entries(labels)) row.querySelector(`[data-label="${name}"]`).textContent = text
    for (const name of ["id", "server", "token_env", "user"]) field(row, name).value = name === "id" ? id : saved[name] ?? ""
    const title = () => {
      row.querySelector("strong").textContent = field(row, "id").value.trim() || t("新连接", "New connection")
      row.querySelector(".address").textContent = field(row, "server").value.trim()
    }
    row.oninput = () => {
      for (const item of rows.keys()) for (const input of item.querySelectorAll("input")) input.setCustomValidity("")
      title()
      dirty(true)
    }
    row.querySelector(".remove").onclick = () => {
      rows.delete(row)
      row.remove()
      $("empty").hidden = rows.size > 0
      dirty(true)
      $("add").focus()
    }
    rows.set(row, saved)
    $("connections").append(row)
    $("empty").hidden = true
    title()
    return row
  }
  const render = (value) => {
    const connections = value.connections ?? {}
    if (!object(connections) || Object.values(connections).some((entry) => !object(entry))) {
      throw new Error(t("连接配置格式无效，未覆盖已有配置。", "Invalid connection configuration; existing settings have not been overwritten."))
    }
    options = value
    baseline = JSON.stringify(connections)
    rows.clear()
    $("connections").replaceChildren()
    for (const [id, saved] of Object.entries(connections)) add(id, saved, Object.keys(connections).length === 1)
    $("empty").hidden = rows.size > 0
    dirty(false)
  }
  $("add").onclick = () => {
    field(add(), "id").focus()
    dirty(true)
  }
  $("reset").onclick = () => render(options)
  $("save").onclick = async () => {
    const entries = [], names = new Set()
    for (const [row, saved] of rows) {
      const id = field(row, "id"), server = field(row, "server")
      id.value = id.value.trim()
      server.value = server.value.trim()
      id.setCustomValidity(names.has(id.value) ? t("连接名称不能重复。", "Connection names must be unique.") : "")
      names.add(id.value)
      let url
      try { url = new URL(server.value) } catch {}
      server.setCustomValidity(!url || !["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash
        ? t("请输入 HTTP(S) 地址，不含用户名、密码、查询参数或片段。", "Enter an HTTP(S) URL without credentials, query or fragment.") : "")
      for (const input of row.querySelectorAll("input")) {
        if (!input.checkValidity()) {
          row.open = true
          row.querySelector(".auth").open = true
          input.reportValidity()
          return
        }
      }
      const connection = { ...saved, server: server.value }
      for (const name of ["token_env", "user"]) {
        const value = field(row, name).value.trim()
        if (value) connection[name] = value
        else delete connection[name]
      }
      entries.push([id.value, connection])
    }
    $("editor").disabled = true
    try {
      const latest = await magpie.options.get()
      if (JSON.stringify(latest.connections ?? {}) !== baseline) {
        throw new Error(t("连接已在其他位置修改，请重新打开设置后再编辑。", "Connections changed elsewhere. Reopen settings before editing again."))
      }
      render(await magpie.options.set({ ...latest, connections: Object.fromEntries(entries) }))
      $("status").textContent = t("已保存", "Saved")
    } catch (error) {
      $("status").textContent = error.message
    } finally {
      $("editor").disabled = false
    }
  }
  render(await magpie.options.get())
  $("editor").disabled = false
})().catch((error) => {
  document.getElementById("status").textContent = error.message
})
