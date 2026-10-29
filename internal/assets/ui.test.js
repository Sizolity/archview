// Tests for the pure UI helpers in ui.js. Run with `node --test`.
const test = require("node:test");
const assert = require("node:assert/strict");
const ui = require("./ui.js");

function fixture() {
  return {
    path: "example.com/app",
    packages: [
      {
        path: "example.com/app",
        name: "app",
        doc: "Package app is the fixture root.",
        imports: ["example.com/app/store"],
        types: [
          {
            id: "example.com/app#App",
            name: "App",
            kind: "struct",
            exported: true,
            pos: "app.go:5",
            fields: [{ name: "srv", type: "Server", exported: true, refs: ["example.com/app#Server"] }],
            methods: []
          },
          {
            id: "example.com/app#Server",
            name: "Server",
            kind: "struct",
            exported: true,
            pos: "server.go:10",
            fields: [
              { name: "Store", type: "store.DB", exported: true, refs: ["example.com/app/store#DB"] },
              { name: "secret", type: "string", exported: false, refs: [] }
            ],
            methods: [
              { name: "Start", sig: "() error", exported: true },
              { name: "stop", sig: "()", exported: false }
            ]
          }
        ],
        funcs: [{ name: "New", sig: "() *Server", exported: true, refs: ["example.com/app#Server"] }],
        consts: [],
        vars: []
      },
      {
        path: "example.com/app/store",
        name: "store",
        imports: [],
        types: [
          {
            id: "example.com/app/store#DB",
            name: "DB",
            kind: "interface",
            exported: true,
            pos: "store/db.go:3",
            fields: [],
            methods: [{ name: "Get", sig: "(string) error", exported: true }]
          }
        ],
        funcs: [],
        consts: [],
        vars: []
      }
    ]
  };
}

function overlaps(p, q) {
  return p.x < q.x + q.w && q.x < p.x + p.w && p.y < q.y + q.h && q.y < p.y + p.h;
}

test("parseHash round-trips the hashes the UI builds", () => {
  assert.deepEqual(ui.parseHash(ui.typeHash("example.com/app/store#DB")), {
    view: "type",
    id: "example.com/app/store#DB"
  });
  assert.deepEqual(ui.parseHash(ui.pkgHash("example.com/app/store")), {
    view: "pkg",
    path: "example.com/app/store"
  });
  for (const h of ["#/", "", "#/pkg/", "#/type/"]) {
    assert.deepEqual(ui.parseHash(h), { view: "home" }, `hash ${JSON.stringify(h)}`);
  }
});

test("parseHash routes functions and values", () => {
  assert.deepEqual(ui.parseHash(ui.funcHash("example.com/app#New")), {
    view: "func",
    id: "example.com/app#New"
  });
  assert.deepEqual(ui.parseHash(ui.valueHash("example.com/app#MaxRetries")), {
    view: "value",
    id: "example.com/app#MaxRetries"
  });
  for (const h of ["#/func/", "#/value/"]) {
    assert.deepEqual(ui.parseHash(h), { view: "home" }, `hash ${JSON.stringify(h)}`);
  }
});

test("esc escapes the five HTML metacharacters", () => {
  assert.equal(ui.esc(`<a href="x">&'`), "&lt;a href=&quot;x&quot;&gt;&amp;&#39;");
  assert.equal(ui.esc(null), "");
  assert.equal(ui.esc(0), "0");
});

test("cut keeps short strings and marks the cut", () => {
  assert.equal(ui.cut("abc", 3), "abc");
  assert.equal(ui.cut("abcdef", 4), "abc…");
  assert.equal(ui.cut(null, 3), "");
});

test("relPath strips only the module prefix", () => {
  assert.equal(ui.relPath("example.com/app", "example.com/app/store"), "store");
  assert.equal(ui.relPath("example.com/app", "example.com/other"), "example.com/other");
  assert.equal(ui.relPath("", "example.com/app"), "example.com/app");
  assert.equal(ui.relPath("example.com/app", "example.com/app"), "example.com/app");
});

test("visible filters unexported declarations unless asked", () => {
  const list = [{ name: "A", exported: true }, { name: "b", exported: false }];
  assert.deepEqual(ui.visible(list, false).map((x) => x.name), ["A"]);
  assert.deepEqual(ui.visible(list, true).map((x) => x.name), ["A", "b"]);
  assert.deepEqual(ui.visible(null, true), []);
});

test("typeRows marks exports and prints embedded fields by type", () => {
  const t = {
    fields: [
      { name: "Base", type: "Base", embedded: true, exported: true },
      { name: "x", type: "int", exported: false }
    ],
    methods: [{ name: "M", sig: "() error", exported: true }]
  };
  assert.deepEqual(ui.typeRows(t, false), ["+ Base", "+ M() error"]);
  assert.deepEqual(ui.typeRows(t, true), ["+ Base", "- x: int", "+ M() error"]);
});

test("indexModel indexes types and records who uses them", () => {
  const model = fixture();
  const idx = ui.indexModel(model);

  assert.equal(idx.pkgByPath.size, 2);
  assert.equal(idx.typeById.get("example.com/app/store#DB").name, "DB");
  assert.equal(model.packages[0].types[0].pkg.path, "example.com/app");

  const usedBy = idx.incoming.get("example.com/app/store#DB").map((u) => u.label);
  assert.deepEqual(usedBy, ["字段 Store"]);
  const serverUsed = idx.incoming.get("example.com/app#Server").map((u) => u.label).sort();
  assert.deepEqual(serverUsed, ["函数 New", "字段 srv"]);

  // Defaults are filled so the rest of the UI can assume arrays.
  assert.deepEqual(model.packages[1].consts, []);
  assert.deepEqual(model.packages[1].imports, []);
});

test("layoutTypes never overlaps a node and is deterministic", () => {
  const types = [];
  for (let i = 0; i < 10; i++) {
    types.push({
      id: "t" + i,
      name: "T" + i,
      kind: "struct",
      exported: true,
      fields: [{ name: "f", type: "int", exported: true }],
      methods: []
    });
  }
  const pkg = { name: "app" };
  const first = ui.layoutTypes(pkg, types, { all: true });
  const second = ui.layoutTypes(pkg, types, { all: true });
  assert.deepEqual(first, second);
  assert.equal(first.length, 10);

  for (let i = 0; i < first.length; i++) {
    for (let j = i + 1; j < first.length; j++) {
      assert.equal(overlaps(first[i], first[j]), false, `overlap: ${first[i].id} / ${first[j].id}`);
    }
  }
});

test("layoutTypes hides rows beyond seven and counts the rest", () => {
  const t = {
    id: "t",
    name: "T",
    kind: "struct",
    exported: true,
    fields: [],
    methods: []
  };
  for (let i = 0; i < 10; i++) t.fields.push({ name: "f" + i, type: "int", exported: true });
  const nodes = ui.layoutTypes({ name: "app" }, [t], { all: true });
  assert.equal(nodes[0].rows.length, 7);
  assert.equal(nodes[0].more, 3);
});

test("layoutPackages puts importers above what they import", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const nodes = ui.layoutPackages(model.packages, idx.pkgByPath, { modelPath: model.path, all: false });
  const app = nodes.find((n) => n.id === "example.com/app");
  const store = nodes.find((n) => n.id === "example.com/app/store");
  assert.ok(app.y < store.y, "app imports store, so it sits above it");
  assert.equal(app.layer, 0);
  assert.equal(store.layer, 1);
  assert.equal(store.sub, "store");
  assert.equal(store.title, "store");
  assert.equal(store.href, "#/", "a click selects a package in place");
  assert.equal(store.drillHref, ui.pkgHash("example.com/app/store"));
});

test("buildScene draws packages and imports on the home view", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false);
  assert.equal(scene.kind, "packages");
  assert.equal(scene.key, "home");
  assert.equal(scene.nodes.length, 2);
  assert.deepEqual(scene.edges, [{ from: "example.com/app", to: "example.com/app/store", ref: false }]);
});

test("buildScene marks the selected package on the home view", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false, {
    selected: "example.com/app/store"
  });
  const selected = scene.nodes.filter((n) => n.selected);
  assert.deepEqual(selected.map((n) => n.id), ["example.com/app/store"]);
  assert.equal(scene.selected, "example.com/app/store");
  // Without a selection, the whole module stays unselected.
  const plain = ui.buildScene(model, idx, { view: "home" }, false);
  assert.equal(plain.nodes.some((n) => n.selected), false);
  assert.equal(plain.selected, "");
});

test("buildScene draws type cards and reference edges for a package", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  assert.equal(scene.kind, "types");
  assert.equal(scene.key, "pkg:example.com/app");
  assert.deepEqual(scene.nodes.filter((n) => n.kind === "type").map((n) => n.id).sort(), ["example.com/app#App", "example.com/app#Server"]);
  assert.deepEqual(scene.edges, [
    { from: "example.com/app#App", to: "example.com/app#Server", ref: true }
  ]);
});

test("buildScene marks the type named by the route as selected", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "type", id: "example.com/app#Server" }, false);
  assert.equal(scene.kind, "types");
  const selected = scene.nodes.filter((n) => n.selected);
  assert.deepEqual(selected.map((n) => n.id), ["example.com/app#Server"]);
  assert.equal(scene.selected, "example.com/app#Server");
});

test("buildScene opens the owning package for a function route", () => {
  const model = fixture();
  model.packages[0].funcs[0] = {
    id: "example.com/app#New",
    name: "New",
    sig: "() *Server",
    pos: "app.go:42",
    exported: true,
    refs: ["example.com/app#Server"]
  };
  const idx = ui.indexModel(model);
  const route = { view: "func", id: "example.com/app#New" };
  const scene = ui.buildScene(model, idx, route, false);
  const pkgScene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);

  assert.equal(scene.kind, "types");
  assert.deepEqual(scene.nodes.map((n) => n.id), pkgScene.nodes.map((n) => n.id));
  assert.deepEqual(scene.edges, pkgScene.edges);

  const selected = scene.nodes.filter((n) => n.selected);
  assert.deepEqual(selected.map((n) => n.id), ["example.com/app#New"]);
  assert.equal(selected[0].href, ui.funcHash("example.com/app#New"));
  assert.equal(scene.selected, "example.com/app#New");

  assert.equal(idx.funcByID.get(route.id).decl.name, "New");
  assert.equal(idx.funcByID.get(route.id).pkg.path, "example.com/app");
});

test("buildScene opens the owning package for a value route", () => {
  const model = fixture();
  model.packages[0].consts = [{
    name: "MaxRetries",
    type: "int",
    value: "3",
    pos: "app.go:9",
    exported: true
  }];
  const idx = ui.indexModel(model);
  const route = { view: "value", id: "example.com/app#MaxRetries" };
  const scene = ui.buildScene(model, idx, route, false);
  const pkgScene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);

  assert.equal(scene.kind, "types");
  assert.deepEqual(scene.nodes.map((n) => n.id), pkgScene.nodes.map((n) => n.id));
  assert.deepEqual(scene.edges, pkgScene.edges);

  const selected = scene.nodes.filter((n) => n.selected);
  assert.deepEqual(selected.map((n) => n.id), ["example.com/app#MaxRetries"]);
  assert.equal(selected[0].href, ui.valueHash("example.com/app#MaxRetries"));
  assert.equal(scene.selected, "example.com/app#MaxRetries");

  assert.equal(idx.valueByID.get(route.id).decl.name, "MaxRetries");
  assert.equal(idx.valueByID.get(route.id).pkg.path, "example.com/app");
});

test("buildScene falls back to the home view for an unknown package", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  assert.equal(ui.buildScene(model, idx, { view: "pkg", path: "example.com/missing" }, false).kind, "packages");
  assert.equal(ui.buildScene(model, idx, { view: "type", id: "nope" }, false), null);
});

