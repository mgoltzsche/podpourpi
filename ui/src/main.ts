import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { store, key } from './store'

const app = createApp(App)

app.use(store, key).use(router).mount('#app')

// TODO: update store
fetch('/api/v1/events')
    .then(r => r.json())
    .then() // TODO: parse chunked response into JSON objects and commit each into the store using a mutation function.
    .catch(error => {
        alert("watch error: "+error)
    })
