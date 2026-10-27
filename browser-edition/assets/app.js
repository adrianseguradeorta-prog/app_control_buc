"use strict";
(() => {
  const M = CBModel,
    A = CBArchive,
    $ = (s) => document.querySelector(s),
    app = $("#app");
  const esc = (s) =>
    String(s ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  const textFields = (obj, keys) =>
    keys.map((k) => [k, String(obj[k] ?? "").trim()]);
  let root,
    store,
    cfg,
    user,
    token,
    snapshot = "",
    configSnapshot = "",
    busy = false,
    blocked = false,
    view = { name: "home" },
    monthlyContext = {},
    timer,
    objectURLs = [];
  const encoder = new TextEncoder();
  function status(text, error = false) {
    $("#status").textContent = text;
    $("#status").classList.toggle("error", error);
  }
  function button(action, label, value = "", cls = "") {
    return `<button type="button" data-action="${action}" data-value="${esc(value)}" class="${cls}">${esc(label)}</button>`;
  }
  function input(name, label, value = "", type = "text", extra = "") {
    return `<label>${esc(label)}<input name="${name}" type="${type}" value="${esc(value)}" ${extra}></label>`;
  }
  function check(name, label, value) {
    return `<label><input type="checkbox" name="${name}" ${value ? "checked" : ""}> ${esc(label)}</label>`;
  }
  function select(name, label, items, value) {
    return `<label>${esc(label)}<select name="${name}">${items.map(([v, t]) => `<option value="${esc(v)}" ${String(v) === String(value) ? "selected" : ""}>${esc(t)}</option>`).join("")}</select></label>`;
  }
  function form(kind, body, extra = "") {
    return `<form data-kind="${kind}" ${extra}>${body}<button type="submit">Guardar</button>${button("home", "Cancelar", "", "secondary")}</form>`;
  }
  const admin = () => {
    if (!user?.is_admin) throw Error("Esta acción requiere administrador.");
  };
  function notice() {
    return (
      "Turno activo · " +
      (user ? user.display_name : "Acceso") +
      " · " +
      root.name
    );
  }
  async function confirm(title, text) {
    const d = $("#confirmation");
    $("#dialog-title").textContent = title;
    $("#dialog-text").textContent = text;
    d.returnValue = "cancel";
    d.showModal();
    return new Promise((resolve) =>
      d.addEventListener("close", () => resolve(d.returnValue === "ok"), {
        once: true,
      }),
    );
  }
  async function run(fn) {
    if (busy) return;
    busy = true;
    document.body.setAttribute("aria-busy", "true");
    try {
      await fn();
    } catch (e) {
      status(
        e?.name === "AbortError"
          ? "Operación cancelada."
          : e.message || String(e),
        true,
      );
    } finally {
      busy = false;
      document.body.removeAttribute("aria-busy");
    }
  }
  async function handle(path, create = false) {
    if (!M.safePath(path)) throw Error("Ruta de datos no segura.");
    const parts = path.split("/");
    let dir = root;
    for (const part of parts.slice(0, -1))
      dir = await dir.getDirectoryHandle(part, { create });
    return dir.getFileHandle(parts.at(-1), { create });
  }
  async function read(path, optional = false) {
    try {
      return new Uint8Array(
        await (await (await handle(path)).getFile()).arrayBuffer(),
      );
    } catch (e) {
      if (optional && e.name === "NotFoundError") return null;
      throw e;
    }
  }
  async function write(path, data) {
    const h = await handle(path, true),
      w = await h.createWritable();
    try {
      await w.write(data);
      await w.close();
    } catch (e) {
      try {
        await w.abort();
      } catch (_) {}
      throw e;
    }
  }
  async function remove(path) {
    const parts = path.split("/");
    let d = root;
    for (const p of parts.slice(0, -1)) d = await d.getDirectoryHandle(p);
    await d.removeEntry(parts.at(-1));
  }
  const json = (b) => JSON.parse(A.text(b));
  async function getLease() {
    const b = await read("data/browser-turn.json", true);
    if (!b) return null;
    try {
      return json(b);
    } catch (_) {
      throw Error(
        "El archivo de turno está dañado. Revise que nadie esté trabajando antes de recuperarlo.",
      );
    }
  }
  async function acquire() {
    const previous = await getLease();
    if (previous) {
      if (new Date(previous.expires_at) > new Date())
        throw Error(
          "Otra sesión tiene el turno activo. Termine su turno en ese navegador antes de continuar.",
        );
      if (
        !(await confirm(
          "Turno anterior interrumpido",
          "El turno anterior ha vencido. Confirme con las demás personas que la aplicación está cerrada antes de recuperar el turno.",
        ))
      )
        throw Error("No se ha tomado el turno.");
    }
    token = M.id();
    await writeLease();
    await new Promise((r) => setTimeout(r, 150));
    await assertLease();
    timer = setInterval(() => {
      if (!busy && !blocked)
        run(async () => {
          await assertLease();
          await writeLease();
        });
    }, 60000);
    $("#close").hidden = false;
  }
  async function writeLease() {
    await write(
      "data/browser-turn.json",
      JSON.stringify(
        {
          token,
          app_version: M.VERSION,
          expires_at: new Date(Date.now() + 10 * 60000).toISOString(),
        },
        null,
        2,
      ),
    );
  }
  async function assertLease() {
    if (blocked)
      throw Error(
        "El guardado está bloqueado. Termine y vuelva a abrir el turno.",
      );
    const l = await getLease();
    if (!l || l.token !== token || new Date(l.expires_at) <= new Date()) {
      blocked = true;
      throw Error(
        "Se ha perdido o ha vencido el turno. No se han sobrescrito los datos. Termine y vuelva a abrir la carpeta.",
      );
    }
  }
  async function unchanged() {
    await assertLease();
    const b = await read("data/store.json", true),
      c = await read("config.json", true);
    if (
      (b ? await M.digest(b) : "") !== snapshot ||
      (c ? await M.digest(c) : "") !== configSnapshot
    ) {
      blocked = true;
      throw Error(
        "Los datos o la configuración han cambiado desde otra sesión. El guardado se ha cancelado para no sobrescribirlos. Termine el turno y vuelva a abrir la carpeta.",
      );
    }
  }
  async function release() {
    clearInterval(timer);
    try {
      const l = await getLease();
      if (l?.token === token) await remove("data/browser-turn.json");
    } catch (e) {
      status(
        "No se pudo liberar el turno. El aviso vencerá en diez minutos.",
        true,
      );
      return false;
    }
    token = null;
    return true;
  }
  async function walkPhotos() {
    let dir;
    try {
      dir = await root.getDirectoryHandle("data");
      dir = await dir.getDirectoryHandle("photos");
    } catch (e) {
      if (e.name === "NotFoundError") return {};
      throw e;
    }
    const out = {};
    let total = 0;
    async function visit(d, prefix) {
      for await (const [name, h] of d.entries()) {
        if (!M.safePath(name)) throw Error("Foto con ruta no admitida.");
        if (h.kind === "directory") await visit(h, prefix + name + "/");
        else {
          const f = await h.getFile();
          total += f.size;
          if (total > 128 * 1024 * 1024)
            throw Error("Las fotos superan el límite de 128 MB.");
          out["data/photos/" + prefix + name] = new Uint8Array(
            await f.arrayBuffer(),
          );
        }
      }
    }
    await visit(dir, "");
    return out;
  }
  async function backup(reason) {
    await unchanged();
    if (!store) throw Error("No hay una base de datos que respaldar.");
    const files = {
      ...(await walkPhotos()),
      "data/store.json": await read("data/store.json"),
      "config.json":
        (await read("config.json", true)) ||
        encoder.encode(JSON.stringify(cfg)),
    };
    const bytes = await A.backup(files, reason);
    const path = "data/backups/browser_" + Date.now() + "_" + M.id() + ".zip";
    await write(path, bytes);
    await unchanged();
    return { bytes, path };
  }
  async function save(
    next,
    reason = "antes de guardar",
    photos = {},
    newCfg = cfg,
  ) {
    M.validate(next);
    await unchanged();
    let previous = null;
    if (store) previous = await backup(reason);
    const nextText = JSON.stringify(next, null, 2),
      cfgText = JSON.stringify(newCfg, null, 2),
      planned = {
        store: await M.digest(nextText),
        config: await M.digest(cfgText),
        photos: {},
      };
    for (const [n, b] of Object.entries(photos))
      planned.photos[n] = await M.digest(b);
    await write(
      "data/browser-recovery.json",
      JSON.stringify(
        { backup: previous?.path || null, planned, created_at: M.now() },
        null,
        2,
      ),
    );
    try {
      for (const [n, b] of Object.entries(photos)) {
        if (!n.startsWith("data/photos/") || !M.safePath(n))
          throw Error("Ruta de foto no segura.");
        await write(n, b);
      }
      await unchanged();
      await write("config.json", cfgText);
      await assertLease();
      const current = await read("data/store.json", true);
      if ((current ? await M.digest(current) : "") !== snapshot)
        throw Error("El archivo de datos cambió durante el guardado.");
      await write("data/store.json", nextText);
      const actual = await read("data/store.json");
      if ((await M.digest(actual)) !== planned.store)
        throw Error("No se ha podido verificar el archivo guardado.");
      store = next;
      cfg = newCfg;
      snapshot = planned.store;
      configSnapshot = planned.config;
      await remove("data/browser-recovery.json");
      status("Guardado y copia de seguridad completados. " + notice());
    } catch (e) {
      blocked = true;
      throw Error(
        "Guardado no completado. Se conserva una copia previa y un registro de recuperación. Termine y vuelva a abrir la carpeta. Detalle: " +
          e.message,
      );
    }
  }
  async function recovery() {
    const journal = await read("data/browser-recovery.json", true);
    if (!journal) return;
    const j = json(journal);
    let complete = true;
    for (const [path, want] of [
      ["data/store.json", j.planned?.store],
      ["config.json", j.planned?.config],
      ...Object.entries(j.planned?.photos || {}),
    ]) {
      const b = await read(path, true);
      if (!b || (await M.digest(b)) !== want) complete = false;
    }
    if (complete) {
      await remove("data/browser-recovery.json");
      return;
    }
    if (
      !j.backup ||
      !M.safePath(j.backup) ||
      !j.backup.startsWith("data/backups/")
    )
      throw Error(
        "La creación inicial se interrumpió. Conserve la carpeta y vuelva a intentarlo en una carpeta vacía; no sobrescriba datos existentes.",
      );
    if (
      !(await confirm(
        "Recuperación de un guardado interrumpido",
        "Existe una copia previa verificada. Recuperarla descartará la operación interrumpida. Confirme que nadie más ha utilizado estos archivos desde entonces.",
      ))
    )
      throw Error(
        "Recuperación pendiente. Se conservan los archivos y la copia previa.",
      );
    const b = await read(j.backup),
      validated = await A.validateBackup(b);
    await assertLease();
    for (const [n, value] of Object.entries(validated.files)) {
      if (
        n === "data/store.json" ||
        n === "config.json" ||
        n.startsWith("data/photos/")
      )
        await write(n, value);
    }
    await remove("data/browser-recovery.json");
  }
  async function connect() {
    if (!$("#accept").checked)
      throw Error("Confirme primero que vais a trabajar por turnos.");
    if (!window.showDirectoryPicker || !crypto.subtle)
      throw Error(
        "Este navegador no admite el acceso necesario. Use Edge o Chrome actualizado; las políticas de la organización pueden impedirlo.",
      );
    root = await window.showDirectoryPicker({
      mode: "readwrite",
      id: "control-buceadores-folder",
    });
    blocked = false;
    try {
      await acquire();
      await recovery();
      const raw = await read("data/store.json", true),
        c = await read("config.json", true);
      snapshot = raw ? await M.digest(raw) : "";
      configSnapshot = c ? await M.digest(c) : "";
      cfg = c ? json(c) : M.config();
      if (!cfg || typeof cfg !== "object" || Array.isArray(cfg))
        throw Error("Configuración no válida.");
      if (raw) {
        store = M.normalize(json(raw));
        login();
      } else {
        store = null;
        app.innerHTML =
          '<section class="card"><h2>Carpeta sin base de datos</h2><p>No se ha encontrado data/store.json. La creación solo está indicada para una carpeta nueva, no para recuperar una instalación que dé errores.</p>' +
          button("create", "Crear base nueva") +
          button("exit", "Cancelar", "", "secondary") +
          "</section>";
      }
      status(notice());
    } catch (e) {
      await release();
      throw e;
    }
  }
  function login() {
    user = null;
    $("#nav").hidden = true;
    app.innerHTML =
      '<section class="card"><h2>Acceso</h2><form data-kind="login">' +
      input(
        "username",
        "Usuario",
        "",
        "text",
        'required autocomplete="username"',
      ) +
      input(
        "password",
        "Contraseña",
        "",
        "password",
        'required autocomplete="current-password"',
      ) +
      "<button>Entrar</button></form>" +
      button("register", "Solicitar cuenta", "", "secondary") +
      (store.users.some(
        (u) => u.username.toLowerCase() === "admin" && !u.hash_iterations,
      ) || !store.users.length
        ? button(
            "legacyRecovery",
            "Recuperar acceso administrativo V7",
            "",
            "secondary",
          )
        : "") +
      "<p>En una instalación nueva: admin / Zocotroco$01. Los hashes V7 sin iteraciones requieren una recuperación administrativa explícita; no se restablecen contraseñas al abrir los datos.</p></section>";
  }
  function nav() {
    const items = [
      ["home", "Inicio"],
      ["divers", "Buceadores"],
      ["dives", "Inmersiones"],
      ["maintenance", "Mantenimiento"],
      ["monthly", "Informes"],
      ["account", "Mi cuenta"],
    ];
    if (user.is_admin) items.push(["admin", "Administración"]);
    $("#nav").innerHTML =
      items.map(([a, t]) => button(a, t)).join("") +
      button("logout", "Salir de cuenta", "", "secondary");
    $("#nav").hidden = false;
    status(notice());
  }
  function show(name, value = "") {
    if (!user) throw Error("Inicie sesión.");
    view = { name, value };
    for (const u of objectURLs) URL.revokeObjectURL(u);
    objectURLs = [];
    nav();
    if (name === "home") home();
    else if (name === "divers") divers();
    else if (name === "diver") diver(value);
    else if (name === "diverForm") diverForm(value);
    else if (name === "dives") dives();
    else if (name === "diveForm") diveForm(value);
    else if (name === "maintenance") maintenance();
    else if (name === "qual") qual(value);
    else if (name === "monthly") monthly();
    else if (name === "individual") individual(value);
    else if (name === "all") all();
    else if (name === "account") account();
    else if (name === "admin") {
      admin();
      administration();
    } else throw Error("Pantalla no disponible.");
  }
  function home() {
    app.innerHTML = `<div class="grid"><section class="card"><h2>${store.divers.filter((d) => d.active).length}</h2>Buceadores activos</section><section class="card"><h2>${store.dives.length}</h2>Inmersiones</section><section class="card"><h2>${esc(cfg.unit_name)}</h2>Datos guardados en la carpeta seleccionada</section></div><section class="card"><h2>Trabajo por turnos</h2><p>Termine el turno con el botón superior antes de que otra persona abra los datos. No abra la edición Go al mismo tiempo.</p><p>Las copias se guardan en data/backups. Esta aplicación no hace conexiones de red.</p></section>`;
  }
  function divers() {
    app.innerHTML =
      '<section class="card"><h2>Buceadores</h2>' +
      button("diverForm", "Nuevo buceador") +
      '<div class="table"><table><thead><tr><th>ID</th><th>Empleo</th><th>Nombre</th><th>Autónomo</th><th>SDS Aire</th><th>SDS HELIOX</th><th>Activo</th><th>Acciones</th></tr></thead><tbody>' +
      [...store.divers]
        .sort((a, b) => a.name.localeCompare(b.name))
        .map(
          (d) =>
            `<tr><td>${esc(d.id)}</td><td>${esc(d.rank)}</td><td>${esc(d.name)}</td><td>${d.qual_basic ? "Sí" : "No"}</td><td>${d.qual_sds_air ? "Sí" : "No"}</td><td>${d.qual_sds_heliox ? "Sí" : "No"}</td><td>${d.active ? "Sí" : "No"}</td><td>${button("diver", "Ver", d.id)}${button("diverForm", "Editar", d.id)}</td></tr>`,
        )
        .join("") +
      "</tbody></table></div></section>";
  }
  function diver(id) {
    const d = store.divers.find((d) => d.id === id);
    if (!d) throw Error("Buceador no encontrado.");
    app.innerHTML =
      `<section class="card"><h2>${esc(d.rank)} ${esc(d.name)}</h2><div id="photo"></div><p>${esc(d.notes)}</p>${button("diverForm", "Editar", d.id)}${button("individual", "Imprimir registro", d.id)}${user.is_admin ? button("deleteDiver", "Eliminar buceador", d.id, "danger") : ""}</section>` +
      diveTable(
        store.dives.filter((x) => x.diver_id === id),
        false,
      );
    loadPhoto(d);
  }
  async function loadPhoto(d, container = "#photo") {
    if (!d.photo) return;
    try {
      const bytes = await read("data/photos/" + d.photo, true);
      if (!bytes) return;
      const url = URL.createObjectURL(new Blob([bytes]));
      objectURLs.push(url);
      const target = $(container);
      if (target) {
        const img = document.createElement("img");
        img.className = "photo";
        img.alt = "Foto de " + d.name;
        img.src = url;
        target.appendChild(img);
      }
    } catch (_) {
      status(
        "No se pudo cargar la foto. El resto del registro sigue disponible.",
        true,
      );
    }
  }
  function diverForm(id) {
    const d = store.divers.find((d) => d.id === id) || { active: true };
    app.innerHTML =
      '<section class="card"><h2>' +
      (id ? "Editar buceador" : "Nuevo buceador") +
      "</h2>" +
      form(
        "diver",
        `<input type="hidden" name="edit" value="${esc(id)}">${input("id", "ID", d.id, "text", `required maxlength="80" pattern="[A-Za-z0-9_-]+" ${id ? "readonly" : ""}`)}${input("rank", "Empleo", d.rank)}${input("name", "Nombre y apellidos", d.name, "text", "required")}${input("entry_date", "Fecha de entrada", d.entry_date, "date")}${check("qual_basic", "Autónomo", d.qual_basic)}${check("qual_sds_air", "SDS Aire", d.qual_sds_air)}${check("qual_sds_heliox", "SDS HELIOX", d.qual_sds_heliox)}${check("active", "Activo", d.active)}<label>Foto opcional (PNG/JPEG, máximo 8 MB)<input type="file" name="photo" accept="image/png,image/jpeg"></label><label>Observaciones<textarea name="notes">${esc(d.notes)}</textarea></label>`,
      ) +
      "</section>";
  }
  const names = (id) => store.divers.find((d) => d.id === id)?.name || id;
  function diveTable(list, actions = true) {
    return (
      '<div class="card table"><table><thead><tr><th>Fecha</th><th>Buceador</th><th>Cualificación</th><th>Tipo</th><th>IB</th><th>Gas</th><th>Metros</th><th>Minutos</th><th>Válida</th>' +
      (actions ? '<th class="no-print">Acciones</th>' : "") +
      "</tr></thead><tbody>" +
      [...list]
        .sort((a, b) => b.date.localeCompare(a.date))
        .map(
          (d) =>
            `<tr><td>${esc(d.date)}</td><td>${esc(names(d.diver_id))}</td><td>${esc(d.qualification)}</td><td>${esc(d.dive_type)}</td><td>${esc(d.ib_code)}</td><td>${esc(d.gas)}</td><td>${d.max_depth}</td><td>${d.bottom_time}</td><td>${d.valid ? "Sí" : "No"}</td>${actions ? `<td class="no-print">${button("diveForm", "Editar", d.id)}${user.is_admin ? button("deleteDive", "Eliminar", d.id, "danger") : ""}</td>` : ""}</tr>`,
        )
        .join("") +
      "</tbody></table></div>"
    );
  }
  function dives() {
    app.innerHTML =
      '<section class="card"><h2>Inmersiones</h2>' +
      button("diveForm", "Nueva inmersión") +
      button("all", "Histórico imprimible", "", "secondary") +
      "</section>" +
      diveTable(store.dives);
  }
  function diveForm(id) {
    const d = store.dives.find((d) => d.id === Number(id)) || {
      valid: true,
      date: M.today(),
      dive_type: M.ib[0][1],
      max_depth: 0,
      bottom_time: 0,
      gas: "AIRE",
      qualification: "Autónomo CA aire/nitrox básico",
    };
    app.innerHTML =
      '<section class="card"><h2>' +
      (id ? "Editar inmersión" : "Nueva inmersión") +
      "</h2>" +
      form(
        "dive",
        `<input type="hidden" name="edit" value="${esc(id)}">${select(
          "diver_id",
          "Buceador",
          store.divers
            .filter((x) => x.active || x.id === d.diver_id)
            .map((x) => [x.id, x.id + " · " + x.name]),
          d.diver_id,
        )}${input("date", "Fecha", d.date, "date", `required max="${M.today()}"`)}${select(
          "qualification",
          "Cualificación",
          ["Autónomo CA aire/nitrox básico", "SDS aire", "SDS HELIOX"].map(
            (x) => [x, x],
          ),
          d.qualification,
        )}${select(
          "dive_type",
          "Tipo de inmersión",
          M.ib.map(([code, label]) => [label, label + " · " + code]),
          d.dive_type,
        )}${input("ib_code", "Código IB (calculado)", M.code(d.dive_type), "text", "readonly")}${input("breathing_mode", "Medio respiratorio", d.breathing_mode)}${input("gas", "Gas / mezcla", d.gas)}${check("surface_supply", "Suministro desde superficie", d.surface_supply)}${input("max_depth", "Profundidad máxima (m)", d.max_depth, "number", 'required min="0" step="0.1"')}${input("bottom_time", "Tiempo de fondo (min)", d.bottom_time, "number", 'required min="0" step="1"')}${input("purpose", "Finalidad / clasificación", d.purpose)}<label>Observaciones<textarea name="notes">${esc(d.notes)}</textarea></label>${check("valid", "Registro válido para mantenimiento", d.valid)}`,
      ) +
      "</section>";
  }
  const badge = (q) =>
    `<span class="badge ${q.status.replaceAll(" ", "_")}">${esc(q.status)}</span>`;
  function maintenance() {
    app.innerHTML =
      '<section class="card"><h2>Mantenimiento de cualificaciones</h2>' +
      button("print", "Imprimir control") +
      '<div class="table"><table id="maintenance"><thead><tr><th>Empleo</th><th>Nombre</th><th>Autónomo</th><th>SDS Aire</th><th>SDS HELIOX</th><th class="no-print"></th></tr></thead><tbody>' +
      [...store.divers]
        .filter((d) => d.active)
        .sort((a, b) => a.name.localeCompare(b.name))
        .map(
          (d) =>
            `<tr><td>${esc(d.rank)}</td><td>${esc(d.name)}</td>${M.maintenance(
              d,
              store.dives,
            )
              .map((q) => "<td>" + badge(q) + "</td>")
              .join(
                "",
              )}<td class="no-print">${button("qual", "Detalle", d.id)}</td></tr>`,
        )
        .join("") +
      "</tbody></table></div></section>";
  }
  function qual(id) {
    const d = store.divers.find((d) => d.id === id);
    if (!d) throw Error("Buceador inexistente.");
    app.innerHTML =
      "<h2>" +
      esc(d.name) +
      "</h2>" +
      button("print", "Imprimir ficha") +
      M.maintenance(d, store.dives)
        .map(
          (q) =>
            `<section class="card"><h3>${q.label} ${badge(q)}</h3><p>Última: ${esc(q.last || "Sin datos")} · ${q.count}/${q.required} inmersiones en 12 meses · Requisito específico: ${q.specific ? "cumplido" : "pendiente"} · Límite orientativo: ${esc(q.deadline)}</p>${diveTable(q.dives, false)}</section>`,
        )
        .join("");
  }
  function all() {
    app.innerHTML =
      "<h2>Histórico completo</h2>" +
      button("print", "Imprimir") +
      diveTable(store.dives, false);
  }
  function individual(id) {
    const d = store.divers.find((d) => d.id === id);
    if (!d) throw Error("Buceador inexistente.");
    app.innerHTML =
      '<div class="toolbar no-print">' +
      button("print", "Imprimir") +
      button("togglePhoto", "Mostrar / ocultar foto") +
      "</div><h2>" +
      esc(d.rank) +
      " " +
      esc(d.name) +
      '</h2><div id="photo"></div>' +
      diveTable(
        store.dives.filter((x) => x.diver_id === id),
        false,
      );
    loadPhoto(d);
  }
  function monthly(
    year = Number(M.today().slice(0, 4)),
    month = Number(M.today().slice(5, 7)),
    format = store.default_report_template_id,
  ) {
    if (
      !Number.isInteger(year) ||
      year < 1900 ||
      year > 9999 ||
      !Number.isInteger(month) ||
      month < 1 ||
      month > 12
    )
      throw Error("Mes/año no válido.");
    const t =
        store.report_templates.find((t) => t.id === format) ||
        store.report_templates[0],
      ctx = M.variables(year, month, cfg),
      list = store.dives
        .filter(
          (d) =>
            d.date.slice(0, 7) === `${year}-${String(month).padStart(2, "0")}`,
        )
        .sort(
          (a, b) =>
            names(a.diver_id).localeCompare(names(b.diver_id)) ||
            a.date.localeCompare(b.date),
        );
    monthlyContext = ctx;
    const editable = (key, value) =>
      `data-report="${key}" contenteditable="true" data-raw="${esc(value)}" data-rendered="${esc(M.replace(value, ctx))}"`;
    const field = (tag, key, value, cls = "") =>
      `<${tag} ${editable(key, value)} class="editable ${cls}">${esc(M.replace(value, ctx))}</${tag}>`;
    const fonts = ["Arial", "Times New Roman", "Calibri", "Helvetica"];
    const font = fonts.includes(t.font_family) ? t.font_family : "Arial",
      bodySize = Math.max(6, Math.min(48, Number(t.body_font_size) || 11)),
      tableSize = Math.max(6, Math.min(48, Number(t.table_font_size) || 9)),
      spacing = Math.max(0, Math.min(300, Number(t.signature_top_space) || 50));
    app.innerHTML = `<section class="card no-print"><h2>Informes</h2><form data-kind="monthlyFilter" class="grid">${input("year", "Año", year, "number", 'min="1900" max="9999" required')}${select(
      "month",
      "Mes",
      M.months.map((n, i) => [i + 1, n]),
      month,
    )}${select(
      "format",
      "Formato",
      store.report_templates.map((x) => [x.id, x.name]),
      t.id,
    )}<button>Mostrar</button></form>${button("all", "Todas las inmersiones")}${button("print", "Imprimir relación mensual")}</section><section id="monthly" contenteditable="false" style="font-family:${font};font-size:${bodySize}pt">${field("h2", "title", t.title)}${field("p", "intro_template", t.intro_template)}<table style="font-size:${tableSize}pt"><thead><tr>${["col_name", "col_date", "col_equipment", "col_gas", "col_code"].map((k) => field("th", k, t[k])).join("")}</tr></thead><tbody id="monthly-rows" contenteditable="false">${list.map((d) => `<tr><td>${esc(names(d.diver_id))}</td><td>${esc(d.date)}</td><td>${esc(d.dive_type)}</td><td>${esc(d.gas)}</td><td>${esc(d.ib_code)}</td></tr>`).join("")}</tbody></table>${field("p", "location_template", t.location_template)}<div class="signatures" style="margin-top:${spacing}px"><div>${field("p", "sign1_role", t.sign1_role)}${field("p", "sign1_name", t.sign1_name)}</div><div>${field("p", "sign2_role", t.sign2_role)}${field("p", "sign2_name", t.sign2_name)}</div></div></section>${
      user.is_admin
        ? '<section class="card no-print"><h3>Guardar como nuevo formato</h3><p>Los textos con fechas pueden editarse conservando sus variables en los campos siguientes.</p><form data-kind="template"><input type="hidden" name="source" value="' +
          esc(t.id) +
          '">' +
          input("name", "Nombre del formato", "", "text", "required") +
          select(
            "font_family",
            "Fuente",
            fonts.map((n) => [n, n]),
            font,
          ) +
          input(
            "body_font_size",
            "Tamaño de texto (pt)",
            bodySize,
            "number",
            'min="6" max="48" required',
          ) +
          input(
            "table_font_size",
            "Tamaño de tabla (pt)",
            tableSize,
            "number",
            'min="6" max="48" required',
          ) +
          input(
            "signature_top_space",
            "Separación de firmas (px)",
            spacing,
            "number",
            'min="0" max="300" required',
          ) +
          '<label>Texto inicial con variables<textarea name="intro_template">' +
          esc(t.intro_template) +
          '</textarea></label><label>Localidad y fecha con variables<textarea name="location_template">' +
          esc(t.location_template) +
          "</textarea></label><button>Guardar formato nuevo</button></form></section>"
        : ""
    }`;
  }
  function account() {
    app.innerHTML =
      '<section class="card"><h2>Mi cuenta</h2>' +
      form(
        "account",
        input(
          "display_name",
          "Nombre visible",
          user.display_name,
          "text",
          "required",
        ) +
          input(
            "current_password",
            "Contraseña actual",
            "",
            "password",
            "required",
          ) +
          input(
            "password",
            "Nueva contraseña (opcional)",
            "",
            "password",
            'minlength="8"',
          ),
      ) +
      "</section>";
  }
  function administration() {
    app.innerHTML =
      '<section class="card"><h2>Administración</h2><p class="warning">Los permisos de esta pantalla son de organización del trabajo. No sustituyen los permisos de acceso a la carpeta compartida.</p>' +
      button("downloadBackup", "Crear y descargar backup ZIP") +
      '<form data-kind="restore"><label>Restaurar backup compatible (.zip)<input type="file" name="backup" accept=".zip" required></label><button>Validar y restaurar</button></form><form data-kind="import"><label>Importar CONTROL BUCEADORES.xlsx<input type="file" name="excel" accept=".xlsx" required></label><button>Importar Excel</button></form></section><section class="card"><h2>Usuarios</h2>' +
      store.users
        .map(
          (u) =>
            `<form data-kind="user"><input type="hidden" name="id" value="${esc(u.id)}"><h3>${esc(u.display_name)} (${esc(u.username)})</h3>${select(
              "status",
              "Estado",
              [
                ["pending", "Pendiente"],
                ["approved", "Aprobado"],
                ["rejected", "Rechazado"],
              ],
              u.status,
            )}${check("is_admin", "Administrador", u.is_admin)}${input("reset_password", "Nueva contraseña (opcional; útil para cuentas V7)", "", "password", 'minlength="8" autocomplete="new-password"')}<button>Guardar usuario</button></form>`,
        )
        .join("") +
      '</section><section class="card"><h2>Datos de informes</h2>' +
      form(
        "settings",
        input("unit_name", "Unidad", cfg.unit_name) +
          input("report_location", "Localidad", cfg.report_location),
      ) +
      "</section>";
  }
  function register() {
    app.innerHTML =
      '<section class="card"><h2>Solicitud de cuenta</h2><form data-kind="register">' +
      input("username", "Usuario", "", "text", 'required maxlength="80"') +
      input("display_name", "Nombre visible", "", "text", "required") +
      input(
        "password",
        "Contraseña",
        "",
        "password",
        'required minlength="8"',
      ) +
      "<button>Solicitar cuenta</button></form>" +
      button("login", "Volver al acceso") +
      "</section>";
  }
  async function photo(file) {
    if (!file?.size) return null;
    if (file.size > 8 * 1024 * 1024 || !/^image\/(png|jpeg)$/.test(file.type))
      throw Error("La foto debe ser PNG/JPEG y ocupar menos de 8 MB.");
    const image = await createImageBitmap(file);
    if (
      image.width * image.height > 30000000 ||
      image.width > 12000 ||
      image.height > 12000
    ) {
      image.close();
      throw Error("La imagen es demasiado grande.");
    }
    image.close();
    return {
      name: M.id() + (file.type === "image/png" ? ".png" : ".jpg"),
      bytes: new Uint8Array(await file.arrayBuffer()),
    };
  }
  async function submit(kind, f) {
    const fd = new FormData(f),
      get = (k) => String(fd.get(k) || ""),
      on = (k) => fd.get(k) === "on";
    if (kind === "login") {
      const u = store.users.find(
        (u) =>
          u.username.toLowerCase() === get("username").trim().toLowerCase(),
      );
      if (!u?.hash_iterations)
        throw Error("Credenciales incorrectas o hash legacy sin migración.");
      if (!(await M.verify(u, get("password"))))
        throw Error("Credenciales incorrectas.");
      if (u.status !== "approved")
        throw Error("La cuenta todavía no está aprobada.");
      user = structuredClone(u);
      show("home");
      return;
    }
    if (kind === "monthlyFilter") {
      monthly(Number(get("year")), Number(get("month")), get("format"));
      return;
    }
    if (kind === "register") {
      const name = get("username").trim(),
        display = get("display_name").trim();
      if (!name || !display || get("password").length < 8)
        throw Error("Datos incompletos o contraseña demasiado corta.");
      if (
        store.users.some((u) => u.username.toLowerCase() === name.toLowerCase())
      )
        throw Error("El usuario ya existe.");
      const next = structuredClone(store);
      next.users.push(await M.user(name, display, get("password")));
      await save(next, "antes de registrar solicitud");
      login();
      status("Solicitud guardada. Un administrador debe aprobarla.");
      return;
    }
    if (kind === "legacyRecovery") {
      if (
        !on("authorized") ||
        get("password").length < 8 ||
        get("password") !== get("repeat")
      )
        throw Error(
          "Confirme la autorización y escriba dos veces una contraseña de al menos 8 caracteres.",
        );
      let next = structuredClone(store),
        u = next.users.find((u) => u.username.toLowerCase() === "admin");
      if (u?.hash_iterations)
        throw Error(
          "La cuenta ya tiene una contraseña moderna. Use el acceso normal.",
        );
      if (
        !(await confirm(
          "Recuperación administrativa explícita",
          "Se creará una copia de seguridad de los datos y hashes anteriores. Se cambiará únicamente la contraseña de admin; los demás usuarios y datos se conservarán.",
        ))
      )
        return;
      const replacement = await M.user(
        "admin",
        u?.display_name || "Administrador",
        get("password"),
        true,
      );
      if (u) {
        u.password_salt = replacement.password_salt;
        u.password_hash = replacement.password_hash;
        u.hash_iterations = replacement.hash_iterations;
        u.status = "approved";
        u.is_admin = true;
      } else {
        if (next.users.length)
          throw Error("La base no tiene una cuenta admin recuperable.");
        next.users.push(replacement);
      }
      next.browser_admin_recovery = {
        at: M.now(),
        reason: "recuperación explícita autorizada de cuenta legacy",
      };
      await save(next, "antes de recuperación administrativa V7");
      login();
      status(
        "Contraseña de admin actualizada. Se conserva una copia con los hashes anteriores.",
      );
      return;
    }
    if (!user) throw Error("Inicie sesión.");
    let next = structuredClone(store);
    if (kind === "diver") {
      const id = get("edit") || get("id").trim(),
        name = get("name").trim();
      if (!M.safeID(id) || !name) throw Error("ID o nombre no válido.");
      const old = next.divers.find((d) => d.id === id);
      if (!get("edit") && old) throw Error("El ID ya existe.");
      if (get("edit") && !old) throw Error("El buceador ya no existe.");
      const d = {
        ...(old || {}),
        id,
        name,
        rank: get("rank").trim(),
        entry_date: get("entry_date"),
        notes: get("notes"),
        qual_basic: on("qual_basic"),
        qual_sds_air: on("qual_sds_air"),
        qual_sds_heliox: on("qual_sds_heliox"),
        active: on("active"),
      };
      const img = await photo(fd.get("photo")),
        files = {};
      if (img) {
        d.photo = img.name;
        files["data/photos/" + img.name] = img.bytes;
      }
      if (old) Object.assign(old, d);
      else next.divers.push(d);
      await save(next, "antes de guardar buceador", files);
      show("diver", id);
      return;
    }
    if (kind === "dive") {
      const edit = Number(get("edit")),
        old = next.dives.find((d) => d.id === edit);
      if (edit && !old) throw Error("La inmersión ya no existe.");
      const dt = get("date"),
        depth = Number(get("max_depth").replace(",", ".")),
        time = Number(get("bottom_time")),
        ib = M.code(get("dive_type"));
      if (
        !M.date(dt) ||
        dt > M.today() ||
        !ib ||
        !get("qualification") ||
        !get("max_depth").trim() ||
        !get("bottom_time").trim() ||
        !Number.isFinite(depth) ||
        depth < 0 ||
        !Number.isSafeInteger(time) ||
        time < 0 ||
        !next.divers.some((d) => d.id === get("diver_id"))
      )
        throw Error("Datos de inmersión no válidos.");
      const d = {
        ...(old || {}),
        id: old?.id || next.next_dive_id,
        created_at: old?.created_at || M.now(),
        diver_id: get("diver_id"),
        date: dt,
        qualification: get("qualification"),
        breathing_mode: get("breathing_mode"),
        gas: get("gas"),
        surface_supply: on("surface_supply"),
        max_depth: depth,
        bottom_time: time,
        dive_type: get("dive_type"),
        ib_code: ib,
        purpose: get("purpose"),
        notes: get("notes"),
        valid: on("valid"),
      };
      if (old) Object.assign(old, d);
      else {
        next.next_dive_id++;
        next.dives.push(d);
      }
      await save(next, "antes de guardar inmersión");
      show("dives");
      return;
    }
    if (kind === "account") {
      if (!(await M.verify(user, get("current_password"))))
        throw Error("Contraseña actual incorrecta.");
      const display = get("display_name").trim(),
        password = get("password");
      if (!display || (password && password.length < 8))
        throw Error("Nombre o contraseña no válidos.");
      const u = next.users.find((x) => x.id === user.id);
      u.display_name = display;
      if (password) {
        const n = await M.user(u.username, display, password, u.is_admin);
        u.password_salt = n.password_salt;
        u.password_hash = n.password_hash;
        u.hash_iterations = n.hash_iterations;
      }
      await save(next, "antes de modificar cuenta");
      user = structuredClone(u);
      show("home");
      return;
    }
    admin();
    if (kind === "user") {
      const u = next.users.find((x) => x.id === get("id"));
      if (!u) throw Error("Usuario inexistente.");
      if (!["pending", "approved", "rejected"].includes(get("status")))
        throw Error("Estado no válido.");
      if (u.username.toLowerCase() === "admin") {
        u.status = "approved";
        u.is_admin = true;
      } else {
        u.status = get("status");
        u.is_admin = on("is_admin");
      }
      if (u.status === "approved" && !u.approved_at) u.approved_at = M.now();
      if (get("reset_password")) {
        if (get("reset_password").length < 8)
          throw Error(
            "La contraseña nueva debe tener al menos ocho caracteres.",
          );
        const replacement = await M.user(
          u.username,
          u.display_name,
          get("reset_password"),
          u.is_admin,
        );
        u.password_salt = replacement.password_salt;
        u.password_hash = replacement.password_hash;
        u.hash_iterations = replacement.hash_iterations;
      }
      await save(next, "antes de modificar usuario");
      const current = next.users.find((x) => x.id === user.id);
      if (!current || current.status !== "approved") {
        login();
      } else {
        user = structuredClone(current);
        show(user.is_admin ? "admin" : "home");
      }
      return;
    }
    if (kind === "settings") {
      const c = {
        ...cfg,
        unit_name: get("unit_name").trim(),
        report_location: get("report_location").trim(),
      };
      await save(next, "antes de modificar configuración", {}, c);
      show("admin");
      return;
    }
    if (kind === "template") {
      const source = next.report_templates.find((x) => x.id === get("source"));
      if (!source || !get("name").trim())
        throw Error("Nombre o formato de origen no válido.");
      const t = {
        ...source,
        id: "FMT" + M.id(),
        name: get("name").trim(),
        created_at: M.now(),
      };
      document.querySelectorAll("[data-report]").forEach((el) => {
        const k = el.dataset.report;
        if (k === "intro_template" || k === "location_template") {
          t[k] = get(k);
          return;
        }
        t[k] = editableText(el);
      });
      t.font_family = get("font_family");
      for (const k of [
        "body_font_size",
        "table_font_size",
        "signature_top_space",
      ])
        t[k] = Number(get(k));
      if (
        !["Arial", "Times New Roman", "Calibri", "Helvetica"].includes(
          t.font_family,
        ) ||
        ![t.body_font_size, t.table_font_size, t.signature_top_space].every(
          Number.isFinite,
        ) ||
        t.body_font_size < 6 ||
        t.body_font_size > 48 ||
        t.table_font_size < 6 ||
        t.table_font_size > 48 ||
        t.signature_top_space < 0 ||
        t.signature_top_space > 300
      )
        throw Error("Estilos de formato no válidos.");
      next.report_templates.push(t);
      await save(next, "antes de guardar formato");
      monthly(undefined, undefined, t.id);
      return;
    }
    if (kind === "import") {
      const file = fd.get("excel");
      if (!file?.size || file.size > 128 * 1024 * 1024)
        throw Error("Seleccione un XLSX válido.");
      const data = await A.excel(await file.arrayBuffer());
      let addedD = 0,
        addedI = 0;
      for (const d of data.divers) {
        const old = next.divers.find((x) => x.id === d.id);
        if (old && old.name !== d.name)
          throw Error(
            "Un ID de Excel pertenece a otro nombre. No se ha importado nada.",
          );
        if (!old) {
          next.divers.push(d);
          addedD++;
        }
      }
      const key = (d) =>
        JSON.stringify([
          d.diver_id,
          d.date,
          d.qualification,
          d.breathing_mode || "",
          d.gas,
          d.surface_supply,
          d.max_depth,
          d.bottom_time,
          d.dive_type,
          d.ib_code,
          d.purpose || "",
          d.notes || "",
          d.valid,
        ]);
      const existing = new Set(next.dives.map(key));
      for (const d of data.dives) {
        const k = key(d);
        if (existing.has(k)) continue;
        existing.add(k);
        next.dives.push({ ...d, id: next.next_dive_id++, created_at: M.now() });
        addedI++;
      }
      await save(next, "antes de importar Excel");
      show("dives");
      status(
        `Importación completada: ${addedD} buceadores y ${addedI} inmersiones nuevos.`,
      );
      return;
    }
    if (kind === "restore") {
      const file = fd.get("backup");
      if (!file?.size || file.size > 128 * 1024 * 1024)
        throw Error("Seleccione un backup ZIP válido.");
      const imported = await A.validateBackup(await file.arrayBuffer());
      if (
        !(await confirm(
          "Restaurar copia de seguridad",
          "Se sustituirán los datos actuales por los del backup y se cerrará la sesión. Se guardará primero una copia del estado actual.",
        ))
      )
        return;
      next = M.normalize(imported.store);
      const files = {},
        photoMap = {},
        prefix = "restored_" + M.id() + "_";
      let index = 0;
      for (const [path, b] of Object.entries(imported.files)) {
        if (path.startsWith("data/photos/")) {
          const old = path.slice("data/photos/".length),
            extension = old.match(/\.[A-Za-z0-9]{1,10}$/)?.[0] || ".bin",
            name = prefix + index++ + extension;
          photoMap[old] = name;
          files["data/photos/" + name] = b;
        }
      }
      for (const d of next.divers)
        if (d.photo && photoMap[d.photo]) d.photo = photoMap[d.photo];
      await save(next, "antes de restaurar backup", files, imported.config);
      login();
      status(
        "Restauración completada. Inicie sesión con las cuentas del backup.",
      );
      return;
    }
    throw Error("Formulario no disponible.");
  }
  async function action(name, value) {
    if (name === "connect") {
      await connect();
      return;
    }
    if (name === "exit") {
      if (
        !(await confirm(
          "Terminar turno",
          "Se liberará la carpeta para que otra persona pueda trabajar. Todos los cambios guardados permanecen en ella.",
        ))
      )
        return;
      if (await release()) {
        user = null;
        store = null;
        root = null;
        $("#nav").hidden = true;
        $("#close").hidden = true;
        app.innerHTML =
          '<section class="card"><h2>Turno terminado</h2><p>Puede cerrar esta pestaña. Para otro turno, vuelva a abrir ABRIR.html.</p></section>';
        status("Turno liberado.");
      }
    } else if (name === "create") {
      if (
        !(await confirm(
          "Crear una instalación nueva",
          "Se creará data/store.json en esta carpeta. No utilice esta opción para recuperar archivos existentes.",
        ))
      )
        return;
      await unchanged();
      const fresh = await M.fresh();
      await save(fresh, "creación inicial");
      login();
      status(
        "Base nueva creada. Usuario admin; contraseña inicial Zocotroco$01.",
      );
      return;
    } else if (name === "legacyRecovery") {
      user = null;
      app.innerHTML =
        '<section class="card"><h2>Recuperación administrativa V7</h2><p>Solo para el responsable autorizado de los archivos. No cambia silenciosamente contraseñas ni afecta a otros usuarios. Se conserva una copia previa.</p><form data-kind="legacyRecovery">' +
        input(
          "password",
          "Contraseña nueva",
          "",
          "password",
          'required minlength="8"',
        ) +
        input(
          "repeat",
          "Repita la contraseña",
          "",
          "password",
          'required minlength="8"',
        ) +
        check(
          "authorized",
          "Soy responsable de la carpeta y estoy autorizado para recuperar admin",
          false,
        ) +
        "<button>Recuperar acceso</button></form>" +
        button("login", "Cancelar") +
        "</section>";
    } else if (name === "register") register();
    else if (name === "login" || name === "logout") login();
    else if (name === "print") window.print();
    else if (name === "togglePhoto") $("#photo").hidden = !$("#photo").hidden;
    else if (name === "downloadBackup") {
      admin();
      const b = await backup("manual desde navegador");
      const url = URL.createObjectURL(
          new Blob([b.bytes], { type: "application/zip" }),
        ),
        a = document.createElement("a");
      a.href = url;
      a.download = "control-buceadores-backup.zip";
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 60000);
      status("Backup guardado en la carpeta y descargado.");
    } else if (name === "deleteDiver") {
      admin();
      const d = store.divers.find((d) => d.id === value);
      if (!d) throw Error("Buceador inexistente.");
      if (
        !(await confirm(
          "Eliminar buceador",
          `Se eliminará ${d.name} y ${store.dives.filter((x) => x.diver_id === value).length} inmersiones. Se creará una copia previa.`,
        ))
      )
        return;
      const next = structuredClone(store);
      next.divers = next.divers.filter((x) => x.id !== value);
      next.dives = next.dives.filter((x) => x.diver_id !== value);
      await save(next, "antes de eliminar buceador");
      show("divers");
    } else if (name === "deleteDive") {
      admin();
      if (
        !(await confirm(
          "Eliminar inmersión",
          "Esta eliminación es definitiva. Se creará una copia de seguridad previa.",
        ))
      )
        return;
      const next = structuredClone(store);
      next.dives = next.dives.filter((x) => x.id !== Number(value));
      await save(next, "antes de eliminar inmersión");
      show("dives");
    } else show(name, value);
  }
  document.addEventListener("click", (e) => {
    const b = e.target.closest("button[data-action]");
    if (b) run(() => action(b.dataset.action, b.dataset.value || ""));
  });
  $("#connect").addEventListener("click", () => run(() => connect()));
  $("#close").addEventListener("click", () => run(() => action("exit")));
  document.addEventListener("submit", (e) => {
    if (!e.target.dataset.kind) return;
    e.preventDefault();
    run(() => submit(e.target.dataset.kind, e.target));
  });
  function editableText(el) {
    let value = el.innerText;
    if (value === el.dataset.rendered) return el.dataset.raw;
    for (const [key, text] of Object.entries(monthlyContext).sort(
      (a, b) => b[1].length - a[1].length,
    )) {
      if (text) value = value.split(text).join("{{" + key + "}}");
    }
    return value;
  }
  document.addEventListener("input", (e) => {
    const el = e.target.closest("[data-report]");
    if (
      el &&
      ["intro_template", "location_template"].includes(el.dataset.report)
    ) {
      const box = document.querySelector(
        'textarea[name="' + el.dataset.report + '"]',
      );
      if (box) box.value = editableText(el);
    }
  });
  document.addEventListener("paste", (e) => {
    if (e.target.closest('[contenteditable="true"]')) {
      e.preventDefault();
      document.execCommand(
        "insertText",
        false,
        e.clipboardData.getData("text/plain"),
      );
    }
  });
  document.addEventListener("change", (e) => {
    if (e.target.name === "dive_type") {
      const code = document.querySelector('input[name="ib_code"]');
      if (code) code.value = M.code(e.target.value);
    }
  });
  window.addEventListener("beforeunload", (e) => {
    if (token) {
      e.preventDefault();
      e.returnValue = "Termine el turno antes de cerrar.";
    }
  });
  $("#compat").textContent =
    window.showDirectoryPicker && crypto.subtle
      ? "El navegador dispone de las funciones necesarias. Las políticas corporativas pueden restringir el permiso de carpeta."
      : "Este navegador no ofrece acceso de lectura/escritura a carpetas. No se guardarán datos aquí: pruebe Edge/Chrome actualizado y consulte las políticas de la organización.";
})();
