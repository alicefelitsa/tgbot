package controller

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"tgbot/config"
	"tgbot/tools"
)

// ==================== 图片库 tg_image ====================
//
// 提前把图片上传给 Telegram 换取可复用的 file_id 存库;之后各处配图字段填 "file:<file_id>",
// 发送端(send/editMessage)识别该前缀后直接用 tg.FileID 秒发,不再依赖后端下载或 TG 抓 URL,
// 从根本上规避「国内 CDN 图 Telegram 拉不到」的问题。入库靠把图发到配置的中转 chat、读回 file_id、删消息。

// tgFileClient 下载 Telegram 文件服务器(api.telegram.org/file)上的图片用于后台预览:
// 走系统代理(与 bot 主客户端一致,境内需代理才能访问 Telegram),带连接超时。
var tgFileClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	},
}

// GetTgImageList 图片库列表(按名称模糊、分类精确过滤,支持分页)
func (r *TgBotRuntime) GetTgImageList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if kw := c.Query("name"); kw != "" {
		conds += fmt.Sprintf(" and name like '%%%v%%'", kw)
	}
	if tag := c.Query("tag"); tag != "" {
		conds += fmt.Sprintf(" and tag = '%v'", tag)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := r.db.Raw("select id,name,tag,tg_file_id,mime,size,source_url,status,created_at,updated_at" +
		" from tg_image" + where + " order by id desc" + pageLimit(c)).Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = r.db.Raw("select count(id) from tg_image" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// AddTgImage 图片入库:接受本地 base64(image_base64)或 URL(url)二选一,
// 后端拿到字节后发到中转 chat 换取 Telegram file_id,存库。
func (r *TgBotRuntime) AddTgImage(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	getStr := func(k string) string {
		if v, ok := raw[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		return ""
	}
	name := getStr("name")
	tag := getStr("tag")
	b64 := getStr("image_base64")
	srcURL := getStr("url")

	var data []byte
	var fileName string
	switch {
	case b64 != "":
		// 去掉可能的 data:image/...;base64, 前缀
		if i := strings.Index(b64, ","); i >= 0 && strings.HasPrefix(b64, "data:") {
			b64 = b64[i+1:]
		}
		dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "message": "base64 解码失败:" + err.Error()})
			return
		}
		data = dec
		fileName = name
		if fileName == "" {
			fileName = "photo.jpg"
		}
	case srcURL != "":
		if !validTgURL(srcURL) {
			c.JSON(http.StatusOK, gin.H{"code": 400, "message": "图片 URL 非法(需 http/https)"})
			return
		}
		d, n, err := r.downloadImage(srcURL)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "message": "下载图片失败:" + err.Error()})
			return
		}
		data, fileName = d, n
	default:
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误:请提供本地图片或图片 URL"})
		return
	}
	if len(data) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "图片内容为空"})
		return
	}
	if len(data) > maxPhotoBytes {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": fmt.Sprintf("图片超过 %dMB,不适合按照片入库", maxPhotoBytes>>20)})
		return
	}
	fileID, err := r.uploadToRelay(data, fileName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "换取 file_id 失败:" + err.Error()})
		return
	}
	if name == "" {
		name = fileName
	}
	mime := http.DetectContentType(data)
	row := map[string]interface{}{
		"name":       name,
		"tag":        tag,
		"tg_file_id": fileID,
		"mime":       mime,
		"size":       len(data),
		"source_url": srcURL,
		"status":     1,
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}
	if err := r.db.Table("tg_image").Create(row).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "入库成功", "data": gin.H{"tg_file_id": fileID}})
}

// SaveTgImage 修改图片库记录(仅名称/分类/状态,图片本体不变)
func (r *TgBotRuntime) SaveTgImage(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, []string{"name", "tag", "status"})
	data["updated_at"] = time.Now()
	if err := r.db.Table("tg_image").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgImage 删除图片库记录(逗号 ids)。只删本地记录,TG 上的 file 无需清理(会随长期不用自动失效);
