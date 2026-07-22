# 批量个股异动 SSE

生产服务通过 `NewRootHandler` 统一提供以下地址：

- `/`：跳转到 `/web`
- `/web`：Web Viewer 页面
- `/web/api/methods`、`/web/api/query`：Web Viewer 接口
- `/api/health`：HTTP 服务存活检查
- `/api/stock/unusual/sse`：批量个股异动 SSE

旧的 `/api/methods`、`/api/query` 不再对外提供。

`routes` 包提供一个独立的批量个股异动 SSE 接口：

```text
GET /api/stock/unusual/sse?stocks=000001,600000
```

它每 2 秒轮询一次通达信 `MACMarketMonitor`，按请求中的股票代码过滤新异动，再通过 SSE 逐条推送。第一次轮询会自动定位到当天异动列表的最新位置，只建立基线，不会补发连接前已经存在的异动。

## 在 Go 服务中注册

```go
package main

import (
	"log"
	"net/http"

	gotdx "github.com/bensema/gotdx"
	"github.com/bensema/gotdx/routes"
)

func main() {
	client := gotdx.NewMAC()
	mux := http.NewServeMux()
	routes.RegisterStockUnusualSSE(mux, client)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

这几行 Go 代码可以这样理解：

- `client := gotdx.NewMAC()` 创建一个通达信 MAC 行情客户端。
- `mux := http.NewServeMux()` 创建网址分发器。
- `routes.RegisterStockUnusualSSE(mux, client)` 把 SSE 路由安装到分发器上。
- `http.ListenAndServe(":8080", mux)` 在本机 8080 端口启动 HTTP 服务。
- `:=` 是 Go 的短变量声明语法，表示“创建变量并把右边的值交给它”。

`RegisterStockUnusualSSE` 接收的是一个小接口，只要求对象具有 `MACMarketMonitor` 方法。因此正式环境可以传 `*gotdx.Client`，测试则可以传不联网的模拟对象。

## 修改服务端间隔

默认轮询间隔为 2 秒，心跳间隔为 1 分钟。普通使用不需要填写配置；需要调整时可以追加可选参数：

```go
routes.RegisterStockUnusualSSE(
	mux,
	client,
	routes.WithPollInterval(3*time.Second),
	routes.WithHeartbeatInterval(time.Minute),
)
```

Go 中的 `...Option` 表示“后面可以继续传零个或多个可选设置”。不传设置时就使用默认值。

## 浏览器调用

```js
const source = new EventSource(
  "/api/stock/unusual/sse?stocks=000001,600000",
);

source.addEventListener("unusual", (event) => {
  const unusual = JSON.parse(event.data);
  console.log("收到异动", unusual);
});

source.addEventListener("error", (event) => {
  if (event.data) {
    console.error("行情轮询错误", JSON.parse(event.data));
  }
});
```

一条正常事件的数据如下：

```json
{
  "code": "000001",
  "name": "平安银行",
  "time": "09:45:12",
  "desc": "加速拉升",
  "value": "1.25%",
  "type": 4,
  "raw": {
    "v1": 0,
    "v2": 0.0125,
    "v3": 0,
    "v4": 0
  }
}
```

`raw.v1`～`raw.v4` 是通达信协议原始参数，其含义随 `type` 变化；日常展示优先使用 `desc` 和 `value`。

## 参数规则

- `stocks` 必填，使用英文逗号分隔。
- 股票代码必须是可识别市场的 6 位 A 股代码。
- 重复代码会自动去重。
- 单个连接最多订阅 100 只股票。
- 参数中只要有一个非法代码，整个请求就返回 `400 Bad Request`。

## 运行行为

- 同一市场的所有 SSE 客户端共享一次轮询，不会按连接数量重复请求通达信。
- 第一次成功轮询通过分页探测定位当天列表尾部，第二次开始只从尾部游标读取新增异动。
- 单次新增达到 600 条时会连续读取后续分页，直到追上最新位置。
- 正式客户端每次查询后都会主动断开 MAC 短连接，下一轮自动建立新连接。
- 单次 MAC 查询失败时会使用新连接重试，连续 3 次失败后才向 SSE 客户端发送 `error` 事件。
- 去重记录按北京时间每天零点重置。
- 每条异动单独发送一个 `unusual` 事件。
- 每 1 分钟发送一次 `: ping` 注释心跳。
- 轮询错误通过 `error` 事件通知，连接保持打开并继续重试。
- 不提供 SSE `id` 和断线历史重放。
- 客户端长期不读取且积压超过 128 条事件时，服务端会关闭该连接。

## CORS 和鉴权

这个 Handler 不自行处理跨域和鉴权。若前端与接口位于不同子域，应在外层 HTTP 服务中配置“只允许相同主域名”的 CORS 规则；登录、Token 或 API Key 也应由外层中间件统一校验。
