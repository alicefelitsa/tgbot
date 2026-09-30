import request from './request'

// 图片库列表(名称模糊、分类精确、page/limit 分页)
export function getTgImageList(params) {
    return request.get('/GetTgImageList', { params })
}

// 图片入库:{ name, tag, image_base64? 或 url? }
export function addTgImage(data) {
    return request.post('/AddTgImage', data)
}

// 修改图片记录(名称/分类/状态)
export function saveTgImage(data) {
    return request.post('/SaveTgImage', data)
}

// 删除图片(逗号 ids)
export function delTgImage(ids) {
    return request.get('/DelTgImage', { params: { ids } })
}

// 预览图:按 id 拿 Telegram file_id 对应的图片,返回 { mime, base64 }
export function getTgImagePreview(id) {
    return request.get('/GetTgImagePreview', { params: { id } })
}
