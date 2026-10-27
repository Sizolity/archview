# archview roadmap

> 状态:生效
> 更新:2026-10-27
> 范围:archview 独立项目(module `github.com/Sizolity/archview`)
> 本文只列**未实现**的部分;已实现的能力见 [README.md](README.md)。
> 已完成条目的编号保留在文内(标"已完成"),避免改动交叉引用。

## 0. 现状

`archview` 目前是一个只读的浏览器视图:从目标 Go module 读取包、类型、字段、
方法、常量/变量与引用,提供模块图下钻、类型卡片、inspector、搜索、源码行查看、
平移缩放。

| 位置 | 职责 |
| --- | --- |
| `cmd/archview` | 入口:flags、装配、退出码 |
| `internal/model` | `go list` + `go/parser` 抽取模型;`go/types` 类型检查与关系;调用图;工作区合并 |
| `internal/crap` | 调 go-crap 取 CRAP/复杂度/覆盖率,按 `file:line` 对齐到函数与方法 |
| `internal/config` | `archview.json`:exclude/editor/layers/metrics,一次性报全的错误校验 |
| `internal/cache` | 模型缓存(指纹)与 `-watch` 的 SSE 广播 |
| `internal/export` | 单文件页面、uml-viewer IR 的导出与导入 |
| `internal/web` | `/`、`/ui.js`、`/model.json`、`/source`、`/events`、`/healthz`、源码页渲染 |
| `internal/assets` | `ui.html`、`ui.js`、`source.css`(embed) |
| `internal/model/testdata` | fixture 模块:`relmod`、`broken`、`exclude`、`calls`、`work` |

已知边界:

- `uses` 关系是**语法级**的:字段与签名里的类型按名字匹配;
- `implements`/`embeds` 来自 `go/types`(指针接收者的方法也算满足接口),但泛型实例仍未建模;
- UI 把 `implements`/`embeds` 画成绿/蓝实线,调用是青色(接口调用虚线);
- `/model.json` 有内存缓存,但**没有 watch**:改代码后要手动刷新页面(缓存会让刷新变快);
- CRAP 指标在启动时采集一次(go-crap 会跑一遍 `go test`),改代码后刷新页面不会重算;
- 调用图只到模块内,图上聚合到类型级,函数级一跳看 inspector;
- 单文件导出是快照,不会随代码更新;
- `ui.js` 的纯函数有 `node --test`,但页面本身仍没有真实浏览器测试;
- 还没有打 tag 的发布产物;CI 两个平台已绿。

## 1. 建议顺序

| 阶段 | 主题 | 状态 | 先做的理由 |
| --- | --- | --- | --- |
| M1 | 关系可信 | **已完成** 2026-10-27 | 接口实现与嵌入是"设计细节"的核心,当前会漏报 |
| M2 | 可分享 | **已完成** 2026-10-27 | 单文件导出与编辑器跳转决定日常是否愿意用 |
| M3 | 可评估 | **已完成** 2026-10-27 | CRAP 着色把"结构"与"风险"放在同一张图上 |
| M4 | 可兼容/可扩展 | 未开工 | EDN 兼容、缓存、watch、多模块 |
| M5 | 可持续 | 进行中(5.1 部分、5.2 已添加) | UI 测试、CI、发布与文档 |

每个条目都给了验收标准;没有验收标准的条目不要开工。

阶段表只覆盖 10 条。按依赖补上的实际顺序:`2.1` → `5.2`+`5.1` → `4.2`+`4.1` →
`2.3` → `4.3`→`3.4`→`3.2`→`3.1`→`2.2` → `4.5`→`4.4`→`2.4`→`5.3`。
理由:`2.2` 要 `2.1` 的 `types.Info`,`3.1` 要 `2.1` 的 `embeds`/`implements` 边,
`3.4` 要 `4.3` 的 `layers` 配置,`4.1` 与 `4.2` 改同一处取数/链接代码。
已完成:`2.1`、`2.2`、`2.3`、`2.4`、`3.1`、`3.2`、`3.3`、`3.4`、`4.1`、`4.2`、`4.3`、`4.4`、`4.5`、`5.1`、`5.2`;部分:`5.3`(版本、发布配置、设计文档已完成;差打 tag 与截图)。

