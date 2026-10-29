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
- Layer order comes from the dependency graph, not from a card budget.
  `layer(to) > layer(from)` for every non-back edge, including
  `uses`/`implements`/`embeds` and, when the 调用边 box is ticked, call edges. A
  rank wider than `ROW_MAX_W` may span several consecutive layers, but all of
  its rows stay between the same neighbouring ranks.

## Layout algorithm

`buildScene` builds the cards and the complete edge list first, then calls
`layoutGraph(nodes, edges)`. The engine has three steps.

### 1. Shortest-path layers and back edges

`assignLayers` (publicly `rankTypes`) works on the drawn edge list:

1. Deduplicate directed pairs and drop self-loops.
2. Run a depth-first search from the indegree-zero nodes in declaration-id
   order, then from any remaining node in declaration-id order. An edge into a
   node still on the DFS stack is a **back edge**: it is recorded and removed.
   Removing DFS back edges leaves an acyclic graph.
3. Rank the remaining DAG with a breadth-first walk from its roots: the nodes
   no kept edge points at, falling back to every id. The queue is FIFO,
   adjacency and roots are id-sorted, and a node is ranked on first discovery.
   Its rank is therefore the **shortest path** from a root, not the longest. A
   hub's targets stay next to the hub even when another path to them is longer;
   that is the compaction the reference gets from its `bfs-ranks`.
4. A shortest walk does not make every DAG edge point downward: a cross edge
   can land on the same rank or back on an earlier one. `layoutGraph` marks
   those edges `back` as well. In this app **`back` means "does not advance a
   layer"**, so it covers both a cycle-closing edge and a shortest-path cross
   edge, and every non-back edge still satisfies `layer(to) > layer(from)`.

The page gives a back edge class `edge back`, an orange dashed stroke, the
`arrow-back` marker and a `回边(环/跨层)` tooltip.

### 2. Ordering within a rank

`orderLayers` runs four barycenter passes, down/up/down/up. Each pass sorts one
rank by the average centre x of the cards it connects to. It uses every direct
neighbour, not only the adjacent rank, because an edge may skip a rank;
`packRows` recomputes the coordinates after each rank so a later rank sees the
current order. A card with no neighbour keeps its current x, so an added,
unrelated card cannot shuffle a whole rank. Every comparison ends with the
declaration ID, making the sort deterministic without relying on the engine's
sort stability. There are no random values anywhere.

### 3. Rows, wrapping and fixed gaps

The position rules are the reference's (`unclebob/uml-viewer`,
`src/uml_viewer/engine/layout.clj`): class-gap 40 inside a rank, a fixed rank
gap, and a 40px margin. We use `GAP_Y = 120`, the compact end of the requested
120-140 range (the reference's `class-rank-gap` is 140).

- `packRows` makes every layer one horizontal row, packed **left to right from
  the same left margin** with `GAP_X = 40`. A card uses only its own width; no
  column is sized by the widest card elsewhere, cards are not centred within
  columns, and a row has no empty slot. Layers are not centred on a common
  axis.
- A BFS rank in the repo model holds 25 wide cards, which made a 10,259px
  single-row ribbon. `wrapRows` therefore continues a rank on the next
  layer when adding the next card would exceed `ROW_MAX_W = 1300`, preserving
  the rank and row order. Every layer is still one left-packed row; a rank may
  occupy several consecutive layers.
- `placeLayers` gives every row its y. A layer is as tall as its tallest card
  and rows are separated by the fixed `GAP_Y = 120`. There is no adaptive
  gutter: the height depends on the cards, never on how far an edge runs
  sideways. Rows never share a y band and a row's cards are laid end to end, so
  no two cards overlap.

`layoutGraph` flattens the cards back into `scene.nodes` in draw order and
writes `x`, `y`, `w`, `h`, `layer` onto each card.

## Public shapes changed

| Shape | Meaning |
| --- | --- |
| `scene.nodes[i].layer` | Integer row index in draw order. `rankTypes` returns the underlying shortest-path rank; a rank wider than `ROW_MAX_W` occupies several consecutive scene layers. All cards in one layer share the same `y`. Previously this was a longest-path layer. |
| `scene.nodes` | Ordered by layer, then by the in-layer barycenter order (declaration ID breaks ties), so drawing and folding see a deterministic order. |
| `scene.edges[i].back` | Present and `true` on DFS cycle-closing edges and on shortest-path cross edges that do not advance a final layer; absent on ordinary edges. Non-back always means strictly deeper. |
| `scene.axis` | Always `"v"`; there is no other value and no option that sets it. |
| `scene.nodes[i].w` / `h` | Variable, derived from text and rows instead of a fixed `CARD_W`/row height. |

Removed outright: `flowAxis`, `opts.axis` and every axis branch, the
`按类型分组` layout mode and its `<select>`, `PER_ROW`/`COLS`, the function and
value bands, the same-layer side routing, `assignX` and its column/centring
model, and the adaptive vertical gutter. Passing `layout` in a view object is
ignored; it cannot bring a second direction back.

## Invariants that guard the contract

| Invariant | Test |
| --- | --- |
| Every non-back edge goes strictly down a layer | `every non-back edge points to a strictly deeper layer` |
| A shortest path lifts a converging node above its longest chain | `shortest-path ranks lift a converging node above its longest chain` |
| Every edge uses bottom/top border centrelines and ends on the target's top centre (where `marker-end` puts the arrow) | `every edge uses bottom-centre and top-centre ports, with the arrow at the target` |
| 40+ cards (including unexported functions and values) never overlap | `forty-plus cards lay out without overlaps and grow the canvas` |
| Each layer is one left-packed row with the fixed gap and no empty slot | `each layer is one left-packed row with no empty column slot` |
| A 40-card scene covers less area than the old centred-column layout (old area hard-coded) | `forty cards fit a smaller canvas than the centred-column layout` |
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
| Layers and cycle break | `assignLayers` / `rankTypes` in `ui.js` | Integer shortest-path (BFS) ranks; DFS cycle edges and non-advancing cross edges marked back | the layer, shortest-path, cycle and add-card tests |
| In-layer order | `orderLayers` in `ui.js` | Four deterministic x-barycenter passes over all direct neighbours; declaration ID tie-break | determinism and unrelated-layer tests |
| Boxes and gutters | `packageCards`, `layoutGraph`, `packRows`, `wrapRows`, `placeLayers` in `ui.js` | Fill `x`, `y`, `w`, `h`, `layer`; left-packed rows, 40px card gap, 1300px row wrap, fixed 120px rank gap; no overlaps | 40-card, row-packing, area, layer-coordinate and determinism tests |
| Card data and size | `buildCard` + `cardWidth` in `ui.js` | Every card has `title`, `sub`, `href`, `drillHref` and a text-derived `w` | card and panel tests |
| Edge geometry | `edgeRoute` / `edgePath` in `ui.js` | Two boxes in, bottom-centre → top-centre out | port, endpoint and outside-approach tests |
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
