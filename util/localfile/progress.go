// Package localfile 本地文件存储，用于进度持久化。
// 原子写入：先写临时文件，再 rename，最后 sync 父目录。
// 不引入 Redis/etcd/MySQL 等外部依赖。
package localfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// FileProgress 基于本地文件的进度存储，线程安全。
//
// 文件内容为纯文本 int64 数字，便于人工查看和调试。
// 写入流程: 写 .tmp → rename → sync dir，保证原子性和持久性。
type FileProgress struct {
	mu   sync.Mutex
	path string
}

// NewFileProgress 创建 FileProgress，默认值为 -1（未开始）。
// path 为进度文件的绝对路径，父目录不存在时自动创建。
func NewFileProgress(path string) (*FileProgress, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("localfile: create dir %s: %v", dir, err)
	}
	return &FileProgress{path: path}, nil
}

// Load 读取进度值。文件不存在时返回 -1。
func (p *FileProgress) Load() (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return 0, fmt.Errorf("localfile: read %s: %v", p.path, err)
	}
	if len(data) == 0 {
		return -1, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("localfile: parse %s: %v", p.path, err)
	}
	return n, nil
}

// Save 原子写入进度值。
func (p *FileProgress) Save(n int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tmp := p.path + ".tmp"
	content := strconv.FormatInt(n, 10)

	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("localfile: write tmp %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, p.path); err != nil {
		return fmt.Errorf("localfile: rename %s -> %s: %v", tmp, p.path, err)
	}

	// fsync 父目录，确保 rename 真正落盘（ext4/xfs 等需要）
	dir, err := os.Open(filepath.Dir(p.path))
	if err != nil {
		return fmt.Errorf("localfile: open dir: %v", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("localfile: sync dir: %v", err)
	}
	return nil
}

// Path 返回进度文件的绝对路径。
func (p *FileProgress) Path() string {
	return p.path
}
