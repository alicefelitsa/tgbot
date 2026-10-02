<template>
  <div class="content">
    <el-card shadow="always">
      <div slot="header">
        <!--搜索栏-->
        <div class="queryForm">
          <el-form :inline="true" :model="where" class="query-form-inline" size="small">
            <el-form-item label="用户名">
              <el-input v-model="where.username" placeholder="模糊搜索" clearable class="queryElInput"></el-input>
            </el-form-item>
            <el-form-item label="TG用户ID">
              <el-input v-model="where.tg_user_id" placeholder="精确匹配" clearable class="queryElInput"></el-input>
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
        <span style="margin-left:12px;color:#888;font-size:13px;">共 {{ total }} 位用户；勾选用户后可群发消息。</span>
      </div>

      <!--数据表格-->
      <el-table ref="table" class="tableData" :data="tableData" :height="tableHeight"
                :border="true" v-loading="loading" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="55" align="center"></el-table-column>
        <el-table-column prop="id" label="ID" width="70px" align="center"></el-table-column>
        <el-table-column prop="tg_user_id" label="TG用户ID" width="140px" align="center"></el-table-column>
        <el-table-column label="用户名" min-width="140px" show-overflow-tooltip>
          <template v-slot="{row}">{{ row.username ? ('@' + row.username) : '—' }}</template>
        </el-table-column>
        <el-table-column prop="first_name" label="昵称" min-width="120px" show-overflow-tooltip>
          <template v-slot="{row}">{{ row.first_name || '—' }}</template>
        </el-table-column>
        <el-table-column prop="lang" label="语言" width="80px" align="center"></el-table-column>
        <el-table-column label="绑定账号" width="120px" align="center">
          <template v-slot="{row}">
            <span v-if="Number(row.bind_user_id) > 0">{{ row.bind_user_id }}</span>
            <span v-else style="color:#c0c4cc;">未绑定</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="80px" align="center">
          <template v-slot="{row}">
            <el-switch :value="row.status === 1" active-color="#13ce66" @change="toggleStatus(row)"></el-switch>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="首次互动" width="160px" align="center"></el-table-column>
        <el-table-column prop="updated_at" label="最近活跃" width="160px" align="center"></el-table-column>
        <el-table-column label="操作" align="center" width="90px" fixed="right">
          <template v-slot="{row}">
            <el-button size="mini" @click="edit(row)">编辑</el-button>
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

    <!--编辑用户-->
    <el-dialog title="编辑用户" :visible.sync="dialogVisible" width="560px" top="10vh"
               class="responsive-dialog" :close-on-click-modal="false">
      <el-form ref="form" :model="form" label-width="90px">
        <el-form-item label="TG用户ID">
          <el-input :value="String(form.tg_user_id)" disabled></el-input>
        </el-form-item>
        <el-form-item label="用户名">
          <el-input :value="form.username ? ('@' + form.username) : ''" disabled placeholder="（该用户未设置用户名）"></el-input>
        </el-form-item>
        <el-form-item label="昵称">
          <el-input :value="form.first_name" disabled></el-input>
        </el-form-item>
        <el-form-item label="绑定账号">
          <el-input v-model="form.bind_user_id" placeholder="关联的业务账号 ID，0 表示未绑定"></el-input>
        </el-form-item>
        <el-form-item label="语言">
          <el-input v-model="form.lang" placeholder="如 zh / en"></el-input>
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.status" :active-value="1" :inactive-value="0" active-color="#13ce66"></el-switch>
          <span style="margin-left:10px;color:#909399;font-size:12px;">停用后不影响查看，仅作业务标记</span>
        </el-form-item>
      </el-form>
      <span slot="footer" class="dialog-footer">
        <el-button @click="dialogVisible=false" size="small">取 消</el-button>
        <el-button type="primary" @click="save" size="small">确 定</el-button>
      </span>
    </el-dialog>

    <!--主动发消息(群发)-->
    <el-dialog title="发送消息" :visible.sync="sendVisible" width="680px" top="10vh"
               class="responsive-dialog send-dialog" :close-on-click-modal="false">
      <el-form :model="sendForm" label-width="90px">
        <el-form-item label="发送对象">
          <span style="color:#606266;">已勾选 <b style="color:#409EFF;">{{ sendForm.count }}</b> 位用户</span>
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
          <el-input ref="richSend" type="textarea" :rows="10" v-model="sendForm.text" placeholder="要推送给用户的文本内容（支持 HTML 富文本）"></el-input>
        </el-form-item>
        <el-form-item label="配图">
          <el-input v-model="sendForm.image" placeholder="选填：图片库引用 file:xxx 或直链 https://...（填了则以图片+文案形式发送）">
            <el-button slot="append" icon="el-icon-picture-outline" @click="openPicker">图片库</el-button>
          </el-input>
          <div v-if="isFileRef(sendForm.image)" class="form-tip">已选用图片库的图（以 file_id 秒发，不受国内图链限制）。</div>
        </el-form-item>
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
import {getTgUserList, saveTgUser, delTgUser, sendTgUserMessage} from "@/api/tgUser";
import ImagePicker from "@/components/ImagePicker";
import tableAutoHeight from "@/mixins/tableAutoHeight";