// 但需同步 evict 本地磁盘缓存,否则缓存文件会因不再有库行引用而永久残留。
func (r *TgBotRuntime) DelTgImage(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	// 删库前先拿到 file_id,才能定位并删除对应缓存文件
	var fileIDs []string
	r.db.Table("tg_image").Where("id in (?)", idList).Pluck("tg_file_id", &fileIDs)
	if err := r.db.Exec("delete from tg_image where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	imageCacheEvict(fileIDs)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// GetTgImagePreview 后台预览:按 id 取 file_id,用 bot getFile 换文件路径再下载,以 base64 返回。
// bot 只下载「自己发出去过」的文件,入库时经中转 chat 发的图正好满足此前提。
func (r *TgBotRuntime) GetTgImagePreview(c *gin.Context) {
	id := c.Query("id")
	var rows []map[string]interface{}
	if err := r.db.Raw("select tg_file_id,mime from tg_image where id = ?", id).Scan(&rows).Error; err != nil || len(rows) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "图片不存在"})
		return
	}
	fileID := fmt.Sprintf("%v", rows[0]["tg_file_id"])
	mime := fmt.Sprintf("%v", rows[0]["mime"])
	// 命中本地磁盘缓存直接返回,不再打 Telegram(无需 bot 在线)
	if cached, ok := imageCacheGet(fileID); ok {
		if mime == "" || mime == "<nil>" {
			mime = http.DetectContentType(cached)
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功",
			"data": gin.H{"mime": mime, "base64": base64.StdEncoding.EncodeToString(cached)}})
		return
	}
	if r.bot == nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "bot 未初始化,无法预览"})
		return
	}
	tf, err := r.bot.GetFile(tg.FileConfig{FileID: fileID})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "getFile 失败:" + err.Error()})
		return
	}
	token := config.Conf.GetString("telegram.botToken")
	fileURL := "https://api.telegram.org/file/bot" + token + "/" + tf.FilePath
	resp, err := tgFileClient.Get(fileURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "下载预览图失败:" + err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": fmt.Sprintf("下载预览图 HTTP %d", resp.StatusCode)})
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if mime == "" || mime == "<nil>" {
		mime = http.DetectContentType(body)
	}
	imageCachePut(fileID, body) // 回填本地缓存,下次刷新列表直接命中
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功",
		"data": gin.H{"mime": mime, "base64": base64.StdEncoding.EncodeToString(body)}})
}

// uploadToRelay 把图片字节发到中转 chat、读回 Telegram file_id、删除该中转消息。
// file_id 与当前 bot token 绑定,删除消息后仍长期有效,可供各处配图字段复用。
func (r *TgBotRuntime) uploadToRelay(data []byte, name string) (string, error) {
	// 中转 chat:优先读数据库系统设置(imageRelayChatID,后台改完即生效),无值则回落 config.yaml
	relay := settingInt64(r.db, "imageRelayChatID", "telegram.imageRelayChatID")
	if relay == 0 {
		return "", fmt.Errorf("未配置图片库中转 ChatID(请到「系统设置」填 imageRelayChatID)")
	}
	if r.bot == nil {
		return "", fmt.Errorf("bot 未初始化(检查 botToken)")
	}
	photo := tg.NewPhoto(relay, tg.FileBytes{Name: name, Bytes: data})
	sent, err := r.bot.Send(photo)
	if err != nil {
		return "", err
	}
	// 取最大尺寸档的 file_id(Telegram 会生成多档缩略,原图档最靠后)
	if len(sent.Photo) == 0 {
		r.deleteMessage(relay, sent.MessageID)
		return "", fmt.Errorf("Telegram 未返回图片 file_id")
	}
	fileID := sent.Photo[len(sent.Photo)-1].FileID
	r.deleteMessage(relay, sent.MessageID) // 拿完即删中转消息,file_id 仍有效
	if fileID == "" {
		return "", fmt.Errorf("Telegram 返回的 file_id 为空")
	}
	return fileID, nil
}
