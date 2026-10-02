<template>
  <div id="app">
    <router-view/>
  </div>
</template>

<style lang="less">
#app {
  font-family: "Helvetica Neue", Helvetica, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "微软雅黑", Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  background-color: #ededed;
}

body {
  margin: 0;
}

input, textarea, select, button {
  font-family: "Helvetica Neue", Helvetica, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "微软雅黑", Arial, sans-serif;
}

.queryForm .el-form-item__label {
  font-size: 13px !important;
}

.el-form-item__label {
  font-size: 14px !important;
}

.el-select {
  width: 100%;
}

.el-main {
  padding: 0 !important;
}

.el-card {
  margin: 10px !important;
}

.el-card__header {
  padding: 10px 20px 0 !important;
}

.el-card__body {
  padding: 10px !important;
}

.el-table {
  font-size: 14px !important;
}

.el-table thead {
  color: #555555 !important;
}

.el-table th {
  background-color: #fafafa !important;
}

.el-table th.el-table__cell > .cell {
  padding-right: 14px !important;
}

.el-table__body tr:hover > td {
  background-color: #fafafa !important;
}

.menu {
  overflow-y: visible !important;
}

.content {
  width: 100%;
}

.el-dialog__title {
  font-size: 16px !important;
}

.el-dialog__header {
  padding: 15px !important;
  border-bottom: 1px solid #eee;
  flex-shrink: 0; /* 限高时不被压缩,标题始终完整可见 */
}

/* 笔记本等矮屏兼容:弹窗整体限高 86vh(最大 top 12vh + 86vh + 底部留白 < 100vh),
   改 flex 纵向布局让 body 内部滚动,标题栏与底部按钮始终可见,不再撑出页面滚动条 */
.el-dialog {
  display: flex;
  flex-direction: column;
  max-height: 86vh;
  margin-bottom: 0 !important; /* Element 默认底部 50px 外边距会把 wrapper 撑出页面滚动条(弹窗限高后属多余) */
}

.el-dialog__body {
  padding: 30px 20px 10px !important;
  flex: 1 1 auto;
  min-height: 0; /* flex 子项允许收缩,否则内容超高时撑破不出滚动条 */
  overflow-y: auto;
}

.el-dialog__footer {
  padding: 15px !important;
  border-top: 1px solid #eee;
  flex-shrink: 0; /* 限高时不被压缩,确定/取消按钮始终可见 */
}

.query-form-inline > .el-form-item {
  vertical-align: baseline !important;
  margin-bottom: 10px !important;
  margin-right: 12px !important;
}

.query-form-inline > .el-form-item > .el-form-item__content {
  vertical-align: baseline;
}

.toolbar {
  display: inline-block !important;
}

.toolbar > .el-button {
  margin-bottom: 10px;
  margin-right: 10px;
  margin-left: 0 !important;
}

.tableData {
  width: 100% !important;
}

.queryElInput {
  width: 132px !important;
}

.el-image {
  overflow: clip !important;
}

/* 顶部轻提示(如长文案报错):Element 默认行高≈字号,长文案换行后挤成一块;加行距减轻阅读负担 */
.el-message__content {
  line-height: 1.7 !important;
}

/* 渐隐渐现的极细滚动条 */
.el-aside::-webkit-scrollbar {
  width: 2px !important;
  height: 2px !important;
}

.el-aside::-webkit-scrollbar-track {
  background: linear-gradient(to bottom,
  transparent 0%,
  rgba(0, 0, 0, 0.02) 20%,
  rgba(0, 0, 0, 0.02) 80%,
  transparent 100%) !important;
  border-radius: 0 !important;
}

.el-aside::-webkit-scrollbar-thumb {
  background: linear-gradient(to bottom,
  transparent 0%,
  rgba(0, 0, 0, 0.15) 30%,
  rgba(0, 0, 0, 0.2) 50%,
  rgba(0, 0, 0, 0.15) 70%,
  transparent 100%) !important;
  border-radius: 0 !important;
  min-height: 40px !important;
}

/* 手机模式：屏幕宽度 ≤ 768px */
@media screen and (max-width: 768px) {

  .el-aside {
    display: none;
  }

  .responsive-dialog .el-dialog {
    width: 100% !important;
    max-width: 100% !important;
  }

  /* 手机模式:弹窗内两列表单字段堆成单列,避免过挤 */
  .responsive-dialog .el-col {
    width: 100% !important;
  }

  .currentPage .el-pagination {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 5px;
  }

  .currentPage .el-pagination__total,
  .currentPage .el-pagination__sizes,
  .currentPage .el-pagination__jump {
    width: auto;
    text-align: center;
    margin-bottom: 5px;
  }

  .currentPage .el-pagination__sizes .el-input {
    width: 100px !important;
  }

  .currentPage .el-pagination__jump {
    margin-left: 10px;
  }

  .currentPage .el-pagination__jump .el-input {
    width: 50px !important;
  }

  .currentPage .el-pager {
    margin-bottom: 5px;
  }

  .el-message-box {
    width: 80% !important;
    vertical-align: super !important;
  }

  .systemMenu {
    display: block !important;
    position: fixed;
    top: 2%;
    right: 3%;
  }
}
</style>