export default {
  name: "TgUser",
  components: {ImagePicker},
  mixins: [tableAutoHeight],
  data() {
    return {
      tableData: [],
      loading: false,
      where: {username: '', tg_user_id: ''},
      page: 1,
      limit: Number(localStorage.getItem('pageSize_tgUser')) || 20,
      total: 0,
      dialogVisible: false,
      multipleSelection: [],
      form: this.emptyForm(),
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
    emptyForm() {
      return {id: '', tg_user_id: '', username: '', first_name: '', lang: '', bind_user_id: 0, status: 1}
    },
    async getList() {
      this.loading = true
      try {
        const params = {username: this.where.username, tg_user_id: this.where.tg_user_id, page: this.page, limit: this.limit}
        const res = await getTgUserList(params)
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
      this.where = {username: '', tg_user_id: ''}
      this.page = 1
      this.getList()
    },
    onPageChange(p) {
      this.page = p
      this.getList()
    },
    onSizeChange(size) {
      this.limit = size
      localStorage.setItem('pageSize_tgUser', size)
      this.page = 1
      this.getList()
    },
    edit(row) {
      this.form = {
        id: row.id, tg_user_id: row.tg_user_id, username: row.username, first_name: row.first_name,
        lang: row.lang, bind_user_id: row.bind_user_id, status: row.status,
      }
      this.dialogVisible = true
    },
    openSend() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning('请先勾选要发送的用户'); return
      }
      this.sendForm = {ids: this.multipleSelection.slice(), count: this.multipleSelection.length, text: '', image: ''}
      this.sendVisible = true
    },
    // 富文本工具栏:与机器人菜单页同一套交互(在选区两侧包标签/插链接/预览)
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
    // 在光标处插入一个空行(段落分隔):Telegram 不支持行距,只能靠空行把长段落拆成几小段拉开间距
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
        this.$message.warning('请先勾选要发送的用户'); return
      }
      if (!this.sendForm.text.trim() && !this.sendForm.image.trim()) {
        this.$message.warning('请填写消息内容或配图地址'); return
      }
      this.sending = true
      try {
        const res = await sendTgUserMessage({
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
    async save() {
      const payload = {
        id: this.form.id, bind_user_id: Number(this.form.bind_user_id) || 0,
        lang: this.form.lang, status: this.form.status,
      }
      try {
        const res = await saveTgUser(payload)
        if (res.data.code === 0) {
          this.$message.success(res.data.message)
          this.dialogVisible = false
          await this.getList()
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      }
    },
    async toggleStatus(row) {
      const payload = {id: row.id, status: row.status === 1 ? 0 : 1}
      try {
        const res = await saveTgUser(payload)
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
        this.$message.warning("请选择要删除的用户"); return
      }
      this.$confirm('删除后该用户记录将移除（用户再次与机器人互动会重新记录），是否继续?').then(async _ => {
        const ids = this.multipleSelection.join(',')
        try {
          const res = await delTgUser(ids)
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
/* 笔记本矮屏兼容:消息文本框高度随视口自适应(不再固定 rows=10 撑高),保证弹窗在 86vh 限高内不出内容滚动条 */
.send-dialog >>> .el-textarea__inner {
  height: clamp(130px, calc(86vh - 360px), 280px);
}

.form-tip {
  color: #909399;
  font-size: 12px;
  line-height: 1.5;
  margin-top: 2px;
}
/* 富文本工具栏 + 预览(与机器人菜单页风格统一) */
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
