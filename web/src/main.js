import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import JoinView from './views/JoinView.vue'
import RoomView from './views/RoomView.vue'

const routes = [
  { path: '/', component: JoinView },
  { path: '/room', component: RoomView },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

const app = createApp(App)
app.use(router)
app.mount('#app')