test("edgePath never emits NaN, whichever the order", () => {
  const a = { x: 0, y: 0, w: 100, h: 50 };
  const b = { x: 300, y: 100, w: 100, h: 50 };
  for (const [from, to] of [[a, b], [b, a]]) {
    const d = ui.edgePath(from, to);
    assert.match(d, /^M/);
    assert.equal(d.includes("NaN"), false);
  }
});

test("buildScene gives every view the same top-down flow", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const home = ui.buildScene(model, idx, { view: "home" }, false);
  assert.equal(home.axis, "v", "package imports flow top to bottom");
  const layered = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  assert.equal(layered.axis, "v", "the package layout flows top to bottom");
  // The removed layout switch cannot reintroduce a second direction.
  const legacy = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, { layout: "kind" });
  assert.equal(legacy.axis, "v");
  assert.deepEqual(legacy.nodes, layered.nodes);
});

test("fitTransform keeps the zoom in range and centres the scene", () => {
  const nodes = [{ x: 0, y: 0, w: 100, h: 50 }, { x: 500, y: 300, w: 100, h: 50 }];
  const t = ui.fitTransform(nodes, 1000, 700);
  assert.ok(t.k >= 0.2 && t.k <= 1.25, `k = ${t.k}`);
  assert.ok(Number.isFinite(t.tx) && Number.isFinite(t.ty));
  assert.deepEqual(ui.fitTransform([], 1000, 700), { k: 1, tx: 0, ty: 0 });
});

test("pos helpers map a declaration to the source endpoint", () => {
  assert.deepEqual(ui.parsePos("internal/web/server.go:42"), {
    file: "internal/web/server.go",
    line: 42
  });
  assert.equal(ui.parsePos("noline"), null);
  assert.equal(ui.sourceHref("internal/web/server.go:42"), "/source?path=internal%2Fweb%2Fserver.go#L42");
  assert.equal(ui.sourceHref(""), "");
});

test("editorHref builds vscode, goland and file URLs from the module root", () => {
  const win = "E:/work/app";
  assert.equal(
    ui.editorHref("vscode", win, "internal/web/server.go:42"),
    "vscode://file/E:/work/app/internal/web/server.go:42:1"
  );
  assert.equal(
    ui.editorHref("goland", win, "internal/web/server.go:42"),
    "goland://open?file=" + encodeURIComponent("E:/work/app/internal/web/server.go") + "&line=42"
  );
  assert.equal(
    ui.editorHref("file", win, "internal/web/server.go:42"),
    "file:///E:/work/app/internal/web/server.go"
  );
  // A Unix root already starts with a slash; it must not gain a second one.
  assert.equal(ui.editorHref("file", "/home/u/app", "a/b.go:7"), "file:///home/u/app/a/b.go");
  assert.equal(ui.editorHref("vscode", "/home/u/app", "a/b.go:7"), "vscode://file/home/u/app/a/b.go:7:1");
});

test("editorHref falls back to /source and hides on none", () => {
  assert.equal(ui.editorHref("source", "", "a.go:1"), "/source?path=a.go#L1");
  assert.equal(ui.editorHref("unknown", "E:/x", "a.go:1"), "/source?path=a.go#L1");
  assert.equal(ui.editorHref("none", "E:/x", "a.go:1"), "");
  assert.equal(ui.editorHref("vscode", "E:/x", ""), "");
});

test("toAbs joins the root and a relative path without doubling separators", () => {
  assert.equal(ui.toAbs("E:\\work\\app", "a/b.go"), "E:/work/app/a/b.go");
  assert.equal(ui.toAbs("/home/u/app/", "/a.go"), "/home/u/app/a.go");
  assert.equal(ui.toAbs("", "a.go"), "a.go");
  assert.equal(ui.toAbs("E:/x", ""), "E:/x");
});

test("crapLevel maps scores onto the three bands", () => {
  assert.equal(ui.crapLevel(0), "ok");
  assert.equal(ui.crapLevel(5), "ok");
  assert.equal(ui.crapLevel(5.1), "warn");
  assert.equal(ui.crapLevel(30), "warn");
  assert.equal(ui.crapLevel(30.1), "bad");
  assert.equal(ui.crapLevel(null), "");
  assert.equal(ui.crapLevel(NaN), "");
});

test("crapLevel honours thresholds from the config file", () => {
  assert.equal(ui.crapLevel(3, [2, 10]), "warn");
  assert.equal(ui.crapLevel(11, [2, 10]), "bad");
  assert.equal(ui.crapLevel(1, [2, 10]), "ok");
  // Malformed thresholds fall back to the defaults instead of breaking.
  assert.equal(ui.crapLevel(6, [1]), "warn");
  assert.equal(ui.crapLevel(6, "nope"), "warn");
});

test("crapThresholds prefers the model and defaults otherwise", () => {
  assert.deepEqual(ui.crapThresholds({ metrics: { thresholds: [2, 9] } }), [2, 9]);
  assert.deepEqual(ui.crapThresholds({ metrics: {} }), [ui.CRAP_WARN, ui.CRAP_BAD]);
  assert.deepEqual(ui.crapThresholds(null), [ui.CRAP_WARN, ui.CRAP_BAD]);
  assert.deepEqual(ui.crapThresholds({ metrics: { thresholds: [1] } }), [ui.CRAP_WARN, ui.CRAP_BAD]);
});

test("typeCrap takes the worst method score", () => {
  const t = {
    methods: [
      { name: "A", metrics: { crap: 3 } },
      { name: "B", metrics: { crap: 42 } },
      { name: "C" }
    ]
  };
  assert.equal(ui.typeCrap(t), 42);
  assert.equal(ui.typeCrap({ methods: [{ name: "A" }] }), null);
  assert.equal(ui.typeCrap({}), null);
});

test("formatCrap keeps one decimal and blanks missing values", () => {
  assert.equal(ui.formatCrap(12.345), "12.3");
  assert.equal(ui.formatCrap(0), "0.0");
  assert.equal(ui.formatCrap(null), "");
});

test("layoutTypes carries the CRAP badge onto the card", () => {
  const withMetrics = {
    id: "x",
    name: "X",
    kind: "struct",
    exported: true,
    fields: [],
    methods: [{ name: "M", sig: "()", exported: true, metrics: { crap: 9 } }]
  };
  const nodes = ui.layoutTypes({ name: "p" }, [withMetrics], { all: true });
  assert.equal(nodes[0].crap, 9);

  const plain = { id: "y", name: "Y", kind: "struct", exported: true, fields: [], methods: [] };
  assert.equal(ui.layoutTypes({ name: "p" }, [plain], { all: true })[0].crap, null);
});

test("layerOf matches whole path segments only", () => {
  const layers = [["cmd"], ["web"], ["wiki", "query"], ["platform"]];
  const mod = "example.com/app";

  assert.equal(ui.layerOf(layers, mod, "example.com/app/cmd"), 0);
  assert.equal(ui.layerOf(layers, mod, "example.com/app/internal/web"), 1);
  assert.equal(ui.layerOf(layers, mod, "example.com/app/internal/wiki"), 2);
  assert.equal(ui.layerOf(layers, mod, "example.com/app/platform"), 3);
  // "web" must not match "webhook".
  assert.equal(ui.layerOf(layers, mod, "example.com/app/internal/webhook"), -1);
  // The root package has no relative path, so it belongs to no layer.
  assert.equal(ui.layerOf(layers, mod, "example.com/app"), -1);
  assert.equal(ui.layerOf(null, mod, "example.com/app/cmd"), -1);
});

test("layerViolation flags an inner layer depending on an outer one", () => {
  const layers = [["cmd"], ["web"], ["wiki", "query"], ["platform"]];
  const mod = "example.com/app";
  const web = "example.com/app/internal/web";
  const platform = "example.com/app/platform";

  // The roadmap's acceptance case: web -> platform is red.
  assert.equal(ui.layerViolation(layers, mod, web, platform), true);
  // The other direction is the allowed one.
  assert.equal(ui.layerViolation(layers, mod, platform, web), false);
  // Same layer and unknown packages never violate.
  assert.equal(ui.layerViolation(layers, mod, web, "example.com/app/internal/web2"), false);
  assert.equal(ui.layerViolation(layers, mod, "example.com/app/x", platform), false);
  assert.equal(ui.layerViolation([], mod, web, platform), false);
});

test("buildScene marks layer violations on import edges", () => {
  const model = {
    path: "example.com/app",
    layers: [["web"], ["platform"]],
    packages: [
      {
        path: "example.com/app/web",
        name: "web",
        imports: ["example.com/app/platform"],
        types: [],
        funcs: []
      },
      { path: "example.com/app/platform", name: "platform", imports: [], types: [], funcs: [] }
    ]
  };
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false);

  assert.equal(scene.edges.length, 1);
  assert.equal(scene.edges[0].from, "example.com/app/web");
  assert.equal(scene.edges[0].to, "example.com/app/platform");
  assert.equal(scene.edges[0].violation, true);
});

test("toggleCollapsed returns a new list", () => {
  const base = ["a"];
  const added = ui.toggleCollapsed(base, "b");
  assert.deepEqual(added, ["a", "b"]);
  assert.deepEqual(base, ["a"], "input list must not be mutated");
  assert.deepEqual(ui.toggleCollapsed(added, "a"), ["b"]);
  assert.deepEqual(ui.toggleCollapsed(null, "x"), ["x"]);
});

test("layoutTypes folds a card down to its title", () => {
  const t = {
    id: "a",
    name: "A",
    kind: "struct",
    exported: true,
    fields: [{ name: "f", type: "int", exported: true }],
    methods: []
  };
  const open = ui.layoutTypes({ name: "p" }, [t], { all: true })[0];
  const folded = ui.layoutTypes({ name: "p" }, [t], { all: true, collapsed: ["a"] })[0];

  assert.equal(open.folded, false);
  assert.equal(folded.folded, true);
  assert.deepEqual(folded.rows, []);
  assert.ok(folded.h < open.h, `folded ${folded.h} should be shorter than ${open.h}`);
});

// ---------- 3.1 layered layout ----------

const mkType = (name, kind) => ({
  id: "example.com/p#" + name,
  name: name,
  kind: kind || "struct",
  exported: true,
  fields: [],
  methods: []
});

const chainEdges = () => [
  { from: "example.com/p#A", to: "example.com/p#B" },
  { from: "example.com/p#B", to: "example.com/p#C" }
];

function byTitle(nodes) {
  const map = {};
  nodes.forEach((n) => { map[n.title] = n; });
  return map;
}

test("rankTypes puts dependencies below their users", () => {
  const types = [mkType("A"), mkType("B"), mkType("C")];
  const rank = ui.rankTypes(types, chainEdges());
  assert.equal(rank["example.com/p#A"], 0);
  assert.equal(rank["example.com/p#B"], 1);
  assert.equal(rank["example.com/p#C"], 2);
});

// ---------- 3.1 condensation depth: topology first, cycles surfaced ----------
//
// The contract is topological, not a graph walk: cycles are condensed first,
// the condensation is a DAG by construction, and a component's depth is the
// longest path over it. A shortest-path/BFS walk would flatten the diamond
// below; these tests fail on that layout.

// condenseInfo maps every scene card onto the SCC that owns it.
function condenseInfo(scene) {
  const compOf = new Map();
  scene.components.forEach((members, i) => {
    members.forEach((id) => compOf.set(id, i));
  });
  return compOf;
}

