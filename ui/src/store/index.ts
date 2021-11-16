import { InjectionKey } from 'vue'
import { createStore, useStore as baseUseStore, Store } from 'vuex'
//import { App } from '@/client/models/App'
//import { EventList } from '@/client/models/EventList'
import { State } from '@vue/runtime-core' // this is overwritten in vuex.d.ts, see https://next.vuex.vuejs.org/guide/typescript-support.html
import { Phase } from '@/client/models/Phase'

// store singleton
export const store = createStore<State>({
  state: {
    apps: [{metadata: {name: "fake-app1"}, status: {phase: Phase.RUNNING}}, {metadata: {name: "fake-app2"}, status: {phase: Phase.FAILED}}],
  },
  mutations: {
    // TODO: implement mutation here that imports data
    /*applyEvents: function(events: EventList) {

    }*/
  }
})

// store injection key
export const key: InjectionKey<Store<State>> = Symbol()

export function useStore(): Store<State> {
  return baseUseStore(key)
}
