import request from './request'

// 获取命令列表
export function getTgCommandList(params) {
    return request.get('/GetTgCommandList', { params })
}

// 新增命令
export function addTgCommand(data) {
    return request.post('/AddTgCommand', data)
}

// 修改命令
export function saveTgCommand(data) {
    return request.post('/SaveTgCommand', data)
}

// 删除命令(逗号分隔 ids)
export function delTgCommand(ids) {
    return request.get('/DelTgCommand', { params: { ids } })
}

// 同步命令到 Telegram 原生「菜单」按钮
export function syncTgCommand() {
    return request.post('/SyncTgCommand')
}
