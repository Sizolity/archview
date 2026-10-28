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

test("layoutPackages puts importers to the right of what they import", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const nodes = ui.layoutPackages(model.packages, idx.pkgByPath, { modelPath: model.path, all: false });
  const app = nodes.find((n) => n.id === "example.com/app");
  const store = nodes.find((n) => n.id === "example.com/app/store");
  assert.ok(app.x > store.x, "app imports store, so it sits in a later column");
  assert.equal(store.sub, "store");
  assert.equal(store.title, "store");
  assert.equal(store.href, ui.pkgHash("example.com/app/store"));
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

test("buildScene falls back to the home view for an unknown package", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  assert.equal(ui.buildScene(model, idx, { view: "pkg", path: "example.com/missing" }, false).kind, "packages");
  assert.equal(ui.buildScene(model, idx, { view: "type", id: "nope" }, false), null);
});

test("edgePath never emits NaN, whichever axis or order", () => {
  const a = { x: 0, y: 0, w: 100, h: 50 };
  const b = { x: 300, y: 100, w: 100, h: 50 };
  for (const axis of ["v", "h"]) {
    for (const [from, to] of [[a, b], [b, a]]) {
      const d = ui.edgePath(from, to, axis);
      assert.match(d, /^M/, axis);
      assert.equal(d.includes("NaN"), false, axis);
    }
  }
});

test("buildScene gives each view a single flow axis", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const home = ui.buildScene(model, idx, { view: "home" }, false);
  assert.equal(home.axis, "h", "package columns flow left to right");
  const layered = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false);
  assert.equal(layered.axis, "v", "the layered type layout flows top to bottom");
  const grid = ui.buildScene(model, idx, { view: "pkg", path: "example.com/app" }, false, { layout: "kind" });
  assert.equal(grid.axis, "h", "the kind grid flows left to right");
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

test("declutter folds every card", () => {
  const types = [
    { id: "a", name: "A", kind: "struct", exported: true, fields: [{ name: "f", type: "int", exported: true }], methods: [] },
    { id: "b", name: "B", kind: "struct", exported: true, fields: [], methods: [] }
  ];
  const nodes = ui.layoutTypes({ name: "p" }, types, { all: true, declutter: true });
  assert.ok(nodes.every((n) => n.folded && n.rows.length === 0));
});

test("focusScene keeps one node plus its one-hop neighbours", () => {
  const scene = {
    kind: "types",
    key: "k",
    selected: "b",
    pkg: {},
    nodes: [{ id: "a" }, { id: "b" }, { id: "c" }, { id: "d" }],
    edges: [
      { from: "a", to: "b" },
      { from: "b", to: "c" }
    ]
  };
  const focused = ui.focusScene(scene, "b");
  assert.deepEqual(focused.nodes.map((n) => n.id).sort(), ["a", "b", "c"]);
  assert.equal(focused.edges.length, 2);
  assert.equal(focused.key, "k:focus:b");

  const lonely = ui.focusScene(scene, "d");
  assert.deepEqual(lonely.nodes.map((n) => n.id), ["d"]);
  assert.deepEqual(lonely.edges, []);
});

test("declutterScene hides edges that miss the selection", () => {
  const scene = {
    kind: "types",
    key: "k",
    selected: "a",
    nodes: [],
    edges: [
      { from: "a", to: "b" },
      { from: "c", to: "d" }
    ]
  };
  assert.deepEqual(ui.declutterScene(scene, "a").edges, [{ from: "a", to: "b" }]);
  assert.equal(ui.declutterScene(scene, null).edges.length, 2, "no selection hides nothing");
});

