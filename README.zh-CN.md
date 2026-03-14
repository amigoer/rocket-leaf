# Rocket-Leaf

<p align="center">
  <img src="docs/images/logo.png" alt="Rocket-Leaf Logo" width="180">
</p>

<p align="center">
  <strong>轻量、美观的 RocketMQ 桌面客户端</strong>
</p>

<p align="center">
  Windows · macOS · Linux
</p>

<p align="center">
  <a href="README.md">English</a>
</p>

## 这是什么？

Rocket-Leaf 是一个**本地桌面应用**，用来连接和管理 RocketMQ 集群。你可以用它查看 Topic、消费者组、消息详情和监控状态，也可以发送测试消息，而不需要额外部署 Web 控制台或暴露管理端口。

- **开箱即用**：下载后即可直接运行
- **跨平台**：支持 Windows、macOS、Linux
- **数据保存在本地**：连接配置保存在当前设备，便于备份和迁移

## 功能概览

| 能力 | 说明 |
| ---- | ---- |
| **连接管理** | 添加多个集群连接，一键切换，配置本地保存 |
| **Topic** | 列表、搜索、详情查看，以及创建和删除 |
| **消费者组** | 查看列表、消费进度、重置位点和订阅关系 |
| **消息** | 按 Topic / Key / MessageId 查询，查看详情，发送测试消息，追踪消息链路 |
| **监控** | 查看集群状态、生产消费 TPS 和堆积情况 |

## 下载与安装

从 [Releases](https://github.com/codermast/rocket-leaf/releases) 下载对应平台的安装包或可执行文件。

### macOS

- **Intel Mac**：`rocket-leaf-macos-amd64.app.zip`
- **Apple Silicon / M 系列 Mac**：`rocket-leaf-macos-arm64.app.zip`
- **不确定机型时**：`rocket-leaf-macos-universal.app.zip`

### Windows

- **x64**：安装包和便携可执行文件
- **ARM64**：安装包和便携可执行文件

### Linux

- **x64 / ARM64**：提供 AppImage、`.deb`、`.rpm`、`.pkg.tar.zst` 等格式（按构建结果提供）

## 快速开始

1. 打开应用，在首页点击 `Add Connection`。
2. 填写集群信息：`NameServer` 地址为必填项；如果集群开启鉴权，再填写账号和密码。
3. 保存并连接。连接成功后，就可以在侧边栏使用 Topic、消费者组、消息查询等功能。

连接配置会保存在本机，下次打开时会自动显示。

<details>
<summary>连接数据存储位置</summary>

- **macOS**: `~/Library/Application Support/rocket-leaf/connections.json`
- **Linux**: `~/.config/rocket-leaf/connections.json`
- **Windows**: `%AppData%\rocket-leaf\connections.json`

</details>

## 路线图与参与开发

- 功能规划见 [路线图](docs/ROADMAP.md)
- 欢迎提交 Issue 和 PR。当前技术栈与项目结构可参考 [架构说明](docs/ARCHITECTURE.md)

## 许可证

[MIT](LICENSE) · Made with love by [CoderMast](https://github.com/codermast)
