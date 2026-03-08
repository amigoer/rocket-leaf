# 🍃 Rocket-Leaf

<p align="center">
  <img src="docs/images/logo.png" alt="Rocket-Leaf Logo" width="128">
</p>

<p align="center">
  <strong>轻量、美观的 RocketMQ 桌面客户端</strong>
</p>

<p align="center">
  Windows · macOS · Linux
</p>

## 这是什么？

Rocket-Leaf 是一个**本地桌面应用**，用来连接和管理 RocketMQ 集群：查 Topic、看消费者、查发消息，不用再部署 Web 控制台，也不用暴露管理端口。

- **开箱即用** — 下载打开即可使用  
- **跨平台** — 支持 Windows、macOS、Linux  
- **数据在本地** — 连接配置保存在本机，可随时备份

## 能做什么？

| 能力         | 说明                                                      |
| ------------ | --------------------------------------------------------- |
| **连接管理** | 添加多个集群连接，一键切换，配置本地保存                  |
| **Topic**    | 列表、搜索、详情（分区/权限/消费者）、创建与删除          |
| **消费者组** | 列表、消费进度、重置位点、订阅关系                        |
| **消息**     | 按 Topic / Key / MessageId 查询，看详情、发测试消息、轨迹 |
| **监控**     | 集群状态、生产/消费 TPS、堆积情况                         |

## 下载与安装

从 [Releases](https://github.com/codermast/rocket-leaf/releases) 下载对应系统的安装包或可执行文件，安装后直接运行即可。

## 快速开始（第一次使用）

1. **打开应用**，在首页点击「添加连接」。
2. **填写集群信息**：NameServer 地址（必填）、账号密码（若集群开启了鉴权）。
3. **保存并连接**，连接成功后即可在侧栏使用 Topic、消费者组、消息查询等功能。

连接配置会保存在本机，下次打开会自动列出，无需重新填写。

<details>
<summary>连接数据存储位置（备份/迁移时可参考）</summary>

- **macOS**: `~/Library/Application Support/rocket-leaf/connections.json`
- **Linux**: `~/.config/rocket-leaf/connections.json`
- **Windows**: `%AppData%\rocket-leaf\connections.json`

</details>

## 路线图与参与开发

- **功能规划**见 [路线图](docs/ROADMAP.md)。  
- **参与开发**：欢迎提 Issue 和 PR，Fork 后按常规流程即可。技术栈与项目结构见 [架构说明](docs/ARCHITECTURE.md)。

## 许可证

[MIT](LICENSE) · Made with ❤️ by [CoderMast](https://github.com/codermast)
