import { MutationTree } from 'vuex'
import { State } from './state'
import { Event as ServerEvent, Metadata } from '@/client'
import { EventAction } from '@/client/models/EventAction'

export enum MutationTypes {
    SYNC_DATA = 'SYNC_DATA',
}

export type Mutations<S = State> = {
  [MutationTypes.SYNC_DATA](state: S, events: ServerEvent[]): void
}

export const mutations: MutationTree<State> & Mutations = {
  [MutationTypes.SYNC_DATA](state: State, events: ServerEvent[]) {
    if (events.length > 1) {
      // Reset state on reconnect.
      // (The server emits all data with the first event. Subsequently one event is emitted per message / at a time.)
      state.apps.length = 0
    }
    events.forEach(evt => {
      if (evt.object.app) {
        state.apps = applyChange('app', state.apps, evt.action, evt.object.app)
      }
    })
  },
}

function applyChange<T extends Resource>(typeName: string, items: Array<T>, action: EventAction, o: T) {
  if (o.metadata.name == '') {
    o.metadata.name = '<unknown>'
  }
  // TODO: don't replace items - otherwise UI looses track
  console.log('mutate:', action, typeName, o)
  switch(action) {
    case EventAction.CREATE:
      items.push(o)
      break
    case EventAction.UPDATE:
      items = items.filter(a => a.metadata.name != o.metadata.name)
      items.push(o)
      break
    case EventAction.DELETE:
      items = items.filter(a => a.metadata.name != o.metadata.name)
      break
    default:
      console.log('ERROR: unexpected server event action received:', action)
  }
  return items
}

interface Resource {
    readonly metadata: Metadata
}
