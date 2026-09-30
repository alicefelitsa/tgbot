import request from '@/api/request';

export async function login(data) {
    const res = await request.post('/login', data);
    if (res.data.code === 0) {
        localStorage.setItem("token", res.data.token);
        localStorage.setItem("account", res.data.account);
        return res.data.message;
    }
    return Promise.reject(new Error(res.data.message));
}