test("condense returns every node's component and an acyclic condensation", () => {
  const p = (name) => "example.com/p#" + name;
  const types = ["A", "B", "C", "D", "E"].map((n) => mkType(n));
  const edges = [
    { from: p("A"), to: p("B") },
    { from: p("B"), to: p("C") },
    { from: p("C"), to: p("A") }, // {A,B,C} is one SCC
    { from: p("B"), to: p("B") }, // a self-loop must not become a component edge
    { from: p("B"), to: p("D") },
    { from: p("C"), to: p("D") }, // duplicate at component level
    { from: p("D"), to: p("E") },
    { from: p("E"), to: p("D") } // {D,E} is one SCC
  ];
  const c = ui.condense(types, edges);

  assert.equal(c.components.length, 2);
  types.forEach((t) => {
    assert.ok(Number.isInteger(c.compOf[t.id]), t.id + " has no component");
    assert.ok(c.components[c.compOf[t.id]].includes(t.id), t.id + " is not in its own component");
  });
  const members = c.components.map((m) => m.slice().sort()).sort();
  assert.deepEqual(members, [[p("A"), p("B"), p("C")].sort(), [p("D"), p("E")].sort()]);

  // No self-loop survives, and the two B/C -> D dependencies are one edge.
  assert.equal(c.compEdges.length, 1);
  assert.deepEqual(c.compEdges[0], { from: c.compOf[p("A")], to: c.compOf[p("D")] });
  assert.ok(!c.compEdges.some((e) => e.from === e.to), "the condensation has no self-loop");

  // Kahn over the component edges must consume every component.
  const indeg = c.components.map(() => 0);
  c.compEdges.forEach((e) => { indeg[e.to]++; });
  const queue = indeg.map((d, i) => (d === 0 ? i : -1)).filter((i) => i >= 0);
  let seen = 0;
  for (let head = 0; head < queue.length; head++) {
    seen++;
    c.compEdges.forEach((e) => {
      if (e.from !== queue[head]) return;
      if (--indeg[e.to] === 0) queue.push(e.to);
    });
  }
  assert.equal(seen, c.components.length, "the condensation must be acyclic");

  // Longest path over the condensation: the {d,e} component sits one step
  // below the {a,b,c} component, even though a->d is also a direct edge.
  assert.equal(c.compRank[c.compOf[p("A")]], 0);
  assert.equal(c.compRank[c.compOf[p("D")]], 1);
});

test("depth comes from the longest path over the condensation, not a BFS", () => {
  const p = (name) => "example.com/p#" + name;
  const diamond = [
    { from: p("A"), to: p("B") },
    { from: p("A"), to: p("C") },
    { from: p("B"), to: p("D") },
    { from: p("C"), to: p("D") }
  ];
  const types = ["A", "B", "C", "D"].map((n) => mkType(n));
  const rank = ui.rankTypes(types, diamond);
  assert.equal(rank[p("A")], 0);
  assert.equal(rank[p("B")], 1);
  assert.equal(rank[p("C")], 1);
  assert.equal(rank[p("D")], 2, "a converging node keeps its longest depth, not its shortest");

  const chain = ui.rankTypes([mkType("a"), mkType("b"), mkType("c")], [
    { from: p("a"), to: p("b") },
    { from: p("b"), to: p("c") }
  ]);
  assert.deepEqual([chain[p("a")], chain[p("b")], chain[p("c")]], [0, 1, 2]);

  // A diamond keeps D at 2 under both a BFS and a longest path. The triangle is
  // the discriminator: the direct A -> C edge makes a BFS rank C at 1, while
  // C's C-after-B chain makes the topological depth 2.
  const triangle = ui.rankTypes(types.filter((t) => t.name !== "D"), [
    { from: p("A"), to: p("B") },
    { from: p("A"), to: p("C") },
    { from: p("B"), to: p("C") }
  ]);
  assert.equal(triangle[p("C")], 2, "a BFS would flatten C to 1");

  // The cards drawn for the diamond carry the same depth, and no BFS flattening
  // leaves a non-cycle edge on its source's row.
  const nodes = ui.layoutTypes({ name: "p" }, types, { all: true, edges: diamond });
  const byId = new Map(nodes.map((n) => [n.id, n]));
  assert.equal(byId.get(p("D")).rank, 2);
  for (const e of diamond) {
    const a = byId.get(e.from);
    const b = byId.get(e.to);
    assert.ok(b.rank > a.rank, `${e.from} -> ${e.to} must descend a rank`);
    assert.ok(b.layer > a.layer, `${e.from} -> ${e.to} must descend a row`);
  }
});

test("cycle edges exist exactly for SCCs of two or more cards", () => {
  for (const { name, scene } of flowScenes()) {
    const compOf = condenseInfo(scene);
    const big = scene.components.map((m) => m.length >= 2);
    let cycleEdges = 0;
    for (const { e } of drawableEdges(scene)) {
      const same = compOf.get(e.from) === compOf.get(e.to) && big[compOf.get(e.from)];
      if (same) {
        assert.equal(e.cycle, true,
          `${name}: ${e.from} -> ${e.to} is inside one SCC and must be a cycle edge`);
        assert.equal(compOf.get(e.from), compOf.get(e.to),
          `${name}: a cycle edge must join one component`);
        cycleEdges++;
      } else {
        assert.ok(!e.cycle,
          `${name}: ${e.from} -> ${e.to} crosses components but is marked cycle`);
      }
    }
    assert.equal(cycleEdges > 0, big.some(Boolean),
      `${name}: cycle edges must exist exactly when an SCC has two or more cards`);
  }
});

test("span edges are produced only by wrapRows, and only for short hops", () => {
  const model = compactModel();
  const idx = ui.indexModel(model);
  const wrapped = ui.buildScene(model, idx, { view: "pkg", path: "example.com/fan" }, true, { calls: true });
  const ranks = new Set(wrapped.nodes.map((n) => n.rank));
  const rows = new Set(wrapped.nodes.map((n) => n.layer));
  assert.ok(rows.size > ranks.size, "the compact fixture must split a rank into several rows");
  let spans = 0;
  for (const { e, a, b } of drawableEdges(wrapped)) {
    if (!e.span) continue;
    spans++;
    assert.ok(!e.cycle, "an edge cannot be both cycle and span");
    const rankDiff = b.rank - a.rank;
    assert.ok(rankDiff >= 0 && rankDiff <= 1, "span is a same-rank or adjacent-rank hop");
    assert.ok(b.layer - a.layer > rankDiff, "wrapRows added the extra rows, not the topology");
  }
  assert.ok(spans > 0, "a wrapped scene must have at least one span edge");

  const flatModel = homeModel();
  const flatIdx = ui.indexModel(flatModel);
  const flat = ui.buildScene(flatModel, flatIdx, { view: "home" }, true, { calls: true });
  assert.ok(flat.nodes.every((n) => n.layer === n.rank), "the home fixture must not need wrapping");
  assert.equal(flat.edges.filter((e) => e.span).length, 0,
    "an unwrapped scene must not produce span edges");
});

test("every non-cycle, non-span edge goes to a strictly deeper rank", () => {
  let checked = 0;
  let cycles = 0;
  let spans = 0;
  for (const { name, scene } of flowScenes()) {
    for (const { e, a, b } of drawableEdges(scene)) {
      assert.ok(Number.isInteger(a.rank), `${name}: ${a.id} has no integer condensation rank`);
      assert.ok(Number.isInteger(b.rank), `${name}: ${b.id} has no integer condensation rank`);
      if (e.cycle) {
        cycles++;
        assert.equal(a.rank, b.rank, `${name}: cycle edge ${e.from} -> ${e.to} must share a rank`);
        continue;
      }
      if (e.span) {
        spans++;
        assert.ok(b.layer > a.layer, `${name}: span edge ${e.from} -> ${e.to} must still point down a row`);
        continue;
      }
      assert.ok(b.rank > a.rank,
        `${name}: ${e.from} (rank ${a.rank}) -> ${e.to} (rank ${b.rank}) must go to a deeper rank`);
      assert.ok(b.layer > a.layer,
        `${name}: ${e.from} (layer ${a.layer}) -> ${e.to} (layer ${b.layer}) must go to a deeper row`);
      checked++;
    }
  }
  assert.ok(checked >= 20, "expected many ordinary edges, got " + checked);
  assert.ok(cycles >= 3, "the cycle scene must mark all three edges, got " + cycles);
  assert.ok(spans >= 0, "span count is reported, not required in every scene");
});

test("an SCC of two or more cards adds one deterministic warning line to the scene", () => {
  const model = cycleModel();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/cycle" }, true, { calls: true });
  assert.equal(scene.components.length, 1);
  assert.deepEqual(scene.components[0], [
    "example.com/cycle#A", "example.com/cycle#B", "example.com/cycle#C"
  ]);
  assert.equal(typeof scene.cycleWarning, "string");
  assert.match(scene.cycleWarning, /环\(强连通\)/);
  for (const n of ["A", "B", "C"]) {
    assert.ok(scene.cycleWarning.includes(n), "the warning must list " + n);
  }
  const again = ui.buildScene(model, idx, { view: "pkg", path: "example.com/cycle" }, true, { calls: true });
  assert.equal(again.cycleWarning, scene.cycleWarning, "the warning line is deterministic");

  const flatModel = bigModel();
  const flatIdx = ui.indexModel(flatModel);
  const flat = ui.buildScene(flatModel, flatIdx, { view: "pkg", path: "example.com/big" }, true, { calls: true });
  assert.ok(flat.components.every((c) => c.length === 1), "the big fixture has no SCC of two cards");
  assert.equal(flat.cycleWarning, null, "no cycle line when every SCC is a single card");
});

test("the old back classification is gone, not kept beside cycle and span", () => {
  for (const { name, scene } of flowScenes()) {
    for (const e of scene.edges) {
      assert.equal(Object.prototype.hasOwnProperty.call(e, "back"), false,
        `${name}: ${e.from} -> ${e.to} still carries the old back flag`);
    }
  }
});

test("longest-path ranks keep a converging node at its deepest chain", () => {
  const types = [mkType("A"), mkType("B"), mkType("C")];
  const edges = [
    { from: "example.com/p#A", to: "example.com/p#B" },
    { from: "example.com/p#A", to: "example.com/p#C" },
    { from: "example.com/p#B", to: "example.com/p#C" }
  ];
  const rank = ui.rankTypes(types, edges);
  assert.equal(rank["example.com/p#A"], 0);
  assert.equal(rank["example.com/p#B"], 1);
  // The direct A -> C edge is shorter, but C also sits below B. Topological
  // depth follows the longest chain, so C is 2; a BFS would flatten it to 1.
  assert.equal(rank["example.com/p#C"], 2);

  const pkg = { name: "p" };
  const cards = types.map((t) => ui.typeCard(t, pkg, { all: true }));
  const sceneEdges = edges.map((e) => ({ from: e.from, to: e.to }));
  ui.layoutGraph(cards, sceneEdges);
  const byId = new Map(cards.map((n) => [n.id, n]));
  assert.equal(byId.get("example.com/p#C").rank, 2);
  for (const e of sceneEdges) {
    assert.equal(e.cycle, undefined, `${e.from} -> ${e.to} is not a cycle`);
    assert.equal(e.span, undefined, `${e.from} -> ${e.to} is not layout-stretched`);
    assert.ok(byId.get(e.to).rank > byId.get(e.from).rank,
      `${e.from} -> ${e.to} must point to a deeper rank`);
    assert.ok(byId.get(e.to).layer > byId.get(e.from).layer,
      `${e.from} -> ${e.to} must point to a deeper row`);
  }
});

test("layer layout stacks users above dependencies", () => {
  const types = [mkType("A"), mkType("B"), mkType("C")];
  const nodes = ui.layoutTypes({ name: "p" }, types, {
    all: true,
    edges: chainEdges()
  });
  const t = byTitle(nodes);
  assert.ok(t.A.y < t.B.y, "A should sit above B");
  assert.ok(t.B.y < t.C.y, "B should sit above C");
});

test("layoutTypes always uses dependency layers", () => {
  const nodes = ui.layoutTypes({ name: "p" }, [mkType("A"), mkType("B")], {
    all: true,
    edges: [{ from: "example.com/p#A", to: "example.com/p#B" }]
  });
  const t = byTitle(nodes);
  assert.ok(t.A.y < t.B.y);
});

test("cards without edges share layer 0 and keep declaration-id order", () => {
  const types = [mkType("Z", "type"), mkType("I", "interface"), mkType("B"), mkType("A")];
  const nodes = ui.layoutTypes({ name: "p" }, types, { all: true });
  assert.deepEqual(nodes.map((n) => n.title), ["A", "B", "I", "Z"]);
  assert.deepEqual(nodes.map((n) => n.sub.split(" · ")[0]), [
    "struct",
    "struct",
    "interface",
    "type"
  ]);
  assert.ok(nodes.every((n) => n.layer === 0));
});

