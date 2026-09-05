# 参与 MQ Studio

[English](CONTRIBUTING.md)

MQ Studio 想做的是一个客户端连所有消息队列，而它是一个驱动一个驱动长起来的。最有价值的贡献，
往往是一份来自这里没人跑过的服务端的报告 —— 一个版本、一种拓扑、一套和测试覆盖到的环境行为不同的
部署。代码同样欢迎，动手之前请先读完这份文档。

## 可以做些什么

- **报告 Bug。** 三种 issue 模板中英文各有一套，用你顺手的那种语言即可。
- **申请驱动。** [驱动支持请求](https://github.com/amigoer/mq-studio/issues/new?template=6-driver-request.zh-CN.yml)
  模板里有一个问题决定了这个驱动能不能做：桌面端能连到什么，怎么连。
- **帮忙验证**：拿这个项目手里没有的集群跑一跑待发布的版本。
- **修翻译。** 所有面向用户的文案都在 `frontend/src/i18n/locales/en.json` 与 `zh.json`，
  别扭的措辞最容易在其中一份里一直留着没人发现。
- **写代码。** 先把下面这些看完。

## 提 issue

空白 issue 是关掉的，原因就是模板本身：维护者要追着问的东西，基本都已经是表单上的一个字段了。
即使问题看起来和版本、消息队列都无关，也请把这两项填上 —— 这一对决定了问题在这边能不能复现出来。

issue 用中文或英文都会读。哪种写着顺手用哪种，模板两种都有。

## 标签

三条轴，外加一小组流程标签。一个 issue 通常带一个 `type:`、一个 `area:`，
如果问题和具体家族有关再带一个 `driver:`。

- **`type:`** —— `bug`、`feature`、`driver`、`docs`、`question`。这一条由 issue
  模板自动打上。
- **`area:`** —— 应用的哪一部分：`connections`、`topics`、`messages`、
  `consumers`、`cluster`、`admin`、`app`、`i18n`、`website`、`ci`。
- **`driver:`** —— 消息队列家族，每个已发布的驱动一个。
- **流程** —— `needs:info`、`needs:repro`、`blocked:upstream`，以及常见的
  `good first issue`、`help wanted`、`duplicate`、`wontfix`。

整套标签放在 [`.github/labels.json`](.github/labels.json) 里，而不是只存在于
GitHub 上，这样它能在 diff 里被审阅，也能在有人手动删掉某个之后重建出来：

```bash
npm run labels:sync              # 只打印差异
npm run labels:sync -- --apply   # 真正写入
```

默认是空跑，因为改名和清理都会动到别人已经提交的 issue 上挂着的东西。

## 环境准备

需要 Go（版本以 `go.mod` 为准）、Node.js 20.19+ 或 22.12+、npm，以及
[Wails 3 CLI](https://v3.wails.io)。Docker 只有跑真实环境测试时才需要。

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
make install
make dev
```

`make help` 会列出全部命令。进程模型与仓库结构见
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 推送之前

```bash
make check
```

这和 CI 跑的是同一道关：版本号在整棵树里是否一致、下面说的 changelog 引用、前端构建与类型检查、
`gofmt`、`go vet`、Go 与前端单元测试，以及生成的 TypeScript binding 有没有过期。

如果改了 Go service 的签名，重新生成 binding 并提交结果：

```bash
npm run generate:bindings
```

## 真实环境测试

每个驱动都由真实服务端的测试把关，而不是 mock。本地是显式开启的，所以即使容器全开着，
直接跑 `go test ./...` 也仍然是离线且快的：

```bash
npm run e2e:kafka:up && npm run e2e:kafka:seed
MQ_STUDIO_E2E=1 go test ./internal/driver/kafka/...
```

每个家族都有自己的 `up`、`seed`、`down` 脚本，`npm run` 不带参数就能列出来。
`npm run test:e2e` 会对着当前起着的环境跑整套真实测试。

在 CI 里，这个开关根本不会被读取：workflow 会把所有环境都起起来，因此少一个服务端是失败，
而不是跳过。由此有两条规则，新增一个真实环境测试两条都要满足：

- 调用 `e2e.Require(t, e2e.Env{...})`，要带探测，**也要带 `Family`** ——
  没有 family 的 `Env` 不会被任何分片认领，结果是悄悄不跑，而不是变红；
- 这个 family 在 [`.github/workflows/ci.yml`](.github/workflows/ci.yml) 里有对应的分片。

## 提交信息

提交信息遵循 [Conventional Commits 1.0.0](https://www.conventionalcommits.org/zh-hans/v1.0.0/)，
并且用英文书写。

```
<type>(<scope>): <subject>

[body]

[footer]
```

- `type` 取 `feat`、`fix`、`docs`、`style`、`refactor`、`test`、`chore`、
  `perf`、`ci`、`revert` 之一。
- `scope` 可选，小写 —— 驱动名、层名或区域名：`kafka`、`update`、`shell`、`e2e`。
- `subject` 用祈使语气，小写开头，结尾不加句号，不超过 50 个字符。
- body 说明*为什么*，按 72 列折行。subject 已经说清楚的就不用写。
- 破坏性变更在 type 后加 `!`，并附 `BREAKING CHANGE:` footer。

仓库里的真实例子：

```
fix(update): repair the update lifecycle end to end
feat(nats): add the NATS driver
ci: gate every change on the changelog naming its issues
```

## 分支命名

`<type>/<短描述>`，kebab-case，type 与提交信息用的是同一套：

```
feat/rocketmq-namespace
fix/e2e-seed-silent-failure
ci/changelog-reference-gate
```

## changelog 规则

关闭 issue 的提交，要在**提交正文**里带上 `Closes #NN` 这行 footer。如果 issue 有意保持打开，
用 `Refs #NN`。`Fixes` 和 `Resolves` 不被识别 —— 这个仓库只用一种写法，多一种只会把它劈成两半。

自上一个 tag 以来所有提交里的每一个 `Closes #NN`，都必须出现在**两份** changelog 的
`Unreleased` 小节里：

- `CHANGELOG.md`
- `CHANGELOG.zh-CN.md`

否则 **Changelog** 这项检查会失败。它会把编号和各自所在的提交都打出来，照着失败信息就能把这一节
写出来。本地用 `npm run check:refs` 自己跑。

有两点容易踩：

- 写在反引号里的编号不算数。代码片段是先匹配的，这是故意的：一条把 `` `#61` ``
  当字面量写出来的条目，什么也没有记录下来。
- 是两份都要，不是任选其一。只在英文那份里出现会被报成不一致。

这条规则的由来是：读者接触一个变更的地方只有发布说明，而除此之外，能说明这个变更是哪个 issue
提出来的，就只剩提交的 footer 了。

## 成对存在的东西

凡是用户会读到的内容都有两份，而被忘掉的总是第二份：

| 英文 | 中文 |
| --- | --- |
| `README.md` | `README.zh-CN.md` |
| `CHANGELOG.md` | `CHANGELOG.zh-CN.md` |
| `CONTRIBUTING.md` | `CONTRIBUTING.zh-CN.md` |
| `docs/INSTALL.md` | `docs/INSTALL.zh-CN.md` |
| `docs/ROADMAP.md` | `docs/ROADMAP.zh-CN.md` |
| `frontend/src/i18n/locales/en.json` | `frontend/src/i18n/locales/zh.json` |
| `website/src/i18n/en.ts` | `website/src/i18n/zh.ts` |
| `docs/images/hero-{light,dark}.svg` | `docs/images/hero-{light,dark}.zh-CN.svg` |

## 新增一个驱动

一个驱动就是 `internal/driver/` 下的一个包，它实现 `internal/driver/driver.go` 里的端口
并声明自己的 capability。页面是根据这些声明自己画出来的，所以一个驱动声明了却答不上来的
capability，画出来的页面看着不像「诚实」，而像「坏了」。

真正容易出错的是驱动包**之外**的部分。两个各自新增驱动的分支在表格里永远不会冲突 ——
两行都会保留下来 —— 而表格旁边的叙述、插图和计数是只有一边动过的单行，合并时会悄悄留下其中一版。
新增一个家族之后，下面这些全都要改：

- `internal/model/mqkind.go` —— 新的 kind 和它的展示名。
- `README.md` 与 `README.zh-CN.md` —— hero 的 `alt` 文案、「目前可用的驱动」那句话、
  驱动支持表格，以及开发计划表格。
- `docs/ROADMAP.md` 与 `docs/ROADMAP.zh-CN.md`。
- `docs/images/hero-{light,dark}{,.zh-CN}.svg` —— 里面的 `<desc>`、驱动数量徽标，
  以及每个家族一条的示意泳道。四个文件，而且是要重新排版，不是改几个字。
- `website/src/i18n/en.ts` 与 `zh.ts` —— `meta.description`、`banner.text`、
  `hero.subtitle`、`drivers.supported`、`drivers.planned` 与 `roadmap.stages`。
  `planned` 最危险：不改它就会一直声称一个已经发布的驱动还没做。
- `frontend/src/i18n/locales/en.json` 与 `zh.json` ——
  `page.settings.about.blurb` 里点了各个家族的名字。
- `frontend/src/App.tsx` 与 `frontend/src/mq/navigation.ts` —— 注释里对家族数量的计数。
- `.github/ISSUE_TEMPLATE/` —— Bug 与功能建议表单里的消息队列下拉项，中英两套都要改，
  同时把这个家族从驱动支持请求表单里去掉。
- `frontend/src/lib/alertRules.ts` 与 `frontend/src/lib/alertDerive.ts` —— 一个家族
  能触发哪些规则，以及挑选规则的分发逻辑。两处都不加，新驱动会落回 RocketMQ 的规则，
  而那些规则读的是它根本不报的数字：告警页看着是武装好的，实际永远不会触发。
- `frontend/src/i18n/degradedReasons.test.ts` —— 驱动声明的每一条 degraded 原因
  **和 caveat** 的手抄副本。没有任何东西把 Go 字符串和 JSON 键绑在一起，改名时只有
  这份副本会变红。
- `.github/labels.json` —— 加一条 `driver:<family>`，然后跑 `npm run labels:sync`。
- `tests/e2e/<family>/compose.yaml`、`package.json` 里的 `e2e:<family>:*` 脚本，
  以及 `.github/workflows/ci.yml` 里的分片。

不要靠眼睛数家族。哪些驱动能答哪个页面，由 capability 声明说了算：

```bash
grep -rn '^\s*model\.Cap<X>,\s*$' internal/driver/*/*.go
```

## Pull Request

提到 `main` 分支。标题就是提交的 subject —— 同样是 Conventional Commits 格式，因为 squash
之后落下来的就是它。模板里问的是改了什么、为什么、怎么验证，第三项才是评审真正需要的。

一个 PR 只做一件事。一个驱动、一个修复、一次重构 —— 不要两件一起，也不要在修复里顺手夹带重构。

PR 上会报告四项检查：

- **Check**（`ci.yml`）—— 起全部服务端环境并跑完整道关。这一项是你的。
- **Build**（`website.yml`）—— 官网。它在构建时会渲染本仓库自己的 markdown，
  所以改一句 changelog 的措辞就可能把它弄挂。
- **Package** —— PR 上按设计就是跳过的。
- **Workers Builds** —— 在任何 PR 分支上都会失败，**和你的改动无关**。
  这个账号没有开预览部署，只有从 `main` 出去的那次部署才可能变绿。忽略它即可。

## 发布

发布由维护者来做，流程见 [RELEASE.md](RELEASE.md)。贡献者不需要动版本号 ——
`package.json` 保持原样，条目写在 `Unreleased` 下面就好。

## 许可

提交贡献即表示你同意你的工作以 [Apache License 2.0](LICENSE) 授权，与项目其余部分一致。
