import { Store } from 'vuex'
import { App as ServerApp } from './client/models/App'

declare module '@vue/runtime-core' {
  interface State {
    apps: ServerApp[]
  }

  // provide typings for `this.$store`
  interface ComponentCustomProperties {
    $store: Store<State>
  }
}