test("ten types and fifteen edges never overlap", () => {
  const types = [];
  for (let i = 0; i < 10; i++) {
    types.push({
      id: "t" + i,
      name: "T" + i,
      kind: "struct",
      exported: true,
      fields: [{ name: "f", type: "int", exported: true }],
      methods: []
    });
  }
  const edges = [];
  for (let i = 0; i < 10 && edges.length < 15; i++) {
    for (let d = 1; d <= 2 && edges.length < 15; d++) {
      if (i + d < 10) edges.push({ from: "t" + i, to: "t" + (i + d) });
    }
  }
  assert.equal(edges.length, 15);

  const first = ui.layoutTypes({ name: "p" }, types, { all: true, edges });
  const second = ui.layoutTypes({ name: "p" }, types, { all: true, edges });
  assert.deepEqual(first, second, "layout must be deterministic");
  for (let i = 0; i < first.length; i++) {
    for (let j = i + 1; j < first.length; j++) {
      assert.equal(overlaps(first[i], first[j]), false, "overlap: " + first[i].id + "/" + first[j].id);
    }
  }
});

test("typePairs deduplicates references and skips self-loops", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const appTypes = model.packages[0].types;

  const pairs = ui.typePairs(appTypes, idx);
  assert.deepEqual(pairs, [
    { from: "example.com/app#App", to: "example.com/app#Server", ref: true },
    { from: "example.com/app#Server", to: "example.com/app/store#DB", ref: true }
  ]);
  // The cross-package target is kept: buildScene filters it for drawing.
  assert.equal(pairs.length, 2);
});

test("buildScene layers the package graph", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const route = { view: "pkg", path: "example.com/app" };

  const layered = ui.buildScene(model, idx, route, false);
  const t = byTitle(layered.nodes);
  // App uses Server, so App sits above it.
  assert.ok(t.App.y < t.Server.y, "layered layout puts the user above the dependency");

  // There is no layout switch left: an old "kind" request produces the same
  // scene, rather than silently selecting a second direction.
  const legacy = ui.buildScene(model, idx, route, false, { layout: "kind" });
  assert.deepEqual(legacy.nodes, layered.nodes);
  assert.deepEqual(legacy.edges, layered.edges);
});

// ---------- 2.2 call graph ----------

test("ownerTypeID maps methods to their type and leaves functions alone", () => {
  assert.equal(ui.ownerTypeID("example.com/p#Server.source"), "example.com/p#Server");
  assert.equal(ui.ownerTypeID("example.com/p#New"), "");
  assert.equal(ui.ownerTypeID("garbage"), "");
  assert.equal(ui.ownerTypeID(""), "");
  assert.equal(ui.funcName("example.com/p#Server.source"), "Server.source");
  assert.equal(ui.funcName("example.com/p#New"), "New");
});

test("indexModel indexes functions and methods by ID", () => {
  const model = fixture();
  model.packages[0].funcs[0].id = "example.com/app#New";
  model.packages[0].types[1].methods[0].id = "example.com/app#Server.Start";

  const idx = ui.indexModel(model);
  assert.equal(idx.funcById.get("example.com/app#New").name, "New");
  assert.equal(idx.funcById.get("example.com/app#Server.Start").owner, "example.com/app#Server");
});

function callModel(calls) {
  return {
    path: "example.com/app",
    calls: calls,
    packages: [{
      path: "example.com/app",
      name: "app",
      imports: [],
      types: [
        {
          id: "example.com/app#A", name: "A", kind: "struct", exported: true, fields: [],
          methods: [
            { id: "example.com/app#A.Run", name: "Run", sig: "()", exported: true },
            { id: "example.com/app#A.Stop", name: "Stop", sig: "()", exported: true }
          ]
        },
        {
          id: "example.com/app#B", name: "B", kind: "struct", exported: true, fields: [],
          methods: [{ id: "example.com/app#B.Serve", name: "Serve", sig: "()", exported: true }]
        }
      ],
      funcs: []
    }]
  };
}

test("call edges are off by default", () => {
  const model = callModel([
    { from: "example.com/app#A.Run", to: "example.com/app#B.Serve" }
  ]);
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  assert.equal(scene.edges.filter((e) => e.call).length, 0);
});

test("call edges aggregate per type pair and go solid on a static call", () => {
  const model = callModel([
    { from: "example.com/app#A.Run", to: "example.com/app#B.Serve" },
    { from: "example.com/app#A.Stop", to: "example.com/app#B.Serve", dynamic: true }
  ]);
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, { calls: true });

  const calls = scene.edges.filter((e) => e.call);
  assert.equal(calls.length, 1, "two calls between the same types collapse into one edge");
  assert.equal(calls[0].from, "example.com/app#A");
  assert.equal(calls[0].to, "example.com/app#B");
  assert.equal(calls[0].dynamic, false, "one static call is enough for a solid line");
});

test("a purely dynamic call edge stays dashed", () => {
  const model = callModel([
    { from: "example.com/app#A.Stop", to: "example.com/app#B.Serve", dynamic: true }
  ]);
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, { calls: true });
  const calls = scene.edges.filter((e) => e.call);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].dynamic, true);
});

test("calls to functions outside the visible types draw nothing", () => {
  const model = callModel([
    { from: "example.com/app#A.Run", to: "example.com/other#X.Y" },
    { from: "example.com/app#New", to: "example.com/app#A.Run" }
  ]);
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, { calls: true });
  assert.equal(scene.edges.filter((e) => e.call).length, 0);
});

test("package view keeps only intra-package calls at card level", () => {
  const model = callModel([
    { from: "example.com/app#New", to: "example.com/app#build" },
    { from: "example.com/app#A.Run", to: "example.com/app#B.Serve", dynamic: true },
    // A method calling another method of its own type collapses onto one card.
    { from: "example.com/app#A.Run", to: "example.com/app#A.Stop" },
    // Calls leaving or entering the package are not part of this view.
    { from: "example.com/app#New", to: "example.com/other#X" },
    { from: "example.com/other#X", to: "example.com/app#New" }
  ]);
  model.packages[0].funcs = [
    { id: "example.com/app#New", name: "New", sig: "()", exported: true },
    { id: "example.com/app#build", name: "build", sig: "()", exported: false }
  ];
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, true, { calls: true });

  const calls = scene.edges
    .filter((e) => e.call)
    .map((e) => e.from + ">" + e.to + ":" + (e.dynamic ? "dynamic" : "static"))
    .sort();
  assert.deepEqual(calls, [
    "example.com/app#A>example.com/app#B:dynamic",
    "example.com/app#New>example.com/app#build:static"
  ]);
});

test("home view adds callOnly edges only in a direction without an import", () => {
  const model = {
    path: "example.com/app",
    packages: [
      { path: "example.com/app/a", name: "a", imports: ["example.com/app/b"], types: [], funcs: [] },
      { path: "example.com/app/b", name: "b", imports: [], types: [], funcs: [] },
      { path: "example.com/app/c", name: "c", imports: [], types: [], funcs: [] }
    ],
    calls: [
      // a imports b, so this direction is already visible as an import.
      { from: "example.com/app/a#A", to: "example.com/app/b#B" },
      // The reverse direction has no import: this is the hidden coupling.
      { from: "example.com/app/b#B", to: "example.com/app/a#A" },
      // Duplicates and a second import-free pair still draw one edge each.
      { from: "example.com/app/a#A", to: "example.com/app/c#C" },
      { from: "example.com/app/a#A", to: "example.com/app/c#C" }
    ]
  };
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false);

  const callOnly = scene.edges.filter((e) => e.callOnly).map((e) => e.from + ">" + e.to).sort();
  assert.deepEqual(callOnly, [
    "example.com/app/a>example.com/app/c",
    "example.com/app/b>example.com/app/a"
  ]);
  // Only the one real import remains an import edge.
  assert.deepEqual(scene.edges.filter((e) => !e.callOnly).map((e) => e.from + ">" + e.to),
    ["example.com/app/a>example.com/app/b"]);
});

test("buildScene still works without the removed declutter and focus options", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const route = { view: "type", id: "example.com/app#Server" };
  const plain = ui.buildScene(model, idx, route, false);
  const legacy = ui.buildScene(model, idx, route, false, {
    focus: "example.com/app#Server",
    declutter: true
  });

  assert.deepEqual(legacy, plain);
  assert.equal(ui.focusScene, undefined);
  assert.equal(ui.declutterScene, undefined);
});

// ---------- 3.3 search ----------

test("searchHits finds methods and members, not just names", () => {
  const model = fixture();
  const hits = ui.searchHits(model, "Get", false);
  assert.deepEqual(hits.map((h) => h.kind + ":" + h.name), ["方法:DB.Get"]);
  assert.equal(hits[0].href, ui.typeHash("example.com/app/store#DB"));
  assert.equal(hits[0].typeId, "example.com/app/store#DB");
});

test("searchHits matches docs and ranks name matches first", () => {
  const model = fixture();
  model.packages[0].types[1].doc = "Server field mentions Resolve stuff";

  const docOnly = ui.searchHits(model, "resolve", false);
  assert.equal(docOnly.length, 1);
  assert.equal(docOnly[0].name, "app.Server");
  assert.equal(docOnly[0].nameMatch, false);

  model.packages[0].types[0].doc = "App resolves things";
  model.packages[0].types[0].name = "Resolver";
  model.packages[0].types[0].id = "example.com/app#Resolver";

  const mixed = ui.searchHits(model, "resolve", false);
  assert.deepEqual(mixed.map((h) => h.name), ["app.Resolver", "app.Server"]);
  assert.equal(mixed[0].nameMatch, true);
});

test("searchHits respects the unexported filter", () => {
  const model = fixture();
  assert.equal(ui.searchHits(model, "stop", false).length, 0);
  const withPrivate = ui.searchHits(model, "stop", true);
  assert.equal(withPrivate.length, 1);
  assert.equal(withPrivate[0].kind, "方法");
});

test("highlight escapes the surrounding text and wraps the match", () => {
  assert.equal(ui.highlight("Server.Start", "start"), "Server.<mark>Start</mark>");
  assert.equal(ui.highlight("<b>", "b"), "&lt;<mark>b</mark>&gt;");
  assert.equal(ui.highlight("abc", "z"), "abc");
  assert.equal(ui.highlight(null, "x"), "");
});

test("matchTypeIDs maps every hit back to its type", () => {
  const model = fixture();
  const ids = ui.matchTypeIDs(ui.searchHits(model, "Get", false));
  assert.deepEqual(Object.keys(ids), ["example.com/app/store#DB"]);
  assert.deepEqual(ui.matchTypeIDs([]), {});
});

test("buildScene fades the types a query does not touch", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const route = { view: "pkg", path: "example.com/app" };

  const plain = ui.buildScene(model, idx, route, false);
  assert.ok(plain.nodes.every((n) => !n.dim));

  const searched = ui.buildScene(model, idx, route, false, { query: "Store" });
  assert.deepEqual(searched.nodes.filter((n) => n.dim && n.kind === "type").map((n) => n.title), ["App"]);
});

test("home view fades packages whose types do not match", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false, { query: "Get" });
  assert.deepEqual(scene.nodes.filter((n) => n.dim).map((n) => n.title), ["app"]);
});

test("searchHits covers constants and variables", () => {
  const model = fixture();
  model.packages[0].consts = [{ name: "MaxRetries", type: "int", value: "3", exported: true }];
  model.packages[0].vars = [{ name: "defaults", type: "Config", exported: false }];

  const c = ui.searchHits(model, "retries", false);
  assert.equal(c.length, 1);
  assert.equal(c[0].kind, "常量");
  assert.equal(c[0].href, ui.pkgHash("example.com/app"));

  assert.equal(ui.searchHits(model, "defaults", false).length, 0);
  const v = ui.searchHits(model, "defaults", true);
  assert.equal(v.length, 1);
  assert.equal(v[0].kind, "变量");
});

