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

  // ---------- cards ----------

  // buildCard is the one place a canvas card is assembled. Every field the
  // renderer knows about is defaulted here, so a capability added once reaches
  // all four card kinds instead of being patched into each layout function.
  // drawNode (ui.html) maps these fields to SVG and owns CARD_STYLE.
  //
  //   kind      type | func | value | pkg (an interface is a type card with
  //             its own style entry)
  //   decl      the model declaration the card stands for
  //   pkg       its owning package
  //   opts      view state: all, folded, expanded, selected, modelPath,
  //             editor, root, w, h
  //
  // href is the route a single click selects; drillHref is the double-click
  // target: the editor/source link for a declaration, the module route for a
  // package (so a package card can never open a source page).
  function buildCard(kind, decl, pkg, opts) {
    opts = opts || {};
    var all = !!opts.all;
    var node = {
      id: "",
      kind: kind,
      title: "",
      sub: "",
      rows: [],
      more: 0,
      folded: !!opts.folded,
      crap: null,
      changed: false,
      entry: false,
      selected: false,
      href: "#/",
      drillHref: "",
      pos: "",
      w: CARD_W,
      h: 60
    };

    if (kind === "pkg") {
      node.id = decl.path;
      node.title = short(decl.path);
      // The module view's single click selects a package without changing the
      // route, so its select href is the module view itself.
      node.href = "#/";
      node.drillHref = pkgHash(decl.path);
      node.sub = docLine(decl.doc) || relPath(opts.modelPath || "", decl.path);
      node.rows = [visible(decl.types, showsAll(decl, all)).length + " 类型 · " +
        visible(decl.funcs, showsAll(decl, all)).length + " 函数" +
        (valuesOf(decl).length ? " · " + valuesOf(decl).length + " 变量" : "") +
        (decl.main ? " · 程序入口" : "")];
      node.crap = packageCrap(decl);
      node.changed = packageChanged(decl);
      node.entry = !!decl.main;
      node.selected = opts.selected === node.id;
      node.h = opts.h || 68;
      node.w = opts.w || 240;
      return node;
    }

    if (kind === "type") {
      node.kind = decl.kind === "interface" ? "interface" : "type";
      node.id = decl.id;
      node.title = decl.name;
      node.sub = docLine(decl.doc) || (decl.kind + " · " + pkg.name);
      // A type card's rows are its printed declaration when the model has one,
      // otherwise its fields and methods; the cap keeps a card readable.
      var allRows = node.folded ? [] : (decl.text ? textRows(decl.text) : typeRows(decl, all));
      var limit = opts.expanded ? allRows.length : 7;
      node.rows = allRows.slice(0, limit);
      node.more = allRows.length - node.rows.length;
      node.h = node.folded ? 44 : 30 + Math.max(1, node.rows.length) * 17 + (node.more > 0 ? 17 : 0) + 12;
      node.crap = typeCrap(decl);
      node.changed = typeChanged(decl);
      node.href = typeHash(decl.id);
    } else {
      var id = kind === "func" ? (decl.id || (pkg.path + "#" + decl.name)) : (pkg.path + "#" + decl.name);
      node.id = id;
      node.title = decl.name;
      if (kind === "func") {
        node.sub = docLine(decl.doc) || decl.sig || "";
        node.href = funcHash(id);
        node.crap = decl.metrics ? decl.metrics.crap : null;
      } else {
        node.sub = docLine(decl.doc) || valueSub(decl);
        node.href = valueHash(id);
      }
      node.changed = !!decl.changed;
      node.h = opts.h || 60;
    }

    node.pos = decl.pos || "";
    node.drillHref = editorHref(opts.editor, opts.root, node.pos);
    node.selected = opts.selected === node.id;
    if (opts.w) node.w = opts.w;
    if (opts.h) node.h = opts.h;
    return node;
  }

  // Thin factories keep the layout call sites short; the defaults stay in
  // buildCard.
  function typeCard(t, pkg, opts) { return buildCard("type", t, pkg, opts); }
  function funcCard(f, pkg, opts) { return buildCard("func", f, pkg, opts); }
  function valueCard(v, pkg, opts) { return buildCard("value", v, pkg, opts); }
  function packageCard(p, opts) { return buildCard("pkg", p, p, opts); }

  // ---------- layout ----------

  // flowAxis is the one decision that fixes the direction of a scene:
  // package columns and the kind grid read left to right, the dependency
  // layered layout reads top to bottom. buildScene passes the result to every
  // layout function, so no later stage can quietly pick a different flow.
  function flowAxis(kind, layout) {
    if (kind === "packages") return "h";
    return layoutMode(layout) === "kind" ? "h" : "v";
  }

  // layoutPackages puts a package in the column of its import depth: a
  // package sits one column right of everything it imports. The layer is the
  // depth, so every node in a layer shares the flow coordinate (`x` for the
  // horizontal flow this view always uses).
  function layoutPackages(pkgs, pkgByPath, opts) {
    opts = opts || {};
    var all = !!opts.all;
    var modelPath = opts.modelPath || "";
    var axis = opts.axis === "v" ? "v" : "h";

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
        var n = packageCard(p, { modelPath: modelPath, all: all, selected: opts.selected || "" });
        if (axis === "h") {
          n.x = 40 + d * 320;
          n.y = 40 + i * 96;
        } else {
          n.x = 40 + i * 320;
          n.y = 40 + d * 96;
        }
        n.layer = d;
        nodes.push(n);
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

  // layoutTypes places the type cards. The axis decides the layer shape:
  //
  //   "v" — one band per dependency depth, users on top, dependencies below,
  //         four cards per line; every node in a band shares `y` and its
  //         `layer` is the row index;
  //   "h" — the old grid, grouped struct → interface → type, one layer per
  //         column; every node in a layer shares `x`.
  //
  // Function and value bands continue the same layer numbering, so a call
  // between two cards in one function row is a same-layer edge in both axes.
  // Folded cards keep only their title, which is a view state: the model is
  // untouched.
  function layoutTypes(pkg, types, opts) {
    opts = opts || {};
    var all = !!opts.all;
    var selected = opts.selected || null;
    var collapsed = opts.collapsed || [];
    var expanded = opts.expanded || [];
    var axis = opts.axis === "v" || opts.axis === "h"
      ? opts.axis : (layoutMode(opts.layout) === "kind" ? "h" : "v");
    var edges = opts.edges || [];
    var cardOpts = { all: all, selected: selected, editor: opts.editor, root: opts.root };

    var cards = types.map(function (t) {
      return typeCard(t, pkg, {
        all: all,
        selected: selected,
        folded: collapsed.indexOf(t.id) >= 0,
        expanded: expanded.indexOf(t.id) >= 0,
        editor: opts.editor,
        root: opts.root
      });
    });

    var kindOf = {};
    types.forEach(function (t) { kindOf[t.id] = t.kind; });

    var placed = [];
    var nextLayer = 0;
    if (axis === "h") {
      var kindOrder = { struct: 0, interface: 1, type: 2 };
      var sorted = cards.slice().sort(function (a, b) {
        var ka = kindOrder[kindOf[a.id]], kb = kindOrder[kindOf[b.id]];
        if (ka === undefined) ka = 3;
        if (kb === undefined) kb = 3;
        if (ka !== kb) return ka - kb;
        return a.title.localeCompare(b.title);
      });
      var ys = [];
      for (var c = 0; c < COLS; c++) ys.push(40);
      sorted.forEach(function (card, i) {
        var col = i % COLS;
        placed.push({ card: card, x: 40 + col * (CARD_W + 40), y: ys[col], layer: col });
        ys[col] += card.h + 26;
      });
    } else {
      var rank = rankTypes(types, edges);
      var bands = {};
      cards.forEach(function (card) {
        var r = rank[card.id] || 0;
        (bands[r] = bands[r] || []).push(card);
      });
      var y = 40;
      Object.keys(bands).map(Number).sort(function (a, b) { return a - b; }).forEach(function (r) {
        var list = bands[r].sort(function (a, b) { return a.title.localeCompare(b.title); });
        for (var i = 0; i < list.length; i += PER_ROW) {
          var chunk = list.slice(i, i + PER_ROW);
          var maxH = 0;
          chunk.forEach(function (card, k) {
            placed.push({ card: card, x: 40 + k * (CARD_W + 40), y: y, layer: nextLayer });
            if (card.h > maxH) maxH = card.h;
          });
          nextLayer++;
          y += maxH + 24;
        }
        y += 24; // gap between dependency depths
      });
    }

    var nodes = placed.map(function (p) {
      p.card.x = p.x;
      p.card.y = p.y;
      p.card.layer = p.layer;
      return p.card;
    });

    // Function band: package-level functions are nodes too. Go packages are
    // often functions-only (a command, a helpers package), and drawing an empty
    // canvas made those look broken. They sit under the types, four per row in
    // a vertical flow and four per column in a horizontal one.
    var fns = visible(pkg.funcs || [], showsAll(pkg, all));
    // bandY is computed even when there are no functions: the value band below
    // uses it too, and an undefined base turned every coordinate into NaN.
    var bandY = 40;
    nodes.forEach(function (n) { bandY = Math.max(bandY, n.y + n.h + 56); });
    if (fns.length) {
      var fnLayer = nextLayer;
      fns.forEach(function (f, i) {
        var n = funcCard(f, pkg, cardOpts);
        n.x = 40 + (i % PER_ROW) * (CARD_W + 40);
        n.y = bandY + Math.floor(i / PER_ROW) * 76;
        n.layer = axis === "h" ? (i % PER_ROW) : (fnLayer + Math.floor(i / PER_ROW));
        nodes.push(n);
      });
      if (axis === "v") nextLayer = fnLayer + Math.ceil(fns.length / PER_ROW);
    }
    // Value band: an embed or constant package holds no types and no functions,
    // so it used to draw an empty canvas. Its consts/vars get cards too.
    var values = visible(valuesOf(pkg), showsAll(pkg, all));
    if (values.length) {
      var valueY = fns.length ? bandY + Math.ceil(fns.length / PER_ROW) * 76 : bandY;
      values.forEach(function (v, i) {
        var n = valueCard(v, pkg, cardOpts);
        n.x = 40 + (i % PER_ROW) * (CARD_W + 40);
        n.y = valueY + Math.floor(i / PER_ROW) * 76;
        n.layer = axis === "h" ? (i % PER_ROW) : (nextLayer + Math.floor(i / PER_ROW));
        nodes.push(n);
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

  // funcIDPkg is the package half of a function ID: "pkg#Type.M" -> "pkg".
  function funcIDPkg(id) {
    var s = String(id || "");
    var i = s.lastIndexOf("#");
    return i > 0 ? s.slice(0, i) : "";
  }

  // callCardID maps one end of a call to the card that shows it: a method is a
  // row on its owner type's card, so it maps to the type ID, while a
  // package-level function has a card of its own. "" drops an endpoint the view
  // cannot draw.
  function callCardID(id, pkgPath, byId) {
    if (funcIDPkg(id) !== pkgPath) return "";
    var card = ownerTypeID(id) || String(id || "");
    return byId.has(card) ? card : "";
  }

  // callEdges turns model.calls into the call edges of one package view. A call
  // is kept only when both ends are declarations of pkg and each has a card:
  // function -> function and method -> method both draw, while a call that
  // leaves the package or collapses onto one card (a self-loop) is dropped.
  // Calls between the same two cards share one edge; it is dashed only when
  // every one of them went through an interface.
  function callEdges(calls, pkg, byId) {
    var out = [];
    var index = {};
    (calls || []).forEach(function (c) {
      var from = callCardID(c.from, pkg.path, byId);
      var to = callCardID(c.to, pkg.path, byId);
      if (!from || !to || from === to) return;
      var k = from + ">" + to;
      var edge = index[k];
      if (!edge) {
        edge = index[k] = { from: from, to: to, ref: false, call: true, dynamic: true };
        out.push(edge);
      }
      if (!c.dynamic) edge.dynamic = false;
    });
    return out;
  }

  function isHome(idx, route) {
    return route.view === "home" || (route.view === "pkg" && !idx.pkgByPath.has(route.path));
  }

  // buildScene turns a model and a route into everything draw() needs. It is
  // pure: same model, route and options give the same scene. view carries the
  // current view state: {collapsed, expanded, layout, calls, query, moved} plus
  // the home view's selected package.
  function buildScene(model, idx, route, all, view) {
    view = view || {};
    if (isHome(idx, route)) {
      var pkgs = model.packages.slice().sort(function (a, b) { return a.path.localeCompare(b.path); });
      var selected = view.selected || "";
      if (selected && !idx.pkgByPath.has(selected)) selected = "";
      // The module view has one selection: the card the reader clicked. It is
      // separate from the route, which only changes on a double-click.
      var axis = flowAxis("packages", null);
      var nodes = layoutPackages(pkgs, idx.pkgByPath, {
        modelPath: model.path,
        all: all,
        selected: selected,
        axis: axis
      });
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
      // A cross-package call in a direction the import graph already shows adds
      // nothing, so it is skipped. A call in a direction no package imports is
      // runtime coupling a plain import would hide, so only that becomes a
      // callOnly edge.
      var imported = {};
      pkgs.forEach(function (p) {
        (p.imports || []).forEach(function (imp) { imported[p.path + ">" + imp] = true; });
      });
      var callPairs = {};
      (model.calls || []).forEach(function (c) {
        var from = funcIDPkg(c.from);
        var to = funcIDPkg(c.to);
        if (!from || !to || from === to) return;
        if (!idx.pkgByPath.has(from) || !idx.pkgByPath.has(to)) return;
        var k = from + ">" + to;
        if (imported[k] || callPairs[k]) return;
        callPairs[k] = true;
        edges.push({ from: from, to: to, ref: false, callOnly: true });
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
      return { kind: "packages", key: "home", nodes: nodes, edges: edges, pkgs: pkgs, axis: axis, selected: selected };
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
    var axis = flowAxis("types", view.layout);
    var tnodes = layoutTypes(pkg, types, {
      expanded: view.expanded,
      all: all,
      selected: selectedID,
      collapsed: view.collapsed,
      layout: view.layout,
      axis: axis,
      edges: pairs,
      editor: model.editor,
      root: model.root
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

    // Call edges stay at declaration level: a package-level function has its
    // own card and a method is shown on its owner type card. Both ends must be
    // declarations of this package, so a call that leaves the view is dropped.
    if (view.calls) {
      tedges = tedges.concat(callEdges(model.calls, pkg, byId));
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
      axis: axis
    };
    applyMoved(scene.nodes, view.moved);
    return scene;
  }

  // ---------- inspector panels ----------
  //
  // A panel is one HTML string. panelFrame owns the frame (title, badges, meta,
  // lead, doc, body); panelHome/panelPkg/panelType/panelFunc/panelValue each own
  // one declaration kind's sections; panelFor is the single route-to-panel map.
  // Nothing here touches the document, so ui.test.js can assert that every
  // declaration with a doc actually renders its doc paragraph.

  function panelFrame(o) {
    var html = "<h2>" + o.title;
    (o.badges || []).forEach(function (b) {
      html += ' <span class="badge' + (b.cls ? " " + b.cls : "") + '">' + esc(b.text) + "</span>";
    });
    html += "</h2>";
    (o.meta || []).forEach(function (line) { html += '<div class="meta">' + line + "</div>"; });
    if (o.lead) html += o.lead;
    if (o.doc) html += '<p class="doc">' + esc(o.doc) + "</p>";
    return html + (o.body || "");
  }

  // crapCellHTML is the one CRAP table cell, or a dash when go-crap was off.
  function crapCellHTML(m, model) {
    if (!m || !m.metrics) return '<td class="meta">—</td>';
    var mt = m.metrics;
    return '<td><span class="crap ' + crapLevel(mt.crap, crapThresholds(model)) + '" title="CC ' +
      mt.cyclomatic + " · 覆盖 " + mt.coverage.toFixed(1) + '%">' + formatCrap(mt.crap) + "</span></td>";
  }

  function crapChipHTML(v, model) {
    if (v == null) return "";
    return '<span class="crap ' + crapLevel(v, crapThresholds(model)) + '">CRAP ' + formatCrap(v) + "</span>";
  }

  function typeLinkHTML(idx, id, label) {
    var t = idx.typeById.get(id);
    if (!t) return "<code>" + esc(label || id) + "</code>";
    return '<a href="' + typeHash(id) + '">' + esc(label || (t.pkg.name + "." + t.name)) + "</a>";
  }

  // callLabelHTML turns a function ID into a link to its owner type, or plain
  // code for a package-level function (or an interface method with no card).
  function callLabelHTML(idx, id) {
    var f = idx.funcById.get(id);
    if (f) {
      return f.owner ? typeLinkHTML(idx, f.owner, f.name) : '<code class="sig">' + esc(f.name) + "</code>";
    }
    var owner = ownerTypeID(id);
    var name = funcName(id);
    if (owner && idx.typeById.has(owner)) return typeLinkHTML(idx, owner, name);
    return '<code class="sig">' + esc(name) + "</code>";
  }

  function sourceButtonHTML(model, from, pos) {
    var href = withFrom(editorHref(model && model.editor, model && model.root, pos), from || "#/");
    return href ? '<a class="src" href="' + href + '" target="_blank" rel="noopener">查看源码</a>' : "";
  }

  function chipListHTML(paths, rel) {
    var html = '<div class="chips">';
    (paths || []).forEach(function (p) {
      html += '<a class="chip" href="' + pkgHash(p) + '">' + esc(rel(p)) + "</a>";
    });
    return html + "</div>";
  }

  function homeDetail(p, ctx) {
    var model = ctx.model, all = showsAll(p, ctx.all);
    var worst = packageCrap(p);
    var rel = function (x) { return relPath(model.path, x); };
    var meta = [esc(rel(p.path)),
      visible(p.types, all).length + " 类型 · " + visible(p.funcs, all).length + " 函数 · " +
      visible(valuesOf(p), all).length + " 变量" + (worst != null ? " " + crapChipHTML(worst, model) : "")];
    var body = "";
    if (packageChanged(p)) body += '<p><span class="badge diff">已修改</span></p>';
    body += '<a class="src" href="' + pkgHash(p.path) + '">进入 ' + esc(short(p.path)) + "</a>";

    var imports = p.imports || [];
    body += "<h3>依赖</h3>";
    if (!imports.length) body += '<p class="empty">没有模块内依赖</p>';
    else body += chipListHTML(imports, rel);

    var importers = model.packages.filter(function (q) {
      return q.path !== p.path && (q.imports || []).indexOf(p.path) >= 0;
    });
    body += "<h3>被依赖</h3>";
    if (!importers.length) body += '<p class="empty">没有其他模块依赖它</p>';
    else body += chipListHTML(importers.map(function (q) { return q.path; }), rel);
    return panelFrame({ title: esc(short(p.path)), meta: meta, doc: p.doc, body: body });
  }

  function panelHome(ctx) {
    var model = ctx.model, idx = ctx.idx;
    var sel = ctx.selected ? idx.pkgByPath.get(ctx.selected) : null;
    var html = sel ? homeDetail(sel, ctx) : panelFrame({ title: "模块总览" });
    // With a selection the detail leads; the full list stays below so another
    // module can be picked without going back to the canvas.
    if (sel) html += "<h3>模块总览</h3>";
    html += '<div class="meta">' + model.packages.length + " 个包 · 单击方框选中 · 双击进入模块</div>";
    if (model.metrics) {
      var s = model.metrics;
      html += '<div class="meta">CRAP 平均 ' + formatCrap(s.average) + " · 超标 " +
        s.exceeded + "/" + s.total_funcs + " 个函数</div>";
    }
    html += '<ul class="pkg-list">';
    model.packages.forEach(function (p) {
      html += '<li><a href="' + pkgHash(p.path) + '"><span>' + esc(relPath(model.path, p.path)) + "</span>" +
        '<span class="count">' + visible(p.types, ctx.all).length + " 类型 · " +
        visible(p.funcs, ctx.all).length + " 函数</span></a></li>";
    });
    html += "</ul>";
    return html;
  }

  function panelPkg(p, ctx) {
    var model = ctx.model, all = showsAll(p, ctx.all);
    var rel = function (x) { return relPath(model.path, x); };
    var body = "";
    if (p.imports && p.imports.length) {
      body += "<h3>依赖</h3>" + chipListHTML(p.imports, rel);
      var bad = p.imports.filter(function (i) {
        return layerViolation(model.layers, model.path, p.path, i);
      });
      if (bad.length) {
        body += "<h3>违规依赖</h3><ul class=\"rel\">";
        bad.forEach(function (i) {
          body += "<li><span class=\"crap bad\">内层→外层</span> " + esc(rel(i)) + "</li>";
        });
        body += "</ul>";
      }
    }
    body += "<h3>类型</h3>";
    var types = visible(p.types, all);
    if (!types.length) body += '<p class="empty">没有导出类型</p>';
    else {
      body += "<table><thead><tr><th>类型</th><th>成员</th></tr></thead><tbody>";
      types.forEach(function (t) {
        body += '<tr><td class="name"><a href="' + typeHash(t.id) + '">' + esc(t.name) + "</a> <span class=\"badge " +
          (t.kind === "interface" ? "iface" : (t.kind === "type" ? "type" : "")) + '">' + esc(t.kind) + "</span></td><td>" +
          visible(t.fields, all).length + " 字段 · " + visible(t.methods, all).length + " 方法</td></tr>";
      });
      body += "</tbody></table>";
    }
    var funcs = visible(p.funcs, all);
    var metricsOn = !!model.metrics;
    body += "<h3>函数</h3>";
    if (!funcs.length) body += '<p class="empty">没有导出函数</p>';
    else {
      body += "<table><thead><tr><th>函数</th><th>签名</th>" + (metricsOn ? "<th>CRAP</th>" : "") +
        "</tr></thead><tbody>";
      funcs.forEach(function (f) {
        body += '<tr><td class="name">' + esc(f.name) + '</td><td><code class="sig">' + esc(f.sig) +
          "</code></td>" + (metricsOn ? crapCellHTML(f, model) : "") + "</tr>";
      });
      body += "</tbody></table>";
    }
    return panelFrame({
      title: esc(rel(p.path)),
      doc: p.doc,
      body: body,
      meta: [esc(p.name) + " · " + visible(p.types, ctx.all).length + " 类型 · " +
        visible(p.funcs, ctx.all).length + " 函数"]
    });
  }

  // callListHTML is the one-hop call list shared by the type and function
  // panels. `item` decides how a call is printed, `list` decides the wording.
  function callListHTML(title, calls, item, empty) {
    return "<div><b>" + title + "</b>" + (calls.length
      ? "<ul>" + calls.slice(0, 12).map(item).join("") + "</ul>" +
        (calls.length > 12 ? '<p class="empty">还有 ' + (calls.length - 12) + " 条</p>" : "")
      : '<p class="empty">' + empty + "</p>") + "</div>";
  }

  function panelType(t, ctx) {
    var model = ctx.model, idx = ctx.idx;
    var badges = [{
      cls: t.kind === "interface" ? "iface" : (t.kind === "type" ? "type" : ""),
      text: t.kind
    }];
    if (typeChanged(t)) badges.push({ cls: "diff", text: "改动" });

    var body = "";
    var fields = visible(t.fields, ctx.all), methods = visible(t.methods, ctx.all);
    body += "<h3>字段</h3>";
    if (!fields.length) body += '<p class="empty">没有字段</p>';
    else {
      body += "<table><thead><tr><th>字段</th><th>类型</th></tr></thead><tbody>";
      fields.forEach(function (f) {
        var typ = (f.refs && f.refs.length)
          ? f.refs.map(function (r) { return typeLinkHTML(idx, r); }).join(" / ")
          : '<code class="sig">' + esc(f.type) + "</code>";
        body += '<tr><td class="name">' + (f.embedded ? "«嵌入»" : esc(f.name)) + "</td><td>" + typ + "</td></tr>";
      });
      body += "</tbody></table>";
    }

    var metricsOn = !!model.metrics;
    body += "<h3>方法</h3>";
    if (!methods.length) body += '<p class="empty">没有方法</p>';
    else {
      body += "<table><thead><tr><th>方法</th><th>签名</th>" + (metricsOn ? "<th>CRAP</th>" : "") +
        "</tr></thead><tbody>";
      methods.forEach(function (m) {
        body += '<tr><td class="name">' + esc(m.name) + '</td><td><code class="sig">' + esc(m.sig) +
          "</code></td>" + (metricsOn ? crapCellHTML(m, model) : "") + "</tr>";
      });
      body += "</tbody></table>";
    }

    var out = [];
    fields.forEach(function (f) { (f.refs || []).forEach(function (r) { out.push({ id: r, label: "字段 " + f.name }); }); });
    methods.forEach(function (m) { (m.refs || []).forEach(function (r) { out.push({ id: r, label: "方法 " + m.name }); }); });
    var seen = {};
    out = out.filter(function (o) { if (seen[o.id]) return false; seen[o.id] = true; return true; });
    var inc = idx.incoming.get(t.id) || [];

    body += "<h3>关系</h3><div class=\"rel\">";
    body += "<div><b>引用</b>" + (out.length ? "<ul>" + out.map(function (o) {
      return "<li>" + esc(o.label) + " → " + typeLinkHTML(idx, o.id) + "</li>";
    }).join("") + "</ul>" : '<p class="empty">没有引用模块内类型</p>') + "</div>";
    body += "<div><b>被引用</b>" + (inc.length ? "<ul>" + inc.map(function (o) {
      var from = idx.typeById.has(o.from) ? typeLinkHTML(idx, o.from)
        : '<a href="' + pkgHash(o.from) + '">' + esc(relPath(model.path, o.from)) + "</a>";
      return "<li>" + from + " · " + esc(o.label) + "</li>";
    }).join("") + "</ul>" : '<p class="empty">没有其他类型或函数引用它</p>') + "</div>";
    body += "</div>";

    // 2.2: one hop of the call graph, straight from the model.
    var calls = model.calls || [];
    if (calls.length) {
      var callsOut = calls.filter(function (c) { return ownerTypeID(c.from) === t.id; });
      var calledBy = calls.filter(function (c) { return ownerTypeID(c.to) === t.id; });
      var item = function (c) {
        return "<li>" + esc(funcName(c.from)) + " → " + callLabelHTML(idx, c.to) +
          (c.dynamic ? ' <span class="badge iface">动态</span>' : "") + "</li>";
      };
      body += "<h3>调用</h3><div class=\"rel\">" +
        callListHTML("调用了", callsOut, item, "没有调用模块内函数") +
        callListHTML("被调用", calledBy, item, "没有被模块内函数调用") + "</div>";
    }
    body += sourceButtonHTML(model, ctx.from, t.pos);

    return panelFrame({
      title: esc(t.name),
      badges: badges,
      doc: t.doc,
      body: body,
      meta: [esc(relPath(model.path, t.pkg.path)) + " · " + esc(t.pos) +
        (model.metrics ? " " + crapChipHTML(typeCrap(t), model) : "")]
    });
  }

  // panelFunc is the inspector for a package-level function: the type panel's
  // shape plus the one-hop call list the model already carries.
  function panelFunc(f, pkg, ctx) {
    var model = ctx.model;
    var id = f.id || (pkg.path + "#" + f.name);
    var calls = model.calls || [];
    var callsOut = calls.filter(function (c) { return c.from === id; });
    var calledBy = calls.filter(function (c) { return c.to === id; });
    var item = function (c) {
      return "<li><code class=\"sig\">" + esc(funcName(c.from)) + "</code> → <code class=\"sig\">" +
        esc(funcName(c.to)) + "</code>" + (c.dynamic ? ' <span class="badge iface">动态</span>' : "") + "</li>";
    };
    var body = "<h3>调用</h3><div class=\"rel\">" +
      callListHTML("调用了", callsOut, item, "没有调用模块内函数") +
      callListHTML("被调用", calledBy, item, "没有被模块内函数调用") + "</div>";
    body += sourceButtonHTML(model, ctx.from, f.pos);

    return panelFrame({
      title: esc(f.name),
      badges: [{ text: "函数" }],
      doc: f.doc,
      body: body,
      meta: ['<a href="' + pkgHash(pkg.path) + '">' + esc(relPath(model.path, pkg.path)) + "</a> · " + esc(f.pos) +
        (f.metrics ? " " + crapChipHTML(f.metrics.crap, model) : "")],
      lead: f.sig ? '<p><code class="sig">' + esc(f.name + f.sig) + "</code></p>" : ""
    });
  }

  // panelValue is the inspector for a constant or variable. ValueInfo has no
  // kind of its own, so the slice it came from decides the badge.
  function panelValue(v, pkg, ctx) {
    var model = ctx.model;
    var isConst = (pkg.consts || []).indexOf(v) >= 0;
    var body = "<h3>声明</h3><table><tbody>";
    body += '<tr><td class="name">类型</td><td><code class="sig">' + esc(v.type || "—") + "</code></td></tr>";
    body += '<tr><td class="name">值</td><td><code class="sig">' + esc(v.value || "—") + "</code></td></tr>";
    body += "</tbody></table>";
    body += sourceButtonHTML(model, ctx.from, v.pos);

    return panelFrame({
      title: esc(v.name),
      doc: v.doc,
      body: body,
      badges: [{ text: isConst ? "常量" : "变量" }],
      meta: ['<a href="' + pkgHash(pkg.path) + '">' + esc(relPath(model.path, pkg.path)) + "</a> · " + esc(v.pos)]
    });
  }

  // routePkg resolves the package a route belongs to, so the crumb and the
  // panel can treat type, function and value routes the same way.
  function routePkg(route, idx) {
    if (!route) return undefined;
    if (route.view === "pkg") return idx.pkgByPath.get(route.path);
    if (route.view === "func") return (idx.funcByID.get(route.id) || {}).pkg;
    if (route.view === "value") return (idx.valueByID.get(route.id) || {}).pkg;
    if (route.view === "type") return (idx.typeById.get(route.id) || {}).pkg;
    return undefined;
  }

  // panelFor is the single route-to-panel map. It returns "" only for a route
  // the index cannot resolve, which the page renders as the module overview.
  function panelFor(route, scene, ctx) {
    ctx = ctx || {};
    if (!route || route.view === "home" || !scene) return panelHome(ctx);
    if (route.view === "pkg" && scene.pkg) return panelPkg(scene.pkg, ctx);
    if (route.view === "func") {
      var fn = ctx.idx.funcByID.get(route.id);
      if (fn) return panelFunc(fn.decl, fn.pkg, ctx);
    } else if (route.view === "value") {
      var val = ctx.idx.valueByID.get(route.id);
      if (val) return panelValue(val.decl, val.pkg, ctx);
    } else if (route.view === "type") {
      var t = ctx.idx.typeById.get(route.id);
      if (t) return panelType(t, ctx);
    }
    return panelHome(ctx);
  }

  // ---------- geometry ----------

  // edgeRoute is the routing contract: two boxes and the scene axis in, one
  // concrete route out. It is the only place that decides which border an edge
  // uses, and it guarantees three things the renderer relies on:
  //
  //   * both endpoints sit exactly on a border, at that border's centreline;
  //   * the route leaves the source and approaches the target from outside, so
  //     the arrowhead is never painted inside its own card;
  //   * flow-layer edges use the flow ports (bottom→top for "v", right→left for
  //     "h"); edges between two boxes in the same layer use the cross ports
  //     (side→side for "v", top/bottom for "h") instead of doubling back
  //     through the target. A same-layer flow-port edge is the diagonal bug.
  //
  // "flow" is the coordinate the axis advances on (y for "v", x for "h");
  // "cross" is the coordinate inside one layer. Every branch below only swaps
  // those two roles, so there is one algorithm, not a vertical copy and a
  // horizontal copy.
  function edgeRoute(a, b, axis) {
    var vertical = axis !== "h";
    var aFlow = vertical ? a.y : a.x, aFlowSize = vertical ? a.h : a.w;
    var bFlow = vertical ? b.y : b.x, bFlowSize = vertical ? b.h : b.w;
    var aCross = vertical ? a.x : a.y, aCrossSize = vertical ? a.w : a.h;
    var bCross = vertical ? b.x : b.y, bCrossSize = vertical ? b.w : b.h;
    var aFlowMid = aFlow + aFlowSize / 2, bFlowMid = bFlow + bFlowSize / 2;
    var aCrossMid = aCross + aCrossSize / 2, bCrossMid = bCross + bCrossSize / 2;
    // Overlapping flow intervals mean the two cards share a layer.
    var intra = aFlow < bFlow + bFlowSize - 1 && bFlow < aFlow + aFlowSize - 1;

    function point(flow, cross) {
      return vertical ? { x: cross, y: flow } : { x: flow, y: cross };
    }

    var startFlow, endFlow, startCross, endCross, startSide, endSide;
    if (intra) {
      // Same layer: run along the cross axis, from the facing sides.
      var positive = bCrossMid >= aCrossMid;
      startFlow = aFlowMid;
      endFlow = bFlowMid;
      startCross = positive ? aCross + aCrossSize : aCross;
      endCross = positive ? bCross : bCross + bCrossSize;
      startSide = vertical ? (positive ? "right" : "left") : (positive ? "bottom" : "top");
      endSide = vertical ? (positive ? "left" : "right") : (positive ? "top" : "bottom");
    } else {
      // Different layers: run along the flow axis, at the centreline.
      var forward = bFlowMid >= aFlowMid;
      startFlow = forward ? aFlow + aFlowSize : aFlow;
      endFlow = forward ? bFlow : bFlow + bFlowSize;
      startCross = aCrossMid;
      endCross = bCrossMid;
      startSide = vertical ? (forward ? "bottom" : "top") : (forward ? "right" : "left");
      endSide = vertical ? (forward ? "top" : "bottom") : (forward ? "left" : "right");
    }

    var start = point(startFlow, startCross);
    var end = point(endFlow, endCross);
    var c1, c2;
    if (intra) {
      // Controls share each end's flow coordinate and meet in the cross gap.
      var midCross = (startCross + endCross) / 2;
      c1 = point(startFlow, midCross);
      c2 = point(endFlow, midCross);
    } else {
      // Controls share each end's cross coordinate and meet in the flow gap.
      var midFlow = (startFlow + endFlow) / 2;
      c1 = point(midFlow, startCross);
      c2 = point(midFlow, endCross);
    }

    return {
      axis: vertical ? "v" : "h",
      intra: intra,
      start: { x: start.x, y: start.y, side: startSide },
      end: { x: end.x, y: end.y, side: endSide },
      c1: c1,
      c2: c2,
      d: "M" + start.x + "," + start.y + " C" + c1.x + "," + c1.y +
        " " + c2.x + "," + c2.y + " " + end.x + "," + end.y
    };
  }

  // edgePath keeps the original string API; the route owns the geometry.
  function edgePath(a, b, axis) {
    return edgeRoute(a, b, axis).d;
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
    buildCard: buildCard,
    typeCard: typeCard,
    funcCard: funcCard,
    valueCard: valueCard,
    packageCard: packageCard,
    layoutPackages: layoutPackages,
    flowAxis: flowAxis,
    typeRows: typeRows,
    layoutTypes: layoutTypes,
    rankTypes: rankTypes,
    typePairs: typePairs,
    callEdges: callEdges,
    toggleCollapsed: toggleCollapsed,
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
    edgeRoute: edgeRoute,
    edgePath: edgePath,
    applyMoved: applyMoved,
    fitTransform: fitTransform,
    panelFrame: panelFrame,
    panelFor: panelFor,
    routePkg: routePkg
  };
});
