import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { store, key } from './store'
import { ActionTypes } from './store/actions'
import { Quasar } from 'quasar'
import quasarUserOptions from './quasar-user-options'

createApp(App)
  .use(store, key)
  .use(router)
  .use(Quasar, quasarUserOptions)
  .mount('#app')

store.dispatch(ActionTypes.WATCH_APPS)
