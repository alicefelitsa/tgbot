package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"tgbot/config"
)

// ==================== 图片库预览的本地磁盘缓存 ====================
//
// 图片库只存 file_id,预览时要现去 Telegram 文件服务器下载,列表每次刷新都全量重拉既慢又耗带宽。
// 这里按 file_id 把下载到的原图字节缓存到本地磁盘——file_id 内容永久不变,天然无「更新失效」问题;
// 命中即直接返回、不再打 Telegram。删图时同步 evict,启动时清扫「库里已无」的孤儿缓存防磁盘泄漏。

// imageCacheDir 缓存目录:优先配置 telegram.imageCacheDir,默认 ./data/imgcache。
func imageCacheDir() string {
	if d := config.Conf.GetString("telegram.imageCacheDir"); d != "" {
		return d
	}
	return filepath.Join("data", "imgcache")
}

// imageCacheName file_id → 缓存文件名(sha256 十六进制,规避 file_id 的非法字符/超长)。
func imageCacheName(fileID string) string {
	sum := sha256.Sum256([]byte(fileID))
	return hex.EncodeToString(sum[:])
}

// imageCachePath file_id → 缓存文件绝对/相对路径。
func imageCachePath(fileID string) string {
	return filepath.Join(imageCacheDir(), imageCacheName(fileID))
}

// imageCacheGet 读缓存;不存在或读失败返回 (nil,false) 视为未命中。
func imageCacheGet(fileID string) ([]byte, bool) {
	b, err := os.ReadFile(imageCachePath(fileID))
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// imageCachePut 写缓存(尽力而为:失败只记日志、不影响主流程,下次仍走网络重取)。
func imageCachePut(fileID string, data []byte) {
	if len(data) == 0 {
		return
	}
	p := imageCachePath(fileID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		config.LogWarning("imgcache mkdir: %v", err)
		return
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		config.LogWarning("imgcache write: %v", err)
	}
}

// imageCacheEvict 删除给定 file_id 的缓存文件(删图时调用;文件不存在则忽略)。
func imageCacheEvict(fileIDs []string) {
	for _, id := range fileIDs {
		if id == "" {
			continue
		}
		_ = os.Remove(imageCachePath(id))
	}
}

// imageCacheSweep 启动清扫:删掉「tg_image 里已无对应 file_id」的孤儿缓存文件,防磁盘无限累积。
// 缓存文件名是 file_id 的 sha256、无法反推,故先把库中所有 file_id 算成哈希集合,再遍历目录逐文件比对。
func imageCacheSweep(db *gorm.DB) {
	dir := imageCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // 目录尚不存在,无需清扫
	}
	var fileIDs []string
	if err := db.Table("tg_image").Pluck("tg_file_id", &fileIDs).Error; err != nil {
		config.LogWarning("imgcache sweep query: %v", err)
		return
	}
	live := make(map[string]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		live[imageCacheName(id)] = struct{}{}
	}
	var removed int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := live[e.Name()]; !ok {
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		config.LogInfo("imgcache 启动清扫:删除孤儿缓存 %d 个", removed)
	}
}
