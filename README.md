# magnet

一个简单的 Go 命令行磁力下载工具，使用 [anacrolix/torrent](https://github.com/anacrolix/torrent) 处理 BitTorrent 协议。

## 构建与使用

需要 Go 1.25.4 或更新版本，首次构建需要联网下载依赖。

```zsh
go build -o magnet .
./magnet -dir ./downloads 'magnet:?xt=urn:btih:替换为实际的哈希值'
```

将示例中的整条磁力链接替换为实际链接。链接必须用引号包裹，避免 zsh 解释 `?`、`&` 等字符。`-dir` 必须放在链接之前，省略时保存到当前目录。

也可以直接运行：

```zsh
go run . -dir ./downloads '你的完整磁力链接'
./magnet -h
```

每次下载一个磁力链接中的所有文件，显示元数据等待状态、下载进度、速度和连接节点数，完成后自动退出。按 Ctrl+C 停止并保留已下载数据；重新使用同一链接和目录运行，会校验已有数据并继续下载。下载过程中会按 BitTorrent 协议向其他节点上传数据。

通过链接中的 Tracker、DHT 和 PEX 发现节点。获取文件信息和实际下载均依赖可用节点及网络连通性；无可用节点时会持续等待，可按 Ctrl+C 退出。

## 验证

```zsh
go test ./...
go vet ./...
```
