import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { store, key } from './store'
import { watchServerChanges } from './store/sync'

const app = createApp(App)

app.use(store, key).use(router).mount('#app')

watchServerChanges()
