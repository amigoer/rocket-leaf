# 路线图

[English](ROADMAP.md)

MQ Studio 正在成为一个覆盖所有消息队列的桌面客户端。每种中间件形态都通过可插拔驱动接入，
驱动声明自己的能力，界面只呈现当前连接的端点真正支持的功能。

本文是交付计划。它所交付的契约在[多 MQ 架构设计](MULTI_MQ_DESIGN.md)，面向用户的简版状态表
在 [README](../README.zh-CN.md)。

## 当前状态

- **已发布** — RocketMQ 4.x / 5.x，通过 Admin API 提供完整功能。
- **已发布** — RabbitMQ 3.x / 4.x，管理面走 HTTP 管理插件，数据面走 AMQP 0-9-1 而不是
  管理接口的 publish 与 get。管理面是完整的：带完整 arguments 的队列、Exchange 与
  Binding、连接与信道、死信、带健康检查与特性开关的节点、虚拟主机、用户与权限、策略与
  参数、定义导入导出、Shovel 与 Federation，以及 stream 队列。
- **已发布** — Kafka 3.x / 4.x，直接走 Kafka 协议本身（franz-go 与 kadm）。带分区、副本与
  配置的 Topic；带每分区 lag 与 Kafka 全部五种 offset 重置的消费组；按 offset、时间戳或
  key 浏览日志并跟随末尾；带 key、header、指定分区与 ack 级别的发送；带生效配置与日志目录
  的 Broker；ACL 与 SCRAM 用户；客户端配额；分区重分配与优先副本选举；以及集群正在跟踪的
  事务。
- **已发布** — MQTT 3.1.1 与 5.0，走 Paho 的两个 Go 库——它们互不重叠：一个只说 3.1.1，
  另一个只说 5.0，而配置成其中一种的 broker 会拒绝另一种的 CONNECT。这是第一个自身没有
  管理面的家族，所以一条连接能做什么是在拨号时按三层决定的：协议本身、多数 broker 会发布
  的 $SYS 树，以及 EMQX 等自带的 REST API。带 QoS、retain 与 5.0 属性的发布；实时订阅
  工作台，会报告丢弃了多少条、会话何时断开；从保留消息得出的主题列表——那是 MQTT 唯一
  能枚举的东西；来自 $SYS 的 broker 计数；以及在管理 API 有应答时，在线客户端与它们的
  会话、订阅、集群节点，并可断开某个会话。探测不到的那一层会连同原因一起上报，而不是
  让页面空着。
- **已发布** — NATS 2.x，走官方 Go 客户端及其 jetstream 子包。这是第二个自身没有管理面
  的家族，也是「可能缺席的东西」最多的一个：连接建立时探测四层——协议、JetStream、服务端的
  HTTP 监控端点、以及系统账户——后三层每一层都有两种不同的缺席原因，指向不同的修复位置，
  所以上报六条退化原因而不是一条。JetStream 的流及其主题、留存策略、存储方式与副本集；
  push 与 pull 两种消费者；按 sequence 浏览与跟随；按 subject 发布，并支持等待回复的
  request；按条数、sequence 或 subject 清理；集群里的服务器与它们的生效配置；客户端连接
  以及断开其中一条；还有账户页，显示各自的 JetStream 用量与被给到的上限。

  核心 NATS 是唯一放不进既有页面的部分，所以它单独占一页。一个 subject 什么都不留：消息
  发给此刻在听的人，之后就没了——没有历史可以往回翻，所以订阅台是实时视图，而不是加了
  过滤条件的消息浏览页。

  有四样东西是明确不做的。没有死信页：消费者用完 max_deliver 之后只是停止重投并发一条
  advisory，消息哪儿也没去。没有位点重置：消费者的起始位置在创建时就定死了，API 不接受
  修改，唯一的「重置」是删掉重建，而那会换掉它的身份。没有未确认投递清单，因为 JetStream
  只报未确认的数量，不报是哪些。账户是只读的：无论配置模式还是 operator 模式，没有任何
  NATS 服务端提供创建账户的请求。
