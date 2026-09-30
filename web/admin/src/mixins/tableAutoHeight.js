// 列表页 el-table 高度自适应 mixin
// 作用:让表格高度在运行时动态计算,精确铺满窗口底部且不产生「页面级」滚动条
//      (表格行太多时仍保留它自身的内部滚动,表头固定)。
// 用法:列表页 `mixins: [tableAutoHeight]`,并把 el-table 写成 ref="table" :height="tableHeight"。
// 原理:
//   1) 表格顶部到视口顶的距离用 getBoundingClientRect().top 实量(自动含顶栏/搜索栏/工具栏高度,
//      窗口缩放或搜索栏换行都能自适应),不再靠 calc(100vh - 固定px) 猜偏移。
//   2) 表格「之后的同父兄弟元素」(如底部分页条 .currentPage)的高度也实量并累加,故带不带分页都自适应。
//   3) 底部固定留白 = el-card body 下内边距(10) + 卡片下外边距(10) + 阴影余量(4) = 24。
export default {
  data() {
    return {
      tableHeight: 400, // 占位初值,mounted 后由 setTableHeight 重算
    }
  },
  mounted() {
    this.setTableHeight()
    window.addEventListener('resize', this.setTableHeight)
  },
  beforeDestroy() {
    window.removeEventListener('resize', this.setTableHeight)
  },
  methods: {
    setTableHeight() {
      this.$nextTick(() => {
        const t = this.$refs.table
        if (!t || !t.$el) return
        const el = t.$el
        const top = el.getBoundingClientRect().top
        // 累加表格之后、同一父容器内的所有兄弟元素高度(含上下 margin),典型为分页条
        let below = 0
        let sib = el.nextElementSibling
        while (sib) {
          const r = sib.getBoundingClientRect()
          const cs = window.getComputedStyle(sib)
          below += r.height + (parseFloat(cs.marginTop) || 0) + (parseFloat(cs.marginBottom) || 0)
          sib = sib.nextElementSibling
        }
        this.tableHeight = Math.max(240, window.innerHeight - top - below - 24)
      })
    },
  },
}
