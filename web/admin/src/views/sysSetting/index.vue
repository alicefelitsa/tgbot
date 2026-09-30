<template>
  <div class="content">
    <el-card shadow="always">
      <div slot="header" class="setting-header">
        <span>系统设置</span>
      </div>
      <div class="setting-form" v-loading="loading">
        <el-form ref="form" :model="form" label-width="90px" @submit.native.prevent>
          <template v-for="f in fields">
            <!-- 单行输入 -->
            <el-form-item v-if="f.type === 'input'" :key="f.key" :label="f.label">
              <el-input v-model="form[f.key]" :placeholder="f.placeholder"></el-input>
              <div v-if="f.tip" class="setting-tip setting-tip-block">{{ f.tip }}</div>
            </el-form-item>

            <!-- 多行输入 -->
            <el-form-item v-else-if="f.type === 'textarea'" :key="f.key" :label="f.label">
              <el-input v-model="form[f.key]" type="textarea" :rows="3" :placeholder="f.placeholder"></el-input>
              <div v-if="f.tip" class="setting-tip setting-tip-block">{{ f.tip }}</div>
            </el-form-item>

            <!-- 数字 -->
            <el-form-item v-else-if="f.type === 'number'" :key="f.key" :label="f.label">
              <el-input-number v-model="form[f.key]" :min="0" controls-position="right" style="width: 120px;"></el-input-number>
              <span v-if="f.tip" class="setting-tip">{{ f.tip }}</span>
            </el-form-item>

            <!-- 单选 -->
            <el-form-item v-else-if="f.type === 'radio'" :key="f.key" :label="f.label">
              <el-radio-group v-model="form[f.key]">
                <el-radio v-for="o in f.options" :key="o.value" :label="o.value">{{ o.label }}</el-radio>
              </el-radio-group>
              <div v-if="f.tip" class="setting-tip setting-tip-block">{{ f.tip }}</div>
            </el-form-item>
          </template>

          <el-form-item class="setting-submit">
            <el-button type="primary" icon="el-icon-check" :loading="saving" @click="save">保存</el-button>
            <span class="setting-tip">修改后立即生效，无需重启服务</span>
          </el-form-item>
        </el-form>
      </div>
    </el-card>
  </div>
</template>

<script>
import {getSysSettingMap, saveSysSettingBatch} from "@/api/sysSetting";

export default {
  name: "SysSetting",
  data() {
    return {
      loading: false,
      saving: false,
      // 固定参数注册表:新增一项配置 = 往这里加一条(与后端读取的 skey 对齐即可)
      fields: [
        {
          key: 'imageRelayChatID',
          label: '中转 ChatID',
          type: 'input',
          placeholder: '例如 7569435732',
          name: '图片库中转 ChatID',
          remark: '入库时把图发给它换取 Telegram file_id，再删除该消息',
          tip: '入库图片时先发给这个 chat 换取 Telegram file_id，随后自动删除该消息；须填 bot 能主动发消息的对象（一般是你自己的私聊 chat_id）。',
        },
      ],
      form: {},
    }
  },
  created() {
    // 先给每个字段建响应式默认值,再拉取已存值回填
    const f = {}
    this.fields.forEach(item => { f[item.key] = item.type === 'number' ? 0 : '' })
    this.form = f
    this.getSetting()
  },
  methods: {
    //获取系统设置
    async getSetting() {
      this.loading = true
      try {
        const res = await getSysSettingMap()
        if (res.data.code === 0) {
          const map = res.data.data || {}
          this.fields.forEach(item => {
            if (map[item.key] !== undefined) {
              this.$set(this.form, item.key, item.type === 'number' ? (Number(map[item.key]) || 0) : map[item.key])
            }
          })
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      } finally {
        this.loading = false
      }
    },
    //保存系统设置
    async save() {
      this.saving = true
      try {
        const items = this.fields.map(item => ({
          skey: item.key,
          svalue: String(this.form[item.key] ?? ''),
          name: item.name || item.label,
          remark: item.remark || '',
        }))
        const res = await saveSysSettingBatch(items)
        if (res.data.code === 0) {
          this.$message.success(res.data.message)
        } else {
          this.$message.error(res.data.message)
        }
      } catch (e) {
        this.$message.error(e.message)
      } finally {
        this.saving = false
      }
    }
  }
}
</script>

<style scoped>
/* 标题行：行高32px + 底部10px留白 + 字号14px */
.setting-header {
  margin-bottom: 10px;
}

.setting-header span {
  display: inline-block;
  line-height: 32px;
  font-size: 14px;
  font-weight: normal;
  color: #606266;
}

/* 表单定宽单列，避免输入框过宽拉伸 */
.setting-form {
  max-width: 560px;
  margin-top: 10px;
}

/* 单选文字加深突出（选中态仍保留主题蓝） */
.setting-form >>> .el-radio__label {
  color: #303133;
}

.setting-form >>> .el-form-item {
  margin-bottom: 20px;
}

/* 保存按钮与说明文字同行，紧凑收尾 */
.setting-submit {
  margin-top: 8px;
  margin-bottom: 0;
}

.setting-tip {
  margin-left: 12px;
  font-size: 12px;
  color: #909399;
}

/* 提示语单独一行置于控件下方：去掉左缩进并换行 */
.setting-tip-block {
  display: block;
  margin-left: 0;
  margin-top: 2px;
  line-height: 1.6;
}
</style>
