import request from './request'

// 设置项 key→value 映射(固定表单页回填用)
export function getSysSettingMap() {
    return request.get('/GetSysSettingMap')
}

// 批量保存固定设置项:{ items:[{ skey, svalue, name, remark }] }
export function saveSysSettingBatch(items) {
    return request.post('/SaveSysSettingBatch', { items })
}

// 机器人资料:从 Telegram 实时拉取当前名称/短简介/简介
export function getBotProfile() {
    return request.get('/GetBotProfile')
}

// 机器人资料:保存并同步到 Telegram { name, shortDescription, description }
export function saveBotProfile(data) {
    return request.post('/SaveBotProfile', data)
}
