package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func TestArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"https://example.com/file"},
		{"magnet:?dn=no-hash"},
		{"magnet:?xt=urn:btih:bad"},
		{"magnet:?xt=urn:btih:0123456789012345678901234567890123456789", "extra"},
	} {
		if err := run(context.Background(), args, io.Discard); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
	if err := run(context.Background(), []string{"-h"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func localConfig(dir string) *torrent.ClientConfig {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = dir
	cfg.ListenHost = func(string) string { return "127.0.0.1" }
	cfg.ListenPort = 0
	cfg.NoDHT = true
	cfg.DisableTrackers = true
	cfg.DisablePEX = true
	cfg.DisableUTP = true
	cfg.DisableIPv6 = true
	cfg.DisableWebtorrent = true
	cfg.NoDefaultPortForwarding = true
	return cfg
}

// Exercise real metadata exchange, piece verification and file storage without
// depending on public trackers or downloading third-party content.
func TestMagnetDownload(t *testing.T) {
	seedDir, destDir := t.TempDir(), t.TempDir()
	payload := bytes.Repeat([]byte("magnet local transfer test\n"), 4096)
	source := filepath.Join(seedDir, "sample.txt")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(source); err != nil {
		t.Fatal(err)
	}
	encoded, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: encoded}
	cfg := localConfig(seedDir)
	cfg.Seed = true
	seed, err := torrent.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Close()
	seedTorrent, err := seed.AddTorrent(&mi)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	select {
	case <-seedTorrent.Complete().On():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	m := mi.Magnet(nil, &info)
	m.Params = url.Values{"x.pe": {seed.ListenAddrs()[0].String()}}
	if err := download(ctx, m.String(), localConfig(destDir), io.Discard); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "sample.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded content differs from source")
	}
	// Existing data must complete successfully on a second run as well.
	if err := download(ctx, m.String(), localConfig(destDir), io.Discard); err != nil {
		t.Fatalf("reuse existing download: %v", err)
	}
}

func TestCancelWhileWaitingForMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := download(ctx, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789", localConfig(t.TempDir()), io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancellation while waiting for metadata, got %v", err)
	}
}