test("searchHits breaks name ties alphabetically", () => {
  const model = fixture();
  const hits = ui.searchHits(model, "e", false);
  const named = hits.filter((h) => h.nameMatch).map((h) => h.name);
  assert.ok(named.length > 1, "need at least two name matches, got " + named.length);
  assert.deepEqual(named, named.slice().sort((a, b) => a.localeCompare(b)));
});


// ---------- edges: routing, styles, dragging ----------

test("edgePath routes vertically when the target sits below", () => {
  const d = ui.edgePath({ x: 0, y: 0, w: 100, h: 50 }, { x: 150, y: 300, w: 100, h: 50 }, "v");
  // Leaves the bottom centre (50,50) and enters the top centre (200,300).
  assert.match(d, /^M50,50 C/);
  assert.match(d, / 200,300$/);
});

test("edgePath always leaves the bottom and enters the top, even beside", () => {
  // The target is to the side but above: the ports still do not rotate.
  const d = ui.edgePath({ x: 0, y: 0, w: 100, h: 50 }, { x: 300, y: 20, w: 100, h: 50 });
  assert.match(d, /^M50,50 C/);
  assert.match(d, / 350,20$/);
});

test("edgePath leaves every source at the bottom and enters every target at the top", () => {
  const a = { x: 0, y: 0, w: 100, h: 50 };
  const b = { x: 300, y: 300, w: 100, h: 50 };
  // Reversing the pair changes the source and target, never the port kind:
  // every edge leaves a bottom border and enters a top border.
  const forward = ui.edgePath(a, b);
  const backward = ui.edgePath(b, a);
  assert.match(forward, /^M50,50 /);
  assert.match(forward, / 350,300$/);
  assert.match(backward, /^M350,350 /);
  assert.match(backward, / 50,0$/);
});

test("applyMoved overrides laid out positions", () => {
  const nodes = [{ id: "a", x: 1, y: 2 }, { id: "b", x: 3, y: 4 }];
  ui.applyMoved(nodes, { a: { x: 10, y: 20 } });
  assert.deepEqual(nodes[0], { id: "a", x: 10, y: 20 });
  assert.deepEqual(nodes[1], { id: "b", x: 3, y: 4 });
  ui.applyMoved(nodes, null);
  assert.deepEqual(nodes[0], { id: "a", x: 10, y: 20 });
});

test("buildScene applies dragged positions", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, {
    moved: { "example.com/app#App": { x: 500, y: 600 } }
  });
  const app = scene.nodes.find((n) => n.id === "example.com/app#App");
  assert.equal(app.x, 500);
  assert.equal(app.y, 600);
});

test("implements and embeds edges keep their kind, uses stays a ref", () => {
  const model = fixture();
  model.edges = [
    { kind: "implements", from: "example.com/app#App", to: "example.com/app#Server" },
    { kind: "embeds", from: "example.com/app#Server", to: "example.com/app#App" },
    { kind: "uses", from: "example.com/app#App", to: "example.com/app#Server" }
  ];
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  const kinds = scene.edges
    .filter((e) => e.kind)
    .map((e) => e.kind + ":" + e.from + ">" + e.to);
  assert.deepEqual(kinds.sort(), [
    "embeds:example.com/app#Server>example.com/app#App",
    "implements:example.com/app#App>example.com/app#Server"
  ]);
});

test("implements edges to a hidden package are not drawn", () => {
  const model = fixture();
  model.edges = [
    { kind: "implements", from: "example.com/app#App", to: "example.com/app/store#DB" }
  ];
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  assert.equal(scene.edges.filter((e) => e.kind).length, 0);
});



// ---------- source page links ----------

test("withFrom puts the current route on a source link", () => {
  assert.equal(
    ui.withFrom("/source?path=a.go#L3", "#/type/example.com/app%23Server"),
    "/source?path=a.go&from=%23%2Ftype%2Fexample.com%2Fapp%2523Server#L3"
  );
  // No fragment: the parameter goes straight on.
  assert.equal(ui.withFrom("/source?path=a.go", "#/"), "/source?path=a.go&from=%23%2F");
  // Other links are left alone.
  assert.equal(ui.withFrom("vscode://file/x.go:3", "#/"), "vscode://file/x.go:3");
  assert.equal(ui.withFrom("", "#/"), "");
});


// ---------- diff markers ----------

test("typeChanged marks a type touched by the diff", () => {
  assert.equal(ui.typeChanged({ changed: true }), true);
  assert.equal(ui.typeChanged({ methods: [{ changed: true }] }), true);
  assert.equal(ui.typeChanged({ methods: [{ changed: false }] }), false);
  assert.equal(ui.typeChanged(undefined), false);
});
// ---------- warnings ----------

test("warningSummary folds warnings into one line", () => {
  assert.equal(ui.warningSummary({}), null);
  assert.equal(ui.warningSummary({ warnings: [] }), null);
  const one = ui.warningSummary({ warnings: ["CRAP 指标不可用: boom"] });
  assert.equal(one.count, 1);
  assert.equal(one.text, "CRAP 指标不可用: boom");
  const many = ui.warningSummary({ warnings: ["a", "b"] });
  assert.equal(many.count, 2);
  assert.equal(many.all, "a\nb");
  assert.ok(ui.warningSummary({ warnings: ["x".repeat(200)] }).text.length <= 120);
});
test("packageChanged sees a change anywhere in the package", () => {
  assert.equal(ui.packageChanged({ types: [{ changed: true }] }), true);
  assert.equal(ui.packageChanged({ types: [{ methods: [{ changed: true }] }] }), true);
  assert.equal(ui.packageChanged({ funcs: [{ changed: true }] }), true);
  assert.equal(ui.packageChanged({ types: [{}], funcs: [{}] }), false);
  assert.equal(ui.packageChanged(null), false);
});
test("showsAll opens up a program entry", () => {
  assert.equal(ui.showsAll({ main: true }, false), true);
  assert.equal(ui.showsAll({ main: false }, false), false);
  assert.equal(ui.showsAll({ main: false }, true), true);
  assert.equal(ui.showsAll(null, false), false);
});
// A package whose declarations are functions only used to draw an empty canvas.
test("buildScene draws function cards", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  const fns = scene.nodes.filter((n) => n.kind === "func");
  assert.deepEqual(fns.map((n) => n.title).sort(), ["New"]);
  assert.ok(fns.every((n) => typeof n.sub === "string" && n.sub.indexOf("(") === 0));
  // Function cards are graph nodes now, not a fixed band: they still need a
  // real layer and finite boxes even when the package graph is types-only.
  assert.ok(fns.every((n) => Number.isInteger(n.layer) && Number.isFinite(n.x) && Number.isFinite(n.y) && n.w > 0 && n.h > 0));
});
// Cards carry their position so a double-click can open the source.
test("buildScene nodes carry a source position", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  const app = scene.nodes.find((n) => n.title === "App");
  assert.equal(app.pos, "app.go:5");
  assert.equal(typeof app.pos, "string");
});
// A package of embed variables (internal/assets is one) has no types and no
// functions; its variables must still reach the canvas and the card count.
test("a package of variables gets value cards and a count", () => {
  const model = {
    path: "example.com/v",
    packages: [{
      path: "example.com/v",
      name: "v",
      types: [],
      funcs: [],
      consts: [],
      vars: [
        { name: "UI", type: "[]byte", exported: true },
        { name: "Limit", type: "int", value: "2", exported: true },
        { name: "JS", type: "[]byte", exported: true },
        { name: "CSS", type: "[]byte", exported: true }
      ]
    }]
  };
  const idx = ui.indexModel(model);

  const pkgScene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/v" }, false);
  const values = pkgScene.nodes.filter((n) => n.kind === "value");
  assert.deepEqual(values.map((n) => n.title).sort(), ["CSS", "JS", "Limit", "UI"]);
  assert.ok(values.filter((n) => n.title !== "Limit").every((n) => n.sub.indexOf("[]byte") >= 0),
    JSON.stringify(values.map((n) => n.sub)));
  // Every card must have finite coordinates: NaN draws an invisible card.
  assert.ok(values.every((n) => Number.isFinite(n.x) && Number.isFinite(n.y) && Number.isFinite(n.w) && Number.isFinite(n.h)),
    JSON.stringify(values.map((n) => [n.x, n.y, n.w, n.h])));

  const home = ui.buildScene(model, idx, { view: "home" }, false);
  assert.ok(home.nodes[0].rows[0].indexOf("4 变量") >= 0, home.nodes[0].rows[0]);
});
test("value cards show the initialiser", () => {
  assert.equal(ui.valueSub({ type: "[]byte" }), "var []byte");
  assert.equal(ui.valueSub({ type: "int", value: "2" }), "var int = 2");
  assert.equal(ui.valueSub({ kind: "const", value: "3" }), "const = 3");
});
test("type cards prefer the declaration source, functions prefer the doc", () => {
  assert.equal(ui.docLine("A is a type.\nMore."), "A is a type.");
  assert.equal(ui.docLine(""), "");
  assert.deepEqual(ui.textRows("type T struct {\n\tA int\n}"), ["type T struct {", "  A int", "}"]);
});
test("package cards carry the worst CRAP in the package", () => {
  const model = fixture();
  model.packages[0].funcs = [{ name: "New", sig: "()", metrics: { crap: 12 } }];
  model.packages[0].types[1].methods = [{ name: "Start", sig: "() error", metrics: { crap: 40 } }];
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false);
  const card = scene.nodes.find((n) => n.title === "app");
  assert.equal(card.crap, 40);
});
test("package cards show the doc comment when the package has one", () => {
  const model = {
    path: "example.com/m",
    packages: [
      { path: "example.com/m/a", name: "a", doc: "Package a does things.", types: [], funcs: [] },
      { path: "example.com/m/b", name: "b", types: [], funcs: [] },
    ],
  };
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "home" }, false);
  const a = scene.nodes.find((n) => n.title === "a");
  const b = scene.nodes.find((n) => n.title === "b");
  assert.equal(a.sub, "Package a does things.");
  assert.equal(b.sub, "b");
});

// ---------- invariant tests: one flow direction, cards, panels ----------
//
// These are the regression net for the diagonal-edge bug: call edges used to
// connect two cards that sat in the same horizontal row while the scene
// declared a vertical flow, so the router left the source's bottom and entered
// the target's top from inside the card. Every check below is phrased against
// geometry (or the panel HTML), never against the router's internals.

