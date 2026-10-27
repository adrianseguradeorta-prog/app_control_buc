/* Data rules and compatible PBKDF2. No network, framework or runtime dependency. */
"use strict";
window.CBModel = (() => {
  const VERSION = "9.0.0-browser",
    SCHEMA = 4;
  const ib = [
    ["61001", "Autónomo Aire"],
    ["62001", "SDS Aire"],
    ["62002", "SDFS Heliox"],
    ["62003", "Autónomo Nitrox"],
    ["62004", "Autónomo Heliox/Trimix"],
    ["62005", "Semicerrado Nitrox (Buceo técnico)"],
    ["62006", "Semicerrado Trimix"],
    ["63001", "Cerrado Oxígeno"],
    ["64001", "Semicerrado Nitrox (Caza de minas)"],
    ["66001", "Control en Cámara"],
    ["66002", "Tratamiento O2"],
  ];
  const id = () =>
    Array.from(crypto.getRandomValues(new Uint8Array(12)), (x) =>
      x.toString(16).padStart(2, "0"),
    ).join("");
  const now = () => new Date().toISOString();
  const today = () => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  };
  const date = (s) => {
    if (!/^\d{4}-\d\d-\d\d$/.test(s || "")) return null;
    const d = new Date(s + "T00:00:00Z");
    return Number.isFinite(+d) && d.toISOString().slice(0, 10) === s ? d : null;
  };
  const code = (label) =>
    ib.find(
      (x) =>
        x[1].toLocaleLowerCase() === String(label).trim().toLocaleLowerCase() ||
        x[0] === label,
    )?.[0] || "";
  const safeID = (s) =>
    typeof s === "string" && /^[A-Za-z0-9_-]{1,80}$/.test(s);
  const safePath = (s) =>
    typeof s === "string" &&
    s.length > 0 &&
    !s.includes("\\") &&
    !s.includes(":") &&
    !s.startsWith("/") &&
    s.split("/").every((p) => p && p !== "." && p !== "..");
  async function hash(password, salt, iterations = 120000) {
    if (!Number.isInteger(iterations) || iterations < 1 || iterations > 2000000)
      throw Error("Iteraciones de contraseña no admitidas.");
    const e = new TextEncoder(),
      key = await crypto.subtle.importKey(
        "raw",
        e.encode(password),
        "PBKDF2",
        false,
        ["deriveBits"],
      );
    return Array.from(
      new Uint8Array(
        await crypto.subtle.deriveBits(
          { name: "PBKDF2", hash: "SHA-256", salt: e.encode(salt), iterations },
          key,
          256,
        ),
      ),
      (b) => b.toString(16).padStart(2, "0"),
    ).join("");
  }
  async function digest(bytes) {
    return Array.from(
      new Uint8Array(
        await crypto.subtle.digest(
          "SHA-256",
          typeof bytes === "string" ? new TextEncoder().encode(bytes) : bytes,
        ),
      ),
      (b) => b.toString(16).padStart(2, "0"),
    ).join("");
  }
  async function user(username, display, password, admin = false) {
    const salt = btoa(
      String.fromCharCode(...crypto.getRandomValues(new Uint8Array(16))),
    )
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
    return {
      id: id(),
      username,
      display_name: display,
      password_salt: salt,
      password_hash: await hash(password, salt),
      hash_iterations: 120000,
      status: admin ? "approved" : "pending",
      is_admin: admin,
      created_at: now(),
      approved_at: admin ? now() : "",
    };
  }
  async function verify(u, p) {
    return (
      !!u?.hash_iterations &&
      (await hash(p, u.password_salt, u.hash_iterations)).toLowerCase() ===
        u.password_hash.toLowerCase()
    );
  }
  const template = () => ({
    id: "FMT001",
    name: "Relación mensual",
    title: "RELACIÓN MENSUAL DE INMERSIONES",
    intro_template:
      "Durante el mes de {{MES}} del año {{AÑO}}, el personal de {{UNIDAD}} ha efectuado las siguientes inmersiones:",
    col_name: "NOMBRE",
    col_date: "FECHA",
    col_equipment: "EQUIPO",
    col_gas: "MEZCLA",
    col_code: "CÓDIGO",
    location_template: "En {{LOCALIDAD}}, a {{FECHA_FIN_MES}}.",
    sign1_role: "Oficial de Buceo",
    sign1_name: "",
    sign2_role: "V.º B.º",
    sign2_name: "",
    font_family: "Arial",
    body_font_size: 11,
    table_font_size: 9,
    signature_top_space: 50,
    created_at: now(),
  });
  const config = () => ({
    app_name: "Control de Buceadores",
    unit_name: "Unidad local",
    report_location: "",
    sign1_role: "Oficial de Buceo",
    sign1_name: "",
    sign2_role: "V.º B.º",
    sign2_name: "",
  });
  async function fresh() {
    return {
      schema_version: SCHEMA,
      next_dive_id: 1,
      users: [await user("admin", "Administrador", "Zocotroco$01", true)],
      divers: [],
      dives: [],
      report_templates: [template()],
      default_report_template_id: "FMT001",
    };
  }
  function validate(s) {
    if (!s || ![3, 4].includes(s.schema_version))
      throw Error("Esquema de datos no compatible (solo 3 y 4).");
    for (const k of ["users", "divers", "dives", "report_templates"])
      if (s[k] != null && !Array.isArray(s[k]))
        throw Error("Colección de datos no válida: " + k);
    const divers = s.divers || [],
      dives = s.dives || [],
      users = s.users || [];
    const ids = new Set();
    for (const d of divers) {
      if (!safeID(d.id) || ids.has(d.id) || !String(d.name || "").trim())
        throw Error("Buceador sin ID/nombre válido o duplicado.");
      ids.add(d.id);
      if (d.entry_date && !date(d.entry_date))
        throw Error("Fecha de entrada no válida.");
      if (d.photo && !safePath(d.photo)) throw Error("Ruta de foto no válida.");
    }
    const divesIDs = new Set();
    for (const d of dives) {
      if (
        !Number.isSafeInteger(d.id) ||
        d.id < 1 ||
        divesIDs.has(d.id) ||
        !ids.has(d.diver_id) ||
        !date(d.date) ||
        !Number.isFinite(d.max_depth) ||
        d.max_depth < 0 ||
        !Number.isSafeInteger(d.bottom_time) ||
        d.bottom_time < 0
      )
        throw Error("Inmersión no válida, duplicada o sin buceador.");
      divesIDs.add(d.id);
      if (d.ib_code && !ib.some((x) => x[0] === d.ib_code))
        throw Error("Código IB no admitido.");
      if (code(d.dive_type) && code(d.dive_type) !== d.ib_code)
        throw Error("Código IB incoherente.");
    }
    const names = new Set(),
      uIDs = new Set();
    for (const u of users) {
      const n = String(u.username || "")
        .trim()
        .toLowerCase();
      if (
        !n ||
        !u.id ||
        names.has(n) ||
        uIDs.has(u.id) ||
        !["pending", "approved", "rejected"].includes(u.status)
      )
        throw Error("Usuario no válido o duplicado.");
      if (n === "admin" && (!u.is_admin || u.status !== "approved"))
        throw Error("admin debe permanecer aprobado y administrador.");
      names.add(n);
      uIDs.add(u.id);
    }
    return s;
  }
  function normalize(s) {
    validate(s);
    s = structuredClone(s);
    for (const k of ["users", "divers", "dives", "report_templates"])
      s[k] ??= [];
    s.next_dive_id = Math.max(
      1,
      Number.isSafeInteger(s.next_dive_id) ? s.next_dive_id : 1,
    );
    for (const d of s.dives)
      s.next_dive_id = Math.max(s.next_dive_id, d.id + 1);
    if (!s.report_templates.length) {
      s.report_templates.push(template());
      s.default_report_template_id = "FMT001";
    }
    return s;
  }
  function maintenance(diver, dives, asof = today()) {
    const end = date(asof);
    if (!end) throw Error("Fecha de cálculo no válida.");
    const cut = new Date(end);
    cut.setUTCFullYear(cut.getUTCFullYear() - 1);
    return [
      ["basic", "Autónomo", diver.qual_basic, 3],
      ["sds_air", "SDS Aire", diver.qual_sds_air, 2],
      ["sds_heliox", "SDS HELIOX", diver.qual_sds_heliox, 3],
    ].map(([key, label, assigned, required]) => {
      let q = {
        key,
        label,
        required,
        status: "NO ASIGNADA",
        count: 0,
        last: "",
        deadline: "",
        specific: false,
        dives: [],
      };
      if (!assigned) return q;
      let all = dives
        .filter((d) => {
          const t = date(d.date),
            qual = String(d.qualification || "").toLowerCase(),
            gas = String(d.gas || "")
              .trim()
              .toUpperCase();
          return (
            d.diver_id === diver.id &&
            d.valid &&
            d.max_depth > 0 &&
            d.bottom_time > 0 &&
            t &&
            t <= end &&
            (key === "basic"
              ? /aut[oó]nomo/.test(qual)
              : key === "sds_air"
                ? qual.includes("sds aire") &&
                  d.surface_supply &&
                  gas === "AIRE"
                : qual.includes("sds heliox"))
          );
        })
        .sort((a, b) => b.date.localeCompare(a.date));
      q.dives = all.filter((d) => date(d.date) >= cut);
      q.count = q.dives.length;
      if (!all.length) {
        q.status = "SIN DATOS";
        return q;
      }
      q.last = all[0].date;
      const limit = date(q.last);
      limit.setUTCFullYear(limit.getUTCFullYear() + 1);
      q.deadline = limit.toISOString().slice(0, 10);
      q.specific =
        key === "basic"
          ? q.dives.some((d) => d.max_depth >= 25 && d.max_depth <= 30)
          : key === "sds_air"
            ? true
            : q.dives.filter(
                (d) => String(d.gas).trim().toUpperCase() === "AIRE",
              ).length >= 2 &&
              q.dives.some(
                (d) =>
                  d.max_depth > 40 &&
                  String(d.gas || "").trim() &&
                  String(d.gas).trim().toUpperCase() !== "AIRE",
              );
      q.status =
        q.count >= required && q.specific
          ? "OK"
          : end - date(q.last) > 2 * 365 * 86400000
            ? "FUERA DE PLAZO"
            : "A RECUPERAR";
      return q;
    });
  }
  const months = [
    "enero",
    "febrero",
    "marzo",
    "abril",
    "mayo",
    "junio",
    "julio",
    "agosto",
    "septiembre",
    "octubre",
    "noviembre",
    "diciembre",
  ];
  const variables = (year, month, c) => ({
    MES: months[month - 1].toUpperCase(),
    AÑO: String(year),
    UNIDAD: c.unit_name || "",
    LOCALIDAD: c.report_location || "",
    FECHA_FIN_MES: `${new Date(Date.UTC(year, month, 0)).getUTCDate()} de ${months[month - 1]} de ${year}`,
  });
  const replace = (s, ctx) =>
    String(s || "").replace(/\{\{([^}]+)\}\}/g, (a, k) => ctx[k] ?? a);
  return {
    VERSION,
    SCHEMA,
    ib,
    id,
    now,
    today,
    date,
    code,
    safeID,
    safePath,
    hash,
    digest,
    user,
    verify,
    template,
    config,
    fresh,
    validate,
    normalize,
    maintenance,
    months,
    variables,
    replace,
  };
})();
