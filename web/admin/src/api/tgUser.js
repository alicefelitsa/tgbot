import request from './request'

// 获取 TG 用户列表(支持 username 模糊、tg_user_id 精确、page/limit 分页)
export function getTgUserList(params) {
    return request.get('/GetTgUserList', { params })
}

// 修改用户(绑定业务账号 / 语言 / 启停状态等)
export function saveTgUser(data) {
    return request.post('/SaveTgUser', data)
}

// 删除用户(逗号分隔 ids)
export function delTgUser(ids) {
    return request.get('/DelTgUser', { params: { ids } })
}

// 主动给指定用户发消息:{ id, text, image?, format? }
export function sendTgUserMessage(data) {
    return request.post('/SendTgUserMessage', data)
}