function invariantModel() {
  return {
    path: "example.com/app",
    root: "E:/app",
    editor: "vscode",
    layers: [["cmd"], ["internal"]],
    metrics: { average: 4.2, exceeded: 1, total_funcs: 5, thresholds: [5, 30] },
    calls: [
      // Two package-level functions share one band: the bug case.
      { from: "example.com/app#Load", to: "example.com/app#LoadWith" },
      // A method call between two type cards.
      { from: "example.com/app#App.Foo", to: "example.com/app#Server.Bar" },
      // A call from the program entry.
      { from: "example.com/app#Root", to: "example.com/app#Load" }
    ],
    edges: [
      { kind: "implements", from: "example.com/app#Impl", to: "example.com/app#Iface" }
    ],
    packages: [
      {
        path: "example.com/app",
        name: "app",
        doc: "Package app is the fixture root.",
        imports: ["example.com/app/internal/store"],
        main: true,
        types: [
          {
            id: "example.com/app#App", name: "App", kind: "struct", exported: true,
            doc: "App wires the server.", pos: "app.go:5",
            fields: [{ name: "srv", type: "Server", exported: true, refs: ["example.com/app#Server"] }],
            methods: [{ id: "example.com/app#App.Foo", name: "Foo", sig: "()", exported: true, pos: "app.go:8" }]
          },
          {
            id: "example.com/app#Server", name: "Server", kind: "struct", exported: true,
            doc: "Server serves requests.", pos: "server.go:10",
            fields: [],
            methods: [{ id: "example.com/app#Server.Bar", name: "Bar", sig: "()", exported: true, pos: "server.go:12" }]
          },
          {
            id: "example.com/app#Iface", name: "Iface", kind: "interface", exported: true,
            doc: "Iface is the contract.", pos: "iface.go:3",
            fields: [], methods: [{ name: "Get", sig: "()", exported: true }]
          },
          {
            id: "example.com/app#Impl", name: "Impl", kind: "struct", exported: true,
            doc: "Impl implements Iface.", pos: "impl.go:7",
            fields: [], methods: []
          },
          {
            // A card with no doc: `sub` must fall back to the declaration kind.
            id: "example.com/app#Plain", name: "Plain", kind: "type", exported: true,
            pos: "plain.go:1", fields: [], methods: []
          }
        ],
        funcs: [
          { id: "example.com/app#Load", name: "Load", sig: "(string) error", doc: "Load reads the model.", pos: "load.go:20", exported: true },
          { id: "example.com/app#LoadWith", name: "LoadWith", sig: "(string, Options) error", doc: "LoadWith reads it with options.", pos: "load.go:31", exported: true },
          { id: "example.com/app#Root", name: "Root", sig: "()", doc: "Root is the entry point.", pos: "main.go:10", exported: true }
        ],
        consts: [{ name: "MaxRetries", type: "int", value: "3", doc: "MaxRetries bounds retries.", pos: "app.go:40", exported: true }],
        vars: [{ name: "DefaultStore", type: "Store", doc: "DefaultStore is the default store.", pos: "app.go:44", exported: true }]
      },
      {
        path: "example.com/app/internal/store",
        name: "store",
        doc: "Package store keeps things.",
        imports: [],
        types: [
          {
            id: "example.com/app/internal/store#DB", name: "DB", kind: "interface", exported: true,
            doc: "DB is the storage contract.", pos: "db.go:3",
            fields: [], methods: [{ name: "Get", sig: "(string) error", exported: true }]
          }
        ],
        funcs: [],
        consts: [],
        vars: []
      }
    ]
  };
}

function invariantScenes() {
  const model = invariantModel();
  const idx = ui.indexModel(model);
  return {
    model,
    idx,
    scenes: [
      {
        name: "home",
        route: { view: "home" },
        scene: ui.buildScene(model, idx, { view: "home" }, true, { calls: true })
      },
      {
        name: "package/layer",
        route: { view: "pkg", path: "example.com/app" },
        scene: ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, true, { calls: true })
      },
      {
        name: "package/no-calls",
        route: { view: "pkg", path: "example.com/app" },
        scene: ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, true, { calls: false })
      }
    ]
  };
}

const EDGE_PATH_RE = new RegExp(
  "^M(-?\\d+(?:\\.\\d+)?),(-?\\d+(?:\\.\\d+)?) " +
  "C(-?\\d+(?:\\.\\d+)?),(-?\\d+(?:\\.\\d+)?) " +
  "(-?\\d+(?:\\.\\d+)?),(-?\\d+(?:\\.\\d+)?) " +
  "(-?\\d+(?:\\.\\d+)?),(-?\\d+(?:\\.\\d+)?)$"
);

function parseEdgePath(d) {
  const m = EDGE_PATH_RE.exec(d);
  assert.ok(m, "unexpected edge path syntax: " + d);
  return {
    start: [Number(m[1]), Number(m[2])],
    c1: [Number(m[3]), Number(m[4])],
    c2: [Number(m[5]), Number(m[6])],
    end: [Number(m[7]), Number(m[8])]
  };
}

function borderSides(p, n) {
  const sides = [];
  if (Math.abs(p[1] - n.y) <= 1) sides.push("top");
  if (Math.abs(p[1] - (n.y + n.h)) <= 1) sides.push("bottom");
  if (Math.abs(p[0] - n.x) <= 1) sides.push("left");
  if (Math.abs(p[0] - (n.x + n.w)) <= 1) sides.push("right");
  return sides;
}

function assertBorderCentre(p, n, where) {
  const sides = borderSides(p, n);
  assert.equal(sides.length, 1,
    `${where}: endpoint ${p} must sit on exactly one border of ${n.id} ${n.x},${n.y} ${n.w}x${n.h}, got ${sides}`);
  const side = sides[0];
  if (side === "top" || side === "bottom") {
    assert.ok(Math.abs(p[0] - (n.x + n.w / 2)) <= 1,
      `${where}: ${side} endpoint ${p} is not on the centreline x=${n.x + n.w / 2}`);
  } else {
    assert.ok(Math.abs(p[1] - (n.y + n.h / 2)) <= 1,
      `${where}: ${side} endpoint ${p} is not on the centreline y=${n.y + n.h / 2}`);
  }
  return side;
}

function assertOutside(point, n, side, where) {
  if (side === "top") {
    assert.ok(point[1] <= n.y + 0.5, `${where}: control ${point} is inside the top of ${n.id}`);
  } else if (side === "bottom") {
    assert.ok(point[1] >= n.y + n.h - 0.5, `${where}: control ${point} is inside the bottom of ${n.id}`);
  } else if (side === "left") {
    assert.ok(point[0] <= n.x + 0.5, `${where}: control ${point} is inside the left of ${n.id}`);
  } else {
    assert.ok(point[0] >= n.x + n.w - 0.5, `${where}: control ${point} is inside the right of ${n.id}`);
  }
}

function eachSceneEdge(where, scene, visit) {
  const byId = new Map(scene.nodes.map((n) => [n.id, n]));
  scene.edges.forEach((e) => {
    const a = byId.get(e.from);
    const b = byId.get(e.to);
    if (!a || !b) return;
    visit({ a, b, e, d: ui.edgePath(a, b) });
  });
}

test("layout puts every node of a layer on the layer's top coordinate", () => {
  const { scenes } = invariantScenes();
  for (const { name, scene } of scenes) {
    assert.ok(scene.nodes.length > 0, name + ": no nodes");
    assert.equal(scene.axis, "v", name + ": a view has one flow direction");
    const layers = new Map();
    scene.nodes.forEach((n) => {
      assert.ok(Number.isInteger(n.layer),
        `${name}: ${n.id} has no integer flow layer (${JSON.stringify(n.layer)})`);
      if (!layers.has(n.layer)) layers.set(n.layer, []);
      layers.get(n.layer).push(n);
    });
    layers.forEach((nodes, layer) => {
      const first = nodes[0].y;
      nodes.forEach((n) => {
        assert.ok(Math.abs(n.y - first) <= 1,
          `${name}: layer ${layer} mixes y=${n.y} (${n.id}) with y=${first}`);
      });
    });
  }
});

test("every edge endpoint sits on the centre of a border of the box it connects", () => {
  const { scenes } = invariantScenes();
  for (const { name, scene } of scenes) {
    eachSceneEdge(name, scene, ({ a, b, e, d }) => {
      const p = parseEdgePath(d);
      assertBorderCentre(p.start, a, `${name} ${e.from}->${e.to} start`);
      assertBorderCentre(p.end, b, `${name} ${e.from}->${e.to} end`);
    });
  }
});

test("every edge uses bottom and top ports, including cycle and span edges", () => {
  const { scenes } = invariantScenes();
  for (const { name, scene } of scenes) {
    eachSceneEdge(name, scene, ({ a, b, e, d }) => {
      const p = parseEdgePath(d);
      const sa = assertBorderCentre(p.start, a, `${name} ${e.from}->${e.to} start`);
      const sb = assertBorderCentre(p.end, b, `${name} ${e.from}->${e.to} end`);
      assert.equal(sa, "bottom", `${name}: ${e.from}->${e.to} must leave the bottom (${d})`);
      assert.equal(sb, "top", `${name}: ${e.from}->${e.to} must enter the top (${d})`);
      // A cycle edge may point across or even up a wrapped row; its style still
      // owns the same port pair. Every other edge descends a row.
      if (e.cycle) return;
      assert.ok(b.layer > a.layer,
        `${name}: ${e.from} (${a.layer}) -> ${e.to} (${b.layer}) must move down a row`);
    });
  }
});

test("regression: a call between two function cards is layered, not side-routed", () => {
  const model = {
    path: "example.com/app",
    calls: [{ from: "example.com/app#Load", to: "example.com/app#LoadWith" }],
    packages: [{
      path: "example.com/app",
      name: "app",
      imports: [],
      types: [],
      funcs: [
        { id: "example.com/app#Load", name: "Load", sig: "()", pos: "a.go:1", exported: true },
        { id: "example.com/app#LoadWith", name: "LoadWith", sig: "()", pos: "a.go:2", exported: true }
      ]
    }]
  };
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, { calls: true });
  const byId = new Map(scene.nodes.map((n) => [n.id, n]));
  const a = byId.get("example.com/app#Load");
  const b = byId.get("example.com/app#LoadWith");
  assert.equal(scene.axis, "v");
  // The old bug was two called functions in one row. The call edge now ranks
  // them into different layers, so a horizontal side route cannot occur.
  assert.ok(b.layer > a.layer, `call target must sit below its caller: ${a.layer} -> ${b.layer}`);
  assert.ok(b.y > a.y, "the two function cards must not share a row");

  const p = parseEdgePath(ui.edgePath(a, b));
  const sa = assertBorderCentre(p.start, a, "function call start");
  const sb = assertBorderCentre(p.end, b, "function call end");
  assert.equal(sa, "bottom");
  assert.equal(sb, "top");
  // The arrowhead is drawn at the path end, so the end is the target's top
  // centre. Compact rows do not promise a vertical tangent: the running order
  // can place the two cards side by side within their rows.
  assert.deepEqual(p.end, [b.x + b.w / 2, b.y]);
});

test("an edge approaches both boxes from outside, so its arrowhead is not buried in a card", () => {
  const { scenes } = invariantScenes();
  for (const { name, scene } of scenes) {
    eachSceneEdge(name, scene, ({ a, b, e, d }) => {
      const p = parseEdgePath(d);
      const sa = assertBorderCentre(p.start, a, `${name} ${e.from}->${e.to} start`);
      const sb = assertBorderCentre(p.end, b, `${name} ${e.from}->${e.to} end`);
      assertOutside(p.c1, a, sa, `${name} ${e.from}->${e.to} source`);
      assertOutside(p.c2, b, sb, `${name} ${e.from}->${e.to} target`);
    });
  }
});

test("every card has title, sub, href and the drill target its kind supports", () => {
  const { scenes } = invariantScenes();
  const seen = new Set();
  for (const { name, scene } of scenes) {
    scene.nodes.forEach((n) => {
      if (seen.has(n.id)) return;
      seen.add(n.id);
      assert.ok(n.title, `${name}: ${n.id} has no title`);
      assert.ok(n.sub, `${name}: ${n.id} has no sub line`);
      assert.ok(typeof n.href === "string" && n.href.charAt(0) === "#",
        `${name}: ${n.id} has no in-page href (${JSON.stringify(n.href)})`);
      if (n.kind === "pkg") {
        assert.equal(n.drillHref, ui.pkgHash(n.id), `${name}: ${n.id} must drill into the package`);
      } else if (n.pos) {
        assert.ok(n.drillHref.indexOf("vscode://file/") === 0,
          `${name}: ${n.id} has a source position but no drill target (${JSON.stringify(n.drillHref)})`);
      }
    });
  }
  assert.ok(seen.size >= 10, "fixture should exercise every card kind, got " + seen.size);
});

test("panelFrame is one frame for title, badges, meta, lead, doc and body", () => {
  assert.equal(typeof ui.panelFrame, "function", "panelFrame must be testable from ui.js");
  const html = ui.panelFrame({
    title: "Server",
    badges: [{ cls: "type", text: "struct" }],
    meta: ["example.com/app · server.go:10"],
    lead: '<p><code class="sig">Server{}</code></p>',
    doc: "Server serves requests.",
    body: "<h3>方法</h3>"
  });
  assert.match(html, /^<h2>Server <span class="badge type">struct<\/span><\/h2>/);
  assert.ok(html.includes('<div class="meta">example.com/app · server.go:10</div>'));
  assert.ok(html.includes('<p><code class="sig">Server{}</code></p>'));
  assert.ok(html.includes('<p class="doc">Server serves requests.</p>'));
  assert.ok(html.endsWith("<h3>方法</h3>"));
  assert.equal(ui.panelFrame({ title: "x" }), "<h2>x</h2>");
});