## 2. 数据与关系

### 2.1 用 `go/types` 做精确关系(P0) — 已完成 2026-10-27

模型现在带 `edges`(排序后的 `{kind, from, to}`,`from`/`to` 是 `<import path>#<TypeName>`)
与 `warnings`;`implements`/`embeds` 由 `go/types` 解析,`uses` 保留语法匹配作为兜底。
实现见 `internal/model/types.go`,验收 fixture 见 `internal/model/testdata/relmod`(值/指针接收者、跨包实现、
嵌入提升)与 `testdata/broken`(类型检查失败的降级)。

实现时的取舍,后续条目要知道:

- 类型 `T` 或 `*T` 任一满足接口就记一条 `implements`(读图的人问的是"算不算实现");
- 空接口不产边,否则每个类型都会挂一条;
- 泛型具名类型与泛型接口不进 `edges`,实例化留给以后;
- `go list -export -deps`(不带 `-deps` 拿不到标准库的导出数据)。

仍然存在的边界:`uses` 是名字匹配,`implements` 不区分指针/值接收者的语义差异,
泛型实例没有边。

### 2.2 调用关系(P1) — 已完成 2026-10-27

模型多了 `calls`:`{from, to, dynamic}`,两端都是函数 ID(`<pkg>#<Func>` /
`<pkg>#<Type>.<Method>`);`FuncInfo`/`MethodInfo` 补了 `id`,函数与方法第一次有了
可被引用的身份。

实现要点:

- 类型检查带上 `types.Info{Uses, Selections}`——没有它分不清函数调用、方法调用和
  接口调用;
- 遍历每个函数体的 `ast.CallExpr`:`Ident` 查 `Uses`,`SelectorExpr` 先查 `Selections`
  (方法)再查 `Uses`(包级函数);函数值、字段、闭包这类调用没有静态目标,直接丢弃;
- 只保留目标包也在本次检查里的调用(模块内);标准库与依赖的调用没有节点,丢掉;
- `dynamic` = 选择子的接收者是接口(`types.IsInterface(sel.Recv())`),画虚线;
- 去重并按 `(from,to)` 排序,JSON 可 diff。

UI:

- 工具栏"调用边"开关(默认关):把调用**聚合到类型级**——A 的方法调用 B 的方法就画
  一条 A→B;同一对类型全静态是实线,只要有一条接口调用就是蓝色虚线;
- 类型 inspector 加"调用"两栏,一跳的"调用了 / 被调用",动态调用带"动态"徽标。

验收核对:`testdata/calls` 里 A→B→C、`Dynamic→I.M`(dynamic)、`T.Do→A` 全部产出,
并断言"非动态调用的目标必须是模型函数";图默认不画调用边,勾选后冒烟测到 `edge call`;
inspector 列表也在冒烟里断言。

**没做**:函数级调用边不单独画——图里只有类型节点,聚合是唯一的画法;看具体的函数级
一跳请用 inspector。

### 2.3 CRAP 与覆盖率着色(P1) — 已完成 2026-10-27

