# archview roadmap

> 状态:生效
> 更新:2026-10-26
> 范围:archview 独立项目(module `github.com/huic/archview`)
> 本文只列**未实现**的部分;已实现的能力见 [README.md](README.md)。

## 0. 现状

`archview` 目前是一个只读的浏览器视图:从目标 Go module 读取包、类型、字段、
方法、常量/变量与引用,提供模块图下钻、类型卡片、inspector、搜索、源码行查看、
平移缩放。

| 文件 | 职责 |
| --- | --- |
| `model.go` | `go list` + `go/parser` 抽取模型;`Load`、`parsePackage`、`resolve`/`resolveSig` |
| `server.go` | `/`、`/model.json`、`/source`、`/healthz` |
| `ui.html` | 全部前端:场景构建、布局、SVG 绘制、路由、inspector |
| `main.go` | `-addr`、优雅关闭 |
| `*_test.go` | 模型解析与 HTTP 路由测试 |

已知边界:

- 关系是**语法级**的:字段类型按名字匹配,不解析接口实现、嵌入提升、别名与泛型实例;
- 没有调用关系、没有指标着色、没有静态导出;
- UI 只有 jsdom 冒烟(不在仓库内),没有真实浏览器测试;
- 没有 CI、没有发布产物、没有配置文件。

## 1. 建议顺序

| 阶段 | 主题 | 先做的理由 |
| --- | --- | --- |
| M1 | 关系可信 | 接口实现与嵌入是"设计细节"的核心,当前会漏报 |
| M2 | 可分享 | 单文件导出与编辑器跳转决定日常是否愿意用 |
| M3 | 可评估 | CRAP 着色把"结构"与"风险"放在同一张图上 |
| M4 | 可兼容/可扩展 | EDN 兼容、缓存、watch、多模块 |
| M5 | 可持续 | UI 测试、CI、发布与文档 |

每个条目都给了验收标准;没有验收标准的条目不要开工。

## 2. 数据与关系

### 2.1 用 `go/types` 做精确关系(P0)

**现状**:`resolve`/`resolveSig` 遍历 AST 的 `Ident`/`SelectorExpr`,按
`pkg.Type` 名字匹配。接口实现、嵌入字段提升的方法集、类型别名、泛型实例都不会
产生边。

**目标**:模型增加显式的 `edges`,至少包含:

- `uses`:字段/参数/返回值引用(现有能力,保留);
- `implements`:具名类型实现接口;
- `embeds`:结构体嵌入(字段级,可与 `uses` 并存)。

**实现要点**:

1. `go list -export -f '{{.ImportPath}}|{{.Export}}' ./...` 拿到每个包的导出数据;
2. `importer.ForCompiler(fset, "gc", lookup)` + `go/types.Config{Importer: ...}` 对每个包做类型检查(纯标准库);
3. 收集 `*types.Named` 与 `*types.Interface`,用 `types.Implements` 建 `implements` 边;
4. 结构体的 `types.Struct` 嵌入字段直接产出 `embeds`;
5. 保留现有语法关系作为兜底,类型检查失败时降级并记录警告。

**验收**:

- `testdata` 里放一个模块:`type I interface{ M() }`、`type S struct{}`、`func (S) M()`、`type E struct{ S }`;
- 模型里出现 `S implements I` 与 `E embeds S`;
- 现有 `model_test.go` 不回归;
- 类型检查失败(如缺少依赖)时服务仍能启动,`/model.json` 带 `warnings`。

**风险**:类型检查耗时(百包级可接受);导出数据路径在 Windows/Linux 不同,必须走 `go list` 而不是拼路径。

### 2.2 调用关系(P1)

**现状**:没有。

**目标**:函数/方法级的 `calls` 边,支持 inspector 里"调用/被调用"列表,图上可开关。

**实现要点**:

- 在 2.1 的类型信息上遍历 `ast.CallExpr`,用 `types.Info.Uses`/`Selections` 解析到
  `*types.Func`,按位置映射到 `FuncInfo`/`MethodInfo`;
- 只保留模块内的边;接口调用标记 `dynamic`,不画实线;
- 深度限制(例如 inspector 只显示一跳),避免大图噪声。

**验收**:

- `testdata` 中 `A` 调用 `B`,`B` 调用 `C`;inspector 显示一跳调用与被调用;
- 动态调用(接口)有 `dynamic` 标记;
- 图默认不画调用边,打开开关后才显示。

### 2.3 CRAP 与覆盖率着色(P1)

**现状**:无。

**目标**:按函数计算 `CRAP = CC² × (1 − coverage)³ + CC`,给类型卡片和函数表着色;
阈值可配;可选只算复杂度。

**实现要点**:

