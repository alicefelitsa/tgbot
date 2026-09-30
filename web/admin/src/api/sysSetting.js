import request from './request'

// 设置项 key→value 映射(固定表单页回填用)
export function getSysSettingMap() {
    return request.get('/GetSysSettingMap')
}

// 批量保存固定设置项:{ items:[{ skey, svalue, name, remark }] }
export function saveSysSettingBatch(items) {
    return request.post('/SaveSysSettingBatch', { items })
}
