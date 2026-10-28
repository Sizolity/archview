# Browser UI: axis, components, and the invariants that hold them

This note records the contract that the canvas UI now enforces. It exists
because a real bug slipped through: in a layered package view, two function
cards sat in one horizontal row while the edge between them was routed as a
vertical flow, so the path left one card's bottom and entered the other card's
top from inside the card. The line ran diagonally across the row and the
arrowhead was hidden at the corner.

The fix is a single routing contract, not adjusted coordinates. The tests that
guard it live in `internal/assets/ui.test.js` and are named in each section
below.

## The axis contract

`buildScene` (in `ui.js`) makes exactly one flow decision per scene, through
`flowAxis(kind, layout)`:

| Scene | `flowAxis` | Layer coordinate | Flow ports | Cross ports |
| --- | --- | --- | --- | --- |
| module view (`packages`) | `"h"` | `x` (import depth) | right → left | top/bottom |
| package view, `layer` layout | `"v"` | `y` (dependency band) | bottom → top | left/right |
| package view, `kind` layout | `"h"` | `x` (kind column) | right → left | top/bottom |

The decision is passed *into* the layout functions and is carried on the scene
as `scene.axis`; the page never re-derives it. Two consequences follow:

- **Layout owns positions and layers.** Every node gets an integer `layer`.
  In a vertical scene every node in a layer shares `y`; in a horizontal scene
  every node in a layer shares `x`. `layoutPackages` uses import depth as its
  layer; `layoutTypes` uses row bands in a vertical flow and kind columns in a
  horizontal one. Function and value bands continue the same numbering, so a
  function row is one layer and a horizontal call between two functions is a
  same-layer edge.
- **Routing owns ports.** `edgeRoute(a, b, axis)` treats `flow` as the
  coordinate the axis advances on (`y` for `"v"`, `x` for `"h"`) and `cross` as
  the coordinate inside a layer (`x` for `"v"`, `y` for `"h"`). It has one
  algorithm with two cases:
  - *different layers*: leave/enter on the flow ports at the border centreline
    (bottom → top for `"v"`, right → left for `"h"`, reversed when the target is
    behind the source);
  - *same layer*: leave/enter on the facing cross ports at that border's
    centreline (side → side for `"v"`, top/bottom for `"h"`). This is the case
    that used to produce the diagonal: a vertical router must not use bottom →
    top when both cards share a row.

Both control points sit outside their boxes, so the final tangent approaches
the target border from outside. `marker-end` is unchanged; the arrowhead is
visible because the route no longer approaches from inside the card.

`edgePath(a, b, axis)` remains the string API and now simply returns
`edgeRoute(...).d`. `edgeRoute` is exported so tests can assert ports, controls
and `intra` directly instead of reverse-engineering path text.

### Invariants that guard the axis

| Invariant | Test |
| --- | --- |
| The scene has one explicit axis and the layouts follow it | `buildScene gives each view a single flow axis` |
| Every node in a layer shares its flow coordinate (`node.layer`) | `layout obeys the scene axis: every node in a layer shares its flow coordinate` |
| Vertical edges go downward across layers or sideways within one layer | `vertical flow edges run downward between layers, or sideways within one` |
| Horizontal edges go sideways across layers or downward within one layer | `horizontal flow edges run sideways between layers, or downward within one` |
| The reported bug cannot return: two function cards in one row use side ports | `regression: a call edge between two function cards in one row is side-routed` |

## Component boundaries

| Concern | Owner | Contract | Guard |
| --- | --- | --- | --- |
| Axis choice | `flowAxis` / `buildScene` in `ui.js` | One `"v"` or `"h"` per scene; layouts and router receive it | `buildScene gives each view a single flow axis`; the layer test |
| Positions and layers | `layoutPackages`, `layoutTypes` in `ui.js` | Fill `x`, `y`, `w`, `h`, `layer`; never route | `layoutTypes never overlaps...`, the layer test |
| Edge geometry | `edgeRoute`/`edgePath` in `ui.js` | Two boxes + axis in, border-centre ports and controls out | endpoint, vertical, horizontal and regression tests |
| Card data | `buildCard` + `typeCard`/`funcCard`/`valueCard`/`packageCard` in `ui.js` | Every card has `title`, `sub`, `href`, and `drillHref` when its kind/position supports one | `every card has title, sub, href and the drill target its kind supports` |
| Card visuals | `CARD_STYLE`, `drawNode` in `ui.html` | Map a card node to SVG; attach no listeners | `node`, `box`, `title`, `kind`, `row`, `crap-badge`, `entry-badge` classes used by smokes |
| Declaration → card | `cardIndex(scene)` in `ui.html` | One map, rebuilt on every draw; used by edges, click and double-click | smoke click/double-click checks |
| SVG creation | `svgEl(name, attrs, text)` in `ui.html` | The only caller of `document.createElementNS` | page source (one occurrence) |
| Inspector HTML | `panelFrame`, `panelFor`, `panelHome`/`panelPkg`/`panelType`/`panelFunc`/`panelValue` in `ui.js` | Pure HTML strings; the frame owns badges/meta/lead/doc/body, each panel function owns one kind's sections | `panelFrame is one frame...`, `every declaration panel renders the doc paragraph` |
| Inspector DOM | `renderSide` in `ui.html` | One assignment to `side.innerHTML` from `A.panelFor`; search is the only other side-panel renderer | smoke inspector checks |
| Input | `onCanvasClick`, `onCanvasDblClick`, pan handlers in `ui.html` | All canvas input is delegated through `#svg`; cards/groups own no handlers | smoke click, double-click, fold, pan checks |

Other one-owner reductions:

- `edgeRoute` is the only port/tangent decision; `drawEdges` only converts it
  to a `<path>`.
- `panelFor` is the only route-to-panel map; `routePkg` is the only route-to-
  package map (the crumb and the panel share it).
- `cardIndex` is the only declaration-to-card map, so the renderer, the click
  handler and the double-click handler cannot disagree about which card a
  declaration is.
- `svgEl` is the only element factory, so namespaces and attributes cannot
  drift between markers, edges and cards.
- The six duplicated `renderX` page functions and their per-node listeners are
  gone. Panels are pure functions covered by `node --test`.

## What the bug class now requires to reappear

To reintroduce the original failure, a change would have to make the layer
test, the vertical/horizontal flow tests, the endpoint/centreline test, *and*
the outside-approach test fail together. A single-coordinate patch cannot pass
them, because the tests assert the relationship between `buildScene`'s axis,
the card boxes, and the router's ports rather than a snapshot path string.

The page-side invariants are structural: cards are built before they are drawn,
`drawNode` cannot register a click listener, and both click paths resolve the
card through the same `cardIndex`. Panels are pure strings, so a missing doc
paragraph is a test failure rather than a browser-only regression.

## Verification commands

```powershell
node --test --test-isolation=none
node .tmp/smoke.js
node .tmp/smoke-layers.js
node .tmp/smoke-focus.js
go test ./...
```

The page and `/ui.js` are served separately by the HTTP server and inlined into
the single-file export by `internal/export`; both paths still consume the same
`ui.html` placeholder and `ui.js`, so the component split introduces no new
asset route.