- CC 用 AST 统计:`if`、`for`、`range`、每个 `case`、`select` 分支、`&&`/`||`;
- 覆盖率:默认执行 `go test ./... -coverprofile=...`,再解析 `go tool cover -func`;
  支持 `-coverage-command` 覆盖,`-no-coverage` 跳过;
- 按 `file:line` + 函数名对齐;对不上覆盖率的函数按 0% 处理并标注;
- 模型增加 `metrics` 字段或独立 `/metrics.json`;UI 用绿/黄/红三档,图例可关。

**验收**:

- 在 `testdata` 上跑出非零 CRAP,且与手算一致(固定测试);
- `-no-coverage` 只显示 CC;
- 阈值改配置后颜色随之变化;
- 覆盖率命令失败时服务仍可用,卡片标"无数据"。

### 2.4 EDN 兼容(P2)

**现状**:自有 JSON 模型。

**目标**:

- `-edn out.edn` 输出 [uml-viewer](https://github.com/unclebob/uml-viewer) 能读的
  topology IR(`:packages`/`:classes`/`:fields`/`:ops`/`:edges`);
- `-import-edn` 读入同类 IR 渲染;
- 可选叠加 `.metrics/crap.edn`(crapper 产物)。

**实现要点**:

- 先做导出(结构简单),再做导入;
- 导入需要最小 EDN reader(支持 map/vector/keyword/string/number/bool/nil),不要引第三方;
- 字段映射:`TypeInfo` → `:classes`,`FieldInfo` → `:fields`,`MethodInfo` → `:ops`;
  id 用稳定 slug(`wiki.Page` → `:wiki-page`)。

**验收**:

- 导出的 EDN 能被 uml-viewer 打开(手动,需 Clojure/JVM);
- 导入后再导出,结构等价;
- 未知 key 忽略并计数,不 panic。

## 3. 渲染与交互

### 3.1 分层/分组布局(P1)

**现状**:包图按 import 深度分列;类型图固定三列网格,边可能交叉。

**目标**:

- 类型图按 `implements`/`embeds`/`uses` 做分层(依赖在下,使用方在上);
- 支持按 `kind`(struct/interface/type)分组;
- 同层节点按名字排序,边走正交/贝塞尔路由,减少交叉。

**验收**:

- 10 个类型、15 条边的 fixture 下无重叠节点;
- 切换分组后布局稳定(同输入同输出);
- 布局是纯函数,可用 Node 单测(见 5.1)。

### 3.2 聚焦、折叠、Declutter(P1)

**现状**:只有全局视图与选中高亮。

**目标**:

- 点击节点"聚焦":只显示一跳邻居,面包屑返回;
- 折叠类型的成员(卡片只留标题);
- Declutter:隐藏跨包边、隐藏未选中边、隐藏成员。

**验收**:

- 聚焦后节点数 = 1 + 邻居数;
- 折叠/展开不改变模型,只改视图;
- Esc/返回恢复上一状态。

### 3.3 搜索增强(P2)

**现状**:按类型名/函数名/字段名过滤 inspector。

**目标**:匹配方法名与注释;命中高亮;键盘上下选择;`Enter` 跳转;搜索时图上淡出未命中节点。

**验收**:

- 搜索 "Resolve" 能命中 `wiki.ResolveLink`;
- 键盘可完成"输入 → 选择 → 跳转";
- 清空后恢复原图。

### 3.4 依赖规则标注(P2)

**现状**:箭头不表达层级违规。

**目标**:配置 `layers`(内层在前),从内层指向外层的边标红;hover 显示 `from -> to`。

**验收**:

- 配置 `layers: [["cmd"],["web"],["wiki","query"],["platform"]]` 后,`web -> platform` 标红;
- 未配置时不标色;
- 图例说明红色含义。

## 4. 工具形态

### 4.1 单文件静态导出(P0)

**现状**:必须起进程,页面从 `/model.json` 取数据。

**目标**:`-out view.html` 生成一个自包含 HTML,双击即可浏览;不依赖服务器与网络。

**实现要点**:

- UI 改为 `window.__ARCHVIEW_DATA__ || fetch("/model.json")`;
- 导出时把模型 JSON 注入页面;
- `-embed-source` 可选:把源文件内容一并嵌入,inspector 内打开;默认关闭,
  源码链接退化为 `file://` 或隐藏;
- 静态模式下隐藏"重载"按钮。

**验收**:

- `archview -out view.html` 后,断网打开 `file://`,模块下钻、搜索、切换导出均可用;
- 页面无外部请求(DevTools Network 为空);
- 文件大小与模型规模线性,10 包级 < 2 MB。

### 4.2 编辑器跳转(P0)

**现状**:"查看源码"只开内置 `/source` 页。

**目标**:`-editor vscode|goland|file|none` 决定跳转方式;默认 `file` 或 `none`。

**实现要点**:

- `vscode://file/<abs>:<line>:<col>`;
- `goland://open?file=<abs>&line=<line>`;
- Windows 路径统一正斜杠;`file://` 用 `url.URL` 生成;
- 配置可覆盖。

**验收**:

- 三种模式生成的 URL 正确(单元测试);
- 未知编辑器回退到 `/source`;
- 相对路径基于 module root 转绝对路径。

### 4.3 配置文件(P1)

**现状**:只有 `-addr`。

**目标**:`archview.json`(标准库 JSON,引 YAML 需决策),支持:

```json
{
  "exclude": ["internal/gen"],
  "editor": "vscode",
  "layers": [["cmd"], ["web"], ["wiki", "query"], ["platform"]],
  "metrics": {"enabled": true, "thresholds": [5, 30]}
}
```

- 查找顺序:显式 `-config` > 当前目录 `archview.json` > 默认;
- 命令行参数覆盖配置;
- 配置错误一次性报全,不半可用。

**验收**:

- 无配置、部分配置、非法配置三种情况都有测试;
- 命令行覆盖文件;
- README 有字段表。

### 4.4 缓存、watch 与刷新(P2)

**现状**:每次 `/model.json` 都重跑 `go list` + 解析。

**目标**:

- 内存缓存:按包目录 mtime + go.mod/go.sum 判断失效;
- `-watch`:文件变化后通过 SSE 通知页面自动重载(不用 WebSocket,标准库足够);
- 大仓库下 `/model.json` P95 < 300ms(缓存命中)。

**验收**:

- 改一个 `.go` 文件后 watch 模式 2 秒内更新;
- 缓存命中时不再调用 `go list`;
- SSE 断开后前端退回手动"重载"。

### 4.5 多模块 / go.work(P2)

**现状**:只扫描当前 module(`go list ./...`)。

**目标**:`-dir` 指定目标目录;识别 `go.work` 并逐模块扫描,模型里标注 module。

**验收**:

- 在 `go.work` 根目录运行时能看到各模块的包;
- 包路径不冲突(用 module + import path 作为 id 前缀);
- 单模块行为不变。

## 5. 质量与发布

### 5.1 测试(P0)

**现状**:Go 侧有 model/server 测试;UI 只有仓库外的 jsdom 冒烟。

**目标**:

- 把布局/格式化等纯函数从 `ui.html` 拆到 `ui.js`(或 `static/`),用 `node --test` 覆盖;
- 增加 `testdata` 模块:接口实现、嵌入、调用、指标各一份 fixture;
- HTTP 测试覆盖 `/source` 越界、超大文件、编码异常;
- 可选:Playwright 截图回归(先不做,等 CI 稳定)。

**验收**:

- `go test ./...` 与 `node --test` 都进 CI;
- 新增模型能力都有 fixture + 断言;
- UI 纯函数覆盖率 > 70%(不设全局硬门槛)。

### 5.2 CI(P0)

**现状**:无。

**目标**:`.github/workflows/ci.yml`:

- matrix `ubuntu-latest` + `windows-latest`;
- `gofmt -l .`、`go vet ./...`、`go test ./...`、`go build ./...`;
- Linux 侧额外 `golangci-lint`(可选);
- 缓存 Go module。

**验收**:

- 两个平台都绿;
- 故意留一个格式错误能被 CI 拦住;
- 无源码探测式跳过。

### 5.3 发布与文档(P2)

**现状**:只能本地 `go build`。

**目标**:

- `-version`(ldflags 注入)+ `go install github.com/huic/archview@latest` 可用;
- `goreleaser` 或手工 release:Windows/Linux/macOS 二进制;
- README 加截图、`-out`/`-editor`/配置说明;
- `docs/design.md`:数据流、模型 schema、端点、性能取舍。

**验收**:

- 打 tag 后 release 页面有三平台产物;
- 新机器按 README 两步跑起来;
- `go install` 的二进制与 `go build` 行为一致。

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

当前无第三方依赖。以下条目可能引入依赖,开工前先定:

| 需求 | 可选方案 | 倾向 |
| --- | --- | --- |
| 类型检查 | `go/importer` + `go list -export` | 标准库,优先 |
| 类型检查(备选) | `golang.org/x/tools/go/packages` | 只在标准库方案不足时 |
| EDN 解析 | 自写最小 reader | 避免依赖 |
| 覆盖率 | `go test` + `go tool cover -func` | 标准库 |
| UI 测试 | `node --test` / Playwright | 纯函数用 node,浏览器测试可选 |
| 发布 | goreleaser | 接受一个构建期工具 |
