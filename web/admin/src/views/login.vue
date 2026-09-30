<template>
  <div class="account-login-container">
    <!-- 背景效果 -->
    <div class="background-effects">
      <div class="gradient-bg"></div>
      <div class="floating-element floating-1"></div>
      <div class="floating-element floating-2"></div>
      <div class="floating-element floating-3"></div>
    </div>

    <!-- 登录主容器 -->
    <div class="login-main">
      <div class="form-section">
        <div class="form-header">
          <h2 style="color: #368a88;">机器人管理后台</h2>
        </div>

        <!-- 登录表单 -->
        <el-form
            ref="loginForm"
            :model="form"
            :rules="rules"
            label-width="0"
            class="account-form"
            @submit.native.prevent
            @keyup.enter.native="handleLogin"
        >
          <el-form-item prop="account">
            <el-input
                v-model="form.account"
                placeholder="请输入账号"
                size="medium"
                clearable
                class="account-input"
            >
              <template slot="prepend">
                <span class="input-label">账号</span>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item prop="password">
            <el-input
                v-model="form.password"
                placeholder="请输入密码"
                size="medium"
                clearable
                class="password-input"
                type="password"
            >
              <template slot="prepend">
                <span class="input-label">密码</span>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item>
            <el-button
                type="primary"
                :loading="loading"
                @click="handleLogin"
                class="login-btn"
            >
              {{ loading ? '登录中...' : '立即登录' }}
            </el-button>
          </el-form-item>
        </el-form>

        <div class="agreement">
          <p>登录即表示您同意
            <el-link type="primary" :underline="false">用户协议</el-link>
            和
            <el-link type="primary" :underline="false">隐私政策</el-link>
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {login} from "@/api/login";

export default {
  name: 'Login',
  data() {
    return {
      form: {
        account: '',
        password: ''
      },
      rules: {
        account: [
          {required: true, message: '请输入账号', trigger: 'blur'}
        ],
        password: [
          {required: true, message: '请输入密码', trigger: 'blur'}
        ]
      },
      loading: false,
    }
  },
  methods: {
    handleLogin() {
      this.$refs.loginForm.validate((valid) => {
        if (!valid) return false
        this.loading = true
        login(this.form).then((msg) => {
          this.loading = false;
          this.$message.success(msg);
          this.$router.push('/')
        }).catch((e) => {
          this.loading = false;
          this.$message.error(e.message);
        });
      })
    },
  }
}
</script>

<style scoped>
.account-login-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  position: relative;
  overflow: hidden;
  background: #f5f7fa;
}

.background-effects {
  position: absolute;
  width: 100%;
  height: 100%;
  top: 0;
  left: 0;
  z-index: 0;
}

.gradient-bg {
  position: absolute;
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #1890ff 0%, #52c41a 100%);
  opacity: 0.1;
}

.floating-element {
  position: absolute;
  border-radius: 50%;
  opacity: 0.3;
  animation: float 20s infinite linear;
}

.floating-1 {
  width: 300px;
  height: 300px;
  background: linear-gradient(135deg, #1890ff, transparent 70%);
  top: 10%;
  right: 10%;
}

.floating-2 {
  width: 250px;
  height: 250px;
  background: linear-gradient(135deg, #52c41a, transparent 70%);
  bottom: 20%;
  left: 10%;
  animation-delay: 7s;
  animation-duration: 18s;
}

.floating-3 {
  width: 200px;
  height: 200px;
  background: linear-gradient(135deg, #13c2c2, transparent 70%);
  top: 60%;
  right: 20%;
  animation-delay: 14s;
  animation-duration: 22s;
}

@keyframes float {
  0%, 100% { transform: translate(0, 0) rotate(0deg); }
  25% { transform: translate(20px, 20px) rotate(90deg); }
  50% { transform: translate(-20px, 10px) rotate(180deg); }
  75% { transform: translate(10px, -20px) rotate(270deg); }
}

.login-main {
  display: flex;
  max-width: 1000px;
  width: 40%;
  border-radius: 20px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.1) inset, 0 1px 2px rgba(255, 255, 255, 0.2) inset;
  overflow: hidden;
  z-index: 1;
  margin: 20px;
  animation: slideUp 0.8s ease-out;
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(30px); }
  to { opacity: 1; transform: translateY(0); }
}

.form-section {
  flex: 1;
  padding: 60px 50px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.form-header {
  text-align: center;
  margin-bottom: 40px;
}

.form-header h2 {
  font-size: 32px;
  margin-bottom: 10px;
  font-weight: 600;
}

.account-form {
  max-width: 400px;
  margin: 0 auto;
  width: 100%;
}

.account-input >>> .el-input-group__prepend {
  background: linear-gradient(135deg, #1890ff 0%, #52c41a 100%);
  border: none;
  color: white;
  width: 40px;
  justify-content: center;
  font-weight: 600;
}

.account-input >>> .el-input__inner {
  border-left: none;
  padding-left: 15px;
  height: 50px;
  font-size: 16px;
}

.password-input >>> .el-input-group__prepend {
  background: linear-gradient(135deg, #ea1d43 0%, #52c41a 100%);
  border: none;
  color: white;
  width: 40px;
  justify-content: center;
  font-weight: 600;
}

.password-input >>> .el-input__inner {
  border-left: none;
  padding-left: 15px;
  height: 50px;
  font-size: 16px;
}

.login-btn {
  width: 100%;
  height: 50px;
  background: linear-gradient(135deg, #1890ff 0%, #52c41a 100%);
  border: none;
  color: white;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 2px;
  border-radius: 8px;
  margin-top: 20px;
}

.login-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(24, 144, 255, 0.3);
}

.agreement {
  text-align: center;
  margin-top: 30px;
  font-size: 12px;
  color: #999;
}

@media (max-width: 768px) {
  .login-main {
    flex-direction: column;
    width: 95%;
  }
  .form-section {
    padding: 40px 20px;
  }
}
</style>