test("buildScene applies the 3.2 view filters", () => {
  const t = (name, refs) => ({
    id: "example.com/app#" + name,
    name: name,
    kind: "struct",
    exported: true,
    fields: refs.map((r, i) => ({ name: "f" + i, type: r, exported: true, refs: ["example.com/app#" + r] })),
    methods: []
  });
  const model = {
    path: "example.com/app",
    packages: [
      {
        path: "example.com/app",
        name: "app",
        imports: [],
        types: [t("A", ["B"]), t("B", ["C"]), t("C", []), t("D", [])],
        funcs: []
      }
    ]
  };
  const idx = ui.indexModel(model);
  const route = { view: "type", id: "example.com/app#B" };

  const plain = ui.buildScene(model, idx, route, false);
  assert.equal(plain.nodes.length, 4);
  assert.equal(plain.edges.length, 2);

  const focused = ui.buildScene(model, idx, route, false, { focus: "example.com/app#B" });
  assert.equal(focused.nodes.length, 3, "one node plus its two neighbours");
  assert.deepEqual(focused.nodes.map((n) => n.id).sort(), [
    "example.com/app#A",
    "example.com/app#B",
    "example.com/app#C"
  ]);

  const lonely = ui.buildScene(model, idx, route, false, { focus: "example.com/app#D" });
  assert.deepEqual(lonely.nodes.map((n) => n.id), ["example.com/app#D"]);
  assert.deepEqual(lonely.edges, []);

  const decluttered = ui.buildScene(model, idx, route, false, { declutter: true });
  assert.ok(decluttered.nodes.every((n) => n.folded));

  // The model itself is untouched by any of it.
  assert.equal(model.packages[0].types.length, 4);
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

test("layer layout stacks users above dependencies", () => {
  const types = [mkType("A"), mkType("B"), mkType("C")];
  const nodes = ui.layoutTypes({ name: "p" }, types, {
    all: true,
    layout: "layer",
    edges: chainEdges()
  });
  const t = byTitle(nodes);
  assert.ok(t.A.y < t.B.y, "A should sit above B");
  assert.ok(t.B.y < t.C.y, "B should sit above C");
});

test("layer layout is the default", () => {
  const nodes = ui.layoutTypes({ name: "p" }, [mkType("A"), mkType("B")], {
    all: true,
    edges: [{ from: "example.com/p#A", to: "example.com/p#B" }]
  });
  const t = byTitle(nodes);
  assert.ok(t.A.y < t.B.y);
});

test("kind layout groups structs, then interfaces, then named types", () => {
  const types = [mkType("Z", "type"), mkType("I", "interface"), mkType("B"), mkType("A")];
  const nodes = ui.layoutTypes({ name: "p" }, types, { all: true, layout: "kind" });
  assert.deepEqual(nodes.map((n) => n.title), ["A", "B", "I", "Z"]);
  assert.deepEqual(nodes.map((n) => n.sub.split(" · ")[0]), [
    "struct",
    "struct",
    "interface",
    "type"
  ]);
});

test("ten types and fifteen edges never overlap in either layout", () => {
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

  for (const mode of ["layer", "kind"]) {
    const first = ui.layoutTypes({ name: "p" }, types, { all: true, layout: mode, edges });
    const second = ui.layoutTypes({ name: "p" }, types, { all: true, layout: mode, edges });
    assert.deepEqual(first, second, mode + " must be deterministic");
    for (let i = 0; i < first.length; i++) {
      for (let j = i + 1; j < first.length; j++) {
        assert.equal(overlaps(first[i], first[j]), false, mode + " overlap: " + first[i].id + "/" + first[j].id);
      }
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

test("buildScene passes the layout mode through", () => {
  const model = fixture();
  const idx = ui.indexModel(model);
  const route = { view: "pkg", path: "example.com/app" };

  const layered = ui.buildScene(model, idx, route, false, { layout: "layer" });
  const t = byTitle(layered.nodes);
  // App uses Server, so App sits above it.
  assert.ok(t.App.y < t.Server.y, "layered layout puts the user above the dependency");

  const grouped = ui.buildScene(model, idx, route, false, { layout: "kind" });
  assert.deepEqual(grouped.nodes.filter((n) => n.kind === "type").map((n) => n.title), ["App", "Server"]);
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

test("edgePath routes horizontally when the target sits beside", () => {
  const d = ui.edgePath({ x: 0, y: 0, w: 100, h: 50 }, { x: 300, y: 20, w: 100, h: 50 }, "h");
  assert.match(d, /^M100,25 C/);
  assert.match(d, / 300,45$/);
});

test("edgePath points the same way whatever the order", () => {
  const a = { x: 0, y: 0, w: 100, h: 50 };
  const b = { x: 300, y: 300, w: 100, h: 50 };
  // Reversing the pair must produce the reverse path, so the arrow always ends
  // on the target and the line never doubles back on itself.
  const forward = ui.edgePath(a, b, "v");
  const backward = ui.edgePath(b, a, "v");
  assert.match(forward, /^M50,50 /);
  assert.match(backward, /^M350,300 /);
  assert.match(backward, / 50,50$/);
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
  // The band sits below every type card.
  const types = scene.nodes.filter((n) => n.kind === "type");
  assert.ok(fns.every((f) => types.every((t) => f.y > t.y + t.h)));
});