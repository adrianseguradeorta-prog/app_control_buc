/* Minimal ZIP reader/writer: stored backups and deflated XLSX, no external code. */
"use strict";
window.CBArchive = (() => {
  const enc = new TextEncoder(),
    dec = new TextDecoder("utf-8", { fatal: true }),
    MAX = 128 * 1024 * 1024;
  const table = Uint32Array.from({ length: 256 }, (_, i) => {
    for (let j = 0; j < 8; j++) i = i & 1 ? 0xedb88320 ^ (i >>> 1) : i >>> 1;
    return i >>> 0;
  });
  const crc = (b) => {
    let c = 0xffffffff;
    for (const x of b) c = table[(c ^ x) & 255] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
  };
  const bytes = (v) =>
    typeof v === "string"
      ? enc.encode(v)
      : v instanceof Uint8Array
        ? v
        : new Uint8Array(v);
  const concat = (arrays) => {
    const n = arrays.reduce((s, b) => s + b.length, 0);
    if (n > MAX * 2) throw Error("Archivo demasiado grande.");
    const out = new Uint8Array(n);
    let p = 0;
    for (const b of arrays) {
      out.set(b, p);
      p += b.length;
    }
    return out;
  };
  const text = (b) =>
    (typeof b === "string" ? b : dec.decode(b)).replace(/^\uFEFF/, "");
  function zip(files) {
    const data = [],
      central = [];
    let pos = 0,
      total = 0;
    for (const [name, value] of Object.entries(files)) {
      if (!CBModel.safePath(name)) throw Error("Ruta ZIP no segura.");
      const n = enc.encode(name),
        b = bytes(value);
      total += b.length;
      if (total > MAX || n.length > 65535)
        throw Error("Backup demasiado grande.");
      const h = new Uint8Array(30 + n.length),
        v = new DataView(h.buffer);
      v.setUint32(0, 0x04034b50, true);
      v.setUint16(4, 20, true);
      v.setUint16(6, 0x800, true);
      v.setUint32(14, crc(b), true);
      v.setUint32(18, b.length, true);
      v.setUint32(22, b.length, true);
      v.setUint16(26, n.length, true);
      h.set(n, 30);
      const c = new Uint8Array(46 + n.length),
        cv = new DataView(c.buffer);
      cv.setUint32(0, 0x02014b50, true);
      cv.setUint16(4, 20, true);
      cv.setUint16(6, 20, true);
      cv.setUint16(8, 0x800, true);
      cv.setUint32(16, crc(b), true);
      cv.setUint32(20, b.length, true);
      cv.setUint32(24, b.length, true);
      cv.setUint16(28, n.length, true);
      cv.setUint32(42, pos, true);
      c.set(n, 46);
      data.push(h, b);
      central.push(c);
      pos += h.length + b.length;
    }
    if (central.length > 10000) throw Error("Demasiados archivos.");
    const c = concat(central),
      end = new Uint8Array(22),
      e = new DataView(end.buffer);
    e.setUint32(0, 0x06054b50, true);
    e.setUint16(8, central.length, true);
    e.setUint16(10, central.length, true);
    e.setUint32(12, c.length, true);
    e.setUint32(16, pos, true);
    const result = concat([...data, c, end]);
    if (result.length > MAX) throw Error("Backup demasiado grande.");
    return result;
  }
  async function unzip(input) {
    const b = bytes(input);
    if (b.length > MAX || b.length < 22)
      throw Error("ZIP inválido o demasiado grande.");
    const v = new DataView(b.buffer, b.byteOffset, b.byteLength);
    let end = -1;
    for (let i = b.length - 22; i >= Math.max(0, b.length - 65557); i--)
      if (
        v.getUint32(i, true) === 0x06054b50 &&
        i + 22 + v.getUint16(i + 20, true) === b.length
      ) {
        end = i;
        break;
      }
    if (end < 0) throw Error("ZIP sin directorio válido.");
    const count = v.getUint16(end + 10, true),
      size = v.getUint32(end + 12, true),
      start = v.getUint32(end + 16, true);
    if (
      count > 10000 ||
      v.getUint16(end + 4, true) ||
      v.getUint16(end + 6, true) ||
      count !== v.getUint16(end + 8, true) ||
      start + size > end
    )
      throw Error("ZIP multipartes/ZIP64 no admitido.");
    const files = Object.create(null);
    let p = start,
      total = 0;
    const folded = new Set();
    for (let i = 0; i < count; i++) {
      if (p + 46 > start + size || v.getUint32(p, true) !== 0x02014b50)
        throw Error("Directorio ZIP dañado.");
      const flags = v.getUint16(p + 8, true),
        method = v.getUint16(p + 10, true),
        want = v.getUint32(p + 16, true),
        packed = v.getUint32(p + 20, true),
        length = v.getUint32(p + 24, true),
        nl = v.getUint16(p + 28, true),
        xl = v.getUint16(p + 30, true),
        cl = v.getUint16(p + 32, true),
        offset = v.getUint32(p + 42, true);
      if (p + 46 + nl + xl + cl > start + size) throw Error("ZIP truncado.");
      if (((v.getUint32(p + 38, true) >>> 16) & 0xf000) === 0xa000)
        throw Error("Enlace simbólico no admitido en ZIP.");
      const name = dec.decode(b.subarray(p + 46, p + 46 + nl));
      p += 46 + nl + xl + cl;
      const safe = name.endsWith("/") ? name.slice(0, -1) : name;
      if (
        !CBModel.safePath(safe) ||
        folded.has(name.toLowerCase()) ||
        flags & 1
      )
        throw Error("Ruta ZIP no segura, duplicada o cifrada.");
      folded.add(name.toLowerCase());
      total += length;
      if (total > MAX || packed > MAX || offset + 30 > start)
        throw Error("ZIP demasiado grande o dañado.");
      if (v.getUint32(offset, true) !== 0x04034b50)
        throw Error("Cabecera ZIP no válida.");
      const localName = v.getUint16(offset + 26, true),
        localExtra = v.getUint16(offset + 28, true),
        from = offset + 30 + localName + localExtra;
      if (
        from + packed > start ||
        dec.decode(b.subarray(offset + 30, offset + 30 + localName)) !== name ||
        v.getUint16(offset + 8, true) !== method
      )
        throw Error("Cabeceras ZIP incoherentes.");
      if (name.endsWith("/")) continue;
      let data = b.slice(from, from + packed);
      if (method === 8) {
        if (!window.DecompressionStream)
          throw Error("El navegador no puede descomprimir este ZIP.");
        const reader = new Blob([data])
            .stream()
            .pipeThrough(new DecompressionStream("deflate-raw"))
            .getReader(),
          parts = [];
        let n = 0;
        while (true) {
          const { value, done } = await reader.read();
          if (done) break;
          n += value.length;
          if (n > length || n > MAX) {
            await reader.cancel();
            throw Error("Expansión ZIP excesiva.");
          }
          parts.push(value);
        }
        data = concat(parts);
      } else if (method !== 0) throw Error("Compresión ZIP no admitida.");
      if (data.length !== length || crc(data) !== want)
        throw Error("CRC o tamaño de ZIP incorrecto.");
      files[name] = data;
    }
    if (p !== start + size) throw Error("Directorio ZIP incoherente.");
    return files;
  }
  async function backup(files, reason, version = CBModel.VERSION) {
    if (!files["data/store.json"] || !files["config.json"])
      throw Error("Faltan archivos del backup.");
    const store = JSON.parse(text(files["data/store.json"]));
    CBModel.validate(store);
    const sums = {};
    for (const [n, b] of Object.entries(files)) {
      if (
        n !== "data/store.json" &&
        n !== "config.json" &&
        !n.startsWith("data/photos/")
      )
        throw Error("Archivo de backup no admitido.");
      sums[n] = await CBModel.digest(bytes(b));
    }
    return zip({
      ...files,
      "manifest.json": JSON.stringify(
        {
          app_id: "control-buceadores-bamis",
          app_version: version,
          schema_version: store.schema_version,
          created_at: CBModel.now(),
          reason,
          files_sha256: sums,
        },
        null,
        2,
      ),
    });
  }
  async function validateBackup(b) {
    const files = await unzip(b);
    if (!files["manifest.json"]) throw Error("Backup sin manifest.json.");
    const m = JSON.parse(text(files["manifest.json"]));
    if (
      m.app_id !== "control-buceadores-bamis" ||
      ![3, 4].includes(m.schema_version) ||
      !m.files_sha256 ||
      !m.files_sha256["data/store.json"] ||
      !m.files_sha256["config.json"]
    )
      throw Error("Manifest de backup no compatible.");
    if (Object.keys(files).length !== Object.keys(m.files_sha256).length + 1)
      throw Error("Backup con archivos sin verificar.");
    for (const [n, hash] of Object.entries(m.files_sha256)) {
      if (
        n !== "data/store.json" &&
        n !== "config.json" &&
        !n.startsWith("data/photos/")
      )
        throw Error("Archivo ajeno al backup.");
      if (
        !files[n] ||
        typeof hash !== "string" ||
        (await CBModel.digest(files[n])).toLowerCase() !== hash.toLowerCase()
      )
        throw Error("SHA-256 incorrecto: " + n);
    }
    const store = JSON.parse(text(files["data/store.json"])),
      config = JSON.parse(text(files["config.json"]));
    CBModel.validate(store);
    if (
      store.schema_version !== m.schema_version ||
      !config ||
      typeof config !== "object" ||
      Array.isArray(config)
    )
      throw Error("Esquema o configuración incoherente.");
    return { files, store, config, manifest: m };
  }
  function xml(bytes) {
    const d = new DOMParser().parseFromString(text(bytes), "application/xml");
    if (d.querySelector("parsererror")) throw Error("XML de Excel no válido.");
    return d;
  }
  async function excel(input) {
    const files = await unzip(input),
      w = files["xl/workbook.xml"],
      rels = files["xl/_rels/workbook.xml.rels"];
    if (!w || !rels) throw Error("No es un Excel XLSX compatible.");
    const workbook = xml(w);
    if (
      ["1", "true"].includes(
        workbook.querySelector("workbookPr")?.getAttribute("date1904"),
      )
    )
      throw Error("Calendario Excel 1904 no admitido.");
    const ss = files["xl/sharedStrings.xml"]
      ? [...xml(files["xl/sharedStrings.xml"]).getElementsByTagName("si")].map(
          (n) =>
            [...n.getElementsByTagName("t")].map((t) => t.textContent).join(""),
        )
      : [];
    const targets = new Map(
      [...xml(rels).getElementsByTagName("Relationship")].map((n) => [
        n.getAttribute("Id"),
        n.getAttribute("Target"),
      ]),
    );
    const sheets = {};
    for (const sh of workbook.getElementsByTagName("sheet")) {
      const name = sh.getAttribute("name");
      if (!["BUCEADORES", "INMERSIONES"].includes(name)) continue;
      let target = targets.get(sh.getAttribute("r:id"));
      if (!target) throw Error("Relación Excel no válida.");
      target = target.startsWith("/") ? target.slice(1) : "xl/" + target;
      const data = files[target];
      if (!data) throw Error("Hoja Excel ausente.");
      sheets[name] = [...xml(data).getElementsByTagName("row")]
        .filter((r) => Number(r.getAttribute("r")) >= 5)
        .map((row) => {
          const out = {};
          for (const c of row.getElementsByTagName("c")) {
            const col = c.getAttribute("r").replace(/[0-9]/g, ""),
              type = c.getAttribute("t");
            let value = c.getElementsByTagName("v")[0]?.textContent || "";
            if (type === "s") {
              if (!/^\d+$/.test(value) || ss[Number(value)] === undefined)
                throw Error("Cadena Excel no válida.");
              value = ss[Number(value)];
            } else if (type === "inlineStr")
              value = [...c.getElementsByTagName("t")]
                .map((t) => t.textContent)
                .join("");
            out[col] = value.trim();
          }
          return out;
        });
    }
    if (!sheets.BUCEADORES || !sheets.INMERSIONES)
      throw Error("Faltan hojas BUCEADORES/INMERSIONES.");
    const yes = (v) =>
        ["SI", "SÍ", "1"].includes(String(v || "").toUpperCase()),
      edate = (v) => {
        if (!v) return "";
        if (CBModel.date(v)) return v;
        const n = Number(v);
        if (!Number.isInteger(n) || n < 1 || n > 2958465)
          throw Error("Fecha Excel no válida.");
        return new Date(Date.UTC(1899, 11, 30) + n * 86400000)
          .toISOString()
          .slice(0, 10);
      };
    const divers = sheets.BUCEADORES.filter((r) => r.C).map((r) => ({
      id: r.A,
      rank: r.B || "",
      name: r.C,
      entry_date: edate(r.F),
      qual_basic: yes(r.G),
      qual_sds_air: yes(r.H),
      qual_sds_heliox: yes(r.I),
      active: yes(r.J),
      notes: r.L || "",
    }));
    const dives = sheets.INMERSIONES.filter((r) => r.D).map((r, i) => {
      const q = String(r.E || "").toLowerCase(),
        g = String(r.G || "").toUpperCase();
      let type = "";
      if (q.includes("sds aire") && g === "AIRE") type = "SDS Aire";
      else if (q.includes("sds heliox")) type = "SDFS Heliox";
      else if (/aut[oó]nomo/.test(q) && g === "AIRE") type = "Autónomo Aire";
      else if (/aut[oó]nomo/.test(q) && g.includes("NITROX"))
        type = "Autónomo Nitrox";
      if (!type)
        throw Error("No se puede deducir el tipo IB de una fila de Excel.");
      return {
        id: i + 1,
        source_record: `excel:${i + 5}:${r.A || ""}`,
        diver_id: r.C,
        date: edate(r.D),
        qualification: r.E,
        breathing_mode: r.F || "",
        gas: r.G || "",
        surface_supply: yes(r.H),
        max_depth: Number(String(r.I || "0").replace(",", ".")),
        bottom_time: Number(r.J || 0),
        dive_type: type,
        ib_code: CBModel.code(type),
        purpose: r.K || "",
        notes: r.L || "",
        valid: yes(r.M),
        created_at: "",
      };
    });
    CBModel.validate({ schema_version: 4, users: [], divers, dives });
    return { divers, dives };
  }
  return { zip, unzip, backup, validateBackup, excel, bytes, text };
})();
