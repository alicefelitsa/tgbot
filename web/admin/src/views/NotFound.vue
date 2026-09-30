<template>
  <div class="not-found">
    <h1>404 - 页面不存在</h1>
    <p>您访问的页面不存在或已被移除</p>
    <router-link to="/">返回首页</router-link>
  </div>
</template>

<script>
export default {
  name: 'NotFound'
}
</script>
<style scoped>
.not-found {
  width: 100%;
  background-color: #fff;
  text-align: center;
  padding-top: 100px;
}
</style>
