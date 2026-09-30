/**
 * axios 实例
 */
import axios from 'axios';
import router from "@/router";
import {Message} from "element-ui";

const service = axios.create({
    baseURL: apiUrl,
    timeout: 30000,
});

/**
 * 清除登录状态并跳转登录页
 */
function clearLoginState() {
    localStorage.removeItem('token');
    localStorage.removeItem("account");
    if (router.currentRoute.path !== '/login') {
        router.push({path: '/login'});
    }
}

/**
 * 添加请求拦截器
 */
service.interceptors.request.use(request => {
        const token = localStorage.getItem("token");
        if (token) {
            request.headers['Authorization'] = token;
        }
        return request;
    }, (error) => {
        return Promise.reject(error);
    }
);

/**
 * 添加响应拦截器
 */
service.interceptors.response.use(response => {
        if (response.data?.code === 401) {
            clearLoginState();
        }
        return response;
    }, (error) => {
        if (error.response && error.response.status === 401) {
            clearLoginState();
        }
        if (!error.response) {
            if (error.code === 'ECONNABORTED' && error.message.includes('timeout')) {
                Message.error('请求超时，请检查网络连接');
            } else if (error.message.includes('ERR_CONNECTION_REFUSED') || error.message.includes('Network Error')) {
                Message.error('无法连接到服务器，请检查网络或稍后重试');
            } else {
                Message.error('网络连接异常，请检查网络设置');
            }
        }
        return Promise.reject(error);
    }
);

export default service;