- **已支持** — ActiveMQ Classic 5.x / 6.x 与 ActiveMQ Artemis 2.x，通过 Jolokia 接入，也就是
  两个 broker 都随 Web 控制台附带的那个 JMX-over-HTTP 代理。一个 MQKind 覆盖两种产品，因为对
  使用者来说它们是一个家族；而对驱动来说它们没有一处相同：代理路径不同、MBean 域不同、
  ObjectName 的键不同、属性名不同，连浏览结果的 map 键都毫无交集。到底是哪一个应答，在连接
  建立时就定下来——按哪个 MBean 域响应了 search 来判断。

  这个驱动特别的地方在于：管理面同时就是数据面。浏览和发送在两个产品上都是 JMX 操作，而且
  浏览什么都不会取走——所以消息页不需要 RabbitMQ 那样的 requeue 警告，而且在所有线协议接入点
  都关掉的 broker 上，每个页面照样能用。队列与主题及其堆积、计数器和配置；两种产品上的持久
  订阅，可建可删；带 JMS 消息头、属性和优先级的发送；沿声明反向查出死信目的地，并把消息重投
  回它们当初失败的地方——这是本应用第一个真正能重投的家族；broker 的存储、journal 与生效配置，
  以及它桥接到的其他 broker；还有客户端连接及各自使用的协议。

  AMQP 1.0 是一个可选层，在连接时探测，只为一件 JMX 做不到的事：实时跟随目的地。管理面是
  请求/响应式的，没有推送。只支持主题，这是安全规则——JMS 的消费者会真的消费，挂到队列上会把
  消息取走。接入点关闭的 broker 保留其余全部页面，并说明它处于三种状态中的哪一种。

  有四样东西是明确不做的。没有延迟投递，而两个产品其实都有：调度标注必须是 Long，而两个发送
  操作都只接受 Map<String,String>，所以通过 Jolokia 设的延迟会被接受、被忽略，然后被当成生效了
  报回来。没有位点、没有分区，因为 JMS 两者都没有——消息确认之后就没了，也没有任何东西把目的地
  切成消费者可寻址的分片。没有权限页：两个产品的认证都写在启动时读取的 XML 里，JMX 没有创建
  用户的操作。还有 Classic 的浏览止步于 maxBrowsePageSize，默认 400 条，无论目的地多深；这个
  上限无法通过 JMX 读取，所以页面把它作为一条 caveat 报出来，而不是假装队列就是 400 条深。
- **已完成设计，尚未实现** — 下面列出的九种形态。

## 交付顺序

| 阶段 | 范围 | 完成判据 |
| --- | --- | --- |
| 0–3 | 驱动接缝本身：契约、后端端口、存储与 bridge、前端注册表 | 对 RocketMQ 而言逐屏与之前完全一致 |
| 4 | **RabbitMQ** | 已完成。Exchanges/Bindings 页面存在，且没有 offset 概念泄漏进 UI |
| 5 | **Kafka** | Topic、消费组、lag、浏览与发布端到端可用 |
| 6 | **Redis Stream** | 已完成。Stream、消费组、浏览、发送、待处理列表（PEL）、服务器与其集群、客户端连接和 ACL 用户都读的是真实实例，也没有去假装存在 maxlen 或消息速率。和预期一样是纯增量，新增四个端口：日志的裁剪、订阅的位置、消息的写入，以及待处理列表 |
| 7 | **Pulsar** | 已完成。主题、命名空间与其上的租户、订阅与游标、浏览与跟随、发送控制台、死信与角色授权全部端到端可用；并且没有任何页面去假装这个中间件有 tag、磁盘用量或用户目录 |
| 8 | **MQTT** | 已完成。第一个自身没有管理面的家族：能做什么在连接时按三层探测——协议层、$SYS 树、broker 自带的 REST API——探测不到的那层会说明原因，而不是默默变空 |
| 9 | **NATS** | 已完成。和预期一样是纯增量——没有任何规范页面改变形状——但它是第一个驱动会去读 profile 认证方式（而不只是读凭据）的家族，也正因此暴露了一个拨号时把 RocketMQ 之外所有家族的认证方式重置掉的缺陷 |
| 10 | **ActiveMQ / Artemis** | 已完成。JMS 语义能套进规范页面，除了那些假定存在日志的地方：没有位点、没有分区、没有 trim。换来的是死信页第一次被填满，以及本应用第一个重投操作 |
| 11 | **NSQ** | 主题与 channel；没有消息历史，因此没有浏览 |
| 12 | **Amazon SQS**、**Google Cloud Pub/Sub**、**Azure Service Bus**、**Amazon Kinesis**，然后 **IBM MQ** 与 **Solace PubSub+** | 连接表单能表达「没有地址，只有 region 与凭证」 |

