<template>
  <div class="content">
    <el-card shadow="always">
      <div slot="header">
        <!--搜索栏-->
        <div class="queryForm">
          <el-form :inline="true" :model="where" class="query-form-inline" size="small">
            <el-form-item label="群名">
              <el-input v-model="where.title" placeholder="群名/用户名模糊" clearable class="queryElInput"></el-input>
            </el-form-item>
            <el-form-item label="类型">
              <el-select v-model="where.type" placeholder="全部" clearable class="queryElInput">
                <el-option label="群组" value="group"></el-option>
                <el-option label="超级群组" value="supergroup"></el-option>
                <el-option label="频道" value="channel"></el-option>
              </el-select>
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
        <el-button type="danger" size="small" icon="el-icon-delete" @click="del">删除</el-button>
        <el-button type="primary" size="small" icon="el-icon-s-promotion" @click="openSend">发消息</el-button>
        <el-button size="small" icon="el-icon-refresh" @click="getList">刷新</el-button>
        <span style="margin-left:12px;color:#888;font-size:13px;">共 {{ total }} 个群组/频道；机器人被拉进群后会自动出现在这里，勾选后可群发消息。</span>
      </div>

      <!--数据表格-->
      <el-table ref="table" class="tableData" :data="tableData" height="calc(100vh - 182px)"
                :border="true" v-loading="loading" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="55" align="center"></el-table-column>
        <el-table-column prop="id" label="ID" width="70px" align="center"></el-table-column>
        <el-table-column prop="chat_id" label="ChatID" width="160px" align="center"></el-table-column>
        <el-table-column label="群名/频道名" min-width="200px" show-overflow-tooltip>
          <template v-slot="{row}">{{ row.title || (row.username ? ('@' + row.username) : '（未命名）') }}</template>
        </el-table-column>
        <el-table-column label="类型" width="110px" align="center">
          <template v-slot="{row}">
            <el-tag size="mini" :type="typeTag(row.type)">{{ typeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110px" align="center">
          <template v-slot="{row}">
            <el-tag v-if="Number(row.status) === 1" size="mini" type="success">在群可推送</el-tag>
            <el-tag v-else size="mini" type="info">已离开/失效</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="首次捕获" width="160px" align="center"></el-table-column>
        <el-table-column prop="updated_at" label="最近更新" width="160px" align="center"></el-table-column>
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

    <!--主动发消息(群发到群组)-->
    <el-dialog title="发送消息到群组" :visible.sync="sendVisible" width="680px" top="10vh"
               class="responsive-dialog" :close-on-click-modal="false">
      <el-form :model="sendForm" label-width="90px">
        <el-form-item label="发送对象">
          <span style="color:#606266;">已勾选 <b style="color:#409EFF;">{{ sendForm.count }}</b> 个群组/频道</span>
        </el-form-item>
        <el-form-item label="消息内容">
          <div class="rich-bar">
            <span class="rb" @click="insertTag('richSend','<b>','</b>')"><b>B</b></span>
            <span class="rb" @click="insertTag('richSend','<i>','</i>')"><i>I</i></span>
            <span class="rb" @click="insertTag('richSend','<u>','</u>')"><u>U</u></span>
            <span class="rb" @click="insertTag('richSend','<s>','</s>')"><s>S</s></span>
            <span class="rb" @click="insertTag('richSend','<code>','</code>')">代码</span>
            <span class="rb" @click="insertTag('richSend','<span class=&quot;spoiler&quot;>','</span>')">剧透</span>
            <span class="rb" @click="insertLink('richSend')">🔗链接</span>
            <span class="rb" @click="insertBreak('richSend')">⏎分段</span>
            <span class="rb rb-preview" @click="previewVisible=true">👁 预览</span>
          </div>
          <el-input ref="richSend" type="textarea" :rows="10" v-model="sendForm.text" placeholder="要推送到群组的内容（支持 HTML 富文本）"></el-input>
        </el-form-item>
        <el-form-item label="配图">
          <el-input v-model="sendForm.image" placeholder="选填：图片库引用 file:xxx 或直链 https://...（填了则以图片+文案形式发送）">
            <el-button slot="append" icon="el-icon-picture-outline" @click="openPicker">图片库</el-button>
          </el-input>
          <div v-if="isFileRef(sendForm.image)" class="form-tip">已选用图片库的图（以 file_id 秒发，不受国内图链限制）。</div>
        </el-form-item>
        <div class="form-tip" style="margin-left:90px;">提示：需机器人当前在该群内且有发言权限，否则该群会进入失败名单。</div>
      </el-form>
      <span slot="footer" class="dialog-footer">
        <el-button @click="sendVisible=false" size="small">取 消</el-button>
        <el-button type="primary" :loading="sending" @click="doSend" size="small">发 送</el-button>
      </span>
    </el-dialog>

    <!--富文本预览-->
    <el-dialog title="富文本预览" :visible.sync="previewVisible" width="760px" append-to-body>
      <div v-if="sendForm.text" class="rich-preview" v-html="sendForm.text"></div>
      <div v-else class="form-tip">（暂无内容，请先在文案框输入内容）</div>
      <span slot="footer"><el-button size="small" @click="previewVisible=false">关 闭</el-button></span>
    </el-dialog>

    <!--图片库选择器-->
    <ImagePicker ref="picker" @select="onPickImage"/>
  </div>
</template>

<script>
import {getTgChatList, delTgChat, sendTgChatMessage} from "@/api/tgChat";
import ImagePicker from "@/components/ImagePicker";

export default {
  name: "TgChat",
  components: {ImagePicker},
  data() {
    return {
      tableData: [],
      loading: false,
      where: {title: '', type: ''},
      page: 1,
      limit: Number(localStorage.getItem('pageSize_tgChat')) || 20,
      total: 0,
      multipleSelection: [],
      sendVisible: false,
      sending: false,
      previewVisible: false,   // 富文本预览弹窗(点工具栏「预览」才弹出)
      sendForm: {ids: [], count: 0, text: '', image: ''},
    }
  },
  mounted() {
    this.getList()
  },
  methods: {
    typeLabel(t) {
      return {group: '群组', supergroup: '超级群组', channel: '频道'}[t] || (t || '—')
    },
    typeTag(t) {
      return {group: '', supergroup: 'warning', channel: 'success'}[t] || 'info'
    },
    async getList() {
      this.loading = true
      try {
        const params = {title: this.where.title, type: this.where.type, page: this.page, limit: this.limit}
        const res = await getTgChatList(params)
        if (res.data.code === 0) {
          this.tableData = res.data.data || []
          this.total = res.data.count || 0
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      } finally {
        this.loading = false
      }
    },
    handleSelectionChange(val) {
      this.multipleSelection = val.map(item => item.id)
    },
    // 从图片库选图:回填 file:<file_id> 到配图字段,发送端会直接用 tg.FileID 秒发
    openPicker() {
      this.$refs.picker.show()
    },
    isFileRef(v) {
      return typeof v === 'string' && v.indexOf('file:') === 0
    },
    onPickImage(val) {
      this.sendForm.image = val
    },
    search() {
      this.page = 1
      this.getList()
    },
    reset() {
      this.where = {title: '', type: ''}
      this.page = 1
      this.getList()
    },
    onPageChange(p) {
      this.page = p
      this.getList()
    },
    onSizeChange(size) {
      this.limit = size
      localStorage.setItem('pageSize_tgChat', size)
      this.page = 1
      this.getList()
    },
    openSend() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning('请先勾选要发送的群组'); return
      }
      this.sendForm = {ids: this.multipleSelection.slice(), count: this.multipleSelection.length, text: '', image: ''}
      this.sendVisible = true
    },
    // 富文本工具栏:与机器人菜单页 / TG用户群发同一套交互(在选区两侧包标签/插链接/预览)
    _richEl(refName) {
      const comp = this.$refs[refName]
      if (!comp || !comp.$el) return null
      return comp.$el.querySelector('textarea') || comp.$el.querySelector('input')
    },
    insertTag(refName, open, close) {
      const el = this._richEl(refName)
      if (!el) return
      const start = el.selectionStart || 0
      const end = el.selectionEnd || 0
      const val = this.sendForm.text || ''
      const sel = val.slice(start, end) || '文字'
      this.sendForm.text = val.slice(0, start) + open + sel + close + val.slice(end)
      this.$nextTick(() => {
        el.focus()
        const p = start + open.length + sel.length + close.length
        try { el.setSelectionRange(p, p) } catch (e) {}
      })
    },
    insertLink(refName) {
      const el = this._richEl(refName)
      if (!el) return
      const start = el.selectionStart || 0
      const end = el.selectionEnd || 0
      const val = this.sendForm.text || ''
      const sel = val.slice(start, end) || '链接文字'
      this.$prompt('请输入链接地址（http/https 开头）', '插入链接', {
        inputValue: 'https://',
        inputPattern: /^https?:\/\/.+/,
        inputErrorMessage: '请输入 http/https 开头的地址',
      }).then(({ value }) => {
        const open = '<a href="' + value + '">'
        this.sendForm.text = val.slice(0, start) + open + sel + '</a>' + val.slice(end)
      }).catch(() => {})
    },
    // 在光标处插入一个空行(段落分隔)
    insertBreak(refName) {
      const el = this._richEl(refName)
      if (!el) return
      const end = el.selectionEnd || 0
      const val = this.sendForm.text || ''
      this.sendForm.text = val.slice(0, end) + '\n\n' + val.slice(end)
      this.$nextTick(() => {
        el.focus()
        const p = end + 2
        try { el.setSelectionRange(p, p) } catch (e) {}
      })
    },
    async doSend() {
      if (this.sendForm.ids.length === 0) {
        this.$message.warning('请先勾选要发送的群组'); return
      }
      if (!this.sendForm.text.trim() && !this.sendForm.image.trim()) {
        this.$message.warning('请填写消息内容或配图地址'); return
      }
      this.sending = true
      try {
        const res = await sendTgChatMessage({
          ids: this.sendForm.ids.join(','), text: this.sendForm.text,
          image: this.sendForm.image, format: 'html',
        })
        if (res.data.code === 0) {
          const failed = (res.data.data && res.data.data.failed) || []
          if (failed.length === 0) {
            this.$message.success(res.data.message)
          } else {
            this.$notify({type: 'warning', title: '部分发送失败', message: failed.join('；'), duration: 8000})
          }
          this.sendVisible = false
          // 发送完成后清空勾选与表单,回到未选中状态重新来
          if (this.$refs.table) this.$refs.table.clearSelection()
          this.multipleSelection = []
          this.sendForm = {ids: [], count: 0, text: '', image: ''}
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      } finally {
        this.sending = false
      }
    },
    async del() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning("请选择要删除的群组"); return
      }
      this.$confirm('删除后仅移除本地记录（不影响机器人在群里的实际状态），是否继续?').then(async _ => {
        const ids = this.multipleSelection.join(',')
        try {
          const res = await delTgChat(ids)
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
.form-tip {
  color: #909399;
  font-size: 12px;
  line-height: 1.5;
  margin-top: 2px;
}
/* 富文本工具栏 + 预览(与机器人菜单页 / TG用户群发风格统一) */
.rich-bar {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
}
.rich-bar .rb {
  cursor: pointer;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  padding: 1px 9px;
  font-size: 13px;
  line-height: 22px;
  color: #606266;
  background: #fff;
  user-select: none;
}
.rich-bar .rb:hover {
  color: #409EFF;
  border-color: #409EFF;
}
.rich-bar .rb-preview {
  margin-left: auto;
  color: #409EFF;
  border-color: #c6e2ff;
}
.rich-preview {
  margin-top: 6px;
  padding: 18px 20px;
  min-height: 160px;
  background: #f7f8fa;
  border: 1px dashed #dcdfe6;
  border-radius: 6px;
  font-size: 16px;
  line-height: 2;
  color: #303133;
  word-break: break-word;
  white-space: pre-wrap;
}
.rich-preview ::v-deep a {
  color: #409EFF;
}
.rich-preview ::v-deep code {
  background: #eef0f3;
  padding: 1px 4px;
  border-radius: 3px;
}
</style>
