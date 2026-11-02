# Browser UI: one flow direction, condensation depth

The old package view placed cards on a fixed four-per-row grid. Card
neighbours in one row were therefore joined by horizontal lines while other
edges ran vertically: one view had two flow directions. The fix is not another
grid offset. Every view now flows top to bottom, and the layer engine makes
that a structural property of the scene.

## The contract

- `scene.axis` is the constant `"v"`. There is no `flowAxis`, no `opts.axis`
  and no layout selector: nothing can choose a direction. The field stays on
  the scene so existing consumers can assert the contract, but no code branches
  on it.
- `edgeRoute(a, b)` takes two boxes and no axis. It always leaves the source's
  bottom border at its centreline and enters the target's top border at its
  centreline; both control points are outside the boxes, so the arrowhead
  approaches from outside. A cycle edge may point back up after wrapping; it
  still uses the same bottom/top ports.
- Depth is topological. `n.rank` is the card's longest-path rank over the
  condensation; every card of one strongly connected component shares it.
  `n.layer` is the drawn row after wrapping (`n.layer >= n.rank`). For every
  edge that is neither `cycle` nor `span`, `rank(to) > rank(from)` and
  `layer(to) > layer(from)`.
- Topology reads structural edges only. Every drawn edge carries a semantic
  `class`: `structural`, `data` or `call`. `condense`, `rankTypes` and the cycle
  warning drop everything except `structural`; data and call edges are still
  drawn.
- A rank wider than `ROW_MAX_W` may span several consecutive rows, but all of
  its rows stay between the same neighbouring ranks.

## Edge meaning: structural, data, call

`kind` and `class` are independent. `kind` stays the drawing axis (`uses`,
`implements`, `embeds`, `value`); `class` is the semantic axis:

| `class` | What it means | Topology? |
| --- | --- | --- |
| `structural` | a reference to a type from a type declaration, method, function signature or body; type-to-type `implements`/`embeds`; a package import; a field reference | yes |
| `data` | a declaration reading a package-level const/var, and a const/var naming the type it belongs to | never |
| `call` | one edge of the call graph | never |