test("every declaration panel renders the doc paragraph", () => {
  const { model, idx, scenes } = invariantScenes();
  const pkgScene = scenes.find((s) => s.name === "package/layer").scene;
  const homeScene = scenes.find((s) => s.name === "home").scene;
  const ctx = { model, idx, all: true, from: "#/", selected: "example.com/app" };

  const cases = [
    { name: "home/package", route: { view: "home" }, scene: homeScene, doc: "Package app is the fixture root." },
    { name: "package", route: { view: "pkg", path: "example.com/app" }, scene: pkgScene, doc: "Package app is the fixture root." },
    { name: "type", route: { view: "type", id: "example.com/app#Server" }, scene: pkgScene, doc: "Server serves requests." },
    { name: "func", route: { view: "func", id: "example.com/app#Load" }, scene: pkgScene, doc: "Load reads the model." },
    { name: "value", route: { view: "value", id: "example.com/app#MaxRetries" }, scene: pkgScene, doc: "MaxRetries bounds retries." }
  ];
  for (const c of cases) {
    const html = ui.panelFor(c.route, c.scene, ctx);
    assert.ok(typeof html === "string" && html.length > 0, c.name + ": no panel");
    assert.ok(html.includes('<p class="doc">' + ui.esc(c.doc) + "</p>"),
      c.name + ": panel omits its doc paragraph\n" + html.slice(0, 200));
  }
  // A declaration without a doc must not grow an empty doc paragraph.
  const plain = ui.panelFor({ view: "type", id: "example.com/app#Plain" }, pkgScene, ctx);
  assert.ok(!plain.includes('class="doc"'), "a doc-less type must not render an empty doc paragraph");
});

// ---------- one flow direction: vertical layers from the graph ----------
//
// The old layout mixed directions: connected cards were placed on a fixed grid,
// so two cards in one row produced a horizontal edge inside a top-down scene.
// These tests describe the replacement contract. They are phrased against the
// scene (layers, boxes) and the router's own path, never against an internal
// loop order, so a different but equivalent layout algorithm can keep them.

// bigModel is the "含未导出" acceptance model: 24 types in a reference chain,
// 12 functions in a call chain, and six constants/variables. With `all: true`
// it has 42 cards, about three times the exported-only view.
function bigModel() {
  const p = "example.com/big";
  const types = [];
  for (let i = 0; i < 24; i++) {
    const next = "T" + String(i + 1).padStart(2, "0");
    types.push({
      id: p + "#T" + String(i).padStart(2, "0"),
      name: "T" + String(i).padStart(2, "0"),
      kind: i % 4 === 0 ? "interface" : "struct",
      exported: i % 3 === 0,
      doc: i % 3 === 0 ? "T" + i + " is the card for step " + i + "." : "",
      pos: "t.go:" + (i + 1),
      fields: i < 23 ? [{ name: "next", type: next, exported: true, refs: [p + "#" + next] }] : [],
      methods: [{ id: p + "#T" + String(i).padStart(2, "0") + ".M" + i, name: "M" + i,
        sig: "(int) error", exported: i % 3 !== 0 }]
    });
  }
  const funcs = [];
  for (let i = 0; i < 12; i++) {
    funcs.push({
      id: p + "#F" + String(i).padStart(2, "0"),
      name: "F" + String(i).padStart(2, "0"),
      sig: "(" + (i % 4) + " args) error",
      doc: i % 2 ? "F" + i + " does work.\nMore detail." : "",
      pos: "f.go:" + (i + 1),
      exported: i % 3 === 0
    });
  }
  const consts = [];
  const vars = [];
  for (let i = 0; i < 3; i++) {
    consts.push({ name: "C" + i, type: "int", value: String(i), exported: i % 3 === 0, pos: "c.go:" + (i + 1) });
    vars.push({ name: "V" + i, type: "string", value: "\"v" + i + "\"", exported: i % 3 === 0, pos: "v.go:" + (i + 1) });
  }
  const calls = [];
  for (let i = 0; i + 1 < funcs.length; i++) {
    calls.push({ from: funcs[i].id, to: funcs[i + 1].id });
  }
  // A call into a method belongs to the owner type's card, so this also makes
  // the first type a successor of the first function.
  calls.push({ from: p + "#F00", to: p + "#T00.M0" });
  return {
    path: p,
    calls,
    packages: [{
      path: p, name: "big", imports: [], types, funcs, consts, vars
    }]
  };
}

// compactModel is the 40-card compactness fixture: five ranks of eight short
// types, with each rank's head referencing every card of the next rank. The old
// centred-column layout had to widen its vertical gutter to the full 8-card row
// span for every hop; the row packer wraps each rank into two rows instead.
function compactModel() {
  const p = "example.com/fan";
  const types = [];
  const ids = [];
  for (let r = 0; r < 5; r++) {
    ids.push([]);
    for (let i = 0; i < 8; i++) {
      const name = "T" + r + i;
      const id = p + "#" + name;
      ids[r].push(id);
      types.push({
        id,
        name,
        kind: "struct",
        exported: true,
        pos: "fan.go:" + (r * 8 + i + 1),
        fields: [{ name: "f", type: "int", exported: true }],
        methods: []
      });
    }
  }
  for (let r = 0; r + 1 < 5; r++) {
    types[r * 8].fields = ids[r + 1].map((id, i) => ({
      name: "to" + i,
      type: id.split("#")[1],
      exported: true,
      refs: [id]
    }));
  }
  return {
    path: p,
    packages: [{ path: p, name: "fan", imports: [], types, funcs: [], consts: [], vars: [] }]
  };
}

// homeModel is four packages, two import edges and one isolated package.
function homeModel() {
  const p = "example.com/home";
  const empty = () => ({ types: [], funcs: [], consts: [], vars: [] });
  return {
    path: p,
    packages: [
      Object.assign({ path: p + "/a", name: "a", imports: [p + "/b"] }, empty()),
      Object.assign({ path: p + "/b", name: "b", imports: [p + "/c"] }, empty()),
      Object.assign({ path: p + "/c", name: "c", imports: [] }, empty()),
      Object.assign({ path: p + "/d", name: "d", imports: [] }, empty())
    ]
  };
}

// cycleModel is a three-type reference cycle: A -> B -> C -> A.
function cycleModel() {
  const p = "example.com/cycle";
  const edge = (from, to) => ({ name: from.toLowerCase(), type: to, exported: true, refs: [p + "#" + to] });
  return {
    path: p,
    packages: [{
      path: p, name: "cycle", imports: [],
      types: [
        { id: p + "#A", name: "A", kind: "struct", exported: true, pos: "a.go:1", fields: [edge("A", "B")], methods: [] },
        { id: p + "#B", name: "B", kind: "struct", exported: true, pos: "b.go:1", fields: [edge("B", "C")], methods: [] },
        { id: p + "#C", name: "C", kind: "struct", exported: true, pos: "c.go:1", fields: [edge("C", "A")], methods: [] }
      ],
      funcs: [], consts: [], vars: []
    }]
  };
}

// drawableEdges joins a scene's edges to the two cards they connect.
function drawableEdges(scene) {
  const byId = new Map(scene.nodes.map((n) => [n.id, n]));
  const out = [];
  scene.edges.forEach((e) => {
    const a = byId.get(e.from);
    const b = byId.get(e.to);
    if (a && b) out.push({ e, a, b });
  });
  return out;
}

// flowScenes is one scene per view shape: module, package, cycle and the
// invariant fixture. Every one of them must obey the same geometric contract.
function flowScenes() {
  const big = bigModel();
  const bigIdx = ui.indexModel(big);
  const cycle = cycleModel();
  const cycleIdx = ui.indexModel(cycle);
  const home = homeModel();
  const homeIdx = ui.indexModel(home);
  return [
    { name: "home", scene: ui.buildScene(home, homeIdx, { view: "home" }, true, { calls: true }) },
    {
      name: "package/all",
      scene: ui.buildScene(big, bigIdx, { view: "pkg", path: "example.com/big" }, true, { calls: true })
    },
    {
      name: "package/cycle",
      scene: ui.buildScene(cycle, cycleIdx, { view: "pkg", path: "example.com/cycle" }, true, { calls: true })
    },
    {
      name: "package/invariant",
      scene: invariantScenes().scenes.find((s) => s.name === "package/layer").scene
    }
  ];
}

test("flow scenes keep cycle, span and normal dependency disjoint", () => {
  let checked = 0;
  let cycles = 0;
  let spans = 0;
  for (const { name, scene } of flowScenes()) {
    const ends = drawableEdges(scene);
    assert.ok(ends.length > 0, name + ": scene has no drawable edges");
    for (const { e, a, b } of ends) {
      assert.ok(Number.isInteger(a.rank), `${name}: ${a.id} has no integer rank`);
      assert.ok(Number.isInteger(b.rank), `${name}: ${b.id} has no integer rank`);
      assert.ok(!(e.cycle && e.span), `${name}: ${e.from} -> ${e.to} is both cycle and span`);
      if (e.cycle) {
        cycles++;
        assert.equal(a.rank, b.rank,
          `${name}: cycle edge ${e.from} -> ${e.to} must share a component rank`);
        continue;
      }
      if (e.span) {
        spans++;
        assert.ok(b.layer > a.layer,
          `${name}: span edge ${e.from} -> ${e.to} must still point down a row`);
        continue;
      }
      assert.ok(b.rank > a.rank,
        `${name}: ${e.from} (rank ${a.rank}) -> ${e.to} (rank ${b.rank}) must deepen`);
      assert.ok(b.layer > a.layer,
        `${name}: ${e.from} (row ${a.layer}) -> ${e.to} (row ${b.layer}) must point down a row`);
      checked++;
    }
  }
  assert.ok(checked >= 20, "expected the fixture set to exercise many normal edges, got " + checked);
  assert.ok(cycles >= 3, "the cycle scene must contribute all three cycle edges, got " + cycles);
  assert.ok(spans >= 0, "span count is reported, not required in every scene");
});

test("every edge uses bottom-centre and top-centre ports, with the arrow at the target", () => {
  for (const { name, scene } of flowScenes()) {
    for (const { e, a, b } of drawableEdges(scene)) {
      const route = ui.edgeRoute(a, b);
      const p = parseEdgePath(route.d);
      const where = `${name}: ${e.from} -> ${e.to}` +
        (e.cycle ? " (cycle)" : (e.span ? " (span)" : ""));
      assert.equal(route.start.side, "bottom", `${where}: the route must leave the bottom`);
      assert.equal(route.end.side, "top", `${where}: the route must enter the top`);
      assert.ok(Math.abs(p.start[0] - (a.x + a.w / 2)) <= 1,
        `${where}: starts at x=${p.start[0]}, not the bottom centreline ${a.x + a.w / 2} (${route.d})`);
      assert.ok(Math.abs(p.start[1] - (a.y + a.h)) <= 1,
        `${where}: starts at y=${p.start[1]}, not the source bottom ${a.y + a.h} (${route.d})`);
      assert.ok(Math.abs(p.end[0] - (b.x + b.w / 2)) <= 1,
        `${where}: ends at x=${p.end[0]}, not the top centreline ${b.x + b.w / 2} (${route.d})`);
      assert.ok(Math.abs(p.end[1] - b.y) <= 1,
        `${where}: ends at y=${p.end[1]}, not the target top ${b.y} (${route.d})`);
      // The arrow is bound to the path end in ui.html (marker-end). The end
      // point is the target's top centre, so the head lands on the target's
      // border rather than in the gap. No |dy| > |dx| check: row packing can
      // put a compact edge at any slope, and only the ports are the contract.
      assert.deepEqual(p.end, [b.x + b.w / 2, b.y]);
    }
  }
});

