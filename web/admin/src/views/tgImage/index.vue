<template>
  <div class="content">
    <el-card shadow="always">
      <div slot="header">
        <!--搜索栏-->
        <div class="queryForm">
          <el-form :inline="true" :model="where" class="query-form-inline" size="small">
            <el-form-item label="名称">
              <el-input v-model="where.name" placeholder="模糊搜索" clearable class="queryElInput"></el-input>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="search">查询</el-button>
              <el-button icon="el-icon-refresh" @click="reset">重置</el-button>
            </el-form-item>
          </el-form>
        </div>
      </div>

      <!--工具栏-->
      <div class="toolbar">
        <el-button type="primary" size="small" icon="el-icon-upload2" @click="openAdd">添加图片</el-button>
        <el-button type="danger" size="small" icon="el-icon-delete" @click="del">删除</el-button>
        <el-button size="small" icon="el-icon-refresh" @click="getList">刷新</el-button>
        <span style="margin-left:12px;color:#888;font-size:13px;">共 {{ total }} 张；各处配图字段选「从图片库选择」即可复用这些图（秒发、不受国内图链限制）。</span>
      </div>

      <!--数据表格-->
      <el-table ref="table" class="tableData" :data="tableData" height="calc(100vh - 182px)"
                :border="true" v-loading="loading" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="55" align="center"></el-table-column>
        <el-table-column prop="id" label="ID" width="70px" align="center"></el-table-column>
        <el-table-column label="缩略图" width="110px" align="center">
          <template v-slot="{row}">
            <el-image v-if="row._thumb" :src="row._thumb" :preview-src-list="[row._thumb]"
                      style="width:72px;height:72px;border-radius:4px;" fit="cover"></el-image>
            <div v-else class="thumb-ph"><i class="el-icon-picture-outline"></i></div>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="150px" show-overflow-tooltip></el-table-column>
        <el-table-column label="大小" width="100px" align="center">
          <template v-slot="{row}">{{ fmtSize(row.size) }}</template>
        </el-table-column>
        <el-table-column label="引用值(file:)" min-width="180px">
          <template v-slot="{row}">
            <el-tooltip effect="light" placement="top">
              <div slot="content" style="max-width:520px;word-break:break-all;line-height:1.6;">file:{{ row.tg_file_id }}</div>
              <code style="font-size:12px;">file:{{ short(row.tg_file_id) }}</code>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="80px" align="center">
          <template v-slot="{row}">
            <el-switch :value="row.status === 1" active-color="#13ce66" @change="toggleStatus(row)"></el-switch>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="入库时间" width="160px" align="center"></el-table-column>
        <el-table-column label="操作" align="center" width="180px">
          <template v-slot="{row}">
            <el-button size="mini" @click="edit(row)">编辑</el-button>
            <el-button size="mini" @click="copyRef(row)">复制引用</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!--分页-->
      <div style="margin-top: 10px; text-align: center;" class="currentPage">
        <el-pagination
          background
          layout="total, sizes, prev, pager, next, jumper"
          :current-page="page"
          :page-size="limit"
          :page-sizes="[20, 50, 100, 200]"
          :total="total"
          @size-change="onSizeChange"
          @current-change="onPageChange">
        </el-pagination>
      </div>
    </el-card>

    <!--添加图片(本地上传 / URL 导入,均支持多图);入库中锁死弹窗:禁关闭X/ESC,禁点遮罩关闭-->
    <el-dialog title="添加图片到图片库" :visible.sync="addVisible" width="720px" top="8vh"
               class="responsive-dialog add-dialog" :close-on-click-modal="false"
               :show-close="!adding" :close-on-press-escape="!adding">
      <!--入库中盖一层 loading 遮罩,阻断对 tab/上传区/缩略图的交互,并显示 i/N 进度-->
      <div class="add-body" v-loading="adding" :element-loading-text="addTotal ? ('正在入库 ' + addDone + '/' + addTotal + '，请稍候…') : '正在入库，请稍候…'">
      <el-tabs v-model="addTab" class="add-tabs">
        <el-tab-pane label="本地上传" name="local">
          <el-upload class="uploader"
            drag multiple :auto-upload="false" :show-file-list="false"
            accept="image/*" :on-change="onLocalFile">
            <i class="el-icon-upload"></i>
            <div class="up-text">把图片拖到此处，或<em>点击选择</em>（可多选）</div>
            <div class="up-hint">支持 JPG / PNG / GIF / WebP；上传前自动压缩到 3MB 以内（优先降尺寸、保守压质量，保持清晰度）</div>
          </el-upload>
          <div v-if="localFiles.length" class="up-multi">
            <div class="up-multi-head">
              <span>已选 {{ localFiles.length }} 张（已自动重命名为简短名称，入库后可在列表逐条改名）</span>
              <el-button type="text" size="mini" @click="localFiles = []">清空</el-button>
            </div>
            <div class="up-multi-grid">
              <div v-for="(f, i) in localFiles" :key="i" class="up-mini">
                <el-image :src="f.base64" fit="cover" style="width:100%;height:88px;box-sizing:border-box;border-radius:6px;border:1px solid #ebeef5;display:block;"></el-image>
                <span class="up-mini-del" @click="localFiles.splice(i, 1)"><i class="el-icon-close"></i></span>
                <div class="up-mini-name" :title="f.name">{{ f.name }}</div>
              </div>
            </div>
          </div>
        </el-tab-pane>
        <el-tab-pane label="URL 导入" name="url">
          <el-input v-model="addForm.url" type="textarea" :rows="5" placeholder="每行一个图片直链 https://...，后端会逐个下载并换取 file_id"></el-input>
          <div class="url-hint">导入后由服务器下载并转存到 Telegram，得到可复用的 file_id（国内图链也能用）；可一次粘贴多行，每行一个。</div>
        </el-tab-pane>
      </el-tabs>
      </div>
      <span slot="footer" class="dialog-footer">
        <el-button @click="addVisible=false" size="small" :disabled="adding">取 消</el-button>
        <el-button type="primary" :loading="adding" @click="doAdd" size="small">确定入库</el-button>
      </span>
    </el-dialog>

    <!--编辑记录(名称)-->
    <el-dialog title="编辑图片" :visible.sync="editVisible" width="460px" top="12vh"
               class="responsive-dialog" :close-on-click-modal="false">
      <el-form :model="form" label-width="70px">
        <el-form-item label="名称">
          <el-input v-model="form.name" clearable></el-input>
        </el-form-item>
      </el-form>
      <span slot="footer" class="dialog-footer">
        <el-button @click="editVisible=false" size="small">取 消</el-button>
        <el-button type="primary" @click="save" size="small">确 定</el-button>
      </span>
    </el-dialog>
  </div>
