// ui.js — the pure half of the browser UI: routing, layout, scene building and
// formatting. It touches neither the DOM nor fetch(), so `node --test` can
// cover it; ui.html keeps only the parts that need a document.
//
// The file is served as /ui.js, inlined into the single-file export, and
// loaded by node --test through module.exports.
(function (root, factory) {
  "use strict";
  var api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.ArchView = api;
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  var COLS = 3;

  function esc(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function cut(s, n) {
    s = String(s == null ? '' : s);
    if (s.length <= n) return s;
    var head = s.slice(0, n - 1);
    var sp = head.lastIndexOf(' ');
    if (sp > n * 0.5) head = head.slice(0, sp);
    return head + '…';
  }
  function cutOld(s, n) {
    s = String(s == null ? "" : s);
    return s.length <= n ? s : s.slice(0, n - 1) + "…";
  }

  function short(p) {
    return String(p).split("/").pop();
  }

  // relPath strips the module prefix so the UI shows "internal/wiki" rather
  // than the full import path.
  function relPath(modelPath, p) {
    if (!modelPath) return p;
    var pre = modelPath + "/";
    return p.indexOf(pre) === 0 ? p.slice(pre.length) : p;
  }

  function visible(list, all) {
    return (list || []).filter(function (x) { return all || x.exported; });
  }

  // withFrom adds the current route to a /source link, so the source page can
  // send the reader back to the exact view it came from. The fragment (the line
  // anchor) must stay last.
  function withFrom(href, from) {
    var h = String(href || "");
    if (h.indexOf("/source?") !== 0) return h;
    var frag = "";
    var i = h.indexOf("#");
    if (i >= 0) {
      frag = h.slice(i);
      h = h.slice(0, i);
    }
    return h + (h.indexOf("?") >= 0 ? "&" : "?") + "from=" + encodeURIComponent(from || "#/") + frag;
  }

  // ---------- layers ----------

  // pkgRel is the module-relative path, "" for the root package.
  function pkgRel(modelPath, pkgPath) {
    if (!modelPath || pkgPath === modelPath) return "";
    return relPath(modelPath, pkgPath);
  }

  function normalizeLayers(layers) {
    return (layers || []).map(function (layer) {
      return (layer || []).map(function (name) {
        return String(name).replace(/^\/+/, "").replace(/\/+$/, "");
      }).filter(Boolean);
    });
  }

  // layerOf returns the innermost layer a package belongs to, or -1. A layer
  // matches a whole path segment, so "web" hits "internal/web" but not
  // "internal/webhook".
  function layerOf(layers, modelPath, pkgPath) {
    var rel = pkgRel(modelPath, pkgPath);
    if (!rel) return -1;
    var segs = rel.split("/");
    var best = -1;
    normalizeLayers(layers).forEach(function (layer, i) {
      for (var k = 0; k < layer.length; k++) {
        if (layer[k] === rel || segs.indexOf(layer[k]) >= 0) {
          if (best === -1 || i < best) best = i;
          return;
        }
      }
    });
    return best;
  }

  // layerViolation reports an inner layer depending on an outer one: layers
  // are listed inner first, so a dependency from a lower index to a higher one
  // points the wrong way.
  function layerViolation(layers, modelPath, from, to) {
    if (!layers || !layers.length) return false;
    var a = layerOf(layers, modelPath, from);
    var b = layerOf(layers, modelPath, to);
    if (a < 0 || b < 0 || a === b) return false;
    return a < b;
  }

  // ---------- routing ----------

  function parseHash(hash) {
    var h = hash || "#/";
    var parts = h.slice(2).split("/");
    if (parts[0] === "pkg" && parts[1]) {
      return { view: "pkg", path: decodeURIComponent(parts.slice(1).join("/")) };
    }
    if (parts[0] === "type" && parts[1]) {
      return { view: "type", id: decodeURIComponent(parts.slice(1).join("/")) };
    }
    if (parts[0] === "func" && parts[1]) {
      return { view: "func", id: decodeURIComponent(parts.slice(1).join("/")) };
    }
    if (parts[0] === "value" && parts[1]) {
      return { view: "value", id: decodeURIComponent(parts.slice(1).join("/")) };
    }
    return { view: "home" };
  }

  function pkgHash(path) { return "#/pkg/" + encodeURIComponent(path); }
  function typeHash(id) { return "#/type/" + encodeURIComponent(id); }
  function funcHash(id) { return "#/func/" + encodeURIComponent(id); }
  function valueHash(id) { return "#/value/" + encodeURIComponent(id); }

  function parsePos(pos) {
    var s = String(pos == null ? "" : pos);
    var i = s.lastIndexOf(":");
    if (i < 0) return null;
    return { file: s.slice(0, i), line: Number(s.slice(i + 1)) || 0 };
  }

  function sourceHref(pos) {
    var p = parsePos(pos);
    if (!p) return "";
    return "/source?path=" + encodeURIComponent(p.file) + "#L" + encodeURIComponent(p.line);
  }

  // toAbs joins the module root and a model-relative file, normalising
  // separators so Windows and Unix roots build the same string.
  function toAbs(root, file) {
    var r = String(root || "").replace(/\\/g, "/").replace(/\/+$/, "");
    var f = String(file || "").replace(/\\/g, "/").replace(/^\/+/, "");
    if (!r) return f;
    if (!f) return r;
    return r + "/" + f;
  }

  // pathForURL turns an absolute path into the form a URL expects: Windows
  // "E:/x" becomes "/E:/x", Unix paths already start with a slash.
  function pathForURL(abs) {
    var p = String(abs || "").replace(/\\/g, "/");
    return p.charAt(0) === "/" ? p : "/" + p;
  }

  // editorHref picks the jump target for a declaration. "source" keeps the
  // built-in page, an unknown mode falls back to it, and "none" hides the link.
  function editorHref(mode, root, pos) {
    var p = parsePos(pos);
    if (!p) return "";
    switch (mode) {
      case "vscode":
        return "vscode://file" + pathForURL(toAbs(root, p.file)) + ":" + p.line + ":1";
      case "goland":
        return "goland://open?file=" + encodeURIComponent(toAbs(root, p.file)) + "&line=" + p.line;
      case "file":
        return "file://" + pathForURL(toAbs(root, p.file));
      case "none":
        return "";
      default:
        return sourceHref(pos);
    }
  }

  // ownerTypeID maps "<pkg>#<Type>.<Method>" to "<pkg>#<Type>"; a package-level
  // function has no owner and yields "".
  function ownerTypeID(id) {
    var s = String(id || "");
    var hash = s.lastIndexOf("#");
    if (hash < 0) return "";
    var rest = s.slice(hash + 1);
    var dot = rest.indexOf(".");
    if (dot <= 0) return "";
    return s.slice(0, hash + 1) + rest.slice(0, dot);
  }

  // funcName is the readable half of a function ID: "Server.source".
  function funcName(id) {
    var s = String(id || "");
    var hash = s.lastIndexOf("#");
    return hash < 0 ? s : s.slice(hash + 1);
  }

  // ---------- search ----------

  // searchHits scans names, signatures, docs and member lists. Name matches
  // sort above doc matches, so "Resolve" finds wiki.ResolveLink before prose.
  function searchHits(model, query, all) {
    var needle = String(query || "").toLowerCase();
    var hits = [];
    if (!needle) return hits;

    function add(kind, name, href, doc, typeId, hay) {
      if (hay.toLowerCase().indexOf(needle) < 0) return;
      hits.push({
        kind: kind,
        name: name,
        href: href,
        doc: doc || "",
        typeId: typeId || "",
        nameMatch: name.toLowerCase().indexOf(needle) >= 0
      });
    }

    model.packages.forEach(function (p) {
      p.types.forEach(function (t) {
        if (!all && !t.exported) return;
        add("类型", p.name + "." + t.name, typeHash(t.id), t.doc, t.id,
          [t.name, t.doc, (t.fields || []).map(function (f) { return f.name + " " + f.type; }).join(" ")].join(" "));
        (t.methods || []).forEach(function (m) {
          if (!all && !m.exported) return;
          add("方法", t.name + "." + m.name, typeHash(t.id), m.doc, t.id,
            [m.name, m.sig, m.doc].join(" "));
        });
      });
      (p.funcs || []).forEach(function (f) {
        if (!all && !f.exported) return;
        add("函数", p.name + "." + f.name, pkgHash(p.path), f.doc, "",
          [f.name, f.sig, f.doc].join(" "));
      });
      (p.consts || []).forEach(function (c) {
        if (!all && !c.exported) return;
        add("常量", p.name + "." + c.name, pkgHash(p.path), "", "",
          [c.name, c.type, c.value].join(" "));
      });
      (p.vars || []).forEach(function (v) {
        if (!all && !v.exported) return;
        add("变量", p.name + "." + v.name, pkgHash(p.path), "", "",
          [v.name, v.type].join(" "));
      });
    });

    hits.sort(function (a, b) {
      if (a.nameMatch !== b.nameMatch) return a.nameMatch ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    return hits;
  }

  // matchTypeIDs is the set of type nodes a hit list touches, used to fade
  // everything else on the canvas.
  function matchTypeIDs(hits) {
    var set = {};
    (hits || []).forEach(function (h) {
      if (h.typeId) set[h.typeId] = true;
    });
    return set;
  }

  // highlight escapes text and wraps the first match in <mark>.
  function highlight(text, query) {
    var s = String(text == null ? "" : text);
    var needle = String(query || "");
    if (!needle) return esc(s);
    var i = s.toLowerCase().indexOf(needle.toLowerCase());
    if (i < 0) return esc(s);
    return esc(s.slice(0, i)) + "<mark>" + esc(s.slice(i, i + needle.length)) + "</mark>" +
      esc(s.slice(i + needle.length));
  }

  // ---------- indexing ----------

  // indexModel fills the defaults the rest of the UI assumes and returns the
  // lookup tables. It annotates the model in place (t.pkg) on purpose: the
  // inspector reads the back-reference.
  function indexModel(model) {
    var pkgByPath = new Map();
    var typeById = new Map();
    var incoming = new Map();
    var funcById = new Map(); // call-graph labels, keyed by method/function ID
    var funcByID = new Map(); // route lookups: declaration ID -> {pkg, decl}
    var valueByID = new Map();

    model.packages.forEach(function (p) {
      p.types = p.types || [];
      p.funcs = p.funcs || [];
      p.consts = p.consts || [];
      p.vars = p.vars || [];
      p.imports = p.imports || [];
      pkgByPath.set(p.path, p);
      p.types.forEach(function (t) {
        t.pkg = p;
        typeById.set(t.id, t);
        (t.methods || []).forEach(function (m) {
          if (!m.id) return;
          funcById.set(m.id, { id: m.id, name: t.name + "." + m.name, owner: t.id, pkg: p });
        });
      });
      p.funcs.forEach(function (f) {
        // Cards use the declaration ID when the model has one and fall back to
        // the "<pkg>#<Name>" form otherwise, so index the same key the card uses.
        var id = f.id || (p.path + "#" + f.name);
        funcByID.set(id, { pkg: p, decl: f });
        if (!f.id) return;
        funcById.set(f.id, { id: f.id, name: f.name, owner: "", pkg: p });
      });
      (p.consts || []).forEach(function (v) {
        valueByID.set(p.path + "#" + v.name, { pkg: p, decl: v });
      });
      (p.vars || []).forEach(function (v) {
        valueByID.set(p.path + "#" + v.name, { pkg: p, decl: v });
      });
    });

    function add(target, from, label) {
      if (!incoming.has(target)) incoming.set(target, []);
      var list = incoming.get(target);
      for (var i = 0; i < list.length; i++) {
        if (list[i].from === from && list[i].label === label) return;
      }
      list.push({ from: from, label: label });
    }

    typeById.forEach(function (t) {
      (t.fields || []).forEach(function (f) {
        (f.refs || []).forEach(function (r) { add(r, t.id, "字段 " + f.name); });
      });
      (t.methods || []).forEach(function (m) {
        (m.refs || []).forEach(function (r) { add(r, t.id, "方法 " + m.name); });
      });
    });
    model.packages.forEach(function (p) {
      p.funcs.forEach(function (f) {
        (f.refs || []).forEach(function (r) { add(r, p.path, "函数 " + f.name); });
      });
    });

    return {
      pkgByPath: pkgByPath,
      typeById: typeById,
      incoming: incoming,
      funcById: funcById,
      funcByID: funcByID,
      valueByID: valueByID
    };
  }

  // ---------- layout ----------

  // layoutPackages puts a package in the column of its import depth: a
  // package sits one column right of everything it imports.
  function layoutPackages(pkgs, pkgByPath, opts) {
    opts = opts || {};
    var all = !!opts.all;
    var modelPath = opts.modelPath || "";

    var depth = new Map();
    pkgs.forEach(function (p) { depth.set(p.path, 0); });
    for (var round = 0; round < pkgs.length; round++) {
      pkgs.forEach(function (p) {
        (p.imports || []).forEach(function (imp) {
          var q = pkgByPath.get(imp);
          if (!q) return;
          depth.set(p.path, Math.max(depth.get(p.path), depth.get(q.path) + 1));
        });
      });
    }

    var cols = new Map();
    pkgs.forEach(function (p) {
      var d = depth.get(p.path);
      if (!cols.has(d)) cols.set(d, []);
      cols.get(d).push(p);
    });

    var nodes = [];
    var ds = Array.from(cols.keys()).sort(function (a, b) { return a - b; });
    ds.forEach(function (d) {
      var list = cols.get(d).slice().sort(function (a, b) { return a.path.localeCompare(b.path); });
      list.forEach(function (p, i) {
        nodes.push({
          id: p.path,
          kind: "pkg",
          x: 40 + d * 320,
          y: 40 + i * 96,
          w: 240,
          h: 68,
          title: short(p.path),
          sub: relPath(modelPath, p.path),
          lines: [visible(p.types, showsAll(p, all)).length + " 类型 · " +
            visible(p.funcs, showsAll(p, all)).length + " 函数" +
            (valuesOf(p).length ? " · " + valuesOf(p).length + " 变量" : "") +
            (p.main ? " · 程序入口" : "")],
          changed: packageChanged(p),
          entry: !!p.main,
          crap: packageCrap(p),
          href: pkgHash(p.path),
          selected: false
        });
      });
    });
    return nodes;
  }

  // docLine is the first line of a doc comment, or "" when there is none.
  function docLine(doc) {
    var s = String(doc || "").trim();
    if (!s) return "";
    var i = s.indexOf("\n");
    return i < 0 ? s : s.slice(0, i);
  }

  // textRows turns a printed declaration into card rows: the source is what a
  // reader wants on a type card, and the row cap already elides the rest.
  function textRows(text) {
    return String(text || "").split("\n").map(function (l) { return l.replace(/\t/g, "  "); });
  }
  function typeRows(t, all) {
    var rows = [];
    visible(t.fields, all).forEach(function (f) {
      rows.push((f.exported ? "+ " : "- ") + (f.embedded ? f.type : f.name + ": " + f.type));
    });
    visible(t.methods, all).forEach(function (m) {
      rows.push((m.exported ? "+ " : "- ") + m.name + m.sig);
    });
    return rows;
  }

  // ---------- view filters ----------

  // toggleCollapsed folds or unfolds one card, returning a new list so the
  // caller keeps plain state.
  function toggleCollapsed(list, id) {
    var out = (list || []).slice();
    var i = out.indexOf(id);
    if (i >= 0) out.splice(i, 1);
    else out.push(id);
    return out;
  }

  // focusScene keeps the selected node and its one-hop neighbours. Nodes and
  // edges are hidden, never removed from the model.
  function focusScene(scene, id) {
    if (!scene || !id || scene.kind !== "types") return scene;
    var keep = {};
    keep[id] = true;
    (scene.edges || []).forEach(function (e) {
      if (e.from === id) keep[e.to] = true;
      if (e.to === id) keep[e.from] = true;
    });
    var nodes = scene.nodes.filter(function (n) { return !!keep[n.id]; });
    var present = {};
    nodes.forEach(function (n) { present[n.id] = true; });
    return {
      kind: scene.kind,
      key: scene.key + ":focus:" + id,
      nodes: nodes,
      edges: (scene.edges || []).filter(function (e) { return present[e.from] && present[e.to]; }),
      pkg: scene.pkg,
      selected: scene.selected
    };
  }

  // declutterScene hides edges that do not touch the selected node.
  function declutterScene(scene, selected) {
    if (!scene || scene.kind !== "types" || !selected) return scene;
    return {
      kind: scene.kind,
      key: scene.key,
      nodes: scene.nodes,
      edges: (scene.edges || []).filter(function (e) {
        return e.from === selected || e.to === selected;
      }),
      pkg: scene.pkg,
      selected: scene.selected
    };
  }

  // ---------- metrics ----------

  // Bands follow go-crap's default threshold of 30; 5 is where a function is
  // still cheap enough to leave alone.
  var CRAP_WARN = 5;
  var CRAP_BAD = 30;

  function crapLevel(v, thresholds) {
    if (v == null || !isFinite(v)) return "";
    var warn = CRAP_WARN, bad = CRAP_BAD;
    if (thresholds && thresholds.length === 2 && isFinite(thresholds[0]) && isFinite(thresholds[1])) {
      warn = thresholds[0];
      bad = thresholds[1];
    }
    if (v > bad) return "bad";
    if (v > warn) return "warn";
    return "ok";
  }

  // crapThresholds returns [warn, bad] from the config file, or the defaults.
  function crapThresholds(model) {
    var t = model && model.metrics && model.metrics.thresholds;
    if (t && t.length === 2 && isFinite(t[0]) && isFinite(t[1])) return [t[0], t[1]];
    return [CRAP_WARN, CRAP_BAD];
  }

  function formatCrap(v) {
    if (v == null || !isFinite(v)) return "";
    return Number(v).toFixed(1);
  }

  // valueSub is the second line of a value card: the kind, the type and the
  // initialiser when the source gives one.
  function valueSub(v) {
    var parts = [v.kind || "var"];
    if (v.type) parts.push(v.type);
    if (v.value) parts.push("= " + v.value);
    return parts.join(" ");
  }

  // valuesOf lists a package's constants and variables in one slice.
  function valuesOf(p) {
    if (!p) return [];
    return (p.consts || []).concat(p.vars || []);
  }

  // showsAll decides the exported filter per package. A program entry has no
  // exported API by design, so hiding its unexported members would draw an
  // empty card for the one package that holds the wiring.
  function showsAll(p, all) {
    return !!all || !!(p && p.main);
  }

  // packageCrap is the worst score in the package, the same rule the type cards
  // use: a package is as risky as its riskiest function.
  function packageCrap(p) {
    var worst = null;
    (p.funcs || []).forEach(function (f) {
      if (f.metrics && (worst == null || f.metrics.crap > worst)) worst = f.metrics.crap;
    });
    (p.types || []).forEach(function (t) {
      var v = typeCrap(t);
      if (v != null && (worst == null || v > worst)) worst = v;
    });
    return worst;
  }
  // packageChanged reports whether the -diff run touched anything inside the
  // package, so the module view can point at the packages that moved.
  function packageChanged(p) {
    if (!p) return false;
    var types = p.types || [];
    for (var i = 0; i < types.length; i++) {
      if (typeChanged(types[i])) return true;
    }
    var funcs = p.funcs || [];
    for (var j = 0; j < funcs.length; j++) {
      if (funcs[j].changed) return true;
    }
    return false;
  }

  // warningSummary folds the model's warnings into one banner-sized line. They
  // are the only place a failed scan step says so, so the page has to show them.
  function warningSummary(model) {
    var list = (model && model.warnings) || [];
    if (!list.length) return null;
    var first = String(list[0]);
    if (first.length > 120) first = first.slice(0, 117) + "…";
    return { count: list.length, text: first, all: list.join("\n") };
  }

  // typeChanged reports whether the -diff run touched this type or any of its
  // members.
  function typeChanged(t) {
    if (!t) return false;
    if (t.changed) return true;
    var methods = t.methods || [];
    for (var i = 0; i < methods.length; i++) {
      if (methods[i].changed) return true;
    }
    return false;
  }

  // typeCrap is the worst method score: a type is as risky as its worst member.
  function typeCrap(t) {
    var worst = null;
    (t.methods || []).forEach(function (m) {
      if (!m.metrics) return;
      if (worst == null || m.metrics.crap > worst) worst = m.metrics.crap;
    });
    return worst;
  }

  // ---------- type layout ----------

  var LAYOUTS = ["layer", "kind"];
  var CARD_W = 290;
  var PER_ROW = 4;

  function layoutMode(v) {
    return LAYOUTS.indexOf(v) >= 0 ? v : "layer";
  }

  // rankTypes walks the edges until the ranks stop moving: a type that is used
  // by another sits above it, so dependencies end up at the bottom. Cycles are
  // bounded by the number of types.
  function rankTypes(types, edges) {
    var rank = {};
    types.forEach(function (t) { rank[t.id] = 0; });
    for (var round = 0; round < types.length; round++) {
      var changed = false;
      edges.forEach(function (e) {
        if (!(e.from in rank) || !(e.to in rank)) return;
        if (rank[e.to] < rank[e.from] + 1) {
          rank[e.to] = rank[e.from] + 1;
          changed = true;
        }
      });
      if (!changed) break;
    }
    return rank;
  }

  function typeCard(t, h, rows, more, folded) {
    return { t: t, h: h, rows: rows, more: more, folded: folded };
  }

  // layoutTypes places the type cards. Two deterministic modes:
  //
  //   layer (default) — one band per dependency depth, users on top,
  //   dependencies below, four cards per line;
  //   kind — the old grid, grouped struct → interface → type.
  //
  // Folded cards keep only their title, which is a view state: the model is
  // untouched.
  function layoutTypes(pkg, types, opts) {
    opts = opts || {};
    var all = !!opts.all;
    var selected = opts.selected || null;
    var collapsed = opts.collapsed || [];
    var expanded = opts.expanded || [];
    var declutter = !!opts.declutter;
    var mode = layoutMode(opts.layout);
    var edges = opts.edges || [];

    var cards = types.map(function (t) {
      var folded = declutter || collapsed.indexOf(t.id) >= 0;
      var rows = folded ? [] : (t.text ? textRows(t.text) : typeRows(t, all));
      var shown = rows.slice(0, expanded.indexOf(t.id) >= 0 ? rows.length : 7);
      var more = rows.length - shown.length;
      var h = folded ? 44 : 30 + Math.max(1, shown.length) * 17 + (more > 0 ? 17 : 0) + 12;
      return typeCard(t, h, shown, more, folded);
    });

    var placed = [];
    if (mode === "kind") {
      var kindOrder = { struct: 0, interface: 1, type: 2 };
      var sorted = cards.slice().sort(function (a, b) {
        var ka = kindOrder[a.t.kind], kb = kindOrder[b.t.kind];
        if (ka === undefined) ka = 3;
        if (kb === undefined) kb = 3;
        if (ka !== kb) return ka - kb;
        return a.t.name.localeCompare(b.t.name);
      });
      var ys = [];
      for (var c = 0; c < COLS; c++) ys.push(40);
      sorted.forEach(function (card, i) {
        var col = i % COLS;
        placed.push({ card: card, x: 40 + col * (CARD_W + 40), y: ys[col] });
        ys[col] += card.h + 26;
      });
    } else {
      var rank = rankTypes(types, edges);
      var bands = {};
      cards.forEach(function (card) {
        var r = rank[card.t.id] || 0;
        (bands[r] = bands[r] || []).push(card);
      });
      var y = 40;
      Object.keys(bands).map(Number).sort(function (a, b) { return a - b; }).forEach(function (r) {
        var list = bands[r].sort(function (a, b) { return a.t.name.localeCompare(b.t.name); });
        for (var i = 0; i < list.length; i += PER_ROW) {
          var chunk = list.slice(i, i + PER_ROW);
          var maxH = 0;
          chunk.forEach(function (card, k) {
            placed.push({ card: card, x: 40 + k * (CARD_W + 40), y: y });
            if (card.h > maxH) maxH = card.h;
          });
          y += maxH + 24;
        }
        y += 24; // gap between dependency depths
      });
    }

    var nodes = placed.map(function (p) {
      return {
        id: p.card.t.id,
        kind: "type",
        x: p.x,
        y: p.y,
        w: CARD_W,
        h: p.card.h,
        title: p.card.t.name,
        rows: p.card.rows,
        more: p.card.more,
        folded: p.card.folded,
        sub: p.card.t.kind + " · " + pkg.name,
        pos: p.card.t.pos,
        crap: typeCrap(p.card.t),
        changed: typeChanged(p.card.t),
        href: typeHash(p.card.t.id),
        selected: selected === p.card.t.id
      };
    });

    // Function band: package-level functions are nodes too. Go packages are
    // often functions-only (a command, a helpers package), and drawing an empty
    // canvas made those look broken. They sit under the types, four per row.
    var fns = visible(pkg.funcs || [], showsAll(pkg, all));
    // bandY is computed even when there are no functions: the value band below
    // uses it too, and an undefined base turned every coordinate into NaN.
    var bandY = 40;
    nodes.forEach(function (n) { bandY = Math.max(bandY, n.y + n.h + 56); });
    if (fns.length) {
      fns.forEach(function (f, i) {
        var id = f.id || (pkg.path + "#" + f.name);
        nodes.push({
          id: id,
          kind: "func",
          x: 40 + (i % PER_ROW) * (CARD_W + 40),
          y: bandY + Math.floor(i / PER_ROW) * 76,
          w: CARD_W,
          h: 60,
          title: f.name,
          sub: docLine(f.doc) || f.sig,
          pos: f.pos,
          crap: f.metrics ? f.metrics.crap : null,
          changed: !!f.changed,
          href: funcHash(id),
          selected: selected === id
        });
      });
    }
    // Value band: an embed or constant package holds no types and no functions,
    // so it used to draw an empty canvas. Its consts/vars get cards too.
    var values = visible(valuesOf(pkg), showsAll(pkg, all));
    if (values.length) {
      var valueY = fns.length ? bandY + Math.ceil(fns.length / PER_ROW) * 76 : bandY;
      values.forEach(function (v, i) {
        var id = pkg.path + "#" + v.name;
        nodes.push({
          id: id,
          kind: "value",
          x: 40 + (i % PER_ROW) * (CARD_W + 40),
          y: valueY + Math.floor(i / PER_ROW) * 76,
          w: CARD_W,
          h: 60,
          title: v.name,
          sub: valueSub(v),
          pos: v.pos,
          changed: !!v.changed,
          href: valueHash(id),
          selected: selected === id
        });
      });
    }
    return nodes;
  }

  // typePairs lists the reference edges between the given types, deduplicated
  // and without self-loops. Targets outside the package stay in the list; the
  // caller filters them out for drawing.
  function typePairs(types, idx) {
    var out = [];
    var seen = {};
    types.forEach(function (t) {
      (t.fields || []).forEach(function (f) {
        (f.refs || []).forEach(function (r) {
          if (r === t.id) return;
          var k = t.id + ">" + r;
          if (seen[k]) return;
          seen[k] = true;
          out.push({ from: t.id, to: r, ref: true });
        });
      });
    });
    return out;
  }

  // ---------- scene ----------

  function isHome(idx, route) {
    return route.view === "home" || (route.view === "pkg" && !idx.pkgByPath.has(route.path));
  }

  // buildScene turns a model and a route into everything draw() needs. It is
  // pure: same model, route and options give the same scene. view carries the
  // 3.2 filters: {focus, collapsed, declutter} plus the home view's selected
  // package.
  function buildScene(model, idx, route, all, view) {
    view = view || {};
    if (isHome(idx, route)) {
      var pkgs = model.packages.slice().sort(function (a, b) { return a.path.localeCompare(b.path); });
      var selected = view.selected || "";
      if (selected && !idx.pkgByPath.has(selected)) selected = "";
      var nodes = layoutPackages(pkgs, idx.pkgByPath, { modelPath: model.path, all: all });
      // The module view has one selection: the card the reader clicked. It is
      // separate from the route, which only changes on a double-click.
      nodes.forEach(function (n) { n.selected = n.id === selected; });
      var edges = [];
      pkgs.forEach(function (p) {
        (p.imports || []).forEach(function (imp) {
          if (!idx.pkgByPath.has(imp)) return;
          // One direction everywhere: the arrow leaves the dependent and points
          // at what it depends on.
          var edge = { from: p.path, to: imp, ref: false };
          if (layerViolation(model.layers, model.path, p.path, imp)) edge.violation = true;
          edges.push(edge);
        });
      });
      // 3.3: while searching, everything that does not match fades back.
      if (view.query) {
        var matchedPkgs = matchTypeIDs(searchHits(model, view.query, all));
        nodes.forEach(function (n) {
          var p = idx.pkgByPath.get(n.id);
          var hit = (p && p.types ? p.types : []).some(function (t) { return matchedPkgs[t.id]; });
          if (!hit) n.dim = true;
        });
      }
      applyMoved(nodes, view.moved);
      return { kind: "packages", key: "home", nodes: nodes, edges: edges, pkgs: pkgs, axis: "h", selected: selected };
    }

    var pkg;
    if (route.view === "type") {
      var selectedType = idx.typeById.get(route.id);
      pkg = selectedType ? selectedType.pkg : null;
    } else if (route.view === "func") {
      var selectedFunc = idx.funcByID.get(route.id);
      pkg = selectedFunc ? selectedFunc.pkg : null;
    } else if (route.view === "value") {
      var selectedValue = idx.valueByID.get(route.id);
      pkg = selectedValue ? selectedValue.pkg : null;
    } else {
      pkg = idx.pkgByPath.get(route.path);
    }
    if (!pkg) return null;

    var types = visible(pkg.types, all);
    if (route.view === "type") {
      // A type hidden by the export filter must still appear when it is the
      // one the URL points at.
      var chosen = idx.typeById.get(route.id);
      if (chosen && types.indexOf(chosen) < 0) types.push(chosen);
    }

    // A function or value route shows the owning package's usual type view and
    // only marks the declaration the URL names.
    var selectedID = route.view === "type" || route.view === "func" || route.view === "value"
      ? route.id
      : null;

    // Relation pairs are needed before layout (3.1 ranks by them) and again
    // for drawing, so they are computed once.
    var pairs = typePairs(types, idx);
    var tnodes = layoutTypes(pkg, types, {
      expanded: view.expanded,
      all: all,
      selected: selectedID,
      collapsed: view.collapsed,
      declutter: view.declutter,
      layout: view.layout,
      edges: pairs
    });
    var byId = new Map();
    tnodes.forEach(function (n) { byId.set(n.id, n); });

    var tedges = pairs.filter(function (e) {
      return byId.has(e.from) && byId.has(e.to);
    });

    // implements / embeds come from go/types and get their own line style, so
    // a reader can tell the three relations apart at a glance.
    (model.edges || []).forEach(function (e) {
      if (e.kind !== "implements" && e.kind !== "embeds") return;
      if (!byId.has(e.from) || !byId.has(e.to)) return;
      tedges.push({ from: e.from, to: e.to, kind: e.kind });
    });

    // Call edges are aggregated to the type level: a method of A calling a
    // method of B draws one A -> B edge. Off by default (2.2).
    if (view.calls) {
      var callIndex = {};
      (model.calls || []).forEach(function (c) {
        var from = ownerTypeID(c.from);
        var to = ownerTypeID(c.to);
        if (!from || !to || from === to) return;
        if (!byId.has(from) || !byId.has(to)) return;
        var k = from + ">" + to;
        var edge = callIndex[k];
        if (!edge) {
          edge = callIndex[k] = { from: from, to: to, ref: false, call: true, dynamic: true };
          tedges.push(edge);
        }
        // One static call is enough to draw a solid line.
        if (!c.dynamic) edge.dynamic = false;
      });
    }

    if (view.query) {
      var matched = matchTypeIDs(searchHits(model, view.query, all));
      tnodes.forEach(function (n) {
        if (!matched[n.id]) n.dim = true;
      });
    }

    var scene = {
      kind: "types",
      key: "pkg:" + pkg.path,
      nodes: tnodes,
      edges: tedges,
      pkg: pkg,
      selected: selectedID,
      // One flow per view: the layered layout reads top to bottom, the grid
      // reads left to right.
      axis: layoutMode(view.layout) === "kind" ? "h" : "v"
    };
    // Declutter is a type-view filter: a function or value selection has no
    // edges of its own, so leave the package's edges untouched.
    if (view.declutter && route.view === "type") scene = declutterScene(scene, scene.selected);
    if (view.focus) scene = focusScene(scene, view.focus);
    applyMoved(scene.nodes, view.moved);
    return scene;
  }

  // ---------- geometry ----------

  // edgePath routes along the one axis the view was laid out on, so a diagram
  // reads as a single flow instead of a mix of directions:
  //
  //   "v" — top to bottom (layered layout: users above, dependencies below)
  //   "h" — left to right (grid layout, and the package columns)
  //
  // The path always ends on the target, so one arrow marker does the job.
  function edgePath(a, b, axis) {
    if (axis === "h") {
      var right = b.x + b.w / 2 >= a.x + a.w / 2;
      var hx1 = right ? a.x + a.w : a.x;
      var hx2 = right ? b.x : b.x + b.w;
      var hy1 = a.y + a.h / 2;
      var hy2 = b.y + b.h / 2;
      var hmx = (hx1 + hx2) / 2;
      return "M" + hx1 + "," + hy1 + " C" + hmx + "," + hy1 + " " + hmx + "," + hy2 + " " + hx2 + "," + hy2;
    }
    var down = b.y + b.h / 2 >= a.y + a.h / 2;
    var vy1 = down ? a.y + a.h : a.y;
    var vy2 = down ? b.y : b.y + b.h;
    var vx1 = a.x + a.w / 2;
    var vx2 = b.x + b.w / 2;
    var vmy = (vy1 + vy2) / 2;
    return "M" + vx1 + "," + vy1 + " C" + vx1 + "," + vmy + " " + vx2 + "," + vmy + " " + vx2 + "," + vy2;
  }

  // applyMoved drops the positions a user dragged onto the freshly laid out
  // nodes. It mutates in place and returns the scene for convenience.
  function applyMoved(nodes, moved) {
    if (!moved || !nodes) return nodes;
    nodes.forEach(function (n) {
      var p = moved[n.id];
      if (p) {
        n.x = p.x;
        n.y = p.y;
      }
    });
    return nodes;
  }

  // fitTransform centres the scene in the viewport and keeps the zoom between
  // 0.2 and 1.25, so a one-node scene never blows up to fill the screen.
  function fitTransform(nodes, width, height) {
    if (!nodes || !nodes.length) return { k: 1, tx: 0, ty: 0 };
    var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    nodes.forEach(function (n) {
      minX = Math.min(minX, n.x);
      minY = Math.min(minY, n.y);
      maxX = Math.max(maxX, n.x + n.w);
      maxY = Math.max(maxY, n.y + n.h);
    });
    var pad = 50;
    minX -= pad; minY -= pad; maxX += pad; maxY += pad;
    var bw = Math.max(1, maxX - minX), bh = Math.max(1, maxY - minY);
    var W = width || 1000, H = height || 700;
    var k = Math.min(1.25, Math.max(0.2, Math.min((W - 60) / bw, (H - 60) / bh)));
    return {
      k: k,
      tx: (W - bw * k) / 2 - minX * k,
      ty: (H - bh * k) / 2 - minY * k
    };
  }

  return {
    esc: esc,
    cut: cut,
    short: short,
    relPath: relPath,
    visible: visible,
    searchHits: searchHits,
    matchTypeIDs: matchTypeIDs,
    highlight: highlight,
    layerOf: layerOf,
    layerViolation: layerViolation,
    ownerTypeID: ownerTypeID,
    funcName: funcName,
    parseHash: parseHash,
    pkgHash: pkgHash,
    typeHash: typeHash,
    funcHash: funcHash,
    valueHash: valueHash,
    parsePos: parsePos,
    sourceHref: sourceHref,
    withFrom: withFrom,
    toAbs: toAbs,
    pathForURL: pathForURL,
    editorHref: editorHref,
    indexModel: indexModel,
    isHome: isHome,
    layoutPackages: layoutPackages,
    typeRows: typeRows,
    layoutTypes: layoutTypes,
    rankTypes: rankTypes,
    typePairs: typePairs,
    toggleCollapsed: toggleCollapsed,
    focusScene: focusScene,
    declutterScene: declutterScene,
    CRAP_WARN: CRAP_WARN,
    CRAP_BAD: CRAP_BAD,
    crapLevel: crapLevel,
    crapThresholds: crapThresholds,
    formatCrap: formatCrap,
    typeCrap: typeCrap,
    docLine: docLine,
    textRows: textRows,
    typeChanged: typeChanged,
    warningSummary: warningSummary,
    packageChanged: packageChanged,
    packageCrap: packageCrap,
    showsAll: showsAll,
    valuesOf: valuesOf,
    valueSub: valueSub,
    buildScene: buildScene,
    edgePath: edgePath,
    applyMoved: applyMoved,
    fitTransform: fitTransform
  };
});
