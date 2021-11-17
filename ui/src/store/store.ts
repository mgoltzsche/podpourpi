import {
  createStore,
  Store as VuexStore,
  CommitOptions,
} from 'vuex'
import { State, state } from './state'
import { Getters, getters } from './getters'
import { Mutations, mutations } from './mutations'

// See https://dev.to/3vilarthas/vuex-typescript-m4j

export const store = createStore({
  state,
  getters,
  mutations,
})

export type Store = Omit<
  VuexStore<State>,
  'getters' | 'commit' | 'dispatch'
> & {
  commit<K extends keyof Mutations, P extends Parameters<Mutations[K]>[1]>(
    key: K,
    payload: P,
    options?: CommitOptions
  ): ReturnType<Mutations[K]>
} & {
  getters: {
    [K in keyof Getters]: ReturnType<Getters[K]>
  }
}
