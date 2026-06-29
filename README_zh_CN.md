# Hyperledger FireFly

[![codecov](https://codecov.io/gh/hyperledger-firefly/firefly/branch/main/graph/badge.svg?token=QdEnpMqB1G)](https://codecov.io/gh/hyperledger-firefly/firefly)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger-firefly/firefly)](https://goreportcard.com/report/github.com/hyperledger-firefly/firefly)
[![FireFy Documentation](https://img.shields.io/static/v1?label=FireFly&message=documentation&color=informational)](https://hyperledger.github.io/firefly//)
![build](https://github.com/hyperledger-firefly/firefly/actions/workflows/docker_main.yml/badge.svg?branch=main)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/7826/badge)](https://www.bestpractices.dev/projects/7826)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/hyperledger-firefly/firefly/badge)](https://scorecard.dev/viewer/?uri=github.com/hyperledger-firefly/firefly)

![Hyperledger FireFly](./images/hyperledger_firefly_logo.png)

Hyperledger FireFly 是首款开源的超级节点：一个安全的企业级的构建和拓展Web3应用的全栈式解决方案。

FireFly 提供的数字资产、数据流和区块链交易API，使企业能够快速在流行的区块链技术和协议上构建生产就绪的应用程序。

[ENGLISH](./README.md) | [简体中文](./README_zh_CN.md)

## 开始使用 Hyperledger FireFly

了解FireFly的最佳方式请参照[文档](https://hyperledger.github.io/firefly)。

您可以在这里找到我们的[入门指南](https://hyperledger.github.io/firefly/latest/gettingstarted/),
通过该指南将帮助您在几分钟内在本地机器上运行起来一个FireFly超级节点网络的开发环境。

您的开发环境将包括:

FireFly CLI                   |  FireFly Explorer UI                | FireFly Sandbox  |
:----------------------------:|:-----------------------------------:|:----------------:|
[![CLI](./images/firefly_cli.png)](https://hyperledger.github.io/firefly/latest/gettingstarted/firefly_cli/#install-the-firefly-cli) | [![UI](./images/firefly_explorer.png)](https://github.com/hyperledger-firefly/ui) | [![Sandbox](./images/firefly_sandbox.png)](https://hyperledger.github.io/firefly/latest/gettingstarted/sandbox/#use-the-sandbox) |

## 加入社区

- [加入我们的 Discord](https://discord.gg/hyperledger)

## 技术架构

Hyperledger FireFly 拥有可插拔的微服务架构。无论是区块链协议、ERC代币标准、自定义智能合约，还是事件分发层以及私有数据库，一切都可以插件化。

因此，即使您所需要的区块链技术目前还没有相对应的支持，您也不必担心。插件化的设计大大降低了添加更多的区块链技术的难度，避免你花费大量时间去重新构建不同区块链技术之间可以复用的基础设施。

[![Hyperledger FireFly 技术架构图](./doc-site/docs/images/firefly_architecture_overview.jpg)](https://raw.githubusercontent.com/kaleido-io/firefly/main/doc-site/docs/images/firefly_architecture_overview.jpg)

## 开始为 Hyperledger FireFly 做贡献

无论您是前端、后端，还是全栈开发者，这里都有适合您的贡献机会。

请查看我们的[贡献者指南](https://hyperledger.github.io/firefly/latest/contributors/)，**欢迎加入**！

## 其他存储库

您当前所在的是“核心”存储库，用Go语言编写，包含了API服务器和中央编排引擎。本库还提供多种插件接口，用于支持使用 TypeScript 和 Java 等语言编写的微服务连接器以及其他关键运行组件。

以下是您可能感兴趣的各个微服务组件、用户经验、CLI 和应用示例的存储库。

> 注意：以下仅列出了开源存储库和插件

### 区块链连接

- Transaction Manager（区块链交易管理组件）- https://github.com/hyperledger-firefly/transaction-manager
- RLP & ABI 编码, Keystore V3实用工具 和 secp256k1 签名运行时 -  https://github.com/hyperledger-firefly/signer
- 通用型以太坊区块链的参考连接器 - https://github.com/hyperledger-firefly/evmconnect
  - EVM兼容公链: 请参见[文档](https://hyperledger.github.io/firefly)
- 针对许可制以太坊区块链的连接器 - https://github.com/hyperledger-firefly/ethconnect
  - 私有/许可制区块链: Hyperledger Besu / Quorum
- Hyperledger Fabric连接器 - https://github.com/hyperledger-firefly/fabconnect
- Tezos连接器 - https://github.com/hyperledger-firefly/tezosconnect
- Corda连接器示例: https://github.com/hyperledger-firefly/cordaconnect
  - 使用该连接器，需要对 CorDapp 进行定制化开发

### 代币标准

- ERC20/ERC721 代币 - https://github.com/hyperledger-firefly/tokens-erc20-erc721
- ERC1155 代币 - https://github.com/hyperledger-firefly/tokens-erc1155

### 私有数据总线连接

- HTTPS 数据交换组件 - https://github.com/hyperledger-firefly/dataexchange-https

### 开发者生态系统

- 命令行界面 (CLI) - https://github.com/hyperledger-firefly/cli
- 图形用户界面 - https://github.com/hyperledger-firefly/ui
- Node.js SDK - https://github.com/hyperledger-firefly/sdk-nodejs
- 沙盒/测试工具 - https://github.com/hyperledger-firefly/sandbox
- 示例 - https://github.com/hyperledger-firefly/samples
- FireFly 性能测试 CLI: https://github.com/hyperledger-firefly/perf-cli
- 部署到Kubernetes的示例Helm Charts: https://github.com/hyperledger-firefly/helm-charts

## FireFly 核心代码层级结构

```
┌──────────┐  ┌───────────────┐
│ cmd      ├──┤ firefly   [Ff]│  - CLI入口
└──────────┘  │               │  - 父级上下文创建
              │               │  - 信号处理
              └─────┬─────────┘
                    │
┌──────────┐  ┌─────┴─────────┐  - HTTP 监听器 (Gorilla mux)
│ internal ├──┤ api       [As]│    * TLS (SSL), CORS 配置等.
└──────────┘  │ server        │    * 同一端口上的 WS 升级
              │               │  - REST 路由定义
              └─────┬─────────┘    * 只负责简单的路由逻辑，所有处理逻辑交由orchestrator
                    │
              ┌─────┴─────────┐  - REST 路由定义框架
              │ openapi   [Oa]│    * 标准化 Body，Path，Query， Filter 语义
              │ spec          |      - 生成 OpenAPI 3.0 (Swagger) 文档
              └─────┬─────────┘    * 包括 Swagger. UI
                    │
              ┌─────┴─────────┐  - WebSocket 服务器
              │           [Ws]│    * 为业务应用开发提供开发者友好的 JSON 协议
              │ websockets    │    * 可靠的有序事件传递
              └─────┬─────────┘    * _Event interface [Ei] 支持集成其他计算框架/传输方式_
                    │
              ┌─────┴─────────┐  - 监听数据库事件改变的拓展接口
              │ admin     [Ae]│    * 用于构建外部运行的微服务扩展核心功能  
              │ events        |    * 被 Transaction Manager 组件使用
              └─────┬─────────┘    * 支持特定对象类型过滤
                    │
              ┌─────┴─────────┐  - 核心数据类型
              │ fftypes   [Ft]│    * 用于 API 和序列化
              │               │    * API 可通过路由定义对输入字段进行屏蔽
              └─────┬─────────┘
                    │
              ┌─────┴─────────┐  - 核心运行时环境服务器，初始化和拥有以下的实例:
              │           [Or]│    * Components：功能组件
  ┌───────┬───┤ orchestrator  │    * Plugins：可插拔的基础设施服务
  │       │   │               │  - 向路由层提供处理逻辑
  │       │   └───────────────┘    * 所有的API调用将会在这里开始处理
  │       │
  │  Components: 包含主要功能的处理逻辑
  │       │
  │       │   ┌───────────────┐  - 跨链技术智能合约逻辑的整合
  │       ├───┤ contract  [Cm]│    * 为智能合约生成 OpenAPI 3 / Swagger 定义，并且传播到网络中
  │       │   │ manager       │    * 管理原生区块链事件监听，并将其路由到应用事件  
  │       │   └───────────────┘    * 负责在原生区块链接口 (ABI 等) 和 FireFly Interface [FFI] 格式之间的转换
  │       │
  │       │   ┌───────────────┐  - 维护整个网络的视图
  │       ├───┤ network   [Nm]│    * 与 network permissioning [NP] 插件的集成
  │       │   │ map           │    * 与广播插件的集成
  │       │   └───────────────┘    * 处理成员身份、节点身份和签名身份的层次结构  
  │       │
  │       │   ┌───────────────┐  - 广播数据给所有网络中的成员
  │       ├───┤ broadcast [Bm]│    * 实现批量组件的分发
  │       │   │ manager       |    * 与 shared storage [Ss] 插件的集成
  │       │   └───────────────┘    * 与blockchain interface [Bi] 插件的集成
  │       │
  │       │   ┌───────────────┐  - 发送私有数据给网络中的成员
  │       ├───┤ private   [Pm]│    * 实现批处理组件的调度器
  │       │   │ messaging     |    * 与 data exchange [Dx] 插件的集成
  │       │   └──────┬────────┘    * 消息可以通过区块链固定或者排序，或者仅发送
  │       │          │
  │       │   ┌──────┴────────┐  - 有着隔离的数据和区块链的各方群组
  │       │   │ group     [Gm]│    * 与 data exchange [Dx] 插件的集成
  │       │   │ manager       │    * 与 blockchain interface [Bi] 插件的集成
  │       │   └───────────────┘
  │       │
  │       │   ┌───────────────┐  - 私有数据管理和验证
  │       ├───┤ data      [Dm]│    * 实现批量组件的调度器
  │       │   │ manager       │    * 与 data exchange [Dx] 插件的集成
  │       │   └──────┬────────┘    * 与 blockchain interface [Bi] 插件的集成
  │       │          │
  │       │   ┌──────┴────────┐  - JSON 数据模式管理与验证（架构可扩展到 XML 等）  
  │       │   │ json      [Jv]│    * 站内和站外消息的JSON语法管理和验证
  │       │   │ validator     │    * 模式传播  
  │       │   └──────┬────────┘    * 与广播插件集成
  │       │          │
  │       │   ┌──────┴────────┐  - 可通过ID或者hash寻址的二进制数据存储
  │       │   │ blobstore [Bs]│    * 与 data exchange [Dx] 插件的集成
  │       │   │               │    * 对数据进行hash化处理, 并且在blob存储中维护负载引用的映射
  │       │   └───────────────┘    * 与 blockchain interface [Bi] 插件的集成
  │       │
  │       │   ┌───────────────┐  - 负责共享存储内容的下载
  │       ├───┤ shared    [Sd]│    * 并行异步下载
  │       │   │ download      │    * 可靠的重试和宕机恢复
  │       │   └───────────────┘    * 完成后通知事件aggregator
  │       │
  │       │   ┌───────────────┐
  │       ├───┤ identity [Im] │  - 跨组件中心化身份管理服务
  │       │   │ manager       │    * 解析API输入的身份和密钥组合（例如短名称、格式化等）
  │       │   │               │    * 将注册的链上签名密钥映射回身份
  │       │   └───────────────┘    * 集成区块链接口和可插拔身份接口（待定）
  │       │
  │       │   ┌───────────────┐  - 记录通过插件对外部组件执行的所有操作
  │       ├───┤ operation [Om]│    * 更新数据库中的输入/输出
  │       │   │ manager       │    * 为插件提供一致的重试语义
  │       │   └───────────────┘
  │       │
  │       │   ┌───────────────┐  - 私有数据管理和验证
  │       ├───┤ event     [Em]│    * 实现批量组件的调度器
  │       │   │ manager       │    * 与 data exchange [Dx] 插件的集成
  │       │   └──────┬────────┘    * 与 blockchain interface [Bi] 插件的集成
  │       │          │
  │       │   ┌──────┴────────┐  - 处理传入的外部数据
  │       │   │           [Ag]│    * 与 data exchange [Dx] 插件的集成
  │       │   │ aggregator    │    * 与 shared storage [Ss] 插件的集成
  │       │   │               │    * 与 blockchain interface [Bi] 插件的集成
  │       │   │               │  - 确保只有所有数据都就绪时有效事件才会被分发
  │       │   └──────┬────────┘    * 上下文感知，避免“全局阻塞”场景的出现
  │       │          │
  │       │   ┌──────┴────────┐  - 订阅管理
  │       │   │           [Sm]│    * 创建和管理订阅
  │       │   │ subscription  │    * 创建和管理订阅
  │       │   │ manager       │    * 发消息到事件匹配逻辑
  │       │   └──────┬────────┘
  │       │          │
  │       │   ┌──────┴────────┐  - 管理事件到已连接应用的传递
  │       │   │ event     [Ed]│    * 与 data exchange [Dx] 插件的集成
  │       │   │ dispatcher    │    * 与 blockchain interface [Bi] 插件的集成
  │       │   └───────────────┘
  │       │
  │       │   ┌───────────────┐  - Token 创建、传输的初始化、索引和协同
  │       ├───┤ asset     [Am]│    * 支持同质化代币（如数字货币）
  │       │   │ manager       │    * 支持非同质化代币： NFTs / globally uniqueness / digital twins
  │       │   └───────────────┘    * 交易历史的全索引
  │       │   [REST/WebSockets]
  │       │   ┌─────┴─────────────┐   ┌──────────┐   ┌─ 
  │       │   │ ERC-20 / ERC-721  ├───┤ ERC-1155 ├───┤  创建 token 连接器的简单框架
  │       │   └───────────────────┘   └──────────┘   └─ 
  │       │
  │       │   ┌───────────────┐
  │       ├───┤ sync /   [Sa] │  - 同步、异步桥接
  │       │   │ async bridge  │    * 提供同步请求、应答APIs
  │       │   │               │    * 转换为底层事件驱动API
  │       │   └───────────────┘
  │       │
  │       │   ┌───────────────┐  - 聚合报文和数据，并生成哈希用于上链
  │       ├───┤ batch     [Ba]│    * 可插拔调度器
  │       │   │ manager       │  - 数据库和主线API处理解耦
  │       │   │               │    * 有关active/active排序的更多信息，请参考架构图
  │       │   └──────┬────────┘  - 管理批处理器实例的创建
  │       │          │
  │       │   ┌──────┴────────┐  - 根据需求启动的短生命周期代理
  │       │   │ batch     [Bp]│    * 与作者+消息类型耦合
  │       │   │ processor     │  - 批量构建100多条信息以优化上链性能
  │       │   │               │    * 聚合消息和数据，并生成哈希用于上链
  │       │   └───────────────┘  - 在配置的空闲时间后自动关闭
  │       ... 更多待定
  │
Plugins: 每个插件都包含一个Go shim，以及一个远程代理微服务运行环境(如有需要)
  │
  │           ┌───────────────┐  - 区块链接口
  ├───────────┤           [Bi]│    * 交易提交 - 包括签名密钥管理
  │           │ blockchain    │    * 事件监听
  │           │ interface     │    * 标准化操作，和自定义链上耦合
  │           └─────┬─────────┘
  │                 │
  │                 ├─────────────────────┬───────────────────┬-───────────────────┐
  │           ┌─────┴─────────┐   ┌───────┴───────┐   ┌───────┴────────┐   ┌───────┴────────┐
  │           │ ethereum      │   │ fabric        │   │ corda/cordapps │   │ tezos          │
  │           └─────┬─────────┘   └───────────────┘   └────────────────┘   └────────────────┘
  │           [REST/WebSockets]
  │           ┌─────┴────────────────────┐   ┌────────────────────────┐   ┌─ 
  │           │ transaction manager [Tm] ├───┤ Connector API [ffcapi] ├───┤ 构建区块链连接器的简单框架
  │           └──────────────────────────┘   └────────────────────────┘   └─ 
  │        
  │           ┌───────────────┐  - 代币接口
  ├───────────┤ tokens    [Ti]│    * 标准化的核心概念：token pools，transfers，approvals
  │           │ interface     │    * 可插拔的跨代币标准
  │           └───────────────┘    * 通过微服务连接器针对自定义代币标准的简单实现方式的支持
  │           [REST/WebSockets]
  │           ┌─────┴─────────────┐   ┌──────────┐   ┌─ 
  │           │ ERC-20 / ERC-721  ├───┤ ERC-1155 ├───┤  构建代币连接器的简单框架
  │           └───────────────────┘   └──────────┘   └─ 
  │
  │           ┌───────────────┐  - P2P 内容寻址文件系统
  ├───────────┤ shared    [Si]│    *  负载 上传 / 下载
  │           │ storage       │    * 有效的负载参考管理
  │           │ interface     │
  │           └─────┬─────────┘
  │                 │
  │                 ├───────── ... 可拓展至任意共享存储系统，可供所有成员访问
  │           ┌─────┴─────────┐
  │           │ ipfs          │
  │           └───────────────┘
  │
  │           ┌───────────────┐  - 私有数据交换
  ├───────────┤ data      [Dx]│    * Blob 存储
  │           │ exchange      │    * 私有安全消息传递
  │           └─────┬─────────┘    * 安全的文件传输
  │                 │
  │                 ├─────────────────────┬────────── ... 可拓展至任意私有数据交换技术
  │           ┌─────┴─────────┐   ┌───────┴───────┐
  │           │ https / MTLS  │   │ Kaleido       │
  │           └───────────────┘   └───────────────┘
  │
  │           ┌───────────────┐  - API 认证和认证接口
  ├───────────┤ api auth  [Aa]│    * 验证安全凭据 (OpenID Connect id token JWTs etc.)
  │           │               │    * 提取 API/user 身份 (用于身份接口映射)
  │           └─────┬─────────┘    * 细粒度 API 访问控制的执行点
  │                 │
  │                 ├─────────────────────┬────────── ... 可拓展到任意单点登录技术
  │           ┌─────┴─────────┐   ┌───────┴───────┐
  │           │ apikey        │   │ jwt           │
  │           └───────────────┘   └───────────────┘
  │
  │           ┌───────────────┐  - 数据库集成
  ├───────────┤ database  [Di]│    * 创建, 读取, 更新, 删除 (CRUD) 操作
  │           │ interface     │    * 过滤和更新定义接口
  │           └─────┬─────────┘    * 迁移和索引
  │                 │
  │                 ├───────── ... 可拓展至任意NoSql (CouchDB / MongoDB etc.)
  │           ┌─────┴─────────┐
  │           │ sqlcommon     │
  │           └─────┬─────────┘
  │                 ├───────────────────────┬───────── ... 可拓展至其他SQL数据库
  │           ┌─────┴─────────┐     ┌───────┴────────┐
  │           │ postgres      │     │ sqlite3        │
  │           └───────────────┘     └────────────────┘
  │
  │           ┌───────────────┐  - 将核心事件引擎连接至外部框架和应用
  ├───────────┤ event     [Ei]│    * 支持长周期 (可持续的) 和短暂的事件订阅
  │           │ interface     │    * 批量、过滤，所有的传输都会在核心区域进行处理
  │           └─────┬─────────┘    * 接口支持连接输入 (websocket) 和连接输出 (代理运行环境插件) 插件
  │                 │
  │                 ├───────────────────────┬──────────   ... 可拓展至其他的事件总线 (Kafka, NATS, AMQP etc.)
  │           ┌─────┴─────────┐     ┌───────┴────────┐
  │           │ websockets    │     │ webhooks       │
  │           └───────────────┘     └────────────────┘
  │  ... 更多待定

  额外的工具类框架
              ┌───────────────┐  - REST API 客户端
              │ rest      [Re]│    * 提供便利性和日志
              │ client        │    * 标准的认证, 配置和重试逻辑
              └───────────────┘    * 构建和重试

              ┌───────────────┐  - WebSocket 客户端
              │ wsclient  [Wc]│    * 提供便利性和日志
              │               │    * 标准化认证，配置和重连逻辑
              └───────────────┘    * 基于 Gorilla WebSockets

              ┌───────────────┐  - 翻译框架
              │ i18n      [In]│    * 所有翻译内容必须要添加到 `en_translations.json` - 用 `FF10101` 作为 key
              │               │    * 错误会被打包, `error` 包内提供了额外的特性 (堆栈等.)
              └───────────────┘    * 也支持描述翻译, 例如 OpenAPI 描述

              ┌───────────────┐  - 日志框架
              │ log       [Lo]│    * 日志框架 (logrus) 集成了上下文标签
              │               │    * 上下文贯穿整个代码，用于传递API调用上下文以及日志上下文
              └───────────────┘    * 样例: 所有的API调用都有着可追溯ID, 以及时长

              ┌───────────────┐  - 配置
              │ config    [Co]│    * 基于日志框架的文件和环境变量 (viper)
              │               │    * 主要配置key全部集中定义
              └───────────────┘    * 插件通过返回其配置结构进行集成 (JSON 标签)

```