test("forty-plus cards lay out without overlaps and grow the canvas", () => {
  const model = bigModel();
  const idx = ui.indexModel(model);
  const route = { view: "pkg", path: "example.com/big" };
  const shown = ui.buildScene(model, idx, route, false, { calls: true });
  const all = ui.buildScene(model, idx, route, true, { calls: true });

  assert.ok(all.nodes.length >= 40, "the all=true model must have 40+ cards, got " + all.nodes.length);
  assert.ok(all.nodes.length >= shown.nodes.length * 2,
    "含未导出 must roughly double the cards: " + shown.nodes.length + " -> " + all.nodes.length);

  let overlapsFound = 0;
  for (let i = 0; i < all.nodes.length; i++) {
    for (let j = i + 1; j < all.nodes.length; j++) {
      if (overlaps(all.nodes[i], all.nodes[j])) overlapsFound++;
    }
  }
  assert.equal(overlapsFound, 0, overlapsFound + " overlapping card pairs out of " + all.nodes.length);

  all.nodes.forEach((n) => {
    assert.ok(Number.isFinite(n.x) && Number.isFinite(n.y), n.id + " has a non-finite position");
    assert.ok(n.w > 0 && n.h > 0, n.id + " has a non-positive size");
    assert.ok(Number.isInteger(n.layer), n.id + " has no integer layer");
  });
  // The cards really are variable-sized: width follows the text, height rows.
  assert.ok(new Set(all.nodes.map((n) => n.w)).size > 1, "card widths must vary with their text");
  assert.ok(new Set(all.nodes.map((n) => n.h)).size > 1, "card heights must vary with their rows");
  const width = Math.max(...all.nodes.map((n) => n.x + n.w)) - Math.min(...all.nodes.map((n) => n.x));
  const height = Math.max(...all.nodes.map((n) => n.y + n.h)) - Math.min(...all.nodes.map((n) => n.y));
  assert.ok(width > 0 && height > 0, "canvas must have positive bounds, got " + width + "x" + height);
  // Measured with the centred-column + adaptive-gutter layout that preceded
  // this change (ui.js at the previous commit): 1602 x 3655 = 5,855,310 px^2
  // for this same all=true scene. The row packer must stay smaller.
  const OLD_AREA = 5855310;
  assert.ok(width * height < OLD_AREA,
    `42 cards cover ${Math.round(width)}x${Math.round(height)} = ${Math.round(width * height)} px^2, ` +
    `not less than the old ${OLD_AREA} px^2`);
});

test("each layer is one left-packed row with no empty column slot", () => {
  const model = compactModel();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/fan" }, true, { calls: true });
  const layers = new Map();
  scene.nodes.forEach((n) => {
    if (!layers.has(n.layer)) layers.set(n.layer, []);
    layers.get(n.layer).push(n);
  });
  assert.equal(layers.size, 10, "five 8-card ranks wrap into two rows each");
  for (const [layer, list] of layers) {
    list.sort((a, b) => a.x - b.x);
    assert.equal(list[0].x, 40, `layer ${layer} must start at the left margin`);
    for (let i = 1; i < list.length; i++) {
      // The row is contiguous: no column is sized by another layer, so there
      // is no empty slot between two cards in the same row.
      assert.equal(list[i].x, list[i - 1].x + list[i - 1].w + 40,
        `layer ${layer}: ${list[i - 1].title} and ${list[i].title} are not one fixed gap apart`);
    }
  }
});

test("forty cards fit a smaller canvas than the centred-column layout", () => {
  const model = compactModel();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/fan" }, true, { calls: true });
  assert.equal(scene.nodes.length, 40, "the compactness fixture must stay at 40 cards");

  let overlapsFound = 0;
  for (let i = 0; i < scene.nodes.length; i++) {
    for (let j = i + 1; j < scene.nodes.length; j++) {
      if (overlaps(scene.nodes[i], scene.nodes[j])) overlapsFound++;
    }
  }
  assert.equal(overlapsFound, 0, overlapsFound + " overlapping pairs out of 40 cards");

  const width = Math.max(...scene.nodes.map((n) => n.x + n.w)) - Math.min(...scene.nodes.map((n) => n.x));
  const height = Math.max(...scene.nodes.map((n) => n.y + n.h)) - Math.min(...scene.nodes.map((n) => n.y));
  // Measured with the centred-column + adaptive-gutter layout that preceded
  // this change (ui.js at the previous commit): 1840 x 4615 = 8,491,600 px^2
  // for this same 40-card model. Keeping the constant here makes a regression
  // back to columns or adaptive gaps fail instead of silently passing.
  const OLD_AREA = 8491600;
  assert.ok(width * height < OLD_AREA,
    `40 cards cover ${Math.round(width)}x${Math.round(height)} = ${Math.round(width * height)} px^2, ` +
    `not less than the old ${OLD_AREA} px^2`);
});

test("laying out the same scene twice produces identical coordinates", () => {
  const model = bigModel();
  const idx = ui.indexModel(model);
  const route = { view: "pkg", path: "example.com/big" };
  const first = ui.buildScene(model, idx, route, true, { calls: true });
  const second = ui.buildScene(model, idx, route, true, { calls: true });
  assert.deepEqual(first.nodes, second.nodes);
  assert.deepEqual(first.edges, second.edges);

  const types = [mkType("A"), mkType("B"), mkType("C")];
  const edges = chainEdges();
  assert.deepEqual(
    ui.layoutTypes({ name: "p" }, types, { all: true, edges }),
    ui.layoutTypes({ name: "p" }, types, { all: true, edges })
  );
});

test("adding a card does not change the layer of any existing card", () => {
  const types = [mkType("A"), mkType("B"), mkType("C")];
  const edges = chainEdges();
  const base = ui.layoutTypes({ name: "p" }, types, { all: true, edges });

  // An isolated card: it joins layer 0 without changing any existing path.
  const withIsolated = ui.layoutTypes({ name: "p" }, types.concat([mkType("Z")]), { all: true, edges });
  // A new leaf: it extends the chain but cannot lengthen a path to A, B or C.
  const withLeaf = ui.layoutTypes({ name: "p" }, types.concat([mkType("D")]), {
    all: true,
    edges: edges.concat([{ from: "example.com/p#C", to: "example.com/p#D" }])
  });

  function layerMap(nodes) {
    const m = {};
    nodes.forEach((n) => { m[n.id] = n.layer; });
    return m;
  }
  const baseLayers = layerMap(base);
  for (const [label, nodes] of [["isolated", withIsolated], ["leaf", withLeaf]]) {
    const next = layerMap(nodes);
    for (const id of Object.keys(baseLayers)) {
      assert.equal(next[id], baseLayers[id], `${label}: ${id} moved from layer ${baseLayers[id]} to ${next[id]}`);
    }
  }
});

test("adding an unrelated card keeps every other layer's order", () => {
  // Two disconnected chains: A -> B and X -> Y.
  const types = [mkType("A"), mkType("B"), mkType("X"), mkType("Y")];
  const edges = [
    { from: "example.com/p#A", to: "example.com/p#B" },
    { from: "example.com/p#X", to: "example.com/p#Y" }
  ];
  const base = ui.layoutTypes({ name: "p" }, types, { all: true, edges });
  const extra = ui.layoutTypes({ name: "p" }, types.concat([mkType("Z")]), { all: true, edges });

  function byLayer(nodes) {
    const out = new Map();
    nodes.forEach((n) => {
      if (!out.has(n.layer)) out.set(n.layer, []);
      out.get(n.layer).push(n.id);
    });
    return out;
  }
  const baseLayers = byLayer(base);
  const extraLayers = byLayer(extra);
  const baseIds = new Set(base.map((n) => n.id));
  for (const [layer, ids] of baseLayers) {
    // The added card may sit in a base layer; only the existing relative
    // order is part of the contract.
    const kept = (extraLayers.get(layer) || []).filter((id) => baseIds.has(id));
    assert.deepEqual(kept, ids, `layer ${layer} was reordered by an unrelated card`);
  }
});

test("a synthetic cycle is one component and every intra-cycle edge is marked cycle", () => {
  const model = cycleModel();
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: "example.com/cycle" }, true, { calls: true });

  assert.equal(scene.nodes.length, 3);
  assert.equal(scene.components.length, 1, "A -> B -> C -> A is one SCC");
  const compOf = condenseInfo(scene);
  const ends = drawableEdges(scene);
  assert.equal(ends.length, 3, "the reference cycle keeps all three edges");
  for (const { e, a, b } of ends) {
    assert.equal(e.cycle, true, `${e.from} -> ${e.to} must be a cycle edge, not a broken edge`);
    assert.equal(e.span, undefined, "a cycle edge is not a layout span");
    assert.equal(compOf.get(e.from), compOf.get(e.to), "both ends of a cycle edge share the SCC");
    assert.equal(a.rank, b.rank, "every card of one SCC has the same rank");
  }
  // Every card still has a real position: a cycle must not drop nodes, and the
  // rank must stay 0 for the whole component (one topological level).
  scene.nodes.forEach((n) => {
    assert.ok(Number.isFinite(n.x) && Number.isFinite(n.y), n.id + " is unplaced");
    assert.equal(n.rank, 0, n.id + " must share the component's rank");
    assert.ok(Number.isInteger(n.layer), n.id + " has no drawn row");
  });
});

test("a cycle that wrapping splits across rows still uses bottom and top ports", () => {
  const p = "example.com/wcycle";
  const types = [];
  for (let i = 0; i < 8; i++) {
    const next = (i + 1) % 8;
    types.push({
      id: p + "#T" + i, name: "T" + i, kind: "struct", exported: true, pos: "w.go:" + (i + 1),
      fields: [{ name: "next", type: "T" + next, exported: true, refs: [p + "#T" + next] }],
      methods: []
    });
  }
  const model = {
    path: p,
    packages: [{ path: p, name: "wcycle", imports: [], types, funcs: [], consts: [], vars: [] }]
  };
  const idx = ui.indexModel(model);
  const scene = ui.buildScene(model, idx, { view: "pkg", path: p }, true, {});
  assert.equal(scene.nodes.length, 8);
  assert.equal(scene.components.length, 1, "one 8-card ring is one SCC");
  const rows = new Set(scene.nodes.map((n) => n.layer));
  assert.ok(rows.size > 1, "eight 200px cards must wrap into more than one row");

  let upward = 0;
  for (const { e, a, b } of drawableEdges(scene)) {
    assert.equal(e.cycle, true, `${e.from} -> ${e.to} belongs to the ring and must be a cycle edge`);
    const route = ui.edgeRoute(a, b);
    assert.equal(route.start.side, "bottom");
    assert.equal(route.end.side, "top");
    const parsed = parseEdgePath(route.d);
    assert.deepEqual(parsed.end, [b.x + b.w / 2, b.y]);
    assert.ok(Math.abs(parsed.start[1] - (a.y + a.h)) <= 1);
    if (b.layer < a.layer) upward++;
  }
  assert.ok(upward >= 1, "wrapping must make at least one cycle edge point back up a row");
  // The banner names the whole ring, not only the first two cards.
  assert.match(scene.cycleWarning, /环\(强连通\)/);
  for (let i = 0; i < 8; i++) assert.ok(scene.cycleWarning.includes("T" + i));
});

test("the horizontal-axis option is gone", () => {
  assert.equal(ui.flowAxis, undefined, "flowAxis must be deleted, not left as a second code path");
  for (const { name, scene } of flowScenes()) {
    assert.equal(scene.axis, "v", name + ": every scene has the same top-down flow");
  }
  // The router takes two boxes; there is no axis argument to disagree about.
  const a = { x: 0, y: 0, w: 100, h: 50 };
  const b = { x: 300, y: 200, w: 100, h: 50 };
  const p = parseEdgePath(ui.edgePath(a, b));
  assert.deepEqual(p.start, [50, 50]);
  assert.deepEqual(p.end, [350, 200]);
});