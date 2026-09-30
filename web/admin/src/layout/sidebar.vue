<template>
  <div class="sidebar-container">
    <div class="avatar-section">
      <el-popover
          placement="right-start"
          width="220"
          trigger="hover"
      >
        <div class="userInfo">账号：{{ account }}</div>
        <div class="userInfo">到期时间：永久</div>
        <el-avatar slot="reference" :size="55" :src="avatar"/>
      </el-popover>
    </div>
    <el-menu :default-active="$route.path" router style="border:0" class="menu">
      <el-menu-item
          v-for="item in menuList"
          :key="item.path"
          :index="item.path"
          class="vertical-menu-item"
      >
        <div class="menu-item-content">
          <i :class="item.icon" class="menu-icon"></i>
          <span class="menu-title">{{ item.title }}</span>
        </div>
      </el-menu-item>

      <el-menu-item
          class="vertical-menu-item logout-item"
          @click="handleLogout"
      >
        <div class="menu-item-content">
          <i class="el-icon-switch-button menu-icon"></i>
          <span class="menu-title">退出登录</span>
        </div>
      </el-menu-item>
    </el-menu>
  </div>
</template>

<script>
import {EventBus} from "@/utils/event-bus";

export default {
  data() {
    return {
      account: localStorage.getItem("account"),
      avatar: './static/avatar.jpg',
      menuList: [
        {path: '/tgMenu', title: '机器人菜单', icon: 'el-icon-menu'},
        {path: '/tgCommand', title: '命令菜单', icon: 'el-icon-s-operation'},
        {path: '/tgUser', title: 'TG用户', icon: 'el-icon-user-solid'},
        {path: '/tgChat', title: 'TG群组', icon: 'el-icon-s-custom'},
        {path: '/tgImage', title: '图片库', icon: 'el-icon-picture-outline'},
        {path: '/sysSetting', title: '系统设置', icon: 'el-icon-setting'},
      ],
    }
  },
  created() {
    EventBus.$on('sidebar', this.fetchData)
  },
  beforeDestroy() {
    EventBus.$off('sidebar', this.fetchData)
  },
  methods: {
    fetchData(status) {
      if (status) {
        Object.assign(this.$data, this.$options.data.call(this))
      }
    },
    handleLogout() {
      this.$confirm('确定要退出登录吗?', '提示', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(() => {
        localStorage.removeItem('token')
        localStorage.removeItem('account')
        this.$router.push('/login')
      }).catch(() => {
      })
    },
  }
}
</script>

<style scoped>
.vertical-menu-item {
  height: auto !important;
  line-height: normal !important;
  padding: 8px 20px !important;
  margin: 0 0 4px 0 !important;
}

.vertical-menu-item .menu-item-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.vertical-menu-item .menu-icon {
  font-size: 26px;
  margin-bottom: 4px;
  display: block;
}

.vertical-menu-item .menu-title {
  font-size: 12px;
  line-height: 1.2;
  white-space: normal;
  word-break: break-all;
}

.logout-item:hover {
  background-color: #f56c6c !important;
  color: #fff !important;
}

.logout-item:hover .menu-icon,
.logout-item:hover .menu-title {
  color: #fff !important;
}

.sidebar-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  height: 100vh;
  padding: 10px 0 0 0;
  background-color: #fff;
  box-sizing: border-box;
}

.avatar-section {
  margin-bottom: 10px;
}

.menu {
  width: 100%;
  flex: 1;
  overflow-y: auto;
}

.vertical-menu-item {
  display: flex;
  justify-content: center;
}

.menu-item-content {
  display: flex;
  align-items: center;
  gap: 4px;
}

.userInfo {
  margin-bottom: 5px;
}
</style>