CRAP 不自己算,用 [go-crap](https://github.com/padiazg/go-crap)(`scan -f json`)。
archview 只做取数、映射与着色,**不新增 Go 依赖**:go-crap 是外部命令,和 `go`、
`go test` 一样从 `PATH` 调用,不在 `go.mod` 里。

实现要点(踩过的坑都记在这):

- 调用形如 `go-crap scan -f json --no-progress .`,读 **stdout**,不落盘(不写目标仓库);
  注意它会**执行目标模块的测试**(`go test`)才能量覆盖率,不想要就 `-no-crap`;
  **必须传 `.`**:v0.5.1 传 `./...` 会报 `discover modules: ...\.git` 失败;
- 报告字段:`entries[].file`(相对 module root)、`line`、`function`、`package`、
  `cyclomatic`、`coverage`(**0–100 的百分数,不是 0–1**)、`crap`,
  以及 `summary.{average,combined,exceeded,total_funcs}`;
- 用 `file:line` 对齐 `FuncInfo`/`MethodInfo`(两边都相对同一个 `go list` 根,行号
  都是 `func` 关键字那一行);Windows 反斜杠与 `./` 前缀统一成斜杠相对路径;
- 指标在**启动时采集一次**(`metricsBundle`),按请求合并进模型:否则每次
  `/model.json` 都要跑一遍 `go test`。代价是刷新页面不会重算指标,4.4 一起解决;
- `-no-crap` 关闭采集;`-crap-cmd` 换命令(默认 `go-crap`);
- 命令缺失、超时或输出不是 JSON 时降级:写一条 `warnings`,页面不显示分数与图例;
- UI:类型卡片右上角 CRAP 徽标(取该方法集里的最大值)、函数表/方法表加 CRAP 列、
  首页显示 `平均 / 超标 N/M`、header 有可隐藏图例。三档:≤5 绿、≤30 黄、>30 红。

验收口径核对:

- 固定测试:`metrics_test.go` 用一份真实 go-crap JSON 断言逐条解析与对齐,含
  Windows 路径归一化、未匹配声明、坏 JSON、命令缺失降级;
- `-no-crap`:导出 61,025 B 且不含 `"metrics"`;开启时 64,369 B 且含汇总;
- go-crap 不在 `PATH`:服务仍启动,`/model.json` 200,`warnings` 有记录(单测覆盖);
- 不写目标仓库:报告走 stdout,`git status` 干净。

### 2.4 EDN 兼容(P2) — 已完成 2026-10-27

导出 `-edn out.edn` 与导入 `-import-edn view.edn` 都做了。schema 来自
[uml-viewer](https://github.com/unclebob/uml-viewer) 的 README(`## IR` 一节),不是猜的。

导出(`internal/export/edn.go`):

- `:title` / `:direction :tb` / `:packages` / `:classes` / `:fields` / `:ops` / `:edges`;
- 关系映射:`uses` → `:association`、`implements` → `:implements`、
  `embeds` → `:aggregation`;调用图在该 IR 里没有对应 kind,不导出;
- id 是稳定 slug(`internal/wiki` 的 `Page` → `:wiki-page`),重名自动加 `-2`;
- 有指标时给类写 `:crap`(方法均值)与 `:coverage`(0–1,uml-viewer 的口径);
- 签名拆成 `:args` / `:returns`(`splitSig`,嵌套括号不误切)。

导入(`internal/export/ednimport.go`):

- 自写最小 EDN reader:map/vector/list/keyword/string/number/bool/nil、`;` 注释、
  逗号当空白;坏输入报"第 N 行";
- IR → 模型:包 → `Package`,`:interface` → interface(其余 struct),`:fields`/`:ops` →
  字段与方法,`Sig` 由 args/returns 拼回;
- 边反向映射回 `uses`/`implements`/`embeds`;未知 kind 与悬空端点计入 `ignored` 并打日志;
- 导入的模型没有源码目录,`/source` 直接 404;`-watch` 对导入无效(启动时提示)。

验收核对:

- [x] 导出的结构符合官方 schema(单测断言 id、边类型、转义、指标);
- [x] 导入后再导出**结构等价**:类型名、方法签名、各 kind 的边数一致,且第二次往返
      逐字节不动点;
- [x] 未知 key 与坏的边被计数,不 panic;
- [ ] 用真 uml-viewer 打开 —— 本机没有 Clojure/JVM,按约定不做。

## 3. 渲染与交互

### 3.1 分层/分组布局(P1) — 已完成 2026-10-27

类型卡片有两种布局,工具栏下拉框切换,都是确定性纯函数(`layoutTypes` + `rankTypes`):

- **依赖分层(默认)**:按类型间引用边(`uses`)算秩
  `rank(to) = max(rank(to), rank(from)+1)`,秩小在上、依赖在下;同秩按名字排序,
  每行 4 张卡,超出的换行;环用轮数上限兜住,不会死循环;
- **按类型分组**:struct → interface → type 三段,段内按名字,回到三列网格。

关系边在布局之前算好(`typePairs`),布局拿它算秩,画图用同一份数据,所以"看到的分层"
和"画出来的边"不会打架。

验收核对:10 类型 / 15 边在两个模式下逐对不相交、两次调用 `deepStrictEqual` 完全一致
(单测);两种布局都在 `node --test` 里覆盖;DOM 冒烟里 A→B→C 的 y 坐标确实递增。

**没做**:跨包引用边仍不画——类型图只显示同包内的节点,`typePairs` 保留跨包目标,
但画之前会按可见节点过滤。边仍是三次贝塞尔,分层后交叉变少,但没有做正交路由。

### 3.2 聚焦、折叠、Declutter(P1) — 已完成 2026-10-27

交互落在工具栏与卡片上(点击卡片仍是"下钻",不改既有手势):

- **聚焦**:选中某个类型后按"聚焦",只留该类型与它的一跳邻居;导航到非类型视图时
  自动退出,Esc 退出,按钮在非类型视图禁用;
- **折叠**:每张类型卡右下角有 ▾/▸,折叠后只剩标题,高度从按行数算降到 44;
- **精简(declutter)**:勾选后所有卡片折叠,并且只保留与当前选中类型相关的边。

实现:状态都在页面 `state`(`focus`/`collapsed`/`declutter`),过滤是 `ui.js` 的纯函数
(`focusScene`/`declutterScene`/`toggleCollapsed`),`buildScene` 多接一个 `view` 参数;
模型完全不动,只有视图变化。

验收核对:聚焦后节点数 = 1 + 邻居数(4 类型链 A→B→C 上聚焦 B 得 3);
折叠只改 `rows`/`h` 与 `folded`,不碰模型;Esc 恢复整图。

**一项没做**:"隐藏跨包边"。类型图只画同包内的引用边,跨包边根本不存在,所以没有可
隐藏的对象;3.1 之后仍然如此(跨包目标在画之前就被过滤)。这条要么随"跨包引用"新条目
一起实现,要么从 3.2 划掉。其余两项按验收完成。

### 3.3 搜索增强(P2) — 已完成 2026-10-27

搜索索引从"类型名 + 函数名"扩到:类型名/文档/字段名与类型、方法名/签名/文档、
常量、变量。纯函数 `searchHits`,**名字命中排在文档命中之前**,同组按名字排序。

- 命中高亮:`highlight` 先按原文定位、再分段转义并包 `<mark>`,用户输入不会被当成 HTML;
- 键盘:`↑`/`↓` 移动选中行、`Enter` 打开、`Esc` 清空;`/` 仍然聚焦搜索框;
- 图上淡出:查询非空时 `buildScene` 给未命中节点打 `dim`,CSS 降到 0.2 透明度;
  类型视图按类型 id 判断,包视图按"这个包里有没有命中的类型"判断;
- 跳转(hashchange)或清空后恢复原图,`dim` 归零。

验收核对:搜索能命中方法名(如 `DB.Get`)与函数名,名字命中优先于文档命中;
"输入 → ↑↓ → Enter → 跳转"在 DOM 冒烟里跑通(Enter 后 hash 指向第二个命中);
清空后 `dim` 归零。

### 3.4 依赖规则标注(P2) — 已完成 2026-10-27

`archview.json` 的 `layers` 就是这里的配置。语义:

- 每层是名字列表,按**整段**匹配 module 相对路径(`web` 命中 `internal/web`,
  不命中 `webhook`;根包不属于任何层);
- 层按配置顺序**从内到外**,下标小的是内层;
- A 依赖 B 且 A 的层下标 < B 的层下标时,这条 import 边标红(也就是"内层依赖外层")。

实现:判定在 `ui.js` 的 `layerOf`/`layerViolation`,`buildScene` 把 `violation` 只挂在
真正违规的边上(正常边不多带字段);`draw` 给红色实线、并挂一个 `<title>` 做 hover
提示(`to → from`,即"谁依赖谁");画布提示追加"红线 = 内层依赖外层";包 inspector
里多一节"违规依赖"。没配 `layers` 时不标色、提示也不出现。

验收核对:在 `[["cmd"],["web"],["wiki","query"],["platform"]]` 下 `web -> platform`
标红(单测 + DOM 冒烟);未配置不标色;红色含义有图例文字。

## 4. 工具形态

### 4.1 单文件静态导出(P0) — 已完成 2026-10-27

`archview -out view.html` 把模型 JSON 与 `ui.js` 一起内联,双击即可离线浏览。
入口在 `export.go`:`exportHTML` 用 `<!-- archview:data -->` 占位注入数据,把
`<script src="/ui.js">` 换成内联脚本,并对 JSON 里的 `</` 做 `<\/` 转义(否则
文档里的 `</script>` 会提前结束脚本块)。页面侧 `fetchModel` 先看
`window.__ARCHVIEW_DATA__`,命中就不发请求,并隐藏"重载"。

验收口径核对:

- 断网可用:导出页在无 `fetch` 的 DOM 环境里完成首页、下钻、类型页、编辑器链接
  全部渲染(11 项断言);
- 无外部请求:导出页没有 `src="/ui.js"`,`fetch` 一次都没被调用;
- 体积:本仓库(1 包 9 类型)52 KB,与模型规模线性。

未做:`-embed-source`(把源码内容嵌入页面),需要时再加。

### 4.2 编辑器跳转(P0) — 已完成 2026-10-27

`-editor source|vscode|goland|file|none`(默认 `source`,保持原行为)。URL 生成在
`ui.js` 的 `editorHref`:`vscode://file/<abs>:<line>:1`、`goland://open?file=<abs>&line=<n>`、
`file:///<abs>`;未知值回退 `/source`,`none` 直接不渲染链接。

- 相对路径按 `go list -m` 给出的 module root 转绝对(`toAbs`),Windows 反斜杠
  统一成 `/`,盘符前补 `/`(`E:/x` → `/E:/x`);
- `-out` 导出时 `source` 降级为 `file`(静态页后面没有 `/source`);
- 单测覆盖三种模式、Unix/Windows 两种 root、未知值回退与 `none`。

### 4.3 配置文件(P1) — 已完成 2026-10-27

`archview.json`(标准库 JSON,不引 YAML),查找顺序:显式 `-config` >
当前目录 `archview.json` > 默认。字段与语义:

| 键 | 语义 |
| --- | --- |
| `exclude` | module 相对路径前缀,**按整段匹配**:`internal/gen` 命中 `internal/gen` 与 `internal/gen/x`,不命中 `internal/genx`;根包永不被排除 |
| `editor` | `-editor` 的默认值 |
| `layers` | 分层(内层在前),进模型后留给 3.4 做依赖规则标注 |
| `metrics.enabled` | 是否跑 go-crap;显式 `-no-crap` 仍然优先 |
| `metrics.thresholds` | `[warn, bad]` 两档,默认 `[5, 30]`,随模型送到前端替掉硬编码常量 |

实现要点:

- 命令行覆盖文件:用 `flag.Visit` 区分"没传"与"传了默认值",`configEditor`
  按 `flag > config > 默认` 决策;
- 错误一次性报全:`Validate` 收集所有字段问题后一起返回,进程以 **exit 2** 退出,
  不做半可用;
- 未知键直接拒绝(`DisallowUnknownFields`),避免拼错的键被静默忽略;
- 空配置文件视为默认;`-config` 指向不存在的文件是错误。

验收核对:无配置/部分配置/非法配置三种情况在 `config_test.go` 里都有用例;
`-editor vscode` 覆盖了配置里的 `goland`;README 有字段表;`exclude` 有专门
fixture(`testdata/exclude`)。

### 4.4 缓存、watch 与刷新(P2) — 已完成 2026-10-27

- **缓存**:`cache.go` 的 `modelCache` 每次请求对模块做指纹(每个 `.go` 与
  `go.mod`/`go.sum`/`go.work` 的路径 + mtime + size),没变就返回上次的模型,变了才重扫。
  错误不进缓存;出缓存前已带上 `-editor` 与指标快照,处理器不再改模型(并发读安全)。
  实测:缓存命中 1–3ms,冷重扫在沙箱约 4.5s。
- **watch**:`-watch` 起一个 500ms 轮询(`watch.go`),指纹变化就向订阅者广播;
  不用 fsnotify,保持纯标准库。
- **SSE**:`GET /events` 推 `event: reload`;页面用 `EventSource` 收到就重新取模型。
  静态导出页不连;`-watch` 没开时端点 404,前端保留"重载"按钮。

验收:

- [x] 改一个 `.go` 文件后 watch 通知 500ms 内到达(整页更新还要加一次重扫,取决于模块大小;
      实测 `touch main.go` 后 curl 收到 `event: reload`);
- [x] 缓存命中时不再调用 `go list`(单测计数验证);
- [x] 没有 SSE 时退回手动"重载"(EventSource 自行重连,按钮一直都在)。

### 4.5 多模块 / go.work(P2) — 已完成 2026-10-27

- `-dir <path>` 指定扫描目录(默认当前目录);
- 目录里有 `go.work` 时进入工作区模式:`go work edit -json` 列模块,逐个
  `loadModule` 后合并——包、边、调用、warnings 全拼起来;包路径本来就是 import path,
  所以不会冲突;
- 源码位置加上模块相对目录(`moda/a.go:8`),`/source` 因此能落到正确文件;
- 跨模块调用保留:加载时先用宽松 scope 收集,拿到所有包路径的并集后再过滤;
- CRAP 在 `-dir` 上采集;go-crap 在工作区根给出的 key 正好也是相对根的路径,照常对齐
  (实测两个函数都有分数);
- 缓存指纹也覆盖 `go.work`,任何模块的文件变了都会失效。

实现见 `workspace.go`。验收核对:工作区 fixture(`testdata/work`,两个模块 + 一次跨模块
调用)能看到两个包、位置带模块前缀、`modb#FB → moda#FA` 存在;单模块行为不变
(其余 fixture 全部照旧)。

**没做**:模块重名时的 id 前缀(现实中极少见,遇到会先报错而不是静默覆盖)。

## 5. 质量与发布

### 5.1 测试(P0) — 已完成 2026-10-27

已完成:

- 纯函数全部搬到 `ui.js`:路由、布局(`layoutTypes`/`rankTypes`)、场景构建
  (`indexModel`/`buildScene`)、搜索(`searchHits`/`highlight`)、分层规则、调用聚合、
  几何与格式化;`ui.html` 只剩 DOM 与事件;
- `ui.test.js` 57 个用例,`node --test` 进 CI,不需要 npm install;
- `go test` 侧补了 `/source` 超大文件(413)与非 UTF-8 源码(转义)两条;
- `testdata` 有 `relmod`(接口/嵌入)、`broken`(降级)、`exclude`(配置过滤)、
  `calls`(调用图)四份 fixture;指标用真实 go-crap JSON 做固定测试(`metrics_test.go`)。

覆盖:`node --test --experimental-test-coverage` 下 `ui.js` 行 100.00%、
分支 84.92%、函数 98.20%。

未做:Playwright 截图回归(按原计划不做)。

验收状态:

- [x] `node --test` 与 `go test` 都在 CI 里;
- [x] 新增模型能力有 fixture + 断言;
- [x] UI 纯函数覆盖率 > 70%。

### 5.2 CI(P0) — 已完成 2026-10-27

`.github/workflows/ci.yml`:`ubuntu-latest` + `windows-latest` 矩阵,跑 `gofmt -l .`
(有输出即失败)、`go vet ./...`、`node --test`、`go test ./...`、`go build ./...`。

首次推送后实测(仓库 <https://github.com/Sizolity/archview>):

- 两个平台全绿(run 40532364571);
- 故意提一个未格式化的 `probe.go` 开 PR:`gofmt` 步骤 15 秒内失败并点名文件
  (PR #1,已关闭、分支已删);
- **首轮 CI 就抓到真 bug**:`filepath.ToSlash` 在 Linux 不会转反斜杠,go-crap 的
  Windows 路径对不上模型,两个测试在 ubuntu 上挂;已改成与平台无关的显式替换
  (`metrics.go`)。

暂缓:`golangci-lint`(引入工具链依赖)、Go module 缓存(模块无依赖,没有 `go.sum`
可做 key)。

**验收状态**:

- [x] 两个平台都绿;
- [x] 故意留一个格式错误能被 CI 拦住;
- [x] 无源码探测式跳过。

### 5.3 发布与文档(P2) — 部分完成 2026-10-27

已做:

- `-version`:`var version = "dev"`,用 `-ldflags "-X main.version=1.2.3"` 构建后
  打印 `archview 1.2.3`(实测);模块路径与 `go install` 约定一致;
- `.goreleaser.yaml`:linux/darwin/windows × amd64/arm64 静态二进制,版本经 ldflags
  注入,Windows 出 zip;
- `docs/design.md`:数据流、模型 schema、端点、性能取舍;
- README 补齐 `-out`/`-editor`/`-dir`/`-config` 与配置字段表。

未做:

- 真的打 tag 跑 goreleaser(需要远端仓库与标签,本地无法验证);
- README 截图(要真实浏览器截图,属于 §7 的同一笔欠账)。

验收状态:

- [ ] 打 tag 后 release 页面有三平台产物 —— 需要你打 tag;
- [x] 新机器按 README 两步跑起来;
- [x] `go install` 的二进制与 `go build` 行为一致(同一条 main,只有版本号不同)。

## 6. 明确不做

- **不改代码**:不做重命名、重构、写文件、PR 审查;
- **不做全程序调用图**:只做模块内、一跳展示;跨模块动态调用只标记不解析;
- **不引前端框架/构建链**:保持单文件 HTML + 原生 JS;
- **不做多语言/多用户/鉴权/远程托管**;
- **不追求像素级 UML**:目标是"看懂结构",不是生成规范 UML 文档。

## 7. 完成定义(每个条目)

- [ ] `gofmt -l .` 无输出,`go vet ./...` 通过,`go test ./...` 通过;
- [ ] 新行为有对应的 Go 测试或 JS 纯函数测试;
- [ ] README / 本文件同步(状态从"未做"改为"已做",并写清用法);
- [ ] 新增依赖必须在 PR/提交说明里给出理由;优先标准库;
- [ ] UI 改动在真实浏览器里看过(截图或录屏),不能只跑 jsdom;
- [ ] 不写目标仓库的任何文件,不联网上传代码。

## 8. 依赖决策待办

当前 `go.mod` 无第三方依赖。以下条目可能引入依赖:

| 需求 | 可选方案 | 结论 |
| --- | --- | --- |
| 类型检查 | `go/importer` + `go list -export` | **已定**:标准库,2.1 已用 |
| 类型检查(备选) | `golang.org/x/tools/go/packages` | 不需要,标准库方案够用 |
| CRAP 与覆盖率 | 自算 / [go-crap](https://github.com/padiazg/go-crap) 的 `scan -f json` | **已定**:用 go-crap,外部命令,不进 `go.mod` |
| EDN 解析 | 自写最小 reader | 自写,避免依赖 |
| UI 测试 | `node --test` / Playwright | 纯函数用 node,浏览器测试可选 |
| 发布 | goreleaser | 接受一个构建期工具 |

判定"外部命令"的边界:只依赖已装工具(PATH 里有),命令缺失时降级为"无数据"并写
`warnings`,绝不因为可选工具缺失而让扫描失败。


