import { Notify } from 'quasar'
import './styles/quasar.scss'
import '@quasar/extras/roboto-font/roboto-font.css'
import '@quasar/extras/material-icons/material-icons.css'

// To be used on app.use(Quasar, { ... })
export default {
  plugins: {
    Notify,
  },
  config: {
    notify: {},
  },
}