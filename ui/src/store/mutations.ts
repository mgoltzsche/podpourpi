import { MutationTree } from 'vuex'
import { State } from './state'
import { WatchEvent, EventType } from '@/k8sclient/watch'
import {
  com_github_mgoltzsche_podpourpi_pkg_apis_app_v1alpha1_App as App,
  io_k8s_apimachinery_pkg_apis_meta_v1_ObjectMeta as ObjectMeta,
} from '@/client'

export enum MutationTypes {
    SYNC_APPS = 'SYNC_APPS',
}

export type Mutations<S = State> = {
  [MutationTypes.SYNC_APPS](state: S, events: WatchEvent<App>[]): void
}

export const mutations: MutationTree<State> & Mutations = {
  [MutationTypes.SYNC_APPS](state: State, events: WatchEvent<App>[]) {
    events.forEach(evt => {
      state.apps = applyChange('app', state.apps, evt.type, evt.object)
    })
  },
}

function applyChange<T extends Resource>(typeName: string, items: Array<T>, evtType: EventType, o: T) {
  if (o.metadata.name == '') {
    o.metadata.name = '<unknown>'
  }
  console.log('mutate:', evtType, typeName, o)
  switch(evtType) {
    case EventType.ADDED, EventType.MODIFIED, EventType.BOOKMARK:
      items = items.filter(a => a.metadata.name != o.metadata.name)
      items.push(o)
      break
    case EventType.DELETED:
      items = items.filter(a => a.metadata.name != o.metadata.name)
      break
    default:
      console.log(`ERROR: received unexpected watch event type: ${evtType}`)
  }
  items.sort(compareResourcesByName)
  return items
}

function compareResourcesByName(a: Resource, b: Resource): number {
  const aName = a.metadata.name || ''
  const bName = b.metadata.name || ''
  if (aName < bName) {
    return -1
  }
  if (aName > bName) {
    return 1
  }
  return 0
}

interface Resource {
    readonly metadata: ObjectMeta
}
