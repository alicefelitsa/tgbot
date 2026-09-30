import request from './request'

// 获取群组/频道列表(bot 被拉进群时自动落库;支持 title 模糊、type 精确、page/limit 分页)
export function getTgChatList(params) {
    return request.get('/GetTgChatList', { params })
}

// 修改群组(目前仅 status:人工修正失效/恢复可推送记录)
export function saveTgChat(data) {
    return request.post('/SaveTgChat', data)
}

// 删除群组记录(逗号分隔 ids)
export function delTgChat(ids) {
    return request.get('/DelTgChat', { params: { ids } })
}

// 主动往指定群组发消息(群发):{ ids: "1,2,3", text, image?, format? }
export function sendTgChatMessage(data) {
    return request.post('/SendTgChatMessage', data)
}
