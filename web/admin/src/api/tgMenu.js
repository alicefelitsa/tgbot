import request from './request'

// 获取菜单列表(params: parent_id / title / page / limit)
export function getTgMenuList(params) {
    return request.get('/GetTgMenuList', { params })
}

// 新增菜单
export function addTgMenu(data) {
    return request.post('/AddTgMenu', data)
}

// 修改菜单
export function saveTgMenu(data) {
    return request.post('/SaveTgMenu', data)
}

// 删除菜单(逗号分隔 ids)
export function delTgMenu(ids) {
    return request.get('/DelTgMenu', { params: { ids } })
}

// 数据类白名单列表(菜单编辑 action_type=handler 时的下拉)
export function getTgHandlerList(params) {
    return request.get('/GetTgHandlerList', { params })
}
