<template>
  <div class="content">
    <el-card shadow="always">
      <div slot="header">
        <!--搜索栏-->
        <div class="queryForm">
          <el-form :inline="true" :model="where" class="query-form-inline" size="small">
            <el-form-item label="菜单名称">
              <el-input v-model="where.title" placeholder="输入名称搜索(含子菜单)" clearable class="queryElInput"
                        @keyup.enter.native="search"></el-input>
            </el-form-item>
            <el-form-item label="点击行为">
              <el-select v-model="where.action_type" placeholder="全部" clearable style="width:160px;" @change="search">
                <el-option v-for="a in actionTypeOptions" :key="a.value" :label="a.label" :value="a.value"></el-option>
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button icon="el-icon-refresh" @click="reset">重置</el-button>
            </el-form-item>
          </el-form>
        </div>
      </div>

      <!--工具栏-->
      <div class="toolbar">
        <el-button type="primary" size="small" icon="el-icon-plus" @click="add(0)">添加菜单</el-button>
        <el-button type="danger" size="small" icon="el-icon-delete" @click="del">删除</el-button>
        <el-button size="small" icon="el-icon-sort" @click="toggleExpand">{{ hasExpanded ? '全部折叠' : '全部展开' }}</el-button>
        <el-button size="small" icon="el-icon-refresh-left" @click="loadMenus">刷新</el-button>
        <span style="margin-left:12px;color:#888;font-size:13px;">整棵菜单以树形展示，缩进即上下级关系；点行首箭头展开/折叠。搜索时列出所有匹配项。</span>
      </div>

      <!--数据表格(树形,无分页):高度由 setTableHeight() 动态算(视口高-表格顶部位置-底部留白),保证铺满窗口又不出页面滚动条-->
      <el-table ref="table" class="tableData" :data="displayData" :height="tableHeight"
                :border="true" v-loading="loading" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="55" align="center"></el-table-column>
        <el-table-column prop="id" label="ID" width="60px" align="center"></el-table-column>
        <el-table-column label="菜单名称" width="210px" class-name="col-title">
          <template v-slot="{row}">
            <div class="tree-cell">
              <span v-for="(cont, j) in (row.__ancLast || [])" :key="'a' + j"
                    class="tree-guide" :class="{ 'tree-guide--vline': !cont }"></span>
              <span v-if="row.__level > 0" class="tree-guide tree-guide--elbow"
                    :class="{ 'tree-guide--elbow-more': !row.__isLast }"></span>
              <span class="tree-node">
                <i v-if="row.__hasChildren"
                   :class="['tree-arrow', isExpanded(row) ? 'is-open el-icon-arrow-down' : 'el-icon-arrow-right']"
                   @click.stop="toggleNode(row)"></i>
                <i v-else class="tree-arrow-hollow" :class="{ 'tree-arrow-hollow--root': row.__level === 0 }"></i>
                <span class="tree-title">{{ row.title }}</span>
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="searching" label="上级菜单" width="160px" align="center" show-overflow-tooltip>
          <template v-slot="{row}">{{ parentLabel(row.parent_id) }}</template>
        </el-table-column>
        <el-table-column prop="action_type" label="点击行为" width="90px" align="center">
          <template v-slot="{row}">
            <el-tag size="mini" :type="actionTagType(row.action_type)" :class="{ 'tag-http': row.action_type === 'http', 'tag-menu': row.action_type === 'menu', 'tag-url': row.action_type === 'url' }">{{ actionLabel(row.action_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="点击后效果" min-width="180px" show-overflow-tooltip>
          <template v-slot="{row}">{{ configSummary(row) }}</template>
        </el-table-column>
        <el-table-column label="每行" width="60px" align="center">
          <template v-slot="{row}">{{ isFolder(row) ? row.cols : (Number(row.cols) === 1 ? '整行' : '—') }}</template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="90px" align="center">
          <template v-slot="{row}">
            <el-input-number v-model="row.sort" size="mini" :min="0" :controls="false" style="width:56px;" @change="saveSort(row)"></el-input-number>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="80px" align="center">
          <template v-slot="{row}">
            <el-switch :value="row.status === 1" active-color="#13ce66" @change="toggleStatus(row)"></el-switch>
          </template>
        </el-table-column>
        <el-table-column label="操作" align="left" :width="actionColWidth">
          <template v-slot="{row}">
            <el-button size="mini" @click="edit(row)">编辑</el-button>
            <el-button v-if="isFolder(row)" size="mini" type="text" @click="add(row.id)">添加子菜单</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!--添加/编辑菜单-->
    <el-dialog :title="form.id?'修改菜单':'添加菜单'" :visible.sync="dialogVisible" width="880px" top="6vh"
               class="responsive-dialog" :close-on-click-modal="false">
      <el-form ref="form" :model="form" label-width="90px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="上级菜单">
              <el-select v-model="form.parent_id" filterable placeholder="选择挂在哪个菜单下" style="width:100%;">
                <el-option v-for="p in parentOptions" :key="p.value" :label="p.label" :value="p.value"></el-option>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="菜单名称">
              <el-input v-model="form.title" placeholder="按钮文字(可加 emoji)"></el-input>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="点击行为">
              <el-select v-model="form.action_type" placeholder="选择点击后做什么" style="width:100%;" @change="onActionTypeChange">
                <el-option v-for="a in actionTypeOptions" :key="a.value" :label="a.label" :value="a.value"></el-option>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12" v-if="form.action_type === 'http' || form.action_type === 'pick_user'">
            <el-form-item label="请求方法">
              <el-select v-model="cfg.method" style="width:100%;">
                <el-option label="GET" value="GET"></el-option>
                <el-option label="POST" value="POST"></el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!--按动作类型动态显示配置项-->
        <template v-if="form.action_type === 'menu'">
          <el-form-item label="页面引导语">
            <div class="rich-bar">
              <span class="rb" @click="insertTag('richMenu','<b>','</b>')"><b>B</b></span>
              <span class="rb" @click="insertTag('richMenu','<i>','</i>')"><i>I</i></span>
              <span class="rb" @click="insertTag('richMenu','<u>','</u>')"><u>U</u></span>
              <span class="rb" @click="insertTag('richMenu','<s>','</s>')"><s>S</s></span>
              <span class="rb" @click="insertTag('richMenu','<code>','</code>')">代码</span>
              <span class="rb" @click="insertTag('richMenu','<span class=&quot;spoiler&quot;>','</span>')">剧透</span>
              <span class="rb" @click="insertLink('richMenu')">🔗链接</span>
              <span class="rb" @click="insertBreak('richMenu')">⏎分段</span>
              <span class="rb rb-preview" @click="previewVisible=true">👁 预览</span>
            </div>
            <el-input ref="richMenu" type="textarea" :rows="4" v-model="cfg.text" placeholder="选填，显示在子按钮上方（默认「请选择：」）"></el-input>
          </el-form-item>
          <el-form-item label="配图">
            <el-input v-model="cfg.image" placeholder="选填：图片库引用 file:xxx 或页面顶部横幅图直链 https://...">
              <el-button slot="append" icon="el-icon-picture-outline" @click="openPicker">图片库</el-button>
            </el-input>
          </el-form-item>
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
              <span class="rb" @click="insertBreak('richText')">⏎分段</span>
              <span class="rb rb-preview" @click="previewVisible=true">👁 预览</span>
            </div>
            <el-input ref="richText" type="textarea" :rows="4" v-model="cfg.text" placeholder="点击后原地显示的固定文案"></el-input>
          </el-form-item>
          <el-form-item label="配图">
            <el-input v-model="cfg.image" placeholder="选填：图片库引用 file:xxx 或图片直链 https://...；填了则以「图片+文案+按钮」呈现">
              <el-button slot="append" icon="el-icon-picture-outline" @click="openPicker">图片库</el-button>
            </el-input>
          </el-form-item>
        </template>

        <template v-if="form.action_type === 'url'">
          <el-form-item label="链接地址">
            <el-input v-model="cfg.url" placeholder="https:// 开头的链接"></el-input>
          </el-form-item>
        </template>

        <template v-if="form.action_type === 'cancel'">
          <div class="form-tip" style="margin:0 0 14px 90px;">点「取消」按钮会删除当前这条菜单消息（会话里直接消失），无需额外配置。</div>
        </template>

        <template v-if="form.action_type === 'http' || form.action_type === 'pick_user'">
          <el-form-item v-if="form.action_type === 'http'" label="输入提示语">
            <el-input type="textarea" :rows="2" v-model="cfg.input_prompt" placeholder="选填，如：请输入你的订单号"></el-input>
          </el-form-item>
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="接口地址">
                <el-input type="textarea" :rows="2" v-model="cfg.url" placeholder="https://api.example.com/{uid}/balance"></el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="请求头">
                <el-input type="textarea" :rows="2" v-model="cfg.headers" placeholder="选填，一行一个，如：&#10;Authorization: Bearer xxx&#10;Content-Type: application/json"></el-input>
              </el-form-item>
            </el-col>
          </el-row>
          <el-form-item v-if="cfg.method === 'POST'" label="请求体">
            <el-input type="textarea" :rows="3" v-model="cfg.body" placeholder='{"name":"alice","age":18,"vip":true,"uid":10086}'></el-input>
          </el-form-item>
          <el-form-item label="显示文案">
            <div class="rich-bar">
              <span class="rb" @click="insertTag('richHttp','<b>','</b>')"><b>B</b></span>
              <span class="rb" @click="insertTag('richHttp','<i>','</i>')"><i>I</i></span>
              <span class="rb" @click="insertTag('richHttp','<u>','</u>')"><u>U</u></span>
              <span class="rb" @click="insertTag('richHttp','<s>','</s>')"><s>S</s></span>
              <span class="rb" @click="insertTag('richHttp','<code>','</code>')">代码</span>
              <span class="rb" @click="insertTag('richHttp','<span class=&quot;spoiler&quot;>','</span>')">剧透</span>
              <span class="rb" @click="insertLink('richHttp')">🔗链接</span>
              <span class="rb" @click="insertBreak('richHttp')">⏎分段</span>
              <span class="rb rb-preview" @click="previewVisible=true">👁 预览</span>
            </div>
            <el-input ref="richHttp" type="textarea" :rows="4" v-model="cfg.text" placeholder="如：你的余额 {data.balance} 元"></el-input>
          </el-form-item>
          <!--配图路径与按钮列数并排一行(列数对 http 无排版语义,仅避免半行空缺)-->
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="配图路径">
                <!--三种写法自适应:图库选图回填 file:<id>/直链(可含 {uid} 等占位)/响应字段路径如 {pci}-->
                <el-input v-model="cfg.image_path" placeholder="图片地址 / {字段路径} / 点「图片库」选图">
                  <el-button slot="append" icon="el-icon-picture-outline" style="padding:8px 12px;" @click="openPicker('http')" title="从图片库选择"/>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="按钮列数">
                <el-input-number v-model="form.cols" :min="1" :max="6" style="width:100%;"></el-input-number>
              </el-form-item>
            </el-col>
          </el-row>
        </template>

        <!--选择用户:固定默认选人页 + 选完把「输入提示语」(feedback_text)当提示让用户输入(如金额),输入作 {input} 随 {payee} 调接口;接口配置复用下方 http 那套-->
        <template v-if="form.action_type === 'pick_user'">
          <el-form-item label="输入提示语">
            <el-input type="textarea" :rows="2" v-model="cfg.feedback_text" placeholder="选完用户后回给用户的一条提示，同时也是让用户输入的引导（如：你已选择 @xxx，请输入金额）。支持 {payee}/{payee_name}/{payee_username}；用户随后输入的文本作为 {input} 变量带进接口"></el-input>
          </el-form-item>
        </template>

        <!--非文件夹也可配列数:设为 1 表示本按钮在父页里独占一行(其余按钮自动断行);http 已并入上行、pick_user 无内联按钮不在此列-->
        <el-form-item v-if="form.action_type !== 'menu' && form.action_type !== 'http' && form.action_type !== 'pick_user'" label="按钮列数">
          <el-input-number v-model="form.cols" :min="1" :max="6" style="width:100%;"></el-input-number>
        </el-form-item>
        <el-form-item v-if="form.action_type === 'menu'" label="每行按钮数">
          <el-input-number v-model="form.cols" :min="1" :max="6" style="width:100%;"></el-input-number>
          <div class="form-tip">这个文件夹展开后，子按钮每行排几个。</div>
        </el-form-item>
        <!--排序、状态不在此设置:改到表格列表里直接改(排序列可编辑、状态列可点开关),弹窗只留内容配置-->
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
import {getTgMenuList, addTgMenu, saveTgMenu, delTgMenu} from "@/api/tgMenu";
import ImagePicker from "@/components/ImagePicker";
import tableAutoHeight from "@/mixins/tableAutoHeight";

export default {
  name: "TgMenu",
  components: {ImagePicker},
  mixins: [tableAutoHeight],
  data() {
    return {
      allMenus: [],        // 全量菜单(扁平),用于构树 + id→名称映射 + 上级下拉
      loading: false,
      expandedIds: [],     // 手动树形:当前展开的节点 id(默认空=全部折叠)
      where: {title: '', action_type: ''},
      dialogVisible: false,
      previewVisible: false,   // 富文本预览弹窗(点工具栏「预览」才弹出)
      pickerTarget: 'image',   // 图库选图回填目标:'image'(配图字段)或 'http'(接口配图路径)
      multipleSelection: [],
      actionTypeOptions: [
        {value: 'menu', label: '菜单导航（menu）'},
        {value: 'text', label: '显示文字（text）'},
        {value: 'url', label: '打开链接（url）'},
        {value: 'http', label: '接口取数（http）'},
        {value: 'pick_user', label: '选择用户（pick_user）'},
        {value: 'cancel', label: '取消（cancel）'},
      ],
      form: this.emptyForm(),
      cfg: {page: '', target: 'parent', text: '', url: '', image: '', image_path: '', format: 'html', method: 'GET', headers: '', body: '', list_path: '', btn_text: '', btn_url: '', input_prompt: '', feedback_text: ''},
    }
  },
  computed: {
    // 是否处于搜索(平铺)模式:平铺时无缩进,需靠“上级菜单”列交代层级
    searching() {
      return !!((this.where.title && this.where.title.trim()) || this.where.action_type)
    },
    // id→菜单 映射(数字键)
    menuMap() {
      const m = {}
      this.allMenus.forEach(x => { m[Number(x.id)] = x })
      return m
    },
    // 是否有展开的节点(控制“全部展开/折叠”按钮文案)
    hasExpanded() {
      return this.expandedIds.length > 0
    },
    // 操作列宽:仅当当前列表存在“文件夹”行(才会多出一个“添加子菜单”按钮)时留 180px,
    // 否则(如按 text/url/http 等筛选、每行只剩“编辑”)收窄为 90px,避免右侧大片留白
    actionColWidth() {
      return this.displayData.some(r => this.isFolder(r)) ? '180px' : '90px'
    },
    // 表格数据:有搜索词或选了点击行为→列出所有匹配(平铺);否则→按展开状态把树拍平成带层级的行
    displayData() {
      const kw = (this.where.title || '').trim().toLowerCase()
      const at = this.where.action_type
      if (kw || at) {
        return this.allMenus
          .filter(x => (!kw || (x.title || '').toLowerCase().includes(kw)) && (!at || x.action_type === at))
          .map(x => ({...x, __level: 0, __hasChildren: false, __ancLast: [], __isLast: true}))
      }
      const out = []
      const walk = (nodes, depth, ancLast) => {
        nodes.forEach((n, i) => {
          const kids = n.children || []
          const isLast = i === nodes.length - 1
          out.push({...n, __level: depth, __hasChildren: kids.length > 0, __ancLast: ancLast, __isLast: isLast})
          if (kids.length && this.expandedIds.includes(Number(n.id))) {
            // 根层不占主干列;子层的祖先列 = 父层祖先列 + 父自身是否末子
            const childAnc = depth === 0 ? [] : ancLast.concat([isLast])
            walk(kids, depth + 1, childAnc)
          }
        })
      }
      walk(this.buildTree(this.allMenus), 0, [])
      return out
    },
    // 上级菜单下拉:根菜单 + 全部“文件夹”(排除自己及子孙防成环)
    parentOptions() {
      const opts = [{value: 0, label: '🏠 根菜单'}]
      const selfId = Number(this.form.id) || 0
      const banned = this.descendantsOf(selfId)
      this.allMenus.forEach(x => {
        const id = Number(x.id)
        if (id === selfId || banned.has(id)) return
        // 只有“文件夹”(菜单导航·展开子菜单)点击后才会渲染子级页,才能当上级;其余类型挂了子菜单也显示不出来
        if (!this.isFolder(x)) return
        opts.push({value: id, label: `${x.title}（ID:${id}）`})
      })
      return opts
    },
  },
  mounted() {
    this.loadMenus()
  },
  methods: {
    emptyForm() {
      return {
        id: '', parent_id: 0, lang: 'all', title: '', action_type: 'menu',
        cols: 2, sort: 0, status: 1,
      }
    },
    // 拉全量菜单
    async loadMenus() {
      this.loading = true
      try {
        const res = await getTgMenuList({})
        if (res.data.code === 0) {
          this.allMenus = res.data.data || []
          this.expandAll() // 默认展开全部层级(含保存/刷新后重拉也保持展开)
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      } finally {
        this.loading = false
      }
    },
    // 展开全部含子节点的菜单
    expandAll() {
      const withKids = new Set()
      this.allMenus.forEach(x => { const p = Number(x.parent_id); if (p) withKids.add(p) })
      this.expandedIds = [...withKids]
    },
    // 扁平列表 → 树(按 sort 排序)
    buildTree(list) {
      const map = {}
      list.forEach(x => { map[Number(x.id)] = {...x, children: []} })
      const roots = []
      list.forEach(x => {
        const node = map[Number(x.id)]
        const pid = Number(x.parent_id)
        if (pid && map[pid]) map[pid].children.push(node)
        else roots.push(node)
      })
      const sortFn = (a, b) => (a.sort - b.sort) || (a.id - b.id)
      const walk = arr => { arr.sort(sortFn); arr.forEach(n => n.children && walk(n.children)) }
      walk(roots)
      return roots
    },
    // 某节点的子孙 id 集合
    descendantsOf(id) {
      const set = new Set()
      if (!id) return set
      const childrenByParent = {}
      this.allMenus.forEach(x => {
        const p = Number(x.parent_id)
        if (!childrenByParent[p]) childrenByParent[p] = []
        childrenByParent[p].push(Number(x.id))
      })
      const stack = [id]
      while (stack.length) {
        const cur = stack.pop()
        const kids = childrenByParent[cur] || []
        kids.forEach(k => { if (!set.has(k)) { set.add(k); stack.push(k) } })
      }
      return set
    },
    parentLabel(pid) {
      pid = Number(pid)
      if (!pid) return '🏠 根菜单'
      const p = this.menuMap[pid]
      return p ? p.title : ('(已删除 #' + pid + ')')
    },
    actionTagType(t) {
      return {menu: '', text: 'success', url: '', handler: 'danger', http: '', back: 'info', cancel: 'danger', pick_user: 'warning'}[t] || 'info'
    },
    // 动作类型的中文标签
    actionLabel(t) {
      return {menu: '菜单导航', text: '显示文字', url: '打开链接', handler: '动态数据', http: '接口取数', back: '返回', cancel: '取消', pick_user: '选择用户'}[t] || t
    },
    // 解析某行的 action_config(容错)
    parseCfg(row) {
      try { return row.action_config ? JSON.parse(row.action_config) : {} } catch (e) { return {} }
    },
    // 是否“文件夹”(菜单导航且展开本级子菜单):只有它点击后才会渲染子级页,才谈得上挂子菜单/每行列数
    isFolder(row) {
      return row.action_type === 'menu' && this.parseCfg(row).page !== 'root'
    },
    // 某菜单的直接子项数量
    childCount(id) {
      id = Number(id)
      return this.allMenus.filter(x => Number(x.parent_id) === id).length
    },
    // 把原始 JSON 配置翻译成“点击后会发生什么”
    configSummary(row) {
      let cfg = {}
      try { cfg = row.action_config ? JSON.parse(row.action_config) : {} } catch (e) { return row.action_config || '—' }
      switch (row.action_type) {
        case 'menu': {
          if (cfg.page === 'root') return '↩️ 返回根主菜单'
          let s = '📂 展开它的子菜单'
          if (cfg.text) s += '｜引导语：' + cfg.text
          if (cfg.image) s += ' 🖼️含配图'
          return s
        }
        case 'back':
          return cfg.target === 'root' ? '🏠 返回根主菜单' : '⬅️ 返回上一级'
        case 'cancel':
          return '🗑️ 取消并删除本条消息'
        case 'text': {
          if (!cfg.text && !cfg.image) return '—'
          return (cfg.image ? '🖼️ ' : '') + '💬 ' + (cfg.text || '(仅配图)')
        }
        case 'url':
          return cfg.url ? ('🔗 打开链接：' + cfg.url) : '—'
        case 'handler':
          return '⚙️ 调用数据：' + (cfg.handler || '—') + (cfg.args ? ('（参数 ' + cfg.args + '）') : '')
        case 'http': {
          const m = (cfg.method || 'GET').toUpperCase()
          const tail = cfg.list_path ? ('列表 @' + cfg.list_path) : (cfg.text || '文案')
          return '🌐 调接口：' + m + ' ' + (cfg.url || '—') + ' → ' + tail
        }
        case 'pick_user': {
          const m = (cfg.method || 'GET').toUpperCase()
          let s = '👥 选用户'
          if (cfg.feedback_text) s += '｜提示：' + cfg.feedback_text
          if (cfg.url) s += '｜调接口：' + m + ' ' + cfg.url
          return s
        }
        default:
          return '—'
      }
    },
    search() { /* 前端过滤,computed 自动响应 */ },
    reset() {
      this.where.title = ''
      this.where.action_type = ''
    },
    toggleExpand() {
      if (this.expandedIds.length) {
        this.expandedIds = []   // 折叠全部
      } else {
        this.expandAll()         // 展开全部(仅含子节点的)
      }
    },
    isExpanded(row) {
      return this.expandedIds.includes(Number(row.id))
    },
    toggleNode(row) {
      const id = Number(row.id)
      const i = this.expandedIds.indexOf(id)
      if (i >= 0) this.expandedIds.splice(i, 1)
      else this.expandedIds.push(id)
    },
    handleSelectionChange(val) {
      this.multipleSelection = val.map(item => item.id)
    },
    add(parentId) {
      this.form = this.emptyForm()
      this.form.parent_id = Number(parentId) || 0
      this.cfg = {page: '', target: 'parent', text: '', url: '', image: '', image_path: '', format: 'html', method: 'GET', headers: '', body: '', list_path: '', btn_text: '', btn_url: '', input_prompt: '', feedback_text: ''}
      this.dialogVisible = true
    },
    edit(row) {
      this.form = {
        id: row.id, parent_id: Number(row.parent_id), lang: row.lang, title: row.title,
        action_type: row.action_type, cols: row.cols, sort: row.sort, status: row.status,
      }
      this.cfg = {page: '', target: 'parent', text: '', url: '', image: '', image_path: '', format: 'html', method: 'GET', headers: '', body: '', list_path: '', btn_text: '', btn_url: '', input_prompt: '', feedback_text: ''}
      let parsed = {}
      try {
        parsed = row.action_config ? JSON.parse(row.action_config) : {}
      } catch (e) { parsed = {} }
      this.cfg.page = parsed.page || ''
      this.cfg.text = parsed.text || ''
      this.cfg.url = parsed.url || ''
      this.cfg.method = parsed.method || 'GET'
      this.cfg.headers = parsed.headers || ''
      this.cfg.body = parsed.body || ''
      this.cfg.list_path = parsed.list_path || ''
      this.cfg.image_path = parsed.image_path || ''
      this.cfg.input_prompt = parsed.input_prompt || ''
      this.cfg.btn_text = parsed.btn_text || ''
      this.cfg.btn_url = parsed.btn_url || ''
      this.cfg.target = parsed.target || 'parent'
      this.cfg.image = parsed.image || ''
      this.cfg.format = parsed.format || 'html'
      this.cfg.feedback_text = parsed.feedback_text || ''
      this.dialogVisible = true
    },
    onActionTypeChange() {
      this.cfg = {page: '', target: 'parent', text: '', url: '', image: '', image_path: '', format: 'html', method: 'GET', headers: '', body: '', list_path: '', btn_text: '', btn_url: '', input_prompt: '', feedback_text: ''}
    },
    // ==================== 富文本工具栏(仅包 Telegram 支持的标签,输出仍是 HTML 字符串) ====================
    // 取 el-input 内部真实 textarea/input DOM(才能拿到光标选区)
    _richEl(refName) {
      const comp = this.$refs[refName]
      if (!comp || !comp.$el) return null
      return comp.$el.querySelector('textarea') || comp.$el.querySelector('input')
    },
    // 从图片库选图:回填 file:<file_id>,发送端会直接用 tg.FileID 秒发
    // http 模式选图回填到 image_path(后端遇 file: 前缀直接当 FileID),其余回填 cfg.image
    openPicker(target) {
      this.pickerTarget = target || 'image'
      this.$refs.picker.show()
    },
    onPickImage(val) {
      if (this.pickerTarget === 'http') { this.cfg.image_path = val }
      else { this.cfg.image = val }
    },
    // 在选区两侧包一对标签(如 <b>...</b>);无选区则插入占位文字
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
    // 插入 <a href="url">选中文字</a>
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
    // 在光标处插入一个空行(段落分隔):Telegram 不支持行距,只能靠空行把长段落拆成几小段拉开间距
    insertBreak(refName) {
      const el = this._richEl(refName)
      if (!el) return
      const end = el.selectionEnd || 0
      const val = this.cfg.text || ''
      this.cfg.text = val.slice(0, end) + '\n\n' + val.slice(end)
      this.$nextTick(() => {
        el.focus()
        const p = end + 2
        try { el.setSelectionRange(p, p) } catch (e) {}
      })
    },
    buildActionConfig() {
      const t = this.form.action_type
      if (t === 'menu') return JSON.stringify({text: this.cfg.text, image: this.cfg.image, format: this.cfg.format})
      if (t === 'back') return JSON.stringify({target: this.cfg.target || 'parent'})
      if (t === 'cancel') return JSON.stringify({})
      if (t === 'text') return JSON.stringify({text: this.cfg.text, image: this.cfg.image, format: this.cfg.format})
      if (t === 'url') return JSON.stringify({url: this.cfg.url})
      if (t === 'pick_user') return JSON.stringify({
        feedback_text: this.cfg.feedback_text,
        method: this.cfg.method || 'GET', url: this.cfg.url, headers: this.cfg.headers,
        body: this.cfg.body, text: this.cfg.text,
        image_path: this.cfg.image_path,
        list_path: this.cfg.list_path, btn_text: this.cfg.btn_text, btn_url: this.cfg.btn_url,
        cols: String(this.form.cols || 1), format: this.cfg.format || '',
      })
      if (t === 'http') return JSON.stringify({
        method: this.cfg.method || 'GET', url: this.cfg.url, headers: this.cfg.headers,
        body: this.cfg.body, text: this.cfg.text,
        image_path: this.cfg.image_path,
        input_prompt: this.cfg.input_prompt,
        list_path: this.cfg.list_path, btn_text: this.cfg.btn_text, btn_url: this.cfg.btn_url,
        cols: String(this.form.cols || 1), format: this.cfg.format || '',
      })
      return '{}'
    },
    async save() {
      if (!this.form.title || !this.form.title.trim()) {
        this.$message.warning("请输入菜单名称"); return
      }
      if (this.form.action_type === 'url' && !this.cfg.url) {
        this.$message.warning("请填写链接地址"); return
      }
      if (this.form.action_type === 'http' && !this.cfg.url && !this.cfg.text) {
        this.$message.warning("接口地址与显示文案至少填一个（只填文案则直接返回模板、不发请求）"); return
      }
      // 逻辑校验(仅对已存在菜单):①空文件夹点开是空页 ②把有子项的文件夹改成非文件夹会隐藏子项
      if (this.form.id) {
        const nowFolder = this.form.action_type === 'menu' && this.cfg.page !== 'root'
        const kids = this.childCount(this.form.id)
        if (nowFolder && kids === 0) {
          try {
            await this.$confirm('这个菜单设为「展开子菜单」，但它下面还没有任何子菜单，用户点开会是没有按钮的空页。建议先给它「添加子菜单」，或把「点击行为」改成「返回」（回上一级/根菜单）。仍要保存吗？', '逻辑提醒', {type: 'warning', confirmButtonText: '仍要保存', cancelButtonText: '返回修改'})
          } catch (e) { return }
        } else if (!nowFolder && kids > 0) {
          // 硬拦截:改了行为后子菜单在 TG 端永远点不到成死数据,必须先删除或改挂子菜单
          this.$message.error('无法保存：这个菜单下还有 ' + kids + ' 个子菜单，改成其他点击行为后它们将无法被点到。请先删除这些子菜单，或编辑子菜单把它们改挂到其他「菜单导航」下，再来修改。')
          return
        }
      }
      const payload = {...this.form, action_config: this.buildActionConfig()}
      try {
        const addOrSave = this.form.id ? saveTgMenu : addTgMenu
        const res = await addOrSave(payload)
        if (res.data.code === 0) {
          this.$message.success(res.data.message)
          this.dialogVisible = false
          await this.loadMenus()
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
        const res = await saveTgMenu(payload)
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
    // 表格内直接改排序:仅提交 id+sort 局部更新,成功后重拉以按新 sort 重排树序
    async saveSort(row) {
      const payload = {id: row.id, sort: row.sort}
      try {
        const res = await saveTgMenu(payload)
        if (res.data.code === 0) {
          await this.loadMenus()
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      }
    },
    async del() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning("请选择要删除的菜单"); return
      }
      this.$confirm('确定删除所选菜单吗？若菜单下仍有子菜单，需先删除子菜单后才能删上级。').then(async _ => {
        const ids = this.multipleSelection.join(',')
        try {
          const res = await delTgMenu(ids)
          if (res.data.code === 0) {
            this.$message.success(res.data.message)
            await this.loadMenus()
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
/* 名称列:去掉单元格内边距,让树形连接线能撑满整行高度 */
.tableData ::v-deep td.col-title {
  padding: 0;
}
.tableData ::v-deep td.col-title .cell {
  padding: 0;
  height: 100%;
}
.tree-cell {
  display: flex;
  align-items: stretch;
  height: 100%;
  min-height: 40px;
  padding-left: 8px;
}
/* 树形连接线:主干竖线 */
.tree-guide {
  position: relative;
  width: 18px;
  flex: 0 0 18px;
}
.tree-guide--vline::before {
  content: '';
  position: absolute;
  left: 9px;
  top: 0;
  bottom: 0;
  width: 1px;
  background: #c0c4cc;
}
/* 接本子项的折角:竖线从顶到中 + 横线接箭头;若非末个兄弟则竖线续到底(连向下一个兄弟) */
.tree-guide--elbow::before {
  content: '';
  position: absolute;
  left: 9px;
  top: 0;
  height: 50%;
  width: 1px;
  background: #c0c4cc;
}
.tree-guide--elbow.tree-guide--elbow-more::before {
  height: 100%;
}
.tree-guide--elbow::after {
  content: '';
  position: absolute;
  left: 9px;
  top: 0;
  bottom: 0;
  margin: auto 0; /* 上下 auto 居中:浏览器吸附到整像素,避免 top:50% 落在半像素被抗锯齿糊成2px */
  width: 26px;
  height: 1px;
  background: #c0c4cc;
}
.tree-node {
  display: flex;
  align-items: center;
  padding-right: 8px;
  position: relative;
  z-index: 1; /* 盖在折角横线之上,避免延长后的横线压过箭头徽标 */
}
.tree-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-right: 8px;
  border-radius: 6px;
  background: #409EFF;
  color: #fff;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.18s ease;
}
.tree-arrow:hover {
  background: #337ecc;
  color: #fff;
}
/* 展开态:变深(实心蓝底白字) */
.tree-arrow.is-open {
  background: #409EFF;
  color: #fff;
  box-shadow: 0 2px 6px rgba(64, 158, 255, 0.4);
}
.tree-arrow-hollow {
  display: inline-block;
  width: 28px;   /* 20 箭头宽 + 8 间距,保证叶子与文件夹标题左对齐 */
}
/* 根级叶子菜单(无子项、无展开箭头)不占箭头位,标题贴左,与根级文件夹箭头左缘齐平 */
.tree-arrow-hollow--root {
  width: 0;
}
.tree-title {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* “打开链接”(url)标签:浅蓝色(更淡) */
.tag-url {
  background-color: #f2f8ff;
  border-color: #d9ecff;
  color: #79b8ff;
}
/* 菜单导航(menu)标签:柔和玫红,与默认灰色区分 */
.tag-menu {
  background-color: #fef0f0;
  border-color: #fbc4c4;
  color: #e0686d;
}
/* 接口取数(http)标签:区别于菜单导航的紫色,避免与灰色/蓝色混淆 */
.tag-http {
  background-color: #f2e9ff;
  border-color: #ddc6f7;
  color: #7a3ff0;
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
