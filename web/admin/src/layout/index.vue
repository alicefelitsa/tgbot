<template>
  <div>
    <el-container>
      <el-aside width="104px" style="background-color:#fff; border-right: 1px solid #eee;">
        <sidebar></sidebar>
      </el-aside>
      <el-main style="background-color:#fff;">
        <router-view></router-view>
      </el-main>
    </el-container>

    <el-button class="systemMenu" style="display: none" type="info" icon="el-icon-menu" circle @click="drawerVisible=true"></el-button>
    <!--抽屉-->
    <el-drawer
        title="菜单栏"
        :visible.sync="drawerVisible"
        direction="ltr"
        :with-header="false"
        size="23%"
    >
      <sidebar></sidebar>
    </el-drawer>

  </div>
</template>

<script>
import sidebar from "./sidebar";

export default {
  name: 'Layout',
  components: {
    sidebar,
  },
  data() {
    return {
      drawerVisible: false,
    }
  },
  watch: {
    $route() {
      this.drawerVisible = false;
    }
  },
  methods: {}
}
</script>

<style scoped>

</style>
