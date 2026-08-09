# Traefik x402 Middleware

这是一个轻量的 Traefik Middleware，可以在不修改原有 HTTP 应用的情况下，
为路由增加 x402 v2 `exact` 支付保护。

它负责返回 `402`、调用 Facilitator 的 `/verify` 和 `/settle`、转发真实上游，
并在结算成功后返回业务内容和 `PAYMENT-RESPONSE`。Traefik 不保存钱包私钥。

> 这是非官方社区项目，与 Traefik Labs、Coinbase 或 x402 Foundation
> 不存在隶属或官方背书关系。

## 功能边界

- 支持 x402 v2。
- 只支持固定价格 `exact`。
- v0.1.0 只支持 EVM（`eip155:*`）网络族。
- 每次成功请求通常支付一次。
- 默认只允许 `GET`、`HEAD`；写操作必须显式加入 `allowedMethods`。
- 同一个插件实例会拒绝相同付款载荷的并发重放。
- 结算成功的响应会强制禁止浏览器和共享代理缓存，避免绕过付款。
- 上游返回 HTTP 400 及以上时不结算。
- 默认删除付款签名，并向上游注入可信 `X-X402-Payer`。
- 不支持 SSE、WebSocket、无限流式响应和大文件下载。
- 不支持 `upto`、批量结算、订阅或永久访问。

如果确实需要保护写接口，可以显式配置：

```yaml
allowedMethods:
  - GET
  - HEAD
  - POST
```

插件采用 `verify -> upstream -> settle`，因此写接口还必须实现幂等。并发防重键
来自 EVM 已签名授权身份，但只覆盖单个插件实例；多副本和
顺序重放仍依赖 Facilitator 的保证及业务幂等。

## 五分钟 Mock 验证

只需要 Docker，不需要钱包，也不会访问区块链：

```bash
docker compose -f e2e/docker-compose.yml up \
  --build --abort-on-container-exit --exit-code-from client
docker compose -f e2e/docker-compose.yml down --volumes
```

成功时会看到：

```text
PASS unpaid request: 402 + PAYMENT-REQUIRED
PASS paid request: verify -> upstream -> settle -> 200
PASS PAYMENT-RESPONSE returned to the client
```

Mock 只验证 Traefik 插件链路，不验证真实钱包签名和链上结算。

## Catalog 安装

Traefik 静态配置：

```yaml
experimental:
  plugins:
    x402:
      moduleName: github.com/wyh0626/traefik-x402
      version: v0.1.0
```

付费路由的完整动态配置请看 [英文 README](README.md#install-from-the-traefik-plugin-catalog)
和 [安装文档](docs/installation.md)。修改静态插件配置后必须重启 Traefik。

## Base Sepolia 真实付款

先准备专用测试钱包、Base Sepolia ETH 和 Circle 测试 USDC，然后执行：

```bash
cd e2e/real
PAY_TO=0x你的收款钱包 ./run.sh
```

另开一个终端：

```bash
cd e2e/real
./pay.sh
```

脚本会先显示网络、资产、数量和收款地址，只有输入 `PAY` 后才读取测试钱包私钥并付款。
每成功运行一次都会再次支付一次测试币。完整说明见
[Base Sepolia 测试文档](e2e/real/README.md)。

## 开发验证

```bash
go vet ./...
go test -race ./...
```

本项目使用 Apache-2.0 License，详见 [LICENSE](LICENSE)。