有两个排序决定值得一直放在视野里。

**RabbitMQ 排在 Kafka 前面是刻意的。** Kafka 与 RocketMQ 足够接近，即使抽象是错的它也会顺利
跑通，因此验证不了任何东西。RabbitMQ 在 offset、分区和消费组这三件事上与 RocketMQ 意见相左，
这种不一致才使它成为真正的检验。

**云托管这一档改变的是连接表单，而不只是驱动。** 阶段 7 之前的每一种形态都是「地址 + 可选
凭证」；云托管是「region + 凭证，完全没有地址」—— 这是第一次出现 `Endpoints` 为空却仍然合法
的连接。表单能否表达这一点，应该在敲定页面契约时就解决，而不是留到阶段 8。

## 各驱动的范围

每个驱动对接什么、点亮哪些规范页面、以及做不到什么。六个规范页面是 `Destinations`、
`Subscriptions`、`Messages`、`Publish`、`Cluster` 与 `Access`。

> RabbitMQ 以下的每一行都是依据各产品公开的管理接口做出的范围估计，属于计划输入而非已验证
> 的行为 —— 每一行都在对应驱动落地时才被确认。

### 自托管

| 驱动 | 管理面 | 点亮的页面 | 主要缺口 |
| --- | --- | --- | --- |
| **RocketMQ** 4.x / 5.x | 基于 remoting 协议的 Admin API | 全部六个 | Proxy 端点能回答的远少于 NameServer，能力在连接时收窄 |
| **RabbitMQ** | HTTP 管理插件，消息面走 AMQP 0-9-1 | 全部六个，外加 Exchanges/Bindings、连接、死信、虚拟主机、策略、定义、数据搬运 | 没有 offset 与分区；没有具名消费组；没有稳定的消息 id；浏览会把读到的消息重新入队，因此带 caveat；Shovel、Federation 与 stream 协议都是插件，未装时能力降级并给出原因 |
| **Kafka** | 基于 Kafka 协议的 AdminClient | 全部六个 | ACL 取决于所配置的 authorizer；浏览是按 offset 区间拉取，不是随机访问 |
| **Pulsar** | Admin REST API + 二进制协议 | 全部六个 | 已完成。tenant 与 namespace 最后两者都做了：既是每个页面上的范围选择器，也有自己的页面 —— 因为主题的地址就是 tenant/namespace/name，选择器的选项总得有个来源 |
| **ActiveMQ / Artemis** | 基于 JMX 的 Jolokia REST，外加用于跟随主题的 AMQP 1.0 | 全部六个，另加死信、连接与主题实时视图 | 已完成。两个产品的管理树没有一个 ObjectName、属性名或消息 map 键是相同的，靠 MBean 域判断哪一个在应答。浏览和发送都是管理操作，因此都不会取走消息、也不需要线协议客户端——可选的 AMQP 层存在的唯一理由是 JMX 无法推送。已确认：没有位点、分区和 trim，因为 JMS 没有；没有延迟投递，因为两个发送操作都只接受 Map<String,String> 而调度标注必须是 Long；没有权限页，因为两者的认证都写在 XML 里；Classic 的浏览止步于 maxBrowsePageSize，该值无法通过 JMX 读取，作为 caveat 报出 |
| **Redis Stream** | `XINFO`、`XRANGE`、`XADD` 等命令 | 全部六个 | 已完成。集群拓扑和 ACL 最后都做了：单机、哨兵与集群三种部署都能读，ACL 用户带键、频道与命令规则 |
| **NATS** | JetStream API、服务端监控端点与 $SYS 账户 | Destinations、Subscriptions、Messages、Publish、Subjects、Cluster、Connections、Accounts、Alerts | 已完成。四层，每一层都在连接时探测：未启用 JetStream 时端点退化为仅发布与订阅；集群相关页面需要监控端点或系统账户其一——监控端点只答一台服务器，$SYS 才答整个集群 |
| **NSQ** | nsqd 与 nsqlookupd HTTP 接口 | Destinations、Subscriptions、Publish、Cluster | 没有消息历史，因此没有浏览 |
| **MQTT** | 协议本身没有。连接时探测：$SYS 树，以及 broker 自带的 REST API（如果有）| 概览、主题、订阅、发布、客户端、集群、告警 | 没有消费组、没有 offset、没有历史——消息只在传输途中存在，无人订阅就没了。主题列的是持有保留消息的那些，因为别的都枚举不出来。客户端页需要管理 API，Mosquitto 没有 |

