import Vue from 'vue'
import VueRouter from 'vue-router'

Vue.use(VueRouter)

const routes = [
    {
        path: '/login',
        component: () => import('@/views/login'),
        meta: {title: '管理员登录'}
    },
    {
        path: '/',
        component: () => import('@/layout/index'),
        redirect: '/tgMenu',
        children: [
            {
                path: 'tgMenu',
                name: 'tgMenu',
                component: () => import('@/views/tgMenu/index'),
                meta: {title: '机器人菜单'}
            },
            {
                path: 'tgCommand',
                name: 'tgCommand',
                component: () => import('@/views/tgCommand/index'),
                meta: {title: '命令菜单'}
            },
            {
                path: 'tgUser',
                name: 'tgUser',
                component: () => import('@/views/tgUser/index'),
                meta: {title: 'TG用户'}
            },
            {
                path: 'tgChat',
                name: 'tgChat',
                component: () => import('@/views/tgChat/index'),
                meta: {title: 'TG群组'}
            },
            {
                path: 'tgImage',
                name: 'tgImage',
                component: () => import('@/views/tgImage/index'),
                meta: {title: '图片库'}
            },
            {
                path: 'sysSetting',
                name: 'sysSetting',
                component: () => import('@/views/sysSetting/index'),
                meta: {title: '系统设置'}
            },
        ]
    },
    {
        path: '*',
        name: 'NotFound',
        component: () => import('@/views/NotFound'),
    }
]

const router = new VueRouter({
    mode: 'history',
    base: process.env.BASE_URL,
    routes
})

// 全局前置守卫
router.beforeEach((to, from, next) => {
    if (to.meta.title) {
        document.title = to.meta.title + ' - 机器人管理后台'
    }

    if (to.path === '/login') {
        if (localStorage.getItem("token")) {
            next({path: '/'})
        }
        next()
        return
    }
    if (localStorage.getItem("token")) {
        next()
    } else {
        next({path: '/login'})
    }
})

const originalPush = VueRouter.prototype.push
VueRouter.prototype.push = function push(location) {
    return originalPush.call(this, location).catch(err => err)
}

export default router
