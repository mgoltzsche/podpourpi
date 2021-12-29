import { ActionTree, ActionContext } from 'vuex'
import { State } from './state'
import { Mutations, MutationTypes } from './mutations'
import { KubeConfig } from '@/k8sclient/k8sclient'
import { CancelablePromise } from '@/k8sclient/CancelablePromise'
import {
  com_github_mgoltzsche_podpourpi_pkg_apis_app_v1alpha1_App as App,
} from '@/client'

export enum ActionTypes {
  WATCH_APPS = 'WATCH_APPS',
  UPDATE_APP = 'UPDATE_APP',
}

const kc = new KubeConfig()
const apps = kc.newClient<App>('/apis/podpourpi.mgoltzsche.github.com/v1alpha1/apps')

export interface Actions {
  [ActionTypes.WATCH_APPS]({ commit }: AugmentedActionContext): void
  [ActionTypes.UPDATE_APP]({ commit }: AugmentedActionContext, app: App): CancelablePromise<App>
}

export const actions: ActionTree<State, State> & Actions = {
  [ActionTypes.WATCH_APPS]({ commit }): void {
    apps.watch((evt) => {
      // TODO: collect events in array and
      // commit them only when a bookmark event is received
      commit(MutationTypes.SYNC_APPS, [evt])
    })
  },
  [ActionTypes.UPDATE_APP](_, app: App): CancelablePromise<App> {
    return apps.update(app)
  },
}

type AugmentedActionContext = {
    commit<K extends keyof Mutations>(
      key: K,
      payload: Parameters<Mutations[K]>[1]
    ): ReturnType<Mutations[K]>
  } & Omit<ActionContext<State, State>, 'commit'>
  