### 云托管

| 驱动 | 管理面 | 点亮的页面 | 主要缺口 |
| --- | --- | --- | --- |
| **Amazon SQS** | SQS API | Destinations、Messages、Publish | 没有消费组也没有集群；接收会启动可见性超时，因此浏览带 caveat |
| **Google Cloud Pub/Sub** | Publisher 与 Subscriber 管理 API | Destinations、Subscriptions、Publish | Subscription 是真实对象，积压量可直接对应 lag；拉取即消费，浏览需要 snapshot 或 caveat |
| **Azure Service Bus** | Service Bus 管理 API | Destinations、Subscriptions、Messages、Publish，以及路由页上的规则 | 没有集群；peek 是非破坏性的，浏览不需要 caveat |
| **Amazon Kinesis** | Kinesis API | Destinations、Subscriptions、Messages、Publish | 没有集群；Shard 不是分区，需要自己的一套列 |

### 企业

| 驱动 | 管理面 | 点亮的页面 | 主要缺口 |
| --- | --- | --- | --- |
| **IBM MQ** | 管理 REST 接口 | 全部六个 | 通道是一等概念且没有规范页面与之对应，很可能需要整页覆写 |
| **Solace PubSub+** | SEMP v2 | 全部六个 | Message VPN 做成范围选择器，与 Pulsar 的 namespace 同理 |

## 由已有驱动覆盖

协议兼容的系统不单独占用一个驱动。它们按自己所讲的协议接入对应驱动，该驱动在连接时把能力
收窄到端点实际支持的范围。

| 按此接入 | 系统 |
| --- | --- |
| Kafka | Redpanda、AutoMQ、WarpStream、Confluent、Amazon MSK、Azure Event Hubs |
| MQTT | EMQX、Mosquitto、HiveMQ、VerneMQ |
| ActiveMQ 或 RabbitMQ | Amazon MQ |
| RocketMQ | 阿里云与腾讯云的 RocketMQ |

## 不在范围内

- **ZeroMQ、nanomsg** —— 没有 broker，也就没有管理面可展示。
- **Celery、Sidekiq、BullMQ** —— 架在 Redis 或 RabbitMQ 之上的应用层任务队列。
  观测它们是另一个产品，而不是再加一个驱动。

## 驱动之外

- 恢复端到端 UI 覆盖。原有的 Playwright 套件通过 CDP 端点驱动 Electron，已随 Electron 一起
  移除；平台自带的 WebView 在 macOS 上没有等价方案。值得评估的选项：在 CI 中驱动 Linux
  WebKitGTK 构建，或用 Go 集成测试对 `tests/e2e/rocketmq` 环境覆盖相同流程。
- 更新下载进度的专门界面
- 更完整的 RocketMQ 5.x Proxy 与 ACL 管理能力

## 版本历史

### v0.1.0

随 Wails 3 重写，版本号回到 1.0 以下：早先的 1.x 线基于另一套架构，已不再发布。

- 从 Electron + 本地 Go 守护进程迁回 Wails 3
- 用进程内 Wails 绑定替换本地回环 HTTP 传输
- 保持 RocketMQ 功能、本地设置与加密格式兼容
- 提供 macOS、Windows 与 Linux 安装包
- 保留 bridge 层的敏感字段脱敏
