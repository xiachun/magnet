package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "\n下载已停止，已下载的数据保留在目标目录。")
		os.Exit(130)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("magnet", flag.ContinueOnError)
	fs.SetOutput(out)
	dir := fs.String("dir", ".", "下载保存目录")
	fs.Usage = func() {
		fmt.Fprintln(out, "用法: magnet [-dir 目录] 'magnet:?xt=urn:btih:...'\n选项必须放在磁力链接之前，链接请用引号包裹。")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("请提供一个磁力链接")
	}
	uri := strings.TrimSpace(fs.Arg(0))
	m, err := metainfo.ParseMagnetV2Uri(uri)
	if err != nil {
		return fmt.Errorf("无效的磁力链接: %w", err)
	}
	if !m.InfoHash.Ok && !m.V2InfoHash.Ok {
		return errors.New("磁力链接缺少有效的 BitTorrent info hash")
	}
	if strings.TrimSpace(*dir) == "" {
		return errors.New("下载目录不能为空")
	}
	path, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("解析下载目录: %w", err)
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("创建下载目录: %w", err)
	}
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = path
	cfg.ListenPort = 0
	return download(ctx, uri, cfg, out)
}

func download(ctx context.Context, uri string, cfg *torrent.ClientConfig, out io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("启动下载客户端: %w", err)
	}
	defer client.Close()
	t, err := client.AddMagnet(uri)
	if err != nil {
		return fmt.Errorf("添加磁力链接: %w", err)
	}
	writeErrors := make(chan error, 1)
	t.SetOnWriteChunkError(func(err error) {
		select {
		case writeErrors <- err:
		default:
		}
	})
	fmt.Fprintln(out, "正在寻找节点并获取文件信息（Ctrl+C 停止）...")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	started := time.Now()
metadata:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.GotInfo():
			break metadata
		case <-ticker.C:
			fmt.Fprintf(out, "\r等待文件信息... %ds | 已连接节点: %d   ", int(time.Since(started).Seconds()), t.Stats().ActivePeers)
		}
	}
	fmt.Fprintf(out, "\n文件: %q\n大小: %s\n目录: %s\n", t.Name(), formatBytes(t.Length()), cfg.DataDir)
	t.DownloadAll()
	lastTime := time.Now()
	initialStats := t.Stats()
	lastBytes := initialStats.BytesReadUsefulData.Int64()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-writeErrors:
			return fmt.Errorf("写入下载文件: %w", err)
		case <-t.Complete().On():
			fmt.Fprintf(out, "\r下载完成: %s / %s (100%%)                       \n", formatBytes(t.Length()), formatBytes(t.Length()))
			return nil
		case now := <-ticker.C:
			stats := t.Stats()
			current := stats.BytesReadUsefulData.Int64()
			speed := float64(current-lastBytes) / now.Sub(lastTime).Seconds()
			done, total := t.BytesCompleted(), t.Length()
			percent := float64(0)
			if total > 0 {
				percent = float64(done) / float64(total) * 100
			}
			fmt.Fprintf(out, "\r%.1f%% | %s / %s | %s/s | 节点: %d       ", percent, formatBytes(done), formatBytes(total), formatBytes(int64(speed)), stats.ActivePeers)
			lastTime, lastBytes = now, current
		}
	}
}

func formatBytes(n int64) string {
	value := float64(n)
	units := [...]string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}
