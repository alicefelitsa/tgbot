<template>
  <div class="content">
    <el-card shadow="always">
      <div slot="header">
        <!--搜索栏-->
        <div class="queryForm">
          <el-form :inline="true" :model="where" class="query-form-inline" size="small">
            <el-form-item label="命令名">
              <el-input v-model="where.command" placeholder="请输入" clearable class="queryElInput"></el-input>
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
        <el-button type="primary" size="small" icon="el-icon-plus" @click="add">添加命令</el-button>
        <el-button type="danger" size="small" icon="el-icon-delete" @click="del">删除</el-button>
        <el-button type="success" size="small" icon="el-icon-refresh" :loading="syncing" @click="sync">同步到 Telegram</el-button>
        <span style="margin-left:12px;color:#888;font-size:13px;">对应 Telegram 原生「菜单」命令；改完记得点「同步到 Telegram」</span>
      </div>

      <!--数据表格-->
      <el-table ref="table" class="tableData" :data="tableData" height="calc(100vh - 182px)"
                :border="true" v-loading="loading" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="55" align="center"></el-table-column>
        <el-table-column prop="id" label="ID" width="70px" align="center"></el-table-column>
        <el-table-column label="命令" width="140px" align="center">
          <template v-slot="{row}">/{{ row.command }}</template>
        </el-table-column>
        <el-table-column prop="description" label="说明" width="160px" show-overflow-tooltip></el-table-column>
        <el-table-column prop="action_type" label="触发行为" width="100px" align="center">
          <template v-slot="{row}">
            <el-tag size="mini" :type="actionTagType(row.action_type)" :class="{ 'tag-menu': row.action_type === 'menu' }">{{ actionLabel(row.action_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="触发后效果" min-width="240px" show-overflow-tooltip>
          <template v-slot="{row}">{{ configSummary(row) }}</template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="70px" align="center"></el-table-column>
        <el-table-column prop="status" label="状态" width="80px" align="center">
          <template v-slot="{row}">
            <el-switch :value="row.status === 1" active-color="#13ce66" @change="toggleStatus(row)"></el-switch>
          </template>
        </el-table-column>
        <el-table-column label="操作" align="center" width="100px">
          <template v-slot="{row}">
            <el-button size="mini" @click="edit(row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!--添加/编辑命令-->
    <el-dialog :title="form.id?'修改命令':'添加命令'" :visible.sync="dialogVisible" width="880px" top="6vh"
               class="responsive-dialog" :close-on-click-modal="false">
      <el-form ref="form" :model="form" label-width="90px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="命令名">
              <el-input v-model="form.command" placeholder="小写字母/下划线，不含斜杠，如 start、help"></el-input>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="说明">
              <el-input v-model="form.description" placeholder="显示在原生「菜单」里的说明文字"></el-input>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="form.action_type === 'menu' ? 12 : 24">
            <el-form-item label="触发行为">
              <el-select v-model="form.action_type" placeholder="选择命令触发后做什么" style="width:100%;" @change="onActionTypeChange">
                <el-option v-for="a in actionTypeOptions" :key="a.value" :label="a.label" :value="a.value"></el-option>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12" v-if="form.action_type === 'menu'">
            <el-form-item label="跳转目标">
              <el-select key="sel-page" v-model="cfg.page" placeholder="命令触发后打开的页面" style="width:100%;">
                <el-option label="🏠 根主菜单" value="root"></el-option>
                <el-option v-for="m in topMenus" :key="m.id" :label="m.title" :value="String(m.id)"></el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <div class="form-tip" style="margin:0 0 14px 90px;">{{ actionHint }}</div>

        <!--按动作类型动态显示配置项-->
        <template v-if="form.action_type === 'menu'">
          <!--仅「根主菜单」横幅配在命令上;选具体一级菜单则去该菜单行本身配-->
          <template v-if="cfg.page === 'root'">
            <el-form-item label="菜单引导语">
              <div class="rich-bar">
                <span class="rb" @click="insertTag('richCmd','<b>','</b>')"><b>B</b></span>
                <span class="rb" @click="insertTag('richCmd','<i>','</i>')"><i>I</i></span>
                <span class="rb" @click="insertTag('richCmd','<u>','</u>')"><u>U</u></span>
                <span class="rb" @click="insertTag('richCmd','<s>','</s>')"><s>S</s></span>
                <span class="rb" @click="insertTag('richCmd','<code>','</code>')">代码</span>
                <span class="rb" @click="insertTag('richCmd','<span class=&quot;spoiler&quot;>','</span>')">剧透</span>
                <span class="rb" @click="insertLink('richCmd')">🔗链接</span>
                <span class="rb rb-preview" @click="previewVisible=true">👁 预览</span>
              </div>
              <el-input ref="richCmd" type="textarea" :rows="4" v-model="cfg.text" placeholder="显示在九宫格按钮上方（默认「请选择：」）"></el-input>
            </el-form-item>
            <el-form-item label="配图">
              <el-input v-model="cfg.image" placeholder="选填：图片库引用 file:xxx 或根主菜单顶部横幅图直链 https://...">
                <el-button slot="append" icon="el-icon-picture-outline" @click="openPicker">图片库</el-button>
              </el-input>
            </el-form-item>
          </template>
          <div v-else class="form-tip" style="margin:0 0 14px 90px;">将打开所选一级菜单页；该页的引导语与配图请到「机器人菜单」里配置该菜单项本身。</div>
        </template>

        <template v-if="form.action_type === 'text'">
          <el-form-item label="回复文案">
            <div class="rich-bar">
              <span class="rb" @click="insertTag('richText','<b>','</b>')"><b>B</b></span>
              <span class="rb" @click="insertTag('richText','<i>','</i>')"><i>I</i></span>
              <span class="rb" @click="insertTag('richText','<u>','</u>')"><u>U</u></span>
              <span class="rb" @click="insertTag('richText','<s>','</s>')"><s>S</s></span>
              <span class="rb" @click="insertTag('richText','<code>','</code>')">代码</span>
              <span class="rb" @click="insertTag('richText','<span class=&quot;spoiler&quot;>','</span>')">剧透</span>
              <span class="rb" @click="insertLink('richText')">🔗链接</span>
              <span class="rb rb-preview" @click="previewVisible=true">👁 预览</span>
            </div>
            <el-input ref="richText" type="textarea" :rows="4" v-model="cfg.text" placeholder="发送的固定文案"></el-input>
          </el-form-item>
          <el-form-item label="配图">
            <el-input v-model="cfg.image" placeholder="选填：图片库引用 file:xxx 或图片直链 https://...；填了则以「图片+文案」呈现">
              <el-button slot="append" icon="el-icon-picture-outline" @click="openPicker">图片库</el-button>
            </el-input>
          </el-form-item>
        </template>

        <template v-if="form.action_type === 'url'">
          <el-form-item label="链接地址">
            <el-input v-model="cfg.url" placeholder="https:// 开头的链接"></el-input>
          </el-form-item>
        </template>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="排序">
              <el-input-number v-model="form.sort" :min="0" style="width:100%;"></el-input-number>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="状态">
              <el-switch v-model="form.status" :active-value="1" :inactive-value="0" active-color="#13ce66"></el-switch>
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <span slot="footer" class="dialog-footer">
        <el-button @click="dialogVisible=false" size="small">取 消</el-button>
        <el-button type="primary" @click="save" size="small">确 定</el-button>
      </span>
    </el-dialog>
    <el-dialog title="富文本预览" :visible.sync="previewVisible" width="760px" append-to-body>
      <div v-if="cfg.text" class="rich-preview" v-html="cfg.text"></div>
      <div v-else class="form-tip">（暂无内容，请先在文案框输入内容）</div>
      <span slot="footer"><el-button size="small" @click="previewVisible=false">关 闭</el-button></span>
    </el-dialog>

    <!--图片库选择器-->
    <ImagePicker ref="picker" @select="onPickImage"/>
  </div>
</template>

<script>
import {getTgCommandList, addTgCommand, saveTgCommand, delTgCommand, syncTgCommand} from "@/api/tgCommand";
import {getTgMenuList} from "@/api/tgMenu";
import ImagePicker from "@/components/ImagePicker";

export default {
  name: "TgCommand",
  components: {ImagePicker},
  data() {
    return {
      tableData: [],
      loading: false,
      syncing: false,
      where: {command: ''},
      dialogVisible: false,
      previewVisible: false,   // 富文本预览弹窗(点工具栏「预览」才弹出)
      multipleSelection: [],
      actionTypeOptions: [
        {value: 'menu', label: '打开菜单（menu）'},
        {value: 'text', label: '回复文字（text）'},
        {value: 'url', label: '打开链接（url）'},
      ],
      form: this.emptyForm(),
      cfg: {page: 'root', text: '', url: '', image: '', format: 'html', args: ''},
      topMenus: [],   // 一级菜单文件夹(parent_id=0 且 action_type=menu),供「跳转目标」下拉
    }
  },
  computed: {
    // 当前选中动作类型的用途说明(随下拉选择实时变化)
    actionHint() {
      return {
        menu: '命令触发后打开主菜单（九宫格）——一般给 /start 用。',
        text: '命令触发后回显一段固定文字——内容写死在下面（如 /help、/support）。',
        url: '命令触发后回一条带外链按钮的消息——点击打开网页 / 频道 / 店铺等。',
      }[this.form.action_type] || ''
    },
  },
  mounted() {
    this.getList()
    this.loadTopMenus()
  },
  methods: {
    emptyForm() {
      return {id: '', command: '', description: '', action_type: 'menu', sort: 0, status: 1}
    },
    // 加载一级菜单文件夹,作为命令「跳转目标」候选(根主菜单之外可直接打开的一级页)
    async loadTopMenus() {
      try {
        const res = await getTgMenuList({parent_id: 0})
        if (res.data.code === 0) {
          this.topMenus = (res.data.data || []).filter(m => m.parent_id === 0 && m.action_type === 'menu')
        }
      } catch (e) { /* 忽略:下拉为空不影响其他配置 */ }
    },
    actionTagType(t) {
      return {menu: '', text: 'success', url: 'warning', handler: 'danger'}[t] || 'info'
    },
    // 动作类型的中文标签
    actionLabel(t) {
      return {menu: '打开菜单', text: '回复文字', url: '打开链接', handler: '动态数据'}[t] || t
    },
    // 把原始 JSON 配置翻译成“命令触发后会发生什么”
    configSummary(row) {
      let cfg = {}
      try { cfg = row.action_config ? JSON.parse(row.action_config) : {} } catch (e) { return row.action_config || '—' }
      switch (row.action_type) {
        case 'menu': {
          if (!cfg.page || cfg.page === 'root') {
            let s = '📋 打开主菜单（九宫格）'
            if (cfg.image) s += ' 🖼️含配图'
            return s
          }
          const m = this.topMenus.find(x => String(x.id) === String(cfg.page))
          return '📂 打开一级菜单：' + (m ? m.title : ('#' + cfg.page))
        }
        case 'text': {
          if (!cfg.text && !cfg.image) return '—'
          return (cfg.image ? '🖼️ ' : '') + '💬 ' + (cfg.text || '(仅配图)')
        }
        case 'url':
          return cfg.url ? ('🔗 打开链接：' + cfg.url) : '—'
        case 'handler':
          return '⚙️ 调用数据：' + (cfg.handler || '—') + (cfg.args ? ('（参数 ' + cfg.args + '）') : '')
        default:
          return '—'
      }
    },
    async getList() {
      this.loading = true;
      setTimeout(async () => {
        try {
          const res = await getTgCommandList(this.where)
          if (res.data.code === 0) {
            this.tableData = res.data.data || [];
          } else {
            this.$message.error(res.data.message)
          }
        } catch (e) {
          this.$message.error(e.message);
        } finally {
          this.loading = false;
        }
      }, 200)
    },
    handleSelectionChange(val) {
      this.multipleSelection = val.map(item => item.id)
    },
    // 从图片库选图:回填 file:<file_id> 到 cfg.image,发送端会直接用 tg.FileID 秒发
    openPicker() {
      this.$refs.picker.show()
    },
    onPickImage(val) {
      this.cfg.image = val
    },
    search() {
      this.getList()
    },
    reset() {
      this.where.command = ''
      this.getList()
    },
    add() {
      this.form = this.emptyForm()
      this.cfg = {page: 'root', text: '', url: '', image: '', format: 'html', args: ''}
      this.dialogVisible = true
    },
    edit(row) {
      this.form = {
        id: row.id, command: row.command, description: row.description,
        action_type: row.action_type, sort: row.sort, status: row.status,
      }
      this.cfg = {page: '', text: '', url: '', image: '', format: 'html', args: ''}
      let parsed = {}
      try {
        parsed = row.action_config ? JSON.parse(row.action_config) : {}
      } catch (e) { parsed = {} }
      this.cfg.page = parsed.page || (row.action_type === 'menu' ? 'root' : '')
      this.cfg.text = parsed.text || ''
      this.cfg.url = parsed.url || ''
      this.cfg.args = parsed.args || ''
      this.cfg.image = parsed.image || ''
      this.cfg.format = parsed.format || 'html'
      this.dialogVisible = true
    },
    onActionTypeChange() {
      this.cfg = {page: this.form.action_type === 'menu' ? 'root' : '', text: '', url: '', image: '', format: 'html', args: ''}
    },
    // ==================== 富文本工具栏(仅包 Telegram 支持的标签,输出仍是 HTML 字符串) ====================
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
      const val = this.cfg.text || ''
      const sel = val.slice(start, end) || '文字'
      this.cfg.text = val.slice(0, start) + open + sel + close + val.slice(end)
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
      const val = this.cfg.text || ''
      const sel = val.slice(start, end) || '链接文字'
      this.$prompt('请输入链接地址（http/https 开头）', '插入链接', {
        inputValue: 'https://',
        inputPattern: /^https?:\/\/.+/,
        inputErrorMessage: '请输入 http/https 开头的地址',
      }).then(({ value }) => {
        const open = '<a href="' + value + '">'
        this.cfg.text = val.slice(0, start) + open + sel + '</a>' + val.slice(end)
      }).catch(() => {})
    },
    buildActionConfig() {
      const t = this.form.action_type
      if (t === 'menu') {
        // 无论选根主菜单还是一级菜单,都保留 text/image/format(选具体菜单时后端忽略它们),
        // 避免“切到具体菜单→保存→再切回根”时横幅数据被抹掉
        return JSON.stringify({page: this.cfg.page || 'root', text: this.cfg.text, image: this.cfg.image, format: this.cfg.format})
      }
      if (t === 'text') return JSON.stringify({text: this.cfg.text, image: this.cfg.image, format: this.cfg.format})
      if (t === 'url') return JSON.stringify({url: this.cfg.url})
      return '{}'
    },
    validateCommand() {
      if (!this.form.command || !this.form.command.trim()) {
        this.$message.warning("请输入命令名"); return false
      }
      if (this.form.command.includes('/')) {
        this.$message.warning("命令名不要包含斜杠 /"); return false
      }
      if (this.form.action_type === 'text' && !this.cfg.text) {
        this.$message.warning("请填写回复文案"); return false
      }
      if (this.form.action_type === 'url' && !this.cfg.url) {
        this.$message.warning("请填写链接地址"); return false
      }
      return true
    },
    async save() {
      if (!this.validateCommand()) return
      const payload = {...this.form, action_config: this.buildActionConfig()}
      try {
        const addOrSave = this.form.id ? saveTgCommand : addTgCommand;
        const res = await addOrSave(payload)
        if (res.data.code === 0) {
          this.$message.success(res.data.message)
          this.dialogVisible = false
          await this.getList()
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message);
      }
    },
    async toggleStatus(row) {
      const payload = {id: row.id, status: row.status === 1 ? 0 : 1}
      try {
        const res = await saveTgCommand(payload)
        if (res.data.code === 0) {
          row.status = row.status === 1 ? 0 : 1
          this.$message.success(res.data.message)
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message);
      }
    },
    async del() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning("请选择要删除的数据"); return
      }
      this.$confirm('即将删除所选命令，是否继续?').then(async _ => {
        const ids = this.multipleSelection.join(',')
        try {
          const res = await delTgCommand(ids)
          this.$message.success(res.data.message)
          await this.getList()
        } catch (e) {
          this.$message.error(e.message);
        }
      }).catch(_ => {})
    },
    // 同步到 Telegram 原生「菜单」按钮
    async sync() {
      this.syncing = true
      try {
        const res = await syncTgCommand()
        if (res.data.code === 0) {
          this.$message.success(res.data.message)
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message);
      } finally {
        this.syncing = false
      }
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
/* 打开菜单(menu)标签:与「机器人菜单」页一致的柔和玫红 */
.tag-menu {
  background-color: #fef0f0;
  border-color: #fbc4c4;
  color: #e0686d;
}
/* 富文本工具栏 + 实时预览 */
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
/* 预览按钮:靠右 + 蓝色描边,与排版工具区分 */
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
