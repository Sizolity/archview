# Browser UI: one flow direction, one layered layout

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
  approaches from outside.
- Layers come from the dependency graph, not from a row width. `layer(to) >
  layer(from)` for every non-back edge, including `uses`/`implements`/`embeds`
  and, when the 调用边 box is ticked, call edges.

## Layout algorithm

`buildScene` builds the cards and the complete edge list first, then calls
`layoutGraph(nodes, edges)`. The engine has three steps.

### 1. Layers and back edges

`assignLayers` (publicly `rankTypes`) works on the drawn edge list:

1. Deduplicate directed pairs and drop self-loops.
2. Run a depth-first search from the indegree-zero nodes in declaration-id
   order, then from any remaining node in declaration-id order. An edge into a
   node still on the DFS stack is a **back edge**: it is recorded and removed.
   Removing DFS back edges leaves an acyclic graph.
3. Relax the surviving edges until the layers stop moving:
   `layer(to) = max(layer(to), layer(from) + 1)`. That is the longest path from
   a root, so every surviving edge advances one or more layers.

`layoutGraph` marks the scene's own edge objects: `scene.edges[i].back = true`
only on the edges chosen in step 2. A back edge is the only edge exempt from
the layer-monotonicity rule. The page gives it a distinct look: class
`edge back`, an orange dashed stroke, the `arrow-back` marker and a
`回边(环路)` tooltip.

### 2. Ordering within a layer

`orderLayers` runs four barycenter passes, down/up/down/up. Each pass sorts one
layer by the average centre x of the cards it connects to. It uses every
direct neighbour, not only the adjacent layer, because an edge may skip a
layer; `assignX` recomputes the coordinates after each layer so a later layer
sees the current order. A card with no neighbour keeps its current x, so an
added, unrelated card cannot shuffle a whole layer. Every comparison ends with
the declaration ID, making the sort deterministic without relying on the
engine's sort stability. There are no random values anywhere.

### 3. Positions, variable sizes and centring

`placeLayers` owns the geometry:

- `cardWidth` estimates a card's width from its title, sub-line and rows;
  heights already vary with rows and the folded state.
- a column is as wide as the widest card at that position across all layers;
- a layer is as tall as its tallest card and is horizontally centred on the
  layout axis;
- cards are centred inside their columns, separated by `GAP_X`; layers by
  `GAP_Y`, with a 40px margin.

The vertical gutter is **adaptive**. For every edge crossing the gap between
layers *i* and *i+1*, the gap grows until it is wider than that edge's
horizontal run. The drop across one or more crossed gaps is then always
larger than the run, so `|y2 - y1| > |x2 - x1|` holds for every edge, not only
for well-shaped samples. The canvas grows with the number of layers and the
number of cards in a layer; nothing overlaps.

`layoutGraph` flattens the cards back into `scene.nodes` in draw order and
writes `x`, `y`, `w`, `h`, `layer` onto each card.

## Public shapes changed

| Shape | Meaning |
| --- | --- |
| `scene.nodes[i].layer` | Integer longest-path layer over the scene's dependency edges. All cards in one layer share the same `y`. Previously this was a row/column index produced by the grid layouts. |
| `scene.nodes` | Ordered by layer, then by the in-layer barycenter order (declaration ID breaks ties), so drawing and folding see a deterministic order. |
| `scene.edges[i].back` | Present and `true` only on DFS back edges; absent on ordinary edges. |
| `scene.axis` | Always `"v"`; there is no other value and no option that sets it. |
| `scene.nodes[i].w` / `h` | Variable, derived from text and rows instead of a fixed `CARD_W`/row height. |

Removed outright: `flowAxis`, `opts.axis` and every axis branch, the
`按类型分组` layout mode and its `<select>`, `PER_ROW`/`COLS`, the function and
value bands, and the same-layer side routing. Passing `layout` in a view object
is ignored; it cannot bring a second direction back.

## Invariants that guard the contract

| Invariant | Test |
| --- | --- |
| Every non-back edge goes strictly down a layer | `every non-back edge points to a strictly deeper layer` |
| Every edge is predominantly vertical and uses bottom/top border centrelines | `every edge is predominantly vertical with bottom-centre and top-centre ports` |
| 40+ cards (including unexported functions and values) never overlap and grow the canvas | `forty-plus cards lay out without overlaps and grow the canvas` |
| Two layouts of one scene are identical | `laying out the same scene twice produces identical coordinates` |
| Adding a card does not change an existing card's layer | `adding a card does not change the layer of any existing card` |
| An unrelated card does not reorder another layer | `adding an unrelated card keeps every other layer's order` |
| A cycle produces one marked back edge and still places every card | `a synthetic cycle produces a marked back edge, not an endless rank walk` |
| There is no horizontal-axis code path left | `the horizontal-axis option is gone` |
| A layer shares one top coordinate | `layout puts every node of a layer on the layer's top coordinate` |
| Endpoints sit on a border centreline | `every edge endpoint sits on the centre of a border of the box it connects` |
| A call between two function cards is layered, not side-routed | `regression: a call between two function cards is layered, not side-routed` |
| The arrowhead approaches from outside | `an edge approaches both boxes from outside, so its arrowhead is not buried in a card` |

## Component boundaries

| Concern | Owner | Contract | Guard |
| --- | --- | --- | --- |
| Flow direction | `FLOW_AXIS` / `buildScene` | Constant `"v"` per scene; no option and no branch | `the horizontal-axis option is gone`; `buildScene gives every view the same top-down flow` |
| Layers and cycle break | `assignLayers` / `rankTypes` in `ui.js` | Integer longest-path layers; DFS back edges marked on scene edges | the layer, cycle and add-card tests |
| In-layer order | `orderLayers` in `ui.js` | Four deterministic x-barycenter passes over all direct neighbours; declaration ID tie-break | determinism and unrelated-layer tests |
| Boxes and gutters | `packageCards`, `layoutGraph`, `assignX`, `placeLayers` in `ui.js` | Fill `x`, `y`, `w`, `h`, `layer`; centre layers; no overlaps | 40-card, layer-coordinate and determinism tests |
| Card data and size | `buildCard` + `cardWidth` in `ui.js` | Every card has `title`, `sub`, `href`, `drillHref` and a text-derived `w` | card and panel tests |
| Edge geometry | `edgeRoute` / `edgePath` in `ui.js` | Two boxes in, bottom-centre → top-centre out | vertical, endpoint and outside-approach tests |
| Edge style | `CARD_STYLE`, `drawNode`, `edgeClass`, `markerFor`, `drawEdges` in `ui.html` | Map a node/edge to SVG; attach no listeners; back edges carry `.back` | smoke click/drill checks; `edge back` CSS |
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
go test ./...
```

The page and `/ui.js` are served separately by the HTTP server and inlined into
the single-file export by `internal/export`; both paths consume the same
`ui.html` placeholder and `ui.js`, so this layout change introduces no new
asset route.
