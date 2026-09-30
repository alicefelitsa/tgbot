<template>
  <el-dialog title="从图片库选择" :visible.sync="visible" width="820px" top="6vh"
             append-to-body class="responsive-dialog ip-dialog">
    <div class="ip-bar">
      <el-input v-model="kw" size="small" placeholder="按名称搜索图片库" clearable
                style="width:240px;" @input="onKwInput" @keyup.native.enter="doSearch" @clear="doSearch"></el-input>
      <span style="margin-left:12px;color:#888;font-size:12px;">点击图片即可插入到配图字段（以 file_id 复用、秒发）。</span>
    </div>
    <div v-loading="loading" class="ip-grid">
      <div v-for="(row, idx) in list" :key="row.id" class="ip-cell" @click="pick(row)">
        <div class="ip-thumb">
          <el-image v-if="row._thumb" :src="row._thumb" fit="cover" style="width:100%;height:100%;"></el-image>
          <span v-else class="ip-loading"><i class="el-icon-picture-outline"></i></span>
        </div>
        <div class="ip-name" :title="row.name">{{ row.name }}</div>
      </div>
      <div v-if="!loading && list.length === 0" class="ip-empty">图片库暂无图片，请先到「图片库」页面添加。</div>
    </div>
    <div style="margin-top:10px;text-align:center;" class="currentPage">
      <el-pagination small background layout="prev, pager, next" :page-size="limit" :total="total"
                     @current-change="onPage"></el-pagination>
    </div>
    <span slot="footer">
      <el-button size="small" @click="visible=false">关 闭</el-button>
    </span>
  </el-dialog>
</template>

<script>
import {getTgImageList, getTgImagePreview} from "@/api/tgImage";

export default {
  name: "ImagePicker",
  data() {
    return {
      visible: false,
      loading: false,
      kw: '',
      list: [],
      page: 1,
      limit: 15,
      total: 0,
    }
  },
  methods: {
    // 父组件通过 ref 调用打开
    show() {
      this.visible = true
      this.page = 1
      this.kw = ''
      this.load()
    },
    async load() {
      this.loading = true
      try {
        const res = await getTgImageList({name: this.kw, status: 1, page: this.page, limit: this.limit})
        if (res.data.code === 0) {
          this.list = (res.data.data || []).map(r => ({...r, _thumb: ''}))
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
    // 输入防抖搜索(避免每敲一个字就请求);回车/清空则立即搜
    onKwInput() {
      clearTimeout(this._kwTimer)
      this._kwTimer = setTimeout(() => this.doSearch(), 400)
    },
    doSearch() {
      clearTimeout(this._kwTimer)
      this.page = 1
      this.load()
    },
    loadThumbs() {
      this.list.forEach((row, idx) => {
        getTgImagePreview(row.id).then(res => {
          if (res.data.code === 0 && res.data.data) {
            const {mime, base64} = res.data.data
            this.$set(this.list, idx, {...this.list[idx], _thumb: `data:${mime};base64,${base64}`})
          }
        }).catch(() => {})
      })
    },
    onPage(p) {
      this.page = p
      this.load()
    },
    pick(row) {
      this.$emit('select', 'file:' + row.tg_file_id)
      this.visible = false
    },
  }
}
</script>

<style scoped>
.ip-bar {
  margin-bottom: 10px;
}
.ip-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  min-height: 200px;
  align-content: flex-start;
}
.ip-cell {
  width: calc((100% - 48px) / 5);
  box-sizing: border-box;
  cursor: pointer;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 6px;
  transition: all .2s;
}
.ip-cell:hover {
  border-color: #409EFF;
  box-shadow: 0 2px 8px rgba(64, 158, 255, .2);
}
.ip-thumb {
  width: 100%;
  height: 110px;
  border-radius: 4px;
  background: #f7f8fa;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.ip-loading {
  color: #c0c4cc;
  font-size: 26px;
}
.ip-name {
  margin-top: 6px;
  font-size: 12px;
  color: #606266;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ip-empty {
  color: #909399;
  font-size: 13px;
  width: 100%;
  text-align: center;
  line-height: 200px;
}
</style>