</template>

<script>
import {getTgImageList, addTgImage, saveTgImage, delTgImage, getTgImagePreview} from "@/api/tgImage";

export default {
  name: "TgImage",
  data() {
    return {
      tableData: [],
      loading: false,
      where: {name: ''},
      page: 1,
      limit: Number(localStorage.getItem('pageSize_tgImage')) || 20,
      total: 0,
      multipleSelection: [],
      // 添加
      addVisible: false,
      addTab: 'local',
      adding: false,
      addTotal: 0,
      addDone: 0,
      addForm: {url: ''},
      localFiles: [],
      // 编辑
      editVisible: false,
      form: {id: '', name: ''},
    }
  },
  mounted() {
    this.getList()
  },
  methods: {
    async getList() {
      this.loading = true
      try {
        const params = {name: this.where.name, page: this.page, limit: this.limit}
        const res = await getTgImageList(params)
        if (res.data.code === 0) {
          this.tableData = (res.data.data || []).map(r => ({...r, _thumb: ''}))
          this.total = res.data.count || 0
          this.loadThumbs()
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      } finally {
        this.loading = false
      }
    },
    // 逐行拉预览图(后端用 file_id 通过 Telegram getFile 下载转 base64)
    // 注意:_thumb 在 getList 建表时已初始化为 ''(响应式),这里直接改属性即可。
    // 切勿用 $set 替换整行对象——那会让 el-table 把行当新行重渲染并触发 doLayout 重算滚动条,
    // 上百个预览响应陆续回来 = 上百次重排 = 刷新时页面/滚动条上下抖动。占位块与真图同为 72x72,原地改不改变行高。
    loadThumbs() {
      const gen = this._thumbGen = (this._thumbGen || 0) + 1 // 代次标记:getList 重跑后丢弃上一批在途响应,避免写错行
      this.tableData.forEach((row, idx) => {
        getTgImagePreview(row.id).then(res => {
          if (gen !== this._thumbGen) return
          if (res.data.code === 0 && res.data.data) {
            const {mime, base64} = res.data.data
            const target = this.tableData[idx]
            if (target && target.id === row.id) target._thumb = `data:${mime};base64,${base64}`
          }
        }).catch(() => {})
      })
    },
    handleSelectionChange(val) {
      this.multipleSelection = val.map(item => item.id)
    },
    search() {
      this.page = 1
      this.getList()
    },
    reset() {
      this.where = {name: ''}
      this.page = 1
      this.getList()
    },
    onPageChange(p) {
      this.page = p
      this.getList()
    },
    onSizeChange(size) {
      this.limit = size
      localStorage.setItem('pageSize_tgImage', size)
      this.page = 1
      this.getList()
    },
    fmtSize(n) {
      n = Number(n) || 0
      if (n < 1024) return n + ' B'
      if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
      return (n / 1024 / 1024).toFixed(2) + ' MB'
    },
    short(id) {
      id = String(id || '')
      return id.length > 44 ? id.slice(0, 40) + '…' : id
    },
    copyRef(row) {
      this.doCopy('file:' + row.tg_file_id)
    },
    // 复制引用值:navigator.clipboard 仅在安全上下文(HTTPS/localhost)可用;HTTP 局域网访问会失效,回退 execCommand
    doCopy(text) {
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text)
          .then(() => this.$message.success('已复制引用值'))
          .catch(() => this.fallbackCopy(text))
      } else {
        this.fallbackCopy(text)
      }
    },
    fallbackCopy(text) {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.top = '-9999px'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      let ok = false
      try {
        ok = document.execCommand('copy')
      } catch (e) {
        ok = false
      }
      document.body.removeChild(ta)
      if (ok) {
        this.$message.success('已复制引用值')
      } else {
        this.$prompt('当前环境不支持自动复制，请手动复制以下引用值填入配图字段', '引用值', {inputValue: text})
      }
    },
    openAdd() {
      this.addTab = 'local'
      this.addForm = {url: ''}
      this.localFiles = []
      this.uploadSeq = 0
      this.addVisible = true
    },
    // 生成简短名称:丢弃本地文件原始的长哈希/UUID 名,用 img_月日_时分秒_序号.jpg
    genShortName() {
      const d = new Date()
      const p = n => String(n).padStart(2, '0')
      const ts = `${p(d.getMonth() + 1)}${p(d.getDate())}_${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
      this.uploadSeq = (this.uploadSeq || 0) + 1
      return `img_${ts}_${this.uploadSeq}.jpg`
    },
    // 本地文件(可多选)→ 前端压缩到 3MB 以内再存入 localFiles
    async onLocalFile(file) {
      const raw = file.raw
      if (!raw) return
      if (!/^image\//.test(raw.type)) {
        this.$message.warning(`${raw.name} 不是图片，已跳过`); return
      }
      if (raw.size > 20 * 1024 * 1024) {
        this.$message.warning(`${raw.name} 超过 20MB，已跳过`); return
      }
      try {
        const compressed = await this.compressImage(raw)
        if (this.dataUrlBytes(compressed) > 3 * 1024 * 1024) {
          this.$message.warning(`${raw.name} 压缩后仍超过 3MB，已跳过`); return
        }
        const name = this.genShortName()
        this.localFiles.push({name, base64: compressed})
      } catch (e) {
        this.$message.error(`${raw.name} 处理失败：${e.message}`)
      }
    },
    // 读文件为 dataURL
    readAsDataURL(file) {
      return new Promise((resolve, reject) => {
        const r = new FileReader()
        r.onload = () => resolve(r.result)
        r.onerror = reject
        r.readAsDataURL(file)
      })
    },
    loadImageEl(dataUrl) {
      return new Promise((resolve, reject) => {
        const im = new Image()
        im.onload = () => resolve(im)
        im.onerror = reject
        im.src = dataUrl
      })
    },
    // 估算 dataURL 的字节数(base64 部分 * 3/4)
    dataUrlBytes(dataUrl) {
      const i = dataUrl.indexOf(',')
      const b64 = i >= 0 ? dataUrl.length - i - 1 : dataUrl.length
      return Math.floor(b64 * 3 / 4)
    },
    // 压缩:最大边降到 maxSide 以内,质量从 0.9 保守递减;仍超限则优先降尺寸而非猛压质量,避免失真。
    // 统一输出 JPEG(Telegram sendPhoto 本就不保留透明通道)。
    async compressImage(file, maxBytes = 3 * 1024 * 1024, maxSide = 2048) {
      const src = await this.readAsDataURL(file)
      const img = await this.loadImageEl(src)
      const bw = img.naturalWidth || img.width
      const bh = img.naturalHeight || img.height
      if (!bw || !bh) return src
      let scale = Math.min(1, maxSide / Math.max(bw, bh))
      let quality = 0.9
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d')
      let out = src
      for (let i = 0; i < 16; i++) {
        const cw = Math.max(1, Math.round(bw * scale))
        const ch = Math.max(1, Math.round(bh * scale))
        canvas.width = cw
        canvas.height = ch
        ctx.fillStyle = '#ffffff'
        ctx.fillRect(0, 0, cw, ch)
        ctx.drawImage(img, 0, 0, cw, ch)
        out = canvas.toDataURL('image/jpeg', quality)
        if (this.dataUrlBytes(out) <= maxBytes) break
        if (quality > 0.8) quality -= 0.04        // 先小幅降质(不低于 0.8)
        else if (scale > 0.3) scale *= 0.85        // 再降尺寸(保清晰度优先)
        else break
      }
      return out
    },
    async doAdd() {
      if (this.adding) return // 入库中守卫:防重复提交
      // 汇总待入库项:本地多张 或 URL 多行(每行一个)
      let jobs = []
      if (this.addTab === 'local') {
        if (this.localFiles.length === 0) { this.$message.warning('请先选择本地图片'); return }
        jobs = this.localFiles.map(f => ({label: f.name, payload: {name: f.name, image_base64: f.base64}}))
      } else {
        const urls = this.addForm.url.split(/\r?\n/).map(s => s.trim()).filter(Boolean)
        if (urls.length === 0) { this.$message.warning('请填写图片 URL'); return }
        jobs = urls.map(u => ({label: u, payload: {url: u}}))
      }
      this.adding = true
      this.addTotal = jobs.length
      this.addDone = 0
      let ok = 0
      const fail = []
      try {
        for (const job of jobs) {
          try {
            const res = await addTgImage(job.payload)
            if (res.data.code === 0) ok++
            else fail.push(`${job.label}：${res.data.message}`)
          } catch (e) {
            fail.push(`${job.label}：${e.message}`)
          }
          this.addDone++
        }
        if (ok > 0) {
          this.$message.success(`成功入库 ${ok} 张${fail.length ? `，失败 ${fail.length} 张` : ''}`)
          this.addVisible = false
          this.page = 1
          await this.getList()
        } else {
          this.$message.error('入库失败')
        }
        if (fail.length) {
          this.$notify({type: 'warning', title: '部分图片未入库', message: fail.join('\n'), duration: 0})
        }
        this.localFiles = []
        this.addForm.url = ''
      } finally {
        this.adding = false
      }
    },
    edit(row) {
      this.form = {id: row.id, name: row.name}
      this.editVisible = true
    },
    async save() {
      try {
        const res = await saveTgImage({id: this.form.id, name: this.form.name})
        if (res.data.code === 0) {
          this.$message.success(res.data.message)
          this.editVisible = false
          await this.getList()
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      }
    },
    async toggleStatus(row) {
      try {
        const res = await saveTgImage({id: row.id, status: row.status === 1 ? 0 : 1})
        if (res.data.code === 0) {
          row.status = row.status === 1 ? 0 : 1
          this.$message.success(res.data.message)
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      }
    },
    async del() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning('请选择要删除的图片'); return
      }
      this.$confirm('删除后库中记录移除（不影响已发出的历史消息），是否继续?').then(async _ => {
        try {
          const res = await delTgImage(this.multipleSelection.join(','))
          if (res.data.code === 0) {
            this.$message.success(res.data.message)
            await this.getList()
          } else {
            this.$message.error(res.data.message)
          }
        } catch (e) {
          this.$message.error(e.message)
        }
      }).catch(_ => {})
    }
  }
}
</script>

<style scoped>
/* 仅本弹窗:收紧顶部留白(全局 App.vue 的 .el-dialog__body padding-top:30px 不动);靠 .add-dialog 高特异性 + !important 覆盖 */
.add-dialog >>> .el-dialog__body {
  padding-top: 6px !important;
  padding-bottom: 22px !important;
}

/* 缩略图加载前的占位块(假图):灰底圆角 + 居中图标,尺寸与真图一致避免跳动 */
.thumb-ph {
  width: 72px;
  height: 72px;
  margin: 0 auto;
  border-radius: 4px;
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  justify-content: center;
}
.thumb-ph i {
  font-size: 26px;
  color: #c0c4cc;
}

.add-tabs {
  margin-top: 4px;
}

/* 上传拖拽区:整宽、加高、圆角、悬停高亮 */
.uploader {
  width: 100%;
}
.uploader >>> .el-upload {
  width: 100%;
}
.uploader >>> .el-upload-dragger {
  width: 100%;
  height: 172px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #fafcff;
  transition: border-color .2s;
}
.uploader >>> .el-upload-dragger:hover {
  border-color: #409EFF;
}
.uploader .el-icon-upload {
  font-size: 50px;
  color: #c0c4cc;
  line-height: 1;
  margin: 0 0 6px;
}
.up-text {
  color: #606266;
  font-size: 14px;
}
.up-text em {
  color: #409EFF;
  font-style: normal;
}
.up-hint {
  margin-top: 6px;
  color: #a8abb2;
  font-size: 12px;
}

/* URL 导入提示 + 多图选择网格 */
.url-hint {
  margin-top: 8px;
  color: #a8abb2;
  font-size: 12px;
  line-height: 1.5;
}
.up-multi {
  margin-top: 14px;
}
.up-multi-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: #909399;
  margin-bottom: 8px;
}
.up-multi-grid {
  display: grid;
  /* 列数按容器宽自适应、每列 1fr 拉伸铺满 → 消除右侧空白(每列最小 88px) */
  grid-template-columns: repeat(auto-fill, minmax(88px, 1fr));
  gap: 10px;
  align-content: start;
  /* 固定高度:选图过多时网格内部滚动,不把弹窗/页面撑高(顶部留 8px 防左上角 ✕ 角标被裁) */
  max-height: 320px;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 8px 12px 4px 2px;
}
.up-multi-grid::-webkit-scrollbar {
  width: 8px;
}
.up-multi-grid::-webkit-scrollbar-thumb {
  background: #dcdfe6;
  border-radius: 4px;
}
.up-multi-grid::-webkit-scrollbar-thumb:hover {
  background: #c0c4cc;
}
.up-mini {
  width: 100%;
  position: relative;
}
.up-mini-del {
  position: absolute;
  top: -7px;
  right: -7px;
  width: 18px;
  height: 18px;
  line-height: 18px;
  text-align: center;
  background: #f56c6c;
  color: #fff;
  border-radius: 50%;
  cursor: pointer;
  font-size: 12px;
  z-index: 1;
}
.up-mini-name {
  margin-top: 4px;
  font-size: 11px;
  color: #909399;
  text-align: center;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