An edge without a `class` is treated as a legacy structural edge, so old
callers of `condense`/`rankTypes` keep the old meaning. The class is set where
the edge is built: `typePairs` and package imports are structural, a model
`uses` edge keeps the model's class (a value declaration's type reference is
data), `value` is data, and `callEdges` is call. The bug this separation fixes
is a typed constant: `Error → Severity` (the const's type) and
`Severity.String → Error` (the method reading the const) are one data loop, not
an architectural cycle.

## Layout algorithm

`buildScene` builds the cards and the complete edge list first, then calls
`layoutGraph(nodes, edges, meta)`. The engine has three steps; every edge
already carries its semantic class, and only structural edges reach the
condensation.

### 1. Condense, then take the longest path

`condense` (public, and the basis of `rankTypes`) works on the structural
subset of the drawn edge list:

1. Drop `data` and `call` edges, deduplicate directed pairs and drop self-loops
   from the adjacency.
2. Run Tarjan's strongly connected components algorithm. Roots are visited in
   declaration-id order and neighbours in edge order, so the component list is
   deterministic. It returns `components` (one array of card IDs per SCC),
   `compOf` (card ID → component index) and `compEdges` (component-level edges,
   deduplicated, no self-loops).
3. The condensation is a DAG by construction. Rank it with one Kahn pass and
   `rank(component) = 1 + max(rank(predecessors))`; roots are 0. Taking the
   maximum is the **longest path**, so a converging node keeps the depth of its
   deepest chain. There is no BFS and no shortest-path flattening.
4. Every card of a component takes the component's rank. A card keeps that
   rank after wrapping: `n.rank` is topology, `n.layer` is the row.

`layoutGraph` exposes the condensation through its optional `meta` argument;
`buildScene` copies `meta.components` onto the scene and adds the warning line
described below. `layoutPackages` and `layoutTypes` forward `opts.meta`, so the
direct layout entry points can expose the same data without touching the
document.

### 2. Ordering within a rank

`orderLayers` runs four barycenter passes, down/up/down/up. Each pass sorts one
rank by the average centre x of the cards it connects to. It uses every direct
neighbour, not only the adjacent rank, because an edge may skip a rank;
`packRows` recomputes the coordinates after each rank so a later rank sees the
current order. A card with no neighbour keeps its current x, so an added,
unrelated card cannot shuffle a whole rank. Every comparison ends with the
declaration ID, making the sort deterministic without relying on the engine's
sort stability. There are no random values anywhere. Both the rank order and
the final row order use the same pass.

### 3. Rows, wrapping and fixed gaps

The position rules are the reference's (`unclebob/uml-viewer`,
`src/uml_viewer/engine/layout.clj`): class-gap 40 inside a rank, a fixed rank
gap, and a 40px margin. We use `GAP_Y = 120`, the compact end of the requested
120-140 range (the reference's `class-rank-gap` is 140).

- `packRows` makes every rank one horizontal row, packed **left to right from
  the same left margin** with `GAP_X = 40`. A card uses only its own width; no
  column is sized by the widest card elsewhere, cards are not centred within
  columns, and a row has no empty slot. Rows are not centred on a common axis.
- A rank in the repo model held 25 wide cards, which made a 10,259px
  single-row ribbon. `wrapRows` therefore continues a rank on the next row when
  adding the next card would exceed `ROW_MAX_W = 1300`, preserving the rank and
  row order. Every row is still one left-packed row; a rank may occupy several
  consecutive rows.
- `placeLayers` gives every row its y. A row is as tall as its tallest card and
  rows are separated by the fixed `GAP_Y = 120`. There is no adaptive gutter:
  the height depends on the cards, never on how far an edge runs sideways. Rows
  never share a y band and a row's cards are laid end to end, so no two cards
  overlap.

`layoutGraph` flattens the cards back into `scene.nodes` in draw order and
writes `x`, `y`, `rank`, `layer` onto each card.

## Three layout marks

These marks are the layout layer on top of the semantic class. Classification
happens after the rows exist, on the scene's own edge objects. The marks are
disjoint: an edge carries `cycle`, `span`, or neither.

| Mark | Field | Condition | Style |
| --- | --- | --- | --- |
| normal dependency | neither field | A structural edge whose endpoints are in different components and which descends at least one rank and one row; also a data/call edge that descends a full rank and row | The kind's existing stroke and colour: `uses` violet (dashed reference), `implements` green, `embeds` blue, call cyan, package dependency grey, violation red |
| layout span | `span: true` | A structural edge whose same/adjacent-rank hop `wrapRows` stretched over extra rows (`layer(to) - layer(from) > rank(to) - rank(from)`), **or** a data/call edge that does not advance a layer (`rank(to) <= rank(from)`, or it stays on its row, or a one-rank hop is wrap-stretched) | Neutral grey dashed (`edge span`, marker `arrow-span`) |
| cycle | `cycle: true` | Both endpoints are in one **structural** SCC of two or more cards | Red dashed (`edge cycle`, marker `arrow-cycle`) |

The page labels normal edges `依赖` / `uses` / `implements` / `embeds` / `调用`,
spans `跨行/未推进` and cycles `环(强连通)` in the card help. The old `back` flag
mixed cycle-closing edges with shortest-path cross edges and is deleted, not
kept beside the new classes.

Span has two causes now. A structural span is still the extra distance a wrap
introduced: with no wrapping, every structural rank is one row, so
`layer(to) - layer(from) === rank(to) - rank(from)` and the edge is normal. A
data or call span is the semantic limit instead: the edge is drawn, but because
it does not advance a layer it cannot be a dependency, so it gets the neutral
style rather than a fake cycle or a banner. A data/call edge that really
descends a rank keeps its kind style. A cycle is never a span, even when
wrapping splits its members across rows.

### Cycles are surfaced

`buildScene` sets `scene.components` (all structural SCCs, member-sorted) and
`scene.cycleWarning`. When at least one structural component has two or more
cards, the warning is one deterministic line:

```
环(强连通): a ⇄ b
```

Multiple SCCs are joined with ` · `, ordered by their first declaration ID. A
scene whose every SCC is a single card has `cycleWarning === null` and grows no
banner. The page renders it in `#cyclewarn`, a red warning strip under the
model warnings; the string itself is built in pure `ui.js` and needs no DOM.

## Public shapes

| Shape | Meaning |
| --- | --- |
| `scene.nodes[i].rank` | Integer longest-path rank over the condensation. Every card of one SCC shares it. |
| `scene.nodes[i].layer` | Integer drawn row in draw order. `rank <= layer`; a rank split by `wrapRows` occupies several consecutive rows. All cards in one row share the same `y`. |
| `scene.nodes` | Ordered by row, then by the in-row barycenter order (declaration ID breaks ties), so drawing and folding see a deterministic order. |
| `scene.edges[i].class` | `structural`, `data` or `call`; the semantic axis. Only structural edges enter the condensation. |
| `scene.edges[i].cycle` | Present and `true` exactly when the edge is structural and both endpoints are in one structural SCC of two or more cards; absent otherwise. |
| `scene.edges[i].span` | Present and `true` when the edge does not advance a layer: a wrap-stretched structural hop, or a data/call edge that stays on its rank or points back up; absent otherwise. |
| `scene.components` | One sorted array of card IDs per structural SCC. Every card has exactly one component. |
| `scene.cycleWarning` | The `环(强连通): …` line, or `null` when the scene is a DAG of single-card SCCs. |
| `scene.axis` | Always `"v"`; there is no other value and no option that sets it. |
| `scene.nodes[i].w` / `h` | Variable, derived from text and rows instead of a fixed `CARD_W`/row height. |

The public `condense(nodes, edges)` returns `{components, compOf, compEdges,
compRank}` for callers that need the SCC tables without a scene; it filters the
input to structural edges, and an edge without a `class` keeps the old
structural meaning. `rankTypes` returns the per-card rank map.

Removed outright: the old `back` flag and every code path that set it,
`flowAxis`, `opts.axis` and every axis branch, the `按类型分组` layout mode and
its `<select>`, `PER_ROW`/`COLS`, the function and value bands, the same-layer
side routing, `assignX` and its column/centring model, and the adaptive
vertical gutter. Passing `layout` in a view object is ignored; it cannot bring
a second direction back.

## Invariants that guard the contract

| Invariant | Test |
| --- | --- |
| The condensation is acyclic, every card has a component, component edges are deduped and self-loop free | `condense returns every node's component and an acyclic condensation` |
| Depth is the longest path, not a BFS: a diamond keeps D at rank 2, a chain has ranks 0/1/2, and a converging triangle keeps C at 2 (a BFS flattens it to 1) | `depth comes from the longest path over the condensation, not a BFS`; `longest-path ranks keep a converging node at its deepest chain` |
| A converging node stays at its deepest chain in a scene | `longest-path ranks keep a converging node at its deepest chain` |
| Every non-cycle, non-span edge goes from a lower rank to a strictly higher rank (and row) | `every non-cycle, non-span edge goes to a strictly deeper rank`; `flow scenes keep cycle, span and normal dependency disjoint` |
| Cycle edges exist exactly when a structural SCC has two or more cards, join one component, and share its rank; data/call edges are never cycles | `cycle edges exist exactly for structural SCCs of two or more cards`; `a synthetic cycle is one component and every intra-cycle edge is marked cycle` |
| A wrapped scene produces structural span edges; an unwrapped one produces none for structural hops; a data/call edge that does not advance a layer also gets `span` | `structural span edges are produced only by wrapRows, and only for short hops`; `a data edge that goes up is a span, not a cycle` |
| A typed constant and its type are data, not an SCC: every structural component is one card and there is no banner | `typed constants are data, not a structural cycle or a banner`; `TestTypedConstantsAreDataNotTopology` |
| Two types that reference each other are one structural SCC of two cards with a deterministic warning | `two types referencing each other are one structural cycle with both edges`; `TestMutualTypeReferencesAreStructural` |
| `condense`/`rankTypes` ignore data and call classes; an unclassed edge stays structural | `condense and rankTypes ignore data and call classes` |
| Every scene reports its SCCs and one deterministic cycle line; a DAG has none | `an SCC of two or more cards adds one deterministic warning line to the scene` |
| The old `back` notion is gone | `the old back classification is gone, not kept beside cycle and span` |
| A wrapped SCC still routes every cycle edge bottom → top, even when wrapping makes it point up a row | `a cycle that wrapping splits across rows still uses bottom and top ports` |
| Every edge uses bottom/top border centrelines and ends on the target's top centre (where `marker-end` puts the arrow), cycle and span included | `every edge uses bottom-centre and top-centre ports, with the arrow at the target`; `every edge uses bottom and top ports, including cycle and span edges` |
| 40+ cards (including unexported functions and values) never overlap | `forty-plus cards lay out without overlaps and grow the canvas` |
| Each row is one left-packed row with the fixed gap and no empty slot | `each layer is one left-packed row with no empty column slot` |
| A 40-card scene covers less area than the re-measured reference budget (structural-only ranks changed the box) | `forty cards fit a smaller canvas than the centred-column layout`; `forty-plus cards lay out without overlaps and grow the canvas` |
| Two layouts of one scene are identical | `laying out the same scene twice produces identical coordinates` |
| Adding a card does not change an existing card's layer | `adding a card does not change the layer of any existing card` |
| An unrelated card does not reorder another layer | `adding an unrelated card keeps every other layer's order` |
| There is no horizontal-axis code path left | `the horizontal-axis option is gone` |
| A row shares one top coordinate | `layout puts every node of a layer on the layer's top coordinate` |
| Endpoints sit on a border centreline | `every edge endpoint sits on the centre of a border of the box it connects` |
| A call edge never ranks a card: it is drawn as a neutral span when it stays on a layer | `regression: a call between two function cards never side-routes` |
| The arrowhead approaches from outside | `an edge approaches both boxes from outside, so its arrowhead is not buried in a card` |
| Every card keeps title/sub/href/drill, and every panel keeps its doc | `every card has title, sub, href and the drill target its kind supports`; `every declaration panel renders the doc paragraph` |

## Component boundaries

| Concern | Owner | Contract | Guard |
| --- | --- | --- | --- |
| Flow direction | `FLOW_AXIS` / `buildScene` | Constant `"v"` per scene; no option and no branch | `the horizontal-axis option is gone`; `buildScene gives every view the same top-down flow` |
| Semantic classes | `buildScene` / `typePairs` / `callEdges` in `ui.js`; `refs.go` in `internal/model` | Every edge has `class` ∈ `structural`, `data`, `call`; only structural edges feed topology | class and typed-constant tests |
| Cycles and depth | `condense` / `assignLayers` / `rankTypes` in `ui.js` | Structural edges only; Tarjan SCCs; longest path over the condensation; card rank = component rank | condensation, depth, cycle and add-card tests |
| Layout marks | `layoutGraph` in `ui.js` | Record `cycle` on structural intra-SCC edges and `span` on wrap-stretched structural hops or data/call edges that do not advance; leave normal edges unmarked | cycle, span, non-cycle/non-span and no-back tests |
| Cycle warning | `cycleWarning` / `buildScene` in `ui.js`; `#cyclewarn` in `ui.html` | One deterministic line for structural SCCs of two or more cards; `null` on a DAG | typed-constant, mutual-reference and warning tests |
| In-row order | `orderLayers` in `ui.js` | Four deterministic x-barycenter passes over all direct neighbours; declaration ID tie-break | determinism and unrelated-layer tests |
| Boxes and gutters | `packageCards`, `layoutGraph`, `packRows`, `wrapRows`, `placeLayers` in `ui.js` | Fill `x`, `y`, `rank`, `layer`, `w`, `h`; left-packed rows, 40px card gap, 1300px row wrap, fixed 120px row gap; no overlaps | 40-card, row-packing, area, row-coordinate and determinism tests |
| Card data and size | `buildCard` + `cardWidth` in `ui.js` | Every card has `title`, `sub`, `href`, `drillHref` and a text-derived `w`; a value card's sub is `ValueInfo.Text` (`const Error Severity = iota`) or the `valueSub` fallback | card and panel tests; `value cards show the declaration, not just the initialiser` |
| Edge geometry | `edgeRoute` / `edgePath` in `ui.js` | Two boxes in, bottom-centre → top-centre out, cycle/span included | port, endpoint and outside-approach tests |
| Edge style | `CARD_STYLE`, `drawNode`, `edgeClass`, `markerFor`, `drawEdges` in `ui.html` | Map a node/edge to SVG; attach no listeners; `cycle` → `.cycle` red dashed, `span` → `.span` grey dashed | smoke click/drill checks; `edge cycle` / `edge span` CSS |
| Declaration → card | `cardIndex(scene)` in `ui.html` | One map, rebuilt on every draw | smoke click/double-click checks |
| Inspector HTML | `panelFrame`, `panelFor`, `panelHome`/`panelPkg`/`panelType`/`panelFunc`/`panelValue` in `ui.js` | Pure HTML strings | panel tests |
| Input | delegated `#svg` click/dblclick/pan handlers in `ui.html` | Cards and groups own no handlers | smoke click, fold, pan checks |

## Verification commands

```powershell
node --test --test-isolation=none
node .tmp/smoke.js
node .tmp/smoke-layers.js
node .tmp/smoke-focus.js
node .tmp/smoke-cards.js
node .tmp/smoke-export.js
node .tmp/smoke-value.js
go test ./...
```

The page and `/ui.js` are served separately by the HTTP server and inlined into
the single-file export by `internal/export`; both paths consume the same
`ui.html` placeholder and `ui.js`, so this layout change introduces no new
asset route.